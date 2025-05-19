package intake

import (
	"context"
	"fmt"
	"io"

	"github.com/cenkalti/backoff/v5"

	"github.com/antimetal/agent/pkg/resource"
	intakev1 "github.com/antimetal/apis/gengo/service/resource/v1"
	"github.com/go-logr/logr"
	"google.golang.org/grpc"
	"google.golang.org/grpc/metadata"
)

const (
	headerAuthorize = "authorization"
)

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
loop:
	for event := range w.store.Subscribe(nil) {
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

				// Break out of the main loop if the context is canceled since that means
				// we're shutting down. Continue on the main loop until the subscription channel
				// is closed.
				if err == nil || ctx.Err() == context.Canceled {
					continue loop
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
			// TODO: This is going to drop events on stream failure. We want to add
			// back the event to the queue so that it can be retried when the stream
			// is re-established.
			if err != io.EOF {
				w.logger.Error(err, "unexpected error sending to intake stream")
			}
			_, err = w.stream.CloseAndRecv()
			if err != nil {
				w.logger.Error(err, "failed to send to intake stream")
			}
			w.stream = nil
		}
	}

	w.logger.Info("shutting down intake worker")
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
