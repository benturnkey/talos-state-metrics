package talos

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	"github.com/benturnkey/talos-state-metrics/internal/eventsource"
	cosiresource "github.com/cosi-project/runtime/pkg/resource"
	"github.com/cosi-project/runtime/pkg/safe"
	cosistate "github.com/cosi-project/runtime/pkg/state"
	talosclient "github.com/siderolabs/talos/pkg/machinery/client"
	taloskubespan "github.com/siderolabs/talos/pkg/machinery/resources/kubespan"
)

type bootstrapState struct {
	complete bool
	peers    map[string]eventsource.Peer
}

type Source struct {
	Endpoint         string
	ConfigPath       string
	FullSyncInterval time.Duration
	Logger           *slog.Logger
	client           *talosclient.Client
	state            cosistate.CoreState
	newClient        func(context.Context, ...talosclient.OptionFunc) (*talosclient.Client, error)
}

func New(endpoint, configPath string, fullSyncInterval time.Duration) *Source {
	return &Source{
		Endpoint:         endpoint,
		ConfigPath:       configPath,
		FullSyncInterval: fullSyncInterval,
		newClient:        talosclient.New,
	}
}

// Watch bridges the Talos COSI watch API into the exporter's generic event stream.
// It buffers the initial bootstrap contents from the watch and emits them as a
// single full-sync event so readiness only flips after the complete startup peer
// set is known. On a long interval it also emits a full-sync event built from a
// fresh list call so the caller can replace any drifted peer set. Any watch setup,
// list, decode, or runtime failure is sent on errs and terminates the stream so
// callers can fail closed and reconnect instead of serving stale peer state.
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
		resyncCh := s.fullSyncTicker(watchCtx)
		bootstrap := bootstrapState{peers: make(map[string]eventsource.Peer)}

		if err := safe.StateWatchKind[*taloskubespan.PeerStatus](watchCtx, st, kind, watchCh, cosistate.WithBootstrapContents(true)); err != nil {
			errs <- fmt.Errorf("start talos watch: %w", err)

			return
		}

		for {
			select {
			case <-watchCtx.Done():
				errs <- fmt.Errorf("talos watch context done: %w", watchCtx.Err())

				return
			case <-resyncCh:
				if err := s.emitFullSync(watchCtx, st, events); err != nil {
					err = fmt.Errorf("full peer sync failed: %w", err)
					errs <- err

					return
				}
			case wrapped, ok := <-watchCh:
				if !ok {
					errs <- fmt.Errorf("talos watch channel closed")

					return
				}
				if err := s.handleWatchEvent(wrapped, &bootstrap, events, errs); err != nil {
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

func (s *Source) fullSyncTicker(ctx context.Context) <-chan struct{} {
	if s.FullSyncInterval <= 0 {
		return nil
	}

	ticker := time.NewTicker(s.FullSyncInterval)
	ch := make(chan struct{})

	go func() {
		defer close(ch)
		defer ticker.Stop()

		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				select {
				case <-ctx.Done():
					return
				case ch <- struct{}{}:
				}
			}
		}
	}()

	return ch
}

func (s *Source) emitFullSync(ctx context.Context, st cosistate.CoreState, events chan<- eventsource.Event) error {
	kind := cosiresource.NewMetadata(taloskubespan.NamespaceName, taloskubespan.PeerStatusType, "", cosiresource.VersionUndefined)
	list, err := safe.StateList[*taloskubespan.PeerStatus](ctx, st, kind)
	if err != nil {
		return fmt.Errorf("list kubespan peers: %w", err)
	}

	peers := make([]eventsource.Peer, 0, list.Len())
	list.ForEach(func(peerStatus *taloskubespan.PeerStatus) {
		peers = append(peers, peerFromStatus(peerStatus))
	})

	events <- eventsource.Event{
		Type:  eventsource.EventFullSync,
		Peers: peers,
		At:    time.Now().UTC(),
	}

	return nil
}

func peersFromBootstrap(peers map[string]eventsource.Peer) []eventsource.Peer {
	fullSyncPeers := make([]eventsource.Peer, 0, len(peers))
	for _, peer := range peers {
		fullSyncPeers = append(fullSyncPeers, peer)
	}

	return fullSyncPeers
}

// handleWatchEvent translates Talos watch events into exporter events while keeping
// Talos-specific edge cases here. During startup it buffers bootstrap contents and
// collapses them into one full-sync barrier event. After bootstrap, created and
// updated resources become upserts, destroyed resources become deletes, and any
// decode or watch error is surfaced so the caller tears down the current stream
// and retries from a clean watch.
func (s *Source) handleWatchEvent(
	wrapped safe.WrappedStateEvent[*taloskubespan.PeerStatus],
	bootstrap *bootstrapState,
	events chan<- eventsource.Event,
	errs chan<- error,
) error {
	switch wrapped.Type() {
	case cosistate.Bootstrapped:
		bootstrap.complete = true
		events <- eventsource.Event{
			Type:  eventsource.EventFullSync,
			Peers: peersFromBootstrap(bootstrap.peers),
			At:    time.Now().UTC(),
		}
	case cosistate.Created, cosistate.Updated:
		peerStatus, err := wrapped.Resource()
		if err != nil {
			err = fmt.Errorf("decode kubespan peer status event: %w", err)
			errs <- err

			return err
		}

		peer := peerFromStatus(peerStatus)
		if !bootstrap.complete {
			bootstrap.peers[peer.ID] = peer
			return nil
		}

		events <- eventsource.Event{
			Type: eventsource.EventPeerUpsert,
			Peer: peer,
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

		peer := deletePeerFromStatus(peerStatus)
		if !bootstrap.complete {
			delete(bootstrap.peers, peer.ID)
			return nil
		}

		events <- eventsource.Event{
			Type: eventsource.EventPeerDelete,
			Peer: peer,
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
