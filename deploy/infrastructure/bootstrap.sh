#!/usr/bin/env bash
# bootstrap.sh — Bootstrap a Kubernetes cluster on Hetzner Cloud via Cluster API
#
# This script:
#   1. Creates a local Kind management cluster
#   2. Installs the CAPI + Hetzner provider
#   3. Applies the cluster manifests (control-plane, workers)
#   4. Waits for the workload cluster to become ready
#   5. Pivots CAPI management to the workload cluster
#   6. Installs FluxCD for GitOps self-management
#   7. Deletes the local Kind cluster
#
# Prerequisites:
#   - kind, kubectl, clusterctl, hcloud, flux CLI installed
#   - A copy of clusterctl-settings.env with your credentials
#     (see clusterctl-settings.env for the template)
#
# Usage:
#   cp clusterctl-settings.env .env   # fill in real values
#   source .env
#   ./bootstrap.sh

set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
DEPLOY_DIR="$(cd "${SCRIPT_DIR}/.." && pwd)"

# ─── Load settings (fall back to the template if .env is absent) ─
if [[ -f "${SCRIPT_DIR}/.env" ]]; then
  # shellcheck source=.env
  source "${SCRIPT_DIR}/.env"
else
  echo "⚠  No .env found — falling back to clusterctl-settings.env template."
  echo "   Copy clusterctl-settings.env → .env and fill in real values."
  # shellcheck source=clusterctl-settings.env
  source "${SCRIPT_DIR}/clusterctl-settings.env"
fi

CLUSTER_NAME="${CLUSTER_NAME:-dkpbot-prod}"
FLUX_BRANCH="${FLUX_BRANCH:-main}"
KIND_CLUSTER="capi-management"

# ─── Required env check ─────────────────────────────────────────
: "${HCLOUD_TOKEN:?Set HCLOUD_TOKEN in .env}"
: "${GITHUB_TOKEN:?Set GITHUB_TOKEN in .env (needed for Flux bootstrap)}"
: "${GITHUB_USER:?Set GITHUB_USER in .env (GitHub owner for Flux)}"

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

echo "    Waiting for CAPI core controller..."
kubectl wait --for=condition=Available --timeout=300s \
  deployment/capi-controller-manager -n capi-system

echo "    Waiting for KubeadmControlPlane controller..."
kubectl wait --for=condition=Available --timeout=300s \
  deployment/capi-kubeadm-control-plane-controller-manager -n capi-kubeadm-control-plane-system

echo "    Waiting for KubeadmBootstrap controller..."
kubectl wait --for=condition=Available --timeout=300s \
  deployment/capi-kubeadm-bootstrap-controller-manager -n capi-kubeadm-bootstrap-system

echo "    Waiting for CAPH controllers to be ready..."
kubectl wait --for=condition=Available --timeout=300s \
  deployment/caph-controller-manager -n caph-system

echo "    Waiting for Hetzner CRDs to be established..."
kubectl wait --for=condition=Established --timeout=60s \
  crd/hetznerclusters.infrastructure.cluster.x-k8s.io \
  crd/hcloudmachinetemplates.infrastructure.cluster.x-k8s.io

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

echo "==> Step 10: Bootstrap FluxCD on the workload cluster (branch: ${FLUX_BRANCH})"
export KUBECONFIG="${SCRIPT_DIR}/${CLUSTER_NAME}.kubeconfig"

flux bootstrap github \
  --owner="${GITHUB_USER}" \
  --repository=discord-dkp-bot \
  --branch="${FLUX_BRANCH}" \
  --path=deploy/flux \
  --personal \
  --token-auth

echo "    Applying Helm values ConfigMaps for Flux HelmReleases..."
kubectl apply -f "${DEPLOY_DIR}/flux/kustomizations/helm-values-configmaps.yaml"

echo "    Applying Flux Kustomizations..."
kubectl apply -f "${DEPLOY_DIR}/flux/kustomizations/"

echo "    Creating DKP bot secrets placeholder (edit with real values)..."
kubectl -n flux-system create secret generic dkpbot-secrets \
  --from-literal=config.discord.token="REPLACE_ME" \
  --from-literal=config.discord.guild_id="REPLACE_ME" \
  --dry-run=client -o yaml | kubectl apply -f -

echo "    Creating CNPG S3 backup credentials..."
kubectl create namespace dkpbot --dry-run=client -o yaml | kubectl apply -f -
if [[ -n "${CNPG_S3_ACCESS_KEY:-}" && -n "${CNPG_S3_SECRET_KEY:-}" ]]; then
  kubectl -n dkpbot create secret generic backup-s3-credentials \
    --from-literal=ACCESS_KEY_ID="${CNPG_S3_ACCESS_KEY}" \
    --from-literal=ACCESS_SECRET_KEY="${CNPG_S3_SECRET_KEY}" \
    --dry-run=client -o yaml | kubectl apply -f -
  echo "    ✅ backup-s3-credentials Secret created."
else
  echo "    ⚠  CNPG_S3_ACCESS_KEY / CNPG_S3_SECRET_KEY not set in .env."
  echo "    CNPG backups will not work until you create the Secret manually:"
  echo "      kubectl -n dkpbot create secret generic backup-s3-credentials \\"
  echo "        --from-literal=ACCESS_KEY_ID=<your-key> \\"
  echo "        --from-literal=ACCESS_SECRET_KEY=<your-secret>"
fi

echo "==> Step 11: Clean up local Kind cluster"
unset KUBECONFIG
kind delete cluster --name "${KIND_CLUSTER}"

echo ""
echo "✅ Cluster '${CLUSTER_NAME}' is ready and self-managed via FluxCD!"
echo "   FluxCD is tracking branch: ${FLUX_BRANCH}"
echo ""
echo "Use the workload cluster:"
echo "  export KUBECONFIG=${SCRIPT_DIR}/${CLUSTER_NAME}.kubeconfig"
echo "  kubectl get nodes"
echo ""
echo "FluxCD will now reconcile all services from Git:"
echo "  flux get kustomizations"
echo "  flux get helmreleases -A"
if [[ "${FLUX_BRANCH}" != "main" ]]; then
  echo ""
  echo "⚠  Flux is tracking branch '${FLUX_BRANCH}', not 'main'."
  echo "   After testing, re-run with FLUX_BRANCH=main or update the"
  echo "   Flux GitRepository to point to 'main'."
fi
echo ""
echo "⚠  Update the dkpbot-secrets Secret with real Discord credentials:"
echo "  kubectl -n flux-system edit secret dkpbot-secrets"
echo ""
if [[ -z "${CNPG_S3_ACCESS_KEY:-}" || -z "${CNPG_S3_SECRET_KEY:-}" ]]; then
  echo "⚠  Create backup-s3-credentials in the dkpbot namespace with real"
  echo "  Hetzner Object Storage credentials for CNPG backups:"
  echo "    kubectl -n dkpbot create secret generic backup-s3-credentials \\"
  echo "      --from-literal=ACCESS_KEY_ID=<your-key> \\"
  echo "      --from-literal=ACCESS_SECRET_KEY=<your-secret>"
else
  echo "✅ CNPG S3 backup credentials configured."
fi
