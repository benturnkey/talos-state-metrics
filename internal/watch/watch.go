package watch

import (
	"context"
	"log/slog"
	"time"

	"github.com/benturnkey/talos-state-metrics/internal/eventsource"
	"github.com/benturnkey/talos-state-metrics/internal/state"
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
		if !sleep(ctx, backoff) {
			return
		}
	}
}

// consume processes one watch session until the source closes, reports an error,
// or the caller cancels the context. The first event, including bootstrap, marks
// the snapshot connected so readiness only flips after the source has delivered
// real watch data rather than after watch setup alone.
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
			if !connected {
				m.Snapshot.SetConnected(true, event.At)
				connected = true
			}
			m.Snapshot.Apply(event)
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

func (m *Manager) minBackoff() time.Duration {
	if m.MinBackoff > 0 {
		return m.MinBackoff
	}
	return time.Second
}

func (m *Manager) maxBackoff() time.Duration {
	if m.MaxBackoff > 0 {
		return m.MaxBackoff
	}
	return 30 * time.Second
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

func sleep(ctx context.Context, duration time.Duration) bool {
	timer := time.NewTimer(duration)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return false
	case <-timer.C:
		return true
	}
}
