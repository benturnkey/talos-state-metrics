# Infrastructure Testing

This directory contains the automated infrastructure testing suite for `talos-state-metrics`.

## Overview

The suite provisions a real 3-worker Talos cluster on AWS, installs Prometheus Operator via Helm, builds a temporary kustomize overlay on top of the repo root deployment base, rewrites the exporter image to a caller-specified artifact, and verifies that Prometheus can scrape exporter self-metrics and peer-derived metrics from every healthy target.

## Prerequisites

All required tools are provided by the project's Nix flake. Run `nix develop` to enter the environment.

Required tools:
- `terraform`
- `talosctl`
- `kubectl`
- `helm`
- `infracost` (optional, for cost estimates)
- `go`
- `yq`

You must provide the exporter image to test:

```bash
export EXPORTER_IMAGE=ghcr.io/<org>/talos-state-metrics:<tag>
```

The harness intentionally does not default to `:latest`, because it is meant to test the branch artifact you specify.

## Cost Management

### Pre-deployment Estimate
To see an estimated monthly cost for the test cluster:
```bash
./infracost.sh
```

### Actual Spend Tracking
All resources are tagged with:
- `Project: talos-state-metrics-test`
- `ManagedBy: terraform`

You can use these tags in AWS Cost Explorer to track actual expenditures for test runs.

## Running the Tests

To run the full test suite (provisioning, deployment, and verification):

```bash
./run-test.sh
```

The script will:
1. Generate Talos cluster secrets in a temporary directory.
2. Provision the VPC and nodes via Terraform.
3. Generate Talos and Kubernetes client configs in a temporary directory.
4. Create the exporter reader Secret in-cluster.
5. Install `kube-prometheus-stack` via Helm with PodMonitor discovery enabled.
6. Build a temporary kustomize overlay from the repo root and rewrite the exporter image to `EXPORTER_IMAGE`.
7. Deploy the rewritten exporter manifests and the `PodMonitor`.
8. Run the Go verification program to validate that each healthy exporter target exposes both self-metrics and peer-derived metrics in Prometheus.

## Cleanup

The harness cleans up generated local secrets/config files automatically on exit. Terraform-managed cloud resources are not destroyed automatically.

To tear down cloud resources:

```bash
cd terraform
terraform destroy -var="talos_secrets=$(cat /path/to/generated/talos_secrets.yaml | yq -o json)"
```

If you want a reusable manual destroy flow, save the generated Talos secrets file from the same test run before cleanup and pass that file to `terraform destroy`.
