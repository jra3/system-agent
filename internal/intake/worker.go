package intake

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"sync"
	"time"

	"github.com/cenkalti/backoff/v5"
	"k8s.io/client-go/util/workqueue"

	"github.com/antimetal/agent/pkg/resource"
	resourcev1 "github.com/antimetal/apis/gengo/resource/v1"
	intakev1 "github.com/antimetal/apis/gengo/service/resource/v1"
	"github.com/go-logr/logr"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/types/known/durationpb"
)

const (
	workerName        = "intake-worker"
	headerAuthorize   = "authorization"
	defaultDeltaTTL   = 5 * time.Minute
	heartbeatInterval = 1 * time.Minute
)

var deltaVersion string

func init() {
	b := make([]byte, 4)
	_, err := rand.Read(b)
	if err != nil {
		// This should never happen because rand.Read should never return an error
		panic(fmt.Sprintf("failed to generate random delta version: %v", err))
	}
	deltaVersion = hex.EncodeToString(b)
}

type worker struct {
	apiKey string
	client intakev1.IntakeServiceClient
	store  resource.Store
	logger logr.Logger
	queue  workqueue.TypedRateLimitingInterface[*intakev1.Delta]

	// runtime fields
	stream intakev1.IntakeService_DeltaClient
}

type WorkerOpts func(*worker)

func WithGRPCConn(conn *grpc.ClientConn) WorkerOpts {
	return func(w *worker) {
		w.client = intakev1.NewIntakeServiceClient(conn)
	}
}

func WithLogger(logger logr.Logger) WorkerOpts {
	return func(w *worker) {
		w.logger = logger
	}
}

func WithAPIKey(apiKey string) WorkerOpts {
	return func(w *worker) {
		w.apiKey = apiKey
	}
}

func NewWorker(store resource.Store, opts ...WorkerOpts) (*worker, error) {
	if store == nil {
		return nil, fmt.Errorf("store can't be nil")
	}

	ratelimiter := workqueue.DefaultTypedControllerRateLimiter[*intakev1.Delta]()
	queue := workqueue.NewTypedRateLimitingQueueWithConfig(ratelimiter,
		workqueue.TypedRateLimitingQueueConfig[*intakev1.Delta]{
			Name: workerName,
		},
	)

	w := &worker{
		store: store,
		queue: queue,
	}
	for _, opt := range opts {
		opt(w)
	}

	if w.client == nil {
		return nil, fmt.Errorf("can't create client")
	}
	return w, nil
}

func (w *worker) Start(ctx context.Context) error {
	var wg sync.WaitGroup
	go func() {
		wg.Add(1)
		defer wg.Done()
		w.streamer(ctx)
	}()

	go func() {
		wg.Add(1)
		defer wg.Done()
		w.heartbeatWorker(ctx)
	}()

	for event := range w.store.Subscribe(nil) {
		for _, obj := range event.Objs {
			obj.Ttl = durationpb.New(defaultDeltaTTL)
			obj.DeltaVersion = deltaVersion
		}

		w.queue.AddRateLimited(&intakev1.Delta{
			Op:      eventTypeToOp(event.Type),
			Objects: event.Objs,
		})
	}

	w.logger.Info("shutting down intake worker")
	w.queue.ShutDownWithDrain()
	wg.Wait()
	return nil
}

func (w *worker) streamer(ctx context.Context) {
	for {
		select {
		case <-ctx.Done():
			if w.stream != nil {
				if _, err := w.stream.CloseAndRecv(); err != nil {
					w.logger.Error(err, "error closing intake stream")
				}
			}
			return
		default:
			w.sendDelta(ctx)
		}
	}
}

func (w *worker) heartbeatWorker(ctx context.Context) {
	ticker := time.NewTicker(heartbeatInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			w.queue.AddRateLimited(&intakev1.Delta{
				Op: intakev1.DeltaOperation_DELTA_OPERATION_HEARTBEAT,
				Objects: []*resourcev1.Object{
					{
						DeltaVersion: deltaVersion,
						Ttl:          durationpb.New(defaultDeltaTTL),
					},
				},
			})
		}
	}
}

func (w *worker) sendDelta(ctx context.Context) {
	delta, shutdown := w.queue.Get()
	if shutdown {
		return
	}
	defer w.queue.Done(delta)

	if w.stream == nil {
		for {
			_, err := backoff.Retry(ctx, func() (bool, error) {
				streamCtx := metadata.NewOutgoingContext(
					context.Background(), metadata.Pairs(headerAuthorize, fmt.Sprintf("bearer %s", w.apiKey)),
				)
				stream, err := w.client.Delta(streamCtx)
				if err != nil {
					w.logger.Error(err, "failed to create intake stream, retrying...")
					return false, err
				}
				w.stream = stream
				return true, nil
			}, backoff.WithBackOff(backoff.NewExponentialBackOff()))

			if err == nil {
				break
			}

			// Return if the context is canceled since that means we're shutting down.
			if ctx.Err() == context.Canceled {
				return
			}
		}
	}

	w.logger.V(1).Info("sending delta", "op", delta.Op, "numObjects", len(delta.Objects), "version", deltaVersion)
	err := w.stream.Send(&intakev1.DeltaRequest{Deltas: []*intakev1.Delta{delta}})
	if err != nil {
		_, err = w.stream.CloseAndRecv()
		if err != nil {
			code := status.Code(err)
			if code == codes.Unavailable || code == codes.Canceled {
				// Resetting stream due server max connection age
				w.logger.V(1).Info("resetting intake stream")
			} else {
				w.logger.Error(err, "failed to send to intake stream")
			}
		}
		w.stream = nil
		if !w.queue.ShuttingDown() {
			w.queue.AddRateLimited(delta)
		}
		return
	}
	w.queue.Forget(delta)
}

func eventTypeToOp(e resource.EventType) intakev1.DeltaOperation {
	switch e {
	case resource.EventTypeAdd:
		return intakev1.DeltaOperation_DELTA_OPERATION_CREATE
	case resource.EventTypeUpdate:
		return intakev1.DeltaOperation_DELTA_OPERATION_UPDATE
	case resource.EventTypeDelete:
		return intakev1.DeltaOperation_DELTA_OPERATION_DELETE
	default:
		return intakev1.DeltaOperation_DELTA_OPERATION_CREATE
	}
}
