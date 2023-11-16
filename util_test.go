package relayer

import (
	"context"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/trace"
	"testing"
	"time"

	"github.com/nbd-wtf/go-nostr"
)

func startTestRelay(t *testing.T, tr *testRelay) *Server {
	t.Helper()
	ready := make(chan struct{})

	onInitializedFn := tr.onInitialized
	tr.onInitialized = func(s *Server) {
		close(ready)
		if onInitializedFn != nil {
			onInitializedFn(s)
		}
	}
	srv := NewServer("127.0.0.1:0", tr)
	go srv.Start()

	select {
	case <-ready:
	case <-time.After(time.Second):
		t.Fatal("server took too long to start up")
	}
	return srv
}

type testRelay struct {
	name          string
	storage       Storage
	init          func() error
	onInitialized func(*Server)
	onShutdown    func(context.Context)
	acceptEvent   func(*nostr.Event) bool
}

func (tr *testRelay) Name() string         { return tr.name }
func (tr *testRelay) Storage() Storage     { return tr.storage }
func (tr *testRelay) Tracer() trace.Tracer { return otel.Tracer("test-tracer") }

func (tr *testRelay) Init() error {
	if fn := tr.init; fn != nil {
		return fn()
	}
	return nil
}

func (tr *testRelay) OnInitialized(s *Server) {
	if fn := tr.onInitialized; fn != nil {
		fn(s)
	}
}

func (tr *testRelay) OnShutdown(ctx context.Context) {
	if fn := tr.onShutdown; fn != nil {
		fn(ctx)
	}
}

func (tr *testRelay) AcceptEvent(e *nostr.Event) bool {
	if fn := tr.acceptEvent; fn != nil {
		return fn(e)
	}
	return true
}

func (tr *testRelay) BroadcastEvent(event nostr.Event) {}

type testStorage struct {
	init        func() error
	queryEvents func(context.Context, *nostr.Filter) ([]nostr.Event, error)
	deleteEvent func(ctx context.Context, id string, pubkey string) error
	saveEvent   func(context.Context, *nostr.Event) error
}

func (st *testStorage) Init() error {
	if fn := st.init; fn != nil {
		return fn()
	}
	return nil
}

func (st *testStorage) QueryEvents(ctx context.Context, f *nostr.Filter) ([]nostr.Event, error) {
	if fn := st.queryEvents; fn != nil {
		return fn(ctx, f)
	}
	return nil, nil
}

func (st *testStorage) DeleteEvent(ctx context.Context, id string, pubkey string) error {
	if fn := st.deleteEvent; fn != nil {
		return fn(ctx, id, pubkey)
	}
	return nil
}

func (st *testStorage) SaveEvent(ctx context.Context, e *nostr.Event) error {
	if fn := st.saveEvent; fn != nil {
		return fn(ctx, e)
	}
	return nil
}
