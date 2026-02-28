#!/usr/bin/env bash
# bootstrap.sh — Bootstrap a Kubernetes cluster on Hetzner Cloud via Cluster API
#
# This script:
#   1. Creates a local Kind management cluster
#   2. Installs the CAPI + Hetzner provider
#   3. Applies the cluster manifests (control-plane, workers)
#   4. Waits for the workload cluster to become ready
#   5. Pivots CAPI management to the workload cluster
#   6. Deletes the local Kind cluster
#
# Prerequisites:
#   - kind, kubectl, clusterctl, hcloud CLI installed
#   - clusterctl-settings.env populated with your Hetzner API token
#
# Usage:
#   ./bootstrap.sh

set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"

# ─── Load settings ──────────────────────────────────────────────
# shellcheck source=clusterctl-settings.env
source "${SCRIPT_DIR}/clusterctl-settings.env"

CLUSTER_NAME="${CLUSTER_NAME:-dkpbot-prod}"
KIND_CLUSTER="capi-management"

echo "==> Step 1: Create local Kind management cluster"
if kind get clusters 2>/dev/null | grep -q "^${KIND_CLUSTER}$"; then
  echo "    Kind cluster '${KIND_CLUSTER}' already exists, reusing."
else
  kind create cluster --name "${KIND_CLUSTER}"
fi
kubectl config use-context "kind-${KIND_CLUSTER}"

echo "==> Step 2: Install Cluster API with Hetzner provider"
export EXP_CLUSTER_RESOURCE_SET="true"
clusterctl init --infrastructure hetzner

echo "    Waiting for CAPH controllers to be ready..."
kubectl wait --for=condition=Available --timeout=300s \
  deployment/caph-controller-manager -n caph-system 2>/dev/null || true

echo "==> Step 3: Create the Hetzner secret"
kubectl create secret generic hetzner \
  --from-literal=hcloud="${HCLOUD_TOKEN}" \
  --dry-run=client -o yaml | kubectl apply -f -

echo "==> Step 4: Apply cluster manifests"
kubectl apply -f "${SCRIPT_DIR}/cluster.yaml"
kubectl apply -f "${SCRIPT_DIR}/control-plane.yaml"
kubectl apply -f "${SCRIPT_DIR}/workers.yaml"

echo "==> Step 5: Wait for workload cluster to be provisioned"
echo "    This may take 5-10 minutes..."
kubectl wait --for=condition=Ready --timeout=600s \
  "cluster/${CLUSTER_NAME}" || {
    echo "Cluster not ready after 10 minutes. Check:"
    echo "  kubectl describe cluster ${CLUSTER_NAME}"
    exit 1
  }

echo "==> Step 6: Get workload cluster kubeconfig"
clusterctl get kubeconfig "${CLUSTER_NAME}" > "${SCRIPT_DIR}/${CLUSTER_NAME}.kubeconfig"
echo "    Kubeconfig saved to ${SCRIPT_DIR}/${CLUSTER_NAME}.kubeconfig"

echo "==> Step 7: Install Hetzner Cloud Controller Manager on workload cluster"
export KUBECONFIG="${SCRIPT_DIR}/${CLUSTER_NAME}.kubeconfig"

kubectl create secret generic hetzner \
  --from-literal=token="${HCLOUD_TOKEN}" \
  --from-literal=network="${CLUSTER_NAME}" \
  -n kube-system --dry-run=client -o yaml | kubectl apply -f -

helm repo add hcloud https://charts.hetzner.cloud
helm repo update
helm install hccm hcloud/hcloud-cloud-controller-manager \
  -n kube-system \
  --set env.HCLOUD_TOKEN.valueFrom.secretKeyRef.name=hetzner \
  --set env.HCLOUD_TOKEN.valueFrom.secretKeyRef.key=token

echo "==> Step 8: Install Cilium CNI"
helm repo add cilium https://helm.cilium.io/
helm install cilium cilium/cilium \
  -n kube-system \
  --set ipam.mode=kubernetes

echo "    Waiting for nodes to become Ready..."
kubectl wait --for=condition=Ready nodes --all --timeout=300s

echo "==> Step 9: Pivot CAPI management to the workload cluster"
unset KUBECONFIG
clusterctl move \
  --to-kubeconfig="${SCRIPT_DIR}/${CLUSTER_NAME}.kubeconfig"

echo "==> Step 10: Clean up local Kind cluster"
kind delete cluster --name "${KIND_CLUSTER}"

echo ""
echo "✅ Cluster '${CLUSTER_NAME}' is ready!"
echo ""
echo "Use the workload cluster:"
echo "  export KUBECONFIG=${SCRIPT_DIR}/${CLUSTER_NAME}.kubeconfig"
echo "  kubectl get nodes"
echo ""
echo "Next steps:"
echo "  1. Install CloudNative-PG:  kubectl apply -f ../cloudnative-pg/"
echo "  2. Install observability:   bash ../observability/install.sh"
echo "  3. Deploy the DKP bot:      helm install dkpbot ../helm/dkpbot/"
