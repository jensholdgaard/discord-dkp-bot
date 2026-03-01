#!/usr/bin/env bash
# bootstrap.sh — Bootstrap a self-managed Kubernetes cluster on Hetzner Cloud
#
# Uses capictl (https://github.com/nicholasdille/capictl) to:
#   1. Create a local Kind management cluster
#   2. Install CAPI + Hetzner provider (CAPH)
#   3. Provision the workload cluster using a pre-baked Hetzner snapshot
#   4. Install Cilium CNI + Hetzner CCM/CSI automatically via ClusterResourceSets
#   5. Pivot CAPI management to the workload cluster (self-managed)
#   6. Delete the local Kind cluster
#
# Then applies project-specific post-bootstrap steps:
#   7. Bootstrap FluxCD for GitOps
#   8. Create required secrets (Discord bot, CNPG S3 backups)
#
# Prerequisites:
#   - docker, kind, kubectl, clusterctl, hcloud, flux, helm, jq, yq installed
#   - A pre-built Hetzner snapshot (run: cd packer && packer build ubuntu.pkr.hcl)
#   - .env filled in (copy from clusterctl-settings.env and edit)
#
# Usage:
#   cp clusterctl-settings.env .env
#   # Edit .env with your values
#   ./bootstrap.sh

set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
DEPLOY_DIR="$(cd "${SCRIPT_DIR}/.." && pwd)"

# ─── Load settings ─────────────────────────────────────────────
if [[ -f "${SCRIPT_DIR}/.env" ]]; then
  # shellcheck source=.env
  source "${SCRIPT_DIR}/.env"
else
  echo "⚠  No .env found — copy clusterctl-settings.env → .env and fill in real values."
  source "${SCRIPT_DIR}/clusterctl-settings.env"
fi

# ─── Required variables ─────────────────────────────────────────
: "${HCLOUD_TOKEN:?Set HCLOUD_TOKEN in .env}"
GITHUB_TOKEN="${GITHUB_TOKEN:-$(gh auth token 2>/dev/null)}"
: "${GITHUB_TOKEN:?Could not obtain GITHUB_TOKEN — set it in .env or run: gh auth login}"
: "${GITHUB_USER:?Set GITHUB_USER in .env}"

export CLUSTER_NAME="${CLUSTER_NAME:-dkpbot-prod}"
export FLUX_BRANCH="${FLUX_BRANCH:-main}"

# capictl env vars (all have defaults in capictl; we set project-specific values)
export HCLOUD_TOKEN
export HCLOUD_REGION="${HCLOUD_REGION:-nbg1}"
export HCLOUD_CONTROL_PLANE_MACHINE_TYPE="${HCLOUD_CONTROL_PLANE_MACHINE_TYPE:-cx23}"
export HCLOUD_WORKER_MACHINE_TYPE="${HCLOUD_WORKER_MACHINE_TYPE:-cx23}"
export CONTROL_PLANE_NODE_COUNT="${CONTROL_PLANE_MACHINE_COUNT:-1}"
export WORKER_NODE_COUNT="${WORKER_MACHINE_COUNT:-1}"
# IMAGE_NAME is auto-detected by capictl from Hetzner snapshots (label caph-image-name)
# Override if you have multiple images and want a specific one:
# export IMAGE_NAME="ubuntu-24.04-amd64-k8s-1.31.6"

# ─── Validate Hetzner token ─────────────────────────────────────
echo "==> Pre-flight: Validating HCLOUD_TOKEN..."
HTTP_CODE=$(curl -s -o /dev/null -w "%{http_code}" \
  -H "Authorization: Bearer ${HCLOUD_TOKEN}" \
  "https://api.hetzner.cloud/v1/locations?per_page=1") || HTTP_CODE="000"
if [[ "${HTTP_CODE}" == "401" || "${HTTP_CODE}" == "403" ]]; then
  echo "ERROR: HCLOUD_TOKEN is invalid (HTTP ${HTTP_CODE})."
  exit 1
fi
[[ "${HTTP_CODE}" == "200" ]] && echo "    ✅ HCLOUD_TOKEN is valid."

# ─── Check Hetzner snapshot exists ──────────────────────────────
echo "==> Pre-flight: Checking for pre-baked Hetzner snapshot..."
IMAGE_COUNT=$(curl -sf -H "Authorization: Bearer ${HCLOUD_TOKEN}" \
  "https://api.hetzner.cloud/v1/images?type=snapshot&label_selector=caph-image-name" \
  | jq '.images | length')
if [[ "${IMAGE_COUNT}" -eq 0 ]]; then
  echo "ERROR: No Hetzner snapshot with label 'caph-image-name' found."
  echo "  Build the image first:"
  echo "    cd ${SCRIPT_DIR}/packer"
  echo "    packer init ."
  echo "    packer build -var 'kubernetes_version=1.31.6' ubuntu.pkr.hcl"
  exit 1
