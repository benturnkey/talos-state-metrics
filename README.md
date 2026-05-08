# talos-state-metrics

`talos-state-metrics` is an initial Go Prometheus exporter for Talos node state. It is intended to run as a Kubernetes DaemonSet with one pod on each Talos node and expose local-node metrics from the Talos API.

Status: initial implementation. The HTTP server, snapshot model, Talos KubeSpan watch integration, metrics rendering, readiness behavior, tests, and Kubernetes examples are present.

## Metrics

- `talos_kubespan_peer_count`: gauge with the number of peers in the local snapshot.
- `talos_kubespan_peer_last_handshake_seconds{peer_id="<talos-resource-id>",peer_label="<talos-label>"}`: gauge with the peer's last handshake Unix timestamp. `peer_label` is omitted when Talos does not expose one. Peers without a last handshake timestamp are omitted.
- `talos_state_metrics_watch_connected`: gauge set to `1` while the Talos watch is connected and `0` otherwise.
- `talos_state_metrics_last_event_timestamp_seconds`: gauge with the latest watch event or connection state timestamp.

Labels are intentionally conservative in v0: no endpoint or redundant node label. The handshake series always uses `peer_id` from the Talos peer status resource ID and adds `peer_label` when Talos exposes a human-readable label.

## Authentication

The DaemonSet expects a mounted Talos client config and certificates with the Talos RBAC role `os:reader`. Generate a least-privileged Talos client configuration following the Talos RBAC documentation, confirm it can read local node resources, and store it in a Kubernetes Secret.

The example manifests mount that Secret at `/var/run/talos/config` and point the exporter at the local Talos API endpoint, defaulting to `127.0.0.1`. The pod uses `hostNetwork: true` so `127.0.0.1` refers to the node network namespace.

## Local Run

```bash
go test ./...
go run ./cmd/talos-state-metrics
```

Useful environment variables:

- `LISTEN_ADDR`: HTTP listen address, default `:8080`.
- `TALOS_ENDPOINT`: local Talos API endpoint, default `127.0.0.1`.
- `TALOS_CONFIG`: mounted Talos client config path, default `/var/run/talos/config`.
- `NODE_NAME`: Kubernetes node name, default host name.
- `WATCH_MIN_BACKOFF_SECONDS`: reconnect backoff floor, default `1`.
- `WATCH_MAX_BACKOFF_SECONDS`: reconnect backoff ceiling, default `30`.

## Deployment

Example manifests live in `deploy/`:

- `deploy/daemonset.yaml`: one exporter pod per Talos node.
- `deploy/service.yaml`: exposes port `8080` for scraping.
- `deploy/servicemonitor.yaml`: optional Prometheus Operator integration.
- `deploy/secret.example.yaml`: shape of the Talos reader config Secret.

Replace the image placeholder in the DaemonSet before applying the manifests.

## Infrastructure Testing

The project includes an automated infrastructure testing suite that provisions a real Talos cluster on AWS to verify metrics.

See [infra-test/README.md](infra-test/README.md) for instructions on running these tests and tracking infrastructure costs.

## Design

See `docs/design.md` for the approved design, watch behavior, and testing approach.
