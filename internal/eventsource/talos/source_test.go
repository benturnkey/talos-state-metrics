package talos

import (
	"testing"
	"time"

	"github.com/siderolabs/talos/pkg/machinery/resources/kubespan"
)

func TestPeerFromStatusUsesLabelAndHandshake(t *testing.T) {
	handshake := time.Unix(123, 0).UTC()
	peerStatus := kubespan.NewPeerStatus(kubespan.NamespaceName, "pubkey-a")
	peerStatus.TypedSpec().Label = "node-a"
	peerStatus.TypedSpec().LastHandshakeTime = handshake

	peer := peerFromStatus(peerStatus)
	if peer.ID != "pubkey-a" {
		t.Fatalf("expected metadata-backed peer ID, got %q", peer.ID)
	}
	if peer.Label != "node-a" {
		t.Fatalf("expected peer label, got %q", peer.Label)
	}
	if peer.LastHandshake == nil || !peer.LastHandshake.Equal(handshake) {
		t.Fatalf("expected handshake %s, got %#v", handshake, peer.LastHandshake)
	}
}

func TestPeerFromStatusOmitsLabelWhenUnset(t *testing.T) {
	peerStatus := kubespan.NewPeerStatus(kubespan.NamespaceName, "pubkey-a")
	peer := peerFromStatus(peerStatus)

	if peer.ID != "pubkey-a" {
		t.Fatalf("expected metadata ID, got %q", peer.ID)
	}
	if peer.Label != "" {
		t.Fatalf("expected empty label, got %q", peer.Label)
	}
}

func TestEventTimeUsesUpdatedTimestamp(t *testing.T) {
	peerStatus := kubespan.NewPeerStatus(kubespan.NamespaceName, "pubkey-a")
	updated := time.Unix(456, 0).UTC()
	peerStatus.Metadata().SetUpdated(updated)

	if got := eventTime(peerStatus); !got.Equal(updated) {
		t.Fatalf("expected updated timestamp %s, got %s", updated, got)
	}
}
