package intake

import (
	"context"
	"fmt"
	"strings"
	"sync"

	"github.com/cenkalti/backoff/v5"
	"k8s.io/client-go/util/workqueue"

	"github.com/antimetal/agent/pkg/resource"
	intakev1 "github.com/antimetal/apis/gengo/service/resource/v1"
	"github.com/go-logr/logr"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"
)

const (
	workerName      = "intake-worker"
	headerAuthorize = "authorization"
)

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

func (w *worker) Start(_ context.Context) error {
	ctx, cancel := context.WithCancel(context.Background())

	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		w.streamer(ctx)
	}()

	for event := range w.store.Subscribe(nil) {
		w.queue.AddRateLimited(&intakev1.Delta{
			Op:      eventTypeToOp(event.Type),
			Objects: event.Objs,
		})
	}

	w.logger.Info("shutting down intake worker")
	w.queue.ShutDownWithDrain()
	cancel()
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

	w.logger.V(1).Info("sending delta", "op", delta.Op, "numObjects", len(delta.Objects))
	err := w.stream.Send(&intakev1.DeltaRequest{Deltas: []*intakev1.Delta{delta}})
	if err != nil {
		_, err = w.stream.CloseAndRecv()
		if err != nil {
			code := status.Code(err)
			if code == codes.Unavailable && strings.Contains(code.String(), "max_age") {
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
