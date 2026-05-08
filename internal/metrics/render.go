package metrics

import (
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/benturnkey/talos-state-metrics/internal/state"
)

func Render(snapshot state.Data) string {
	var b strings.Builder

	if snapshot.Connected {
		b.WriteString("# HELP talos_kubespan_peer_count Number of KubeSpan peers visible to the local Talos node.\n")
		b.WriteString("# TYPE talos_kubespan_peer_count gauge\n")
		fmt.Fprintf(&b, "talos_kubespan_peer_count %d\n", len(snapshot.Peers))

		b.WriteString("# HELP talos_kubespan_peer_last_handshake_seconds Last KubeSpan peer handshake timestamp.\n")
		b.WriteString("# TYPE talos_kubespan_peer_last_handshake_seconds gauge\n")
		ids := make([]string, 0, len(snapshot.Peers))
		for id := range snapshot.Peers {
			ids = append(ids, id)
		}
		sort.Strings(ids)
		for _, id := range ids {
			peer := snapshot.Peers[id]
			if peer.LastHandshake == nil {
				continue
			}
			fmt.Fprintf(&b, "talos_kubespan_peer_last_handshake_seconds{peer_id=\"%s\"", escapeLabelValue(peer.ID))
			if peer.Label != "" {
				fmt.Fprintf(&b, ",peer_label=\"%s\"", escapeLabelValue(peer.Label))
			}
			fmt.Fprintf(&b, "} %d\n", peer.LastHandshake.Unix())
		}
	}

	b.WriteString("# HELP talos_state_metrics_watch_connected Whether the Talos watch is currently connected.\n")
	b.WriteString("# TYPE talos_state_metrics_watch_connected gauge\n")
	if snapshot.Connected {
		b.WriteString("talos_state_metrics_watch_connected 1\n")
	} else {
		b.WriteString("talos_state_metrics_watch_connected 0\n")
	}

	b.WriteString("# HELP talos_state_metrics_last_event_timestamp_seconds Unix timestamp of the latest watch or connection state event.\n")
	b.WriteString("# TYPE talos_state_metrics_last_event_timestamp_seconds gauge\n")
	fmt.Fprintf(&b, "talos_state_metrics_last_event_timestamp_seconds %d\n", lastEventUnix(snapshot.LastEvent))

	return b.String()
}

func lastEventUnix(at time.Time) int64 {
	if at.IsZero() {
		return 0
	}

	return at.Unix()
}

func escapeLabelValue(value string) string {
	value = strings.ReplaceAll(value, `\`, `\\`)
	value = strings.ReplaceAll(value, "\n", `\n`)
	value = strings.ReplaceAll(value, `"`, `\"`)
	return value
}
