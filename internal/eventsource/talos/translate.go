package talos

import (
	"time"

	"github.com/tkhq/talos-state-metrics/internal/eventsource"
	taloskubespan "github.com/siderolabs/talos/pkg/machinery/resources/kubespan"
)

func peerFromStatus(peerStatus *taloskubespan.PeerStatus) eventsource.Peer {
	peer := deletePeerFromStatus(peerStatus)
	lastHandshake := peerStatus.TypedSpec().LastHandshakeTime
	if !lastHandshake.IsZero() {
		peer.LastHandshake = lastHandshake.UTC()
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
