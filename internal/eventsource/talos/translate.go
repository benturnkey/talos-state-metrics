package talos

import (
	"time"

	"github.com/benturnkey/talos-state-metrics/internal/eventsource"
	taloskubespan "github.com/siderolabs/talos/pkg/machinery/resources/kubespan"
)

func bootstrapEvent() eventsource.Event {
	return eventsource.Event{Type: eventsource.EventBootstrap, At: time.Now().UTC()}
}

func peerFromStatus(peerStatus *taloskubespan.PeerStatus) eventsource.Peer {
	peer := deletePeerFromStatus(peerStatus)
	lastHandshake := peerStatus.TypedSpec().LastHandshakeTime
	if !lastHandshake.IsZero() {
		handshake := lastHandshake.UTC()
		peer.LastHandshake = &handshake
	}

	return peer
}

func deletePeerFromStatus(peerStatus *taloskubespan.PeerStatus) eventsource.Peer {
	return eventsource.Peer{
		ID:    peerStatus.Metadata().ID(),
		Label: peerStatus.TypedSpec().Label,
	}
}

func eventTime(peerStatus *taloskubespan.PeerStatus) time.Time {
	updated := peerStatus.Metadata().Updated().UTC()
	if !updated.IsZero() {
		return updated
	}

	return time.Now().UTC()
}
