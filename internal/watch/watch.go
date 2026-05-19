package watch

import (
	"context"
	"log/slog"
	"time"

	"github.com/tkhq/talos-state-metrics/internal/eventsource"
	"github.com/tkhq/talos-state-metrics/internal/state"
)

const (
	defaultMinBackoff = time.Second
	defaultMaxBackoff = 30 * time.Second
)

type Manager struct {
	Snapshot   *state.Snapshot
	Factory    eventsource.Factory
	MinBackoff time.Duration
	MaxBackoff time.Duration
	Logger     *slog.Logger
}

// Run owns the long-lived watch lifecycle for one snapshot. It keeps a watch
// open, applies events until the stream ends or errors, then marks the snapshot
// disconnected and retries with bounded backoff. This keeps peer-derived metrics
// fail-closed across disconnects instead of leaving the last good data in place.
func (m *Manager) Run(ctx context.Context) {
	backoff := m.minBackoff()

	for ctx.Err() == nil {
		src := m.Factory()
		events, errs := src.Watch(ctx)

		connected, err := m.consume(ctx, events, errs)
		if err != nil && m.Logger != nil {
			m.Logger.Warn("talos watch disconnected", "err", err)
		}

		m.Snapshot.SetConnected(false, time.Now().UTC())
		backoff = m.nextBackoff(backoff, connected)
		select {
		case <-ctx.Done():
			return
		case <-time.After(backoff):
		}
	}
}

// consume processes one watch session until the source closes, reports an error,
// or the caller cancels the context. The session is considered connected only
// after the source emits a full-sync event, which acts as the barrier that says
// the initial complete peer set is now known. That keeps startup unready until
// the initial full peer set has been delivered.
func (m *Manager) consume(ctx context.Context, events <-chan eventsource.Event, errs <-chan error) (bool, error) {
	connected := false

	for {
		select {
		case <-ctx.Done():
			return connected, ctx.Err()
		case event, ok := <-events:
			if !ok {
				return connected, nil
			}
			m.Snapshot.Apply(event)
			if !connected && marksConnected(event.Type) {
				m.Snapshot.SetConnected(true, event.At)
				connected = true
			}
		case err, ok := <-errs:
			if !ok {
				errs = nil
				continue
			}
			if err != nil {
				return connected, err
			}
		}
	}
}

func marksConnected(eventType eventsource.EventType) bool {
	return eventType == eventsource.EventFullSync
}

func (m *Manager) minBackoff() time.Duration {
	if m.MinBackoff > 0 {
		return m.MinBackoff
	}
	return defaultMinBackoff
}

func (m *Manager) maxBackoff() time.Duration {
	if m.MaxBackoff > 0 {
		return m.MaxBackoff
	}
	return defaultMaxBackoff
}

func (m *Manager) nextBackoff(current time.Duration, connected bool) time.Duration {
	if connected {
		return m.minBackoff()
	}

	next := current * 2
	if next > m.maxBackoff() {
		return m.maxBackoff()
	}

	return next
}
