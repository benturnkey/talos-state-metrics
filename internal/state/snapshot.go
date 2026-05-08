package state

import (
	"sync"
	"time"

	"github.com/benturnkey/talos-state-metrics/internal/eventsource"
)

type Data struct {
	Peers     map[string]eventsource.Peer
	Connected bool
	LastEvent time.Time
}

type Snapshot struct {
	mu        sync.RWMutex
	peers     map[string]eventsource.Peer
	connected bool
	lastEvent time.Time
}

func NewSnapshot() *Snapshot {
	return &Snapshot{peers: make(map[string]eventsource.Peer)}
}

func (s *Snapshot) Apply(event eventsource.Event) {
	if event.At.IsZero() {
		event.At = time.Now().UTC()
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	s.lastEvent = event.At
	switch event.Type {
	case eventsource.EventBootstrap:
		return
	case eventsource.EventPeerUpsert:
		if event.Peer.ID == "" {
			return
		}
		s.peers[event.Peer.ID] = clonePeer(event.Peer)
	case eventsource.EventPeerDelete:
		delete(s.peers, event.Peer.ID)
	}
}

func (s *Snapshot) SetConnected(connected bool, at time.Time) {
	if at.IsZero() {
		at = time.Now().UTC()
	}

	s.mu.Lock()
	defer s.mu.Unlock()
	s.connected = connected
	if !connected {
		s.peers = make(map[string]eventsource.Peer)
	}
	s.lastEvent = at
}

func (s *Snapshot) Ready() bool {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.connected
}

func (s *Snapshot) Copy() Data {
	s.mu.RLock()
	defer s.mu.RUnlock()

	peers := make(map[string]eventsource.Peer, len(s.peers))
	for id, peer := range s.peers {
		peers[id] = clonePeer(peer)
	}

	return Data{
		Peers:     peers,
		Connected: s.connected,
		LastEvent: s.lastEvent,
	}
}

func clonePeer(peer eventsource.Peer) eventsource.Peer {
	cloned := eventsource.Peer{ID: peer.ID, Label: peer.Label}
	if peer.LastHandshake != nil {
		handshake := *peer.LastHandshake
		cloned.LastHandshake = &handshake
	}
	return cloned
}
