package talos

import (
	"context"
	"fmt"

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
	NodeName   string
	client     *talosclient.Client
	state      cosistate.CoreState
	newClient  func(context.Context, ...talosclient.OptionFunc) (*talosclient.Client, error)
}

func New(endpoint, configPath, nodeName string) *Source {
	return &Source{
		Endpoint:   endpoint,
		ConfigPath: configPath,
		NodeName:   nodeName,
		newClient:  talosclient.New,
	}
}

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
			defer client.Close() //nolint:errcheck
		}

		watchCh := make(chan safe.WrappedStateEvent[*taloskubespan.PeerStatus])
		kind := cosiresource.NewMetadata(taloskubespan.NamespaceName, taloskubespan.PeerStatusType, "", cosiresource.VersionUndefined)

		if err := safe.StateWatchKind[*taloskubespan.PeerStatus](watchCtx, st, kind, watchCh, cosistate.WithBootstrapContents(true)); err != nil {
			errs <- err

			return
		}

		for {
			select {
			case <-watchCtx.Done():
				errs <- watchCtx.Err()

				return
			case wrapped := <-watchCh:
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
			errs <- fmt.Errorf("decode kubespan peer status event: %w", err)

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
				errs <- fmt.Errorf("decode kubespan peer delete event: %w", err)

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
		errs <- err

		return err
	}

	return nil
}
