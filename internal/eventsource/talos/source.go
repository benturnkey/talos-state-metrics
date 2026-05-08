package talos

import (
	"context"
	"fmt"
	"log/slog"

	"github.com/benturnkey/talos-state-metrics/internal/eventsource"
	cosiresource "github.com/cosi-project/runtime/pkg/resource"
	"github.com/cosi-project/runtime/pkg/safe"
	cosistate "github.com/cosi-project/runtime/pkg/state"
	talosclient "github.com/siderolabs/talos/pkg/machinery/client"
	taloskubespan "github.com/siderolabs/talos/pkg/machinery/resources/kubespan"
)

type Source struct {
	Endpoint   string
	ConfigPath string
	Logger     *slog.Logger
	client     *talosclient.Client
	state      cosistate.CoreState
	newClient  func(context.Context, ...talosclient.OptionFunc) (*talosclient.Client, error)
}

func New(endpoint, configPath string) *Source {
	return &Source{
		Endpoint:   endpoint,
		ConfigPath: configPath,
		newClient:  talosclient.New,
	}
}

// Watch bridges the Talos COSI watch API into the exporter's generic event stream.
// It emits a bootstrap event before peer updates so the watch manager can mark the
// snapshot ready only after Talos has delivered real watch data. Any watch setup,
// decode, or runtime failure is sent on errs and terminates the stream so callers
// can fail closed and reconnect instead of serving stale peer state.
func (s *Source) Watch(ctx context.Context) (<-chan eventsource.Event, <-chan error) {
	events := make(chan eventsource.Event)
	errs := make(chan error, 1)

	go func() {
		defer close(events)
		defer close(errs)

		watchCtx, cancel := context.WithCancel(ctx)
		defer cancel()

		st, client, err := s.openState(watchCtx)
		if err != nil {
			errs <- err

			return
		}
		if client != nil {
			defer func() {
				if err := client.Close(); err != nil && s.Logger != nil {
					s.Logger.Warn("talos client close failed", "err", err)
				}
			}()
		}

		watchCh := make(chan safe.WrappedStateEvent[*taloskubespan.PeerStatus])
		kind := cosiresource.NewMetadata(taloskubespan.NamespaceName, taloskubespan.PeerStatusType, "", cosiresource.VersionUndefined)

		if err := safe.StateWatchKind[*taloskubespan.PeerStatus](watchCtx, st, kind, watchCh, cosistate.WithBootstrapContents(true)); err != nil {
			errs <- fmt.Errorf("start talos watch: %w", err)

			return
		}

		for {
			select {
			case <-watchCtx.Done():
				errs <- fmt.Errorf("talos watch context done: %w", watchCtx.Err())

				return
			case wrapped, ok := <-watchCh:
				if !ok {
					errs <- fmt.Errorf("talos watch channel closed")

					return
				}
				if err := s.handleWatchEvent(wrapped, events, errs); err != nil {
					return
				}
			}
		}
	}()

	return events, errs
}

func (s *Source) TalosClient() *talosclient.Client {
	return s.client
}

// handleWatchEvent translates Talos watch events into exporter events while keeping
// Talos-specific edge cases here. Created and updated resources become upserts,
// destroyed resources become deletes, and any decode or watch error is surfaced so
// the caller tears down the current stream and retries from a clean watch.
func (s *Source) handleWatchEvent(
	wrapped safe.WrappedStateEvent[*taloskubespan.PeerStatus],
	events chan<- eventsource.Event,
	errs chan<- error,
) error {
	switch wrapped.Type() {
	case cosistate.Bootstrapped:
		events <- bootstrapEvent()
	case cosistate.Created, cosistate.Updated:
		peerStatus, err := wrapped.Resource()
		if err != nil {
			err = fmt.Errorf("decode kubespan peer status event: %w", err)
			errs <- err

			return err
		}

		events <- eventsource.Event{
			Type: eventsource.EventPeerUpsert,
			Peer: peerFromStatus(peerStatus),
			At:   eventTime(peerStatus),
		}
	case cosistate.Destroyed:
		peerStatus, err := wrapped.Resource()
		if err != nil {
			peerStatus, err = wrapped.Old()
			if err != nil {
				err = fmt.Errorf("decode kubespan peer delete event: %w", err)
				errs <- err

				return err
			}
		}

		events <- eventsource.Event{
			Type: eventsource.EventPeerDelete,
			Peer: deletePeerFromStatus(peerStatus),
			At:   eventTime(peerStatus),
		}
	case cosistate.Errored:
		err := wrapped.Error()
		if err == nil {
			err = fmt.Errorf("talos watch returned an unspecified error")
		}
		err = fmt.Errorf("talos watch event errored: %w", err)
		errs <- err

		return err
	}

	return nil
}