fi
echo "    ✅ Found ${IMAGE_COUNT} snapshot(s) with 'caph-image-name' label."

# ─── Clone capictl ──────────────────────────────────────────────
CAPICTL_DIR="${TMPDIR:-/tmp}/capictl-$$"
echo "==> Cloning capictl to ${CAPICTL_DIR}..."
git clone --depth=1 https://github.com/nicholasdille/capictl.git "${CAPICTL_DIR}"
trap 'rm -rf "${CAPICTL_DIR}"' EXIT

# ─── Run capictl ────────────────────────────────────────────────
echo "==> Running capictl (this takes 10-20 minutes)..."
cd "${CAPICTL_DIR}"
bash capictl \
  -n "${CLUSTER_NAME}" \
  -i hetzner \
  -b kind \
  -v "v${KUBERNETES_VERSION:-1.31.6}" \
  -c "${CONTROL_PLANE_NODE_COUNT}" \
  -w "${WORKER_NODE_COUNT}"

KUBECONFIG_FILE="${CAPICTL_DIR}/kubeconfig-${CLUSTER_NAME}"
if [[ ! -f "${KUBECONFIG_FILE}" ]]; then
  echo "ERROR: capictl did not produce ${KUBECONFIG_FILE}"
  exit 1
fi

# Copy kubeconfig to a stable path
cp "${KUBECONFIG_FILE}" "${SCRIPT_DIR}/${CLUSTER_NAME}.kubeconfig"
export KUBECONFIG="${SCRIPT_DIR}/${CLUSTER_NAME}.kubeconfig"
cd "${SCRIPT_DIR}"

echo "    ✅ Cluster '${CLUSTER_NAME}' is ready. Kubeconfig: ${CLUSTER_NAME}.kubeconfig"

# ─── Step: Bootstrap FluxCD ─────────────────────────────────────
echo "==> Bootstrapping FluxCD (branch: ${FLUX_BRANCH})..."
flux bootstrap github \
  --owner="${GITHUB_USER}" \
  --repository=discord-dkp-bot \
  --branch="${FLUX_BRANCH}" \
  --path=deploy/flux \
  --personal \
  --token-auth

echo "    Applying Flux resources..."
kubectl apply -f "${DEPLOY_DIR}/flux/kustomizations/helm-values-configmaps.yaml"
kubectl apply -f "${DEPLOY_DIR}/flux/kustomizations/"

# ─── Step: DKP bot secrets ──────────────────────────────────────
echo "==> Creating dkpbot-secrets placeholder..."
kubectl -n flux-system create secret generic dkpbot-secrets \
  --from-literal=config.discord.token="${DISCORD_TOKEN:-REPLACE_ME}" \
  --from-literal=config.discord.guild_id="${DISCORD_GUILD_ID:-REPLACE_ME}" \
  --dry-run=client -o yaml | kubectl apply -f -

# ─── Step: CNPG S3 backup credentials ───────────────────────────
echo "==> Creating CNPG namespace and backup credentials..."
kubectl create namespace dkpbot --dry-run=client -o yaml | kubectl apply -f -
if [[ -n "${CNPG_S3_ACCESS_KEY:-}" && -n "${CNPG_S3_SECRET_KEY:-}" ]]; then
  kubectl -n dkpbot create secret generic backup-s3-credentials \
    --from-literal=ACCESS_KEY_ID="${CNPG_S3_ACCESS_KEY}" \
    --from-literal=ACCESS_SECRET_KEY="${CNPG_S3_SECRET_KEY}" \
    --dry-run=client -o yaml | kubectl apply -f -
  echo "    ✅ backup-s3-credentials created."
else
  echo "    ⚠  CNPG_S3_ACCESS_KEY / CNPG_S3_SECRET_KEY not set — create the Secret manually:"
  echo "      kubectl -n dkpbot create secret generic backup-s3-credentials \\"
  echo "        --from-literal=ACCESS_KEY_ID=<key> \\"
  echo "        --from-literal=ACCESS_SECRET_KEY=<secret>"
fi

# ─── Summary ────────────────────────────────────────────────────
echo ""
echo "✅ Cluster '${CLUSTER_NAME}' is bootstrapped and self-managed via FluxCD!"
echo "   FluxCD tracking branch: ${FLUX_BRANCH}"
echo ""
echo "Use the cluster:"
echo "  export KUBECONFIG=${SCRIPT_DIR}/${CLUSTER_NAME}.kubeconfig"
echo "  kubectl get nodes"
echo ""
if [[ "${DISCORD_TOKEN:-REPLACE_ME}" == "REPLACE_ME" ]]; then
  echo "⚠  Update dkpbot-secrets with real Discord credentials:"
  echo "  kubectl -n flux-system edit secret dkpbot-secrets"
  echo ""
fi
