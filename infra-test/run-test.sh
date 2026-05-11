#!/bin/bash
set -euo pipefail

SCRIPT_DIR=$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")" && pwd)
REPO_ROOT=$(cd -- "${SCRIPT_DIR}/.." && pwd)
WORK_DIR=$(mktemp -d "${SCRIPT_DIR}/.tmp.XXXXXX")
TERRAFORM_DIR="${SCRIPT_DIR}/terraform"
OVERLAY_DIR="${WORK_DIR}/kustomize"
PF_PID=""

cleanup() {
    if [[ -n "${PF_PID}" ]] && kill -0 "${PF_PID}" 2>/dev/null; then
        kill "${PF_PID}"
    fi
    rm -rf "${WORK_DIR}"
}

write_kustomization_overlay() {
    local target=$1
    local image_name
    local image_tag
    local image_digest

    image_name="${EXPORTER_IMAGE}"
    image_tag=""
    image_digest=""

    if [[ "${EXPORTER_IMAGE}" == *@* ]]; then
        image_name="${EXPORTER_IMAGE%@*}"
        image_digest="${EXPORTER_IMAGE#*@}"
    elif [[ "${EXPORTER_IMAGE##*/}" == *:* ]]; then
        image_name="${EXPORTER_IMAGE%:*}"
        image_tag="${EXPORTER_IMAGE##*:}"
    fi

    cat > "${target}" <<EOF
apiVersion: kustomize.config.k8s.io/v1beta1
kind: Kustomization

resources:
  - ../..

images:
  - name: ghcr.io/benturnkey/talos-state-metrics
    newName: ${image_name}
EOF

    if [[ -n "${image_digest}" ]]; then
        cat >> "${target}" <<EOF
    digest: ${image_digest}
EOF
    elif [[ -n "${image_tag}" ]]; then
        cat >> "${target}" <<EOF
    newTag: ${image_tag}
EOF
    fi
}
trap cleanup EXIT

if [[ -z "${EXPORTER_IMAGE:-}" ]]; then
    echo "EXPORTER_IMAGE must point at the branch artifact to test, for example ghcr.io/<org>/talos-state-metrics:<tag>." >&2
    exit 1
fi

TALOS_SECRETS_PATH="${WORK_DIR}/talos_secrets.yaml"
TALOSCONFIG_PATH="${WORK_DIR}/talosconfig.yaml"
KUBECONFIG_PATH="${WORK_DIR}/kubeconfig.yaml"

echo "Generating Talos secrets..."
talosctl gen secrets -o "${TALOS_SECRETS_PATH}"

echo "Provisioning infrastructure..."
pushd "${TERRAFORM_DIR}" >/dev/null
terraform init
terraform apply -var="talos_secrets=$(cat "${TALOS_SECRETS_PATH}" | yq -o json)" -auto-approve

terraform output -raw talosconfig > "${TALOSCONFIG_PATH}"
ENDPOINT=$(terraform output -raw kubernetes_api_server_endpoint)
popd >/dev/null

echo "Waiting for LoadBalancer to be ready..."
sleep 30
talosctl --talosconfig "${TALOSCONFIG_PATH}" kubeconfig "${KUBECONFIG_PATH}" -n "${ENDPOINT}"
export KUBECONFIG="${KUBECONFIG_PATH}"

echo "Generating reader secret..."
kubectl create namespace monitoring --dry-run=client -o yaml | kubectl apply -f -
kubectl create secret generic talos-state-metrics-reader -n monitoring \
    --from-file=config="${TALOSCONFIG_PATH}" \
    --dry-run=client -o yaml | kubectl apply -f -

echo "Installing Prometheus Operator stack..."
helm upgrade --install monitoring-stack oci://ghcr.io/prometheus-community/charts/kube-prometheus-stack \
    --namespace monitoring \
    --create-namespace \
    --wait \
    --timeout 10m \
    -f "${SCRIPT_DIR}/helm-values.yaml"

echo "Deploying exporter and PodMonitor..."
mkdir -p "${OVERLAY_DIR}"
write_kustomization_overlay "${OVERLAY_DIR}/kustomization.yaml"
kubectl apply -k "${OVERLAY_DIR}"
kubectl apply -f "${SCRIPT_DIR}/manifests/podmonitor.yaml"

echo "Waiting for Exporter to be ready..."
kubectl rollout status daemonset/talos-state-metrics -n monitoring --timeout=300s

echo "Running verification program..."
PROMETHEUS_POD=$(kubectl get pods -n monitoring -l app.kubernetes.io/name=prometheus -o jsonpath='{.items[0].metadata.name}')
kubectl port-forward "pod/${PROMETHEUS_POD}" 9090:9090 -n monitoring &
PF_PID=$!
sleep 5

pushd "${SCRIPT_DIR}" >/dev/null
go run ./cmd/verify
popd >/dev/null

echo "TEST SUCCESSFUL"
