# talos-state-metrics Design

## Goals

- Export Talos KubeSpan peer metrics for each Talos node.
- Run as a small Go binary in a Kubernetes DaemonSet with one pod per node.
- Authenticate to the local Talos API with a mounted client config and certificates using the Talos RBAC role `os:reader`.
- Keep metric labels conservative and avoid exposing public keys, endpoints, or redundant node labels.

## Non-Goals

- No cluster-wide aggregation in v0.
- No peer public key, endpoint, or node labels in v0 metrics.
- No requirement for CI to contact a real Talos node.
- No broad Talos resource inventory beyond the KubeSpan peer resource.

## Architecture

The exporter starts an HTTP server on port `8080` and a background watch manager. The watch manager constructs a Talos watch source, consumes peer add, update, and delete events, and applies them to a concurrency-safe in-memory snapshot. The `/metrics` handler renders Prometheus text directly from a snapshot copy.

The Talos-specific code is isolated in `internal/eventsource/talos`. It uses the Talos machinery client and COSI watch API to follow local `KubeSpanPeerStatuses.kubespan.talos.dev` resources. Core packages depend on the generic `internal/eventsource.Source` interface so tests can use synthetic events without a Talos API server.

## Authentication

The runtime expects a Talos client config mounted at `/var/run/talos/config` by default. That config should contain certificates generated for the Talos RBAC role `os:reader`, giving read-only access to the local node resources needed for KubeSpan peer state.

The DaemonSet uses `hostNetwork: true` and defaults `TALOS_ENDPOINT` to `127.0.0.1`, so the exporter talks to the node-local Talos API from the host network namespace.

## Metrics

- `talos_kubespan_peer_count`: total number of peers in the snapshot.
- `talos_kubespan_peer_last_handshake_seconds{peer_id="<talos-resource-id>",peer_label="<talos-label>"}`: last handshake timestamp for peers that expose one, with `peer_label` omitted when absent.
- `talos_state_metrics_watch_connected`: `1` when the watch is connected, `0` when disconnected or reconnecting.
- `talos_state_metrics_last_event_timestamp_seconds`: Unix timestamp of the latest watch event or connection state change.

The exporter treats the Talos peer status resource ID as the canonical peer identity and uses it for `peer_id`. When Talos exposes a peer label, the exporter also publishes `peer_label` so dashboards can show human-readable names and operators can detect label churn independently from peer identity churn.
Kubernetes node identity should come from Prometheus scrape target metadata such as the pod's node name, not from an exporter-emitted metric label.

## Watch Behavior

The exporter uses a watch-first design:

1. Establish a Talos watch for the local node's KubeSpan peer resource.
2. Load the current full peer set from the watch bootstrap before marking readiness true.
3. Apply add/update/delete events to the in-memory snapshot.
4. Periodically relist the full peer set and replace the snapshot contents to repair drift.

The Talos source watches local KubeSpan peer status resources through the Talos machinery client and converts the bootstrapped startup set into a full-sync event, then follows with upsert, delete, and periodic full-sync events. The watch manager does not mark readiness until the initial full-sync barrier has arrived, so the exporter only serves ready after it has the complete startup peer set.

## Failure Scenarios

The exporter is expected to fail closed for peer-derived metrics rather than continue serving stale or misleading data.

- Before the watch has produced its first full-sync event, readiness remains false.
- If the watch disconnects or returns an error after becoming ready, readiness flips false and peer-state metrics stop being reported.
- If the Talos API is unavailable, authentication fails, or the watch cannot be established, the exporter keeps retrying with bounded exponential backoff.
- While disconnected or reconnecting, `talos_state_metrics_watch_connected` reports `0` and `talos_state_metrics_last_event_timestamp_seconds` reflects the latest event or connection state transition.

These failure modes are meant to make degraded behavior obvious to operators and Prometheus. The exporter should prefer missing peer-derived series over stale values that imply the local Talos watch is still healthy.

## Deployment

The deployment target is a Kubernetes DaemonSet. Each pod mounts the Talos reader client config Secret and exposes port `8080` with `/metrics`, `/healthz`, and `/readyz`.

The example Service and ServiceMonitor select pods labeled `app.kubernetes.io/name: talos-state-metrics`. The DaemonSet image is intentionally a placeholder for the first release pipeline.

## Testing

The documented verification command is `go test ./...`. GitHub Actions CI is intentionally omitted from this initial PR because the GitHub App credential used to push the branch lacks `workflows` permission. Tests cover:

- Metric rendering from synthetic snapshots.
- Watch event application for add, update, and delete behavior.
- Readiness transitions across connected and disconnected watch states.

Tests do not require a real Talos node. Future integration tests should use a fake Talos API or recorded KubeSpan peer watch events.
