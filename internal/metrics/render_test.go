package metrics

import (
	"strings"
	"testing"
	"time"

	"github.com/benturnkey/talos-state-metrics/internal/eventsource"
	"github.com/benturnkey/talos-state-metrics/internal/state"
)

func TestRenderIncludesPeerCountHandshakeAndWatchState(t *testing.T) {
	s := state.NewSnapshot()
	lastEvent := time.Unix(1234, 0).UTC()
	handshake := time.Unix(1200, 0).UTC()
	s.SetConnected(true, lastEvent)
	s.Apply(eventsource.Event{Type: eventsource.EventPeerUpsert, Peer: eventsource.Peer{ID: "peer-a", Label: "node-a", LastHandshake: &handshake}, At: lastEvent})
	s.Apply(eventsource.Event{Type: eventsource.EventPeerUpsert, Peer: eventsource.Peer{ID: "peer-b"}, At: lastEvent})

	got := Render(s.Copy())

	for _, want := range []string{
		"talos_kubespan_peer_count 2",
		"talos_kubespan_peer_last_handshake_seconds{peer_id=\"peer-a\",peer_label=\"node-a\"} 1200",
		"talos_state_metrics_watch_connected 1",
		"talos_state_metrics_last_event_timestamp_seconds 1234",
	} {
		if !strings.Contains(got, want) {
			t.Fatalf("expected rendered metrics to contain %q\n%s", want, got)
		}
	}
	if strings.Contains(got, "peer-b") {
		t.Fatalf("expected peer without handshake to omit handshake metric\n%s", got)
	}
}

func TestRenderEscapesPeerLabels(t *testing.T) {
	s := state.NewSnapshot()
	handshake := time.Unix(10, 0).UTC()
	s.SetConnected(true, handshake)
	s.Apply(eventsource.Event{Type: eventsource.EventPeerUpsert, Peer: eventsource.Peer{ID: "peer\\\"a", Label: "node\\\"a", LastHandshake: &handshake}, At: handshake})

	got := Render(s.Copy())
	if !strings.Contains(got, `peer_id="peer\\\"a",peer_label="node\\\"a"`) {
		t.Fatalf("expected escaped peer labels, got\n%s", got)
	}
}

func TestRenderOmitsPeerLabelWhenUnset(t *testing.T) {
	s := state.NewSnapshot()
	handshake := time.Unix(10, 0).UTC()
	s.SetConnected(true, handshake)
	s.Apply(eventsource.Event{Type: eventsource.EventPeerUpsert, Peer: eventsource.Peer{ID: "peer-a", LastHandshake: &handshake}, At: handshake})

	got := Render(s.Copy())
	if !strings.Contains(got, `talos_kubespan_peer_last_handshake_seconds{peer_id="peer-a"} 10`) {
		t.Fatalf("expected peer_id-only metric, got\n%s", got)
	}
	if strings.Contains(got, "peer_label=") {
		t.Fatalf("expected peer_label to be omitted when unset, got\n%s", got)
	}
}

func TestRenderOmitsPeerDerivedMetricsWhenDisconnected(t *testing.T) {
	s := state.NewSnapshot()
	handshake := time.Unix(10, 0).UTC()
	s.SetConnected(true, handshake)
	s.Apply(eventsource.Event{Type: eventsource.EventPeerUpsert, Peer: eventsource.Peer{ID: "peer-a", Label: "node-a", LastHandshake: &handshake}, At: handshake})
	s.SetConnected(false, handshake.Add(time.Second))

	got := Render(s.Copy())
	for _, unwanted := range []string{
		"talos_kubespan_peer_count",
		"talos_kubespan_peer_last_handshake_seconds",
		`peer_id="peer-a"`,
	} {
		if strings.Contains(got, unwanted) {
			t.Fatalf("expected disconnected render to omit %q\n%s", unwanted, got)
		}
	}
	for _, want := range []string{
		"talos_state_metrics_watch_connected 0",
		"talos_state_metrics_last_event_timestamp_seconds 11",
	} {
		if !strings.Contains(got, want) {
			t.Fatalf("expected disconnected render to contain %q\n%s", want, got)
		}
	}
}

func TestRenderUsesZeroForUnsetLastEvent(t *testing.T) {
	s := state.NewSnapshot()

	got := Render(s.Copy())
	if !strings.Contains(got, "talos_state_metrics_last_event_timestamp_seconds 0") {
		t.Fatalf("expected unset last event timestamp to render as 0, got\n%s", got)
	}
}
