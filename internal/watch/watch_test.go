package watch

import (
	"context"
	"testing"
	"time"

	"github.com/benturnkey/talos-state-metrics/internal/eventsource"
	"github.com/benturnkey/talos-state-metrics/internal/state"
)

func TestManagerMarksReadyOnlyAfterInitialFullSync(t *testing.T) {
	snapshot := state.NewSnapshot()
	src := &fakeSource{
		events: make(chan eventsource.Event, 2),
		errs:   make(chan error, 1),
	}

	manager := &Manager{
		Snapshot: snapshot,
		Factory:  func() eventsource.Source { return src },
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	done := make(chan struct{})
	go func() {
		defer close(done)
		manager.Run(ctx)
	}()

	time.Sleep(20 * time.Millisecond)
	if snapshot.Ready() {
		t.Fatalf("snapshot should stay unready before the initial full sync")
	}

	src.events <- eventsource.Event{Type: eventsource.EventFullSync, At: time.Unix(100, 0).UTC()}
	waitFor(t, time.Second, snapshot.Ready)

	cancel()
	<-done
}

func TestManagerWaitsForFullSyncBeforeReadyAfterPeerEvents(t *testing.T) {
	snapshot := state.NewSnapshot()
	src := &fakeSource{
		events: make(chan eventsource.Event, 3),
		errs:   make(chan error, 1),
	}

	manager := &Manager{
		Snapshot: snapshot,
		Factory:  func() eventsource.Source { return src },
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	done := make(chan struct{})
	go func() {
		defer close(done)
		manager.Run(ctx)
	}()

	handshake := time.Unix(100, 0).UTC()
	src.events <- eventsource.Event{
		Type: eventsource.EventPeerUpsert,
		Peer: eventsource.Peer{ID: "peer-a", LastHandshake: handshake},
		At:   handshake,
	}

	time.Sleep(20 * time.Millisecond)
	if snapshot.Ready() {
		t.Fatalf("snapshot should stay unready until full sync completes")
	}

	src.events <- eventsource.Event{
		Type: eventsource.EventFullSync,
		Peers: []eventsource.Peer{
			{ID: "peer-a", LastHandshake: handshake},
		},
		At: handshake.Add(time.Second),
	}
	waitFor(t, time.Second, snapshot.Ready)

	snap := snapshot.Copy()
	if len(snap.Peers) != 1 {
		t.Fatalf("expected full sync to establish one peer, got %d peers", len(snap.Peers))
	}

	cancel()
	<-done
}

func TestManagerClearsPeerStateAcrossDisconnect(t *testing.T) {
	snapshot := state.NewSnapshot()
	src := &fakeSource{
		events: make(chan eventsource.Event, 4),
		errs:   make(chan error, 1),
	}

	manager := &Manager{
		Snapshot:   snapshot,
		Factory:    func() eventsource.Source { return src },
		MinBackoff: 10 * time.Millisecond,
		MaxBackoff: 10 * time.Millisecond,
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	done := make(chan struct{})
	go func() {
		defer close(done)
		manager.Run(ctx)
	}()

	handshake := time.Unix(101, 0).UTC()
	src.events <- eventsource.Event{
		Type: eventsource.EventFullSync,
		Peers: []eventsource.Peer{
			{ID: "peer-a", LastHandshake: handshake},
		},
		At: handshake,
	}
	waitFor(t, time.Second, snapshot.Ready)

	src.errs <- context.Canceled
	waitFor(t, time.Second, func() bool { return !snapshot.Ready() })

	copy := snapshot.Copy()
	if len(copy.Peers) != 0 {
		t.Fatalf("expected peer data to be cleared on disconnect, got %d peers", len(copy.Peers))
	}

	cancel()
	<-done
}

func TestManagerNextBackoffResetsAfterConnectedSession(t *testing.T) {
	manager := &Manager{
		MinBackoff: 10 * time.Millisecond,
		MaxBackoff: 80 * time.Millisecond,
	}

	if got := manager.nextBackoff(10*time.Millisecond, false); got != 20*time.Millisecond {
		t.Fatalf("expected first failure to double backoff to 20ms, got %s", got)
	}
	if got := manager.nextBackoff(40*time.Millisecond, true); got != 10*time.Millisecond {
		t.Fatalf("expected connected session to reset backoff to 10ms, got %s", got)
	}
	if got := manager.nextBackoff(80*time.Millisecond, false); got != 80*time.Millisecond {
		t.Fatalf("expected capped backoff to stay at 80ms, got %s", got)
	}
}

type fakeSource struct {
	events chan eventsource.Event
	errs   chan error
}

func (f *fakeSource) Watch(context.Context) (<-chan eventsource.Event, <-chan error) {
	return f.events, f.errs
}

func waitFor(t *testing.T, timeout time.Duration, fn func() bool) {
	t.Helper()

	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		if fn() {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}

	t.Fatalf("condition was not met before %s", timeout)
}
