#!/usr/bin/env bash
# install.sh — Install the full observability stack on the workload cluster
#
# This script installs:
#   1. Namespace
#   2. kube-prometheus-stack (Prometheus + Grafana + Alertmanager)
#   3. Grafana Loki (log aggregation)
#   4. Grafana Tempo (distributed tracing)
#   5. OpenTelemetry Collector (OTLP → Tempo/Prometheus/Loki)
#
# Prerequisites:
#   - kubectl configured for the target cluster
#   - helm v3 installed
#
# Usage:
#   ./install.sh

set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
NAMESPACE="observability"

echo "==> Creating namespace"
kubectl apply -f "${SCRIPT_DIR}/namespace.yaml"

echo "==> Adding Helm repositories"
helm repo add prometheus-community https://prometheus-community.github.io/helm-charts
helm repo add grafana https://grafana.github.io/helm-charts
helm repo update

echo "==> Installing kube-prometheus-stack (Prometheus + Grafana)"
helm upgrade --install kube-prometheus-stack \
  prometheus-community/kube-prometheus-stack \
  -n "${NAMESPACE}" \
  -f "${SCRIPT_DIR}/kube-prometheus-stack-values.yaml" \
  --wait --timeout 5m

echo "==> Installing Grafana Loki"
helm upgrade --install loki grafana/loki \
  -n "${NAMESPACE}" \
  -f "${SCRIPT_DIR}/loki-values.yaml" \
  --wait --timeout 5m

echo "==> Installing Grafana Tempo"
helm upgrade --install tempo grafana/tempo \
  -n "${NAMESPACE}" \
  -f "${SCRIPT_DIR}/tempo-values.yaml" \
  --wait --timeout 5m

echo "==> Installing OpenTelemetry Collector"
kubectl apply -f "${SCRIPT_DIR}/otel-collector.yaml"

echo ""
echo "✅ Observability stack installed!"
echo ""
echo "Access Grafana:"
echo "  kubectl port-forward -n ${NAMESPACE} svc/kube-prometheus-stack-grafana 3000:80"
echo "  Open http://localhost:3000 (admin / changeme)"
echo ""
echo "Configure DKP bot telemetry:"
echo "  telemetry:"
echo "    otlp_endpoint: otel-collector.${NAMESPACE}.svc:4318"
echo "    insecure: true"
