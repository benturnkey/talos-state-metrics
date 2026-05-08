package state

import (
	"testing"
	"time"

	"github.com/benturnkey/talos-state-metrics/internal/eventsource"
)

func TestSnapshotAppliesPeerAddUpdateAndDelete(t *testing.T) {
	s := NewSnapshot()
	first := time.Unix(100, 0).UTC()
	second := time.Unix(200, 0).UTC()

	s.Apply(eventsource.Event{Type: eventsource.EventPeerUpsert, Peer: eventsource.Peer{ID: "peer-a", LastHandshake: &first}, At: first})
	s.Apply(eventsource.Event{Type: eventsource.EventPeerUpsert, Peer: eventsource.Peer{ID: "peer-a", LastHandshake: &second}, At: second})

	snap := s.Copy()
	peer, ok := snap.Peers["peer-a"]
	if !ok {
		t.Fatalf("expected peer-a to exist")
	}
	if peer.LastHandshake == nil || !peer.LastHandshake.Equal(second) {
		t.Fatalf("expected updated handshake %s, got %#v", second, peer.LastHandshake)
	}
	if !snap.LastEvent.Equal(second) {
		t.Fatalf("expected last event timestamp %s, got %s", second, snap.LastEvent)
	}

	s.Apply(eventsource.Event{Type: eventsource.EventPeerDelete, Peer: eventsource.Peer{ID: "peer-a"}, At: second.Add(time.Second)})
	if got := len(s.Copy().Peers); got != 0 {
		t.Fatalf("expected peer delete to remove peer, got %d peers", got)
	}
}

func TestSnapshotTracksReadinessFromWatchConnection(t *testing.T) {
	s := NewSnapshot()
	if s.Ready() {
		t.Fatalf("new snapshot should not be ready")
	}

	s.SetConnected(true, time.Unix(300, 0).UTC())
	if !s.Ready() {
		t.Fatalf("snapshot should be ready while watch is connected")
	}

	s.SetConnected(false, time.Unix(301, 0).UTC())
	if s.Ready() {
		t.Fatalf("snapshot should not be ready after watch disconnect")
	}
}

func TestSnapshotClearsPeersOnDisconnect(t *testing.T) {
	s := NewSnapshot()
	handshake := time.Unix(100, 0).UTC()

	s.SetConnected(true, handshake)
	s.Apply(eventsource.Event{Type: eventsource.EventPeerUpsert, Peer: eventsource.Peer{ID: "peer-a", LastHandshake: &handshake}, At: handshake})
	s.SetConnected(false, handshake.Add(time.Second))

	snap := s.Copy()
	if len(snap.Peers) != 0 {
		t.Fatalf("expected peers to be cleared on disconnect, got %d", len(snap.Peers))
	}
	if !snap.LastEvent.Equal(handshake.Add(time.Second)) {
		t.Fatalf("expected disconnect timestamp %s, got %s", handshake.Add(time.Second), snap.LastEvent)
	}
}
