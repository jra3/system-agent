package intake

import (
	"context"
	"fmt"

	"github.com/cenkalti/backoff/v5"

	"github.com/antimetal/agent/pkg/resource"
	intakev1 "github.com/antimetal/apis/gengo/service/resource/v1"
	"github.com/go-logr/logr"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"
)

const (
	headerAuthorize = "authorization"
)

var retryableCodes = []codes.Code{
	codes.Canceled,
	codes.DeadlineExceeded,
	codes.Unavailable,
	codes.Aborted,
}

type worker struct {
	apiKey string
	client intakev1.IntakeServiceClient
	store  resource.Store
	logger logr.Logger

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
	w := &worker{store: store}
	for _, opt := range opts {
		opt(w)
	}
	if w.client == nil {
		return nil, fmt.Errorf("can't create client")
	}
	return w, nil
}

func (w *worker) Start(ctx context.Context) error {
	eventSub := w.store.Subscribe(nil)
loop:
	for {
		select {
		case <-ctx.Done():
			break loop
		case event, ok := <-eventSub:
			if !ok {
				// resource store is closed so start the shutdown process
				break loop
			}
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
				}
			}
			w.logger.V(1).Info("sending intake stream", "eventType", event.Type, "numObjects", len(event.Objs))
			err := w.stream.Send(&intakev1.DeltaRequest{
				Deltas: []*intakev1.Delta{
					{
						Op:      eventTypeToOp(event.Type),
						Objects: event.Objs,
					},
				},
			})
			if err != nil {
				code := status.Code(err)
				for _, c := range retryableCodes {
					if code == c {
						w.logger.Error(err, "failed to send to intake stream, reseting stream...")
						// TODO: This is going to drop events on stream failure. We want to add
						// back the event to the queue so that it can be retried when the stream
						// is re-established.
						w.stream = nil
						continue loop
					}
				}
				_, err = w.stream.CloseAndRecv()
				w.logger.Error(err, "failed to send to intake stream")
				return err
			}
		}
	}
	w.logger.Info("shutting down intake worker")
	// Shutdown
	if w.stream != nil {
		if _, err := w.stream.CloseAndRecv(); err != nil {
			return fmt.Errorf("failed to close intake stream: %w", err)
		}
	}
	return nil
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
