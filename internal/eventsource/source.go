package eventsource

import (
	"context"
	"time"
)

type EventType string

const (
	EventFullSync   EventType = "full_sync"
	EventPeerUpsert EventType = "peer_upsert"
	EventPeerDelete EventType = "peer_delete"
)

type Peer struct {
	ID            string
	Label         string
	LastHandshake time.Time
}

type Event struct {
	Type  EventType
	Peer  Peer
	Peers []Peer
	At    time.Time
}

type Source interface {
	Watch(ctx context.Context) (<-chan Event, <-chan error)
}

type Factory func() Source
