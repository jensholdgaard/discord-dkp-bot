# DKP Bot Migration Plan: JavaScript → Go on Hetzner Cloud

## Executive Summary

This document outlines the migration of the Discord DKP Bot from the legacy
JavaScript/Node.js implementation to the new Go implementation, deployed on
Hetzner Cloud using Cluster API (CAPI) for Kubernetes lifecycle management,
CloudNative-PG for PostgreSQL, and a lightweight CNCF-aligned observability
stack.

---

## 1  Current State Assessment

### 1.1 Legacy JavaScript Bot

| Component        | Technology         | Notes                                    |
|------------------|--------------------|------------------------------------------|
| Runtime          | Node.js            | `discord.js` v14, MongoDB driver         |
| Database         | MongoDB            | Hosted externally                        |
| Deployment       | Manual / PM2       | No containerisation                      |
| Observability    | `console.log`      | Error log written to disk                |
| Commands         | Slash commands      | ~10 commands (DKP, auction, admin)       |

### 1.2 New Go Bot (already implemented)

| Component        | Technology                        | Status |
|------------------|-----------------------------------|--------|
| Runtime          | Go 1.25                           | ✅     |
| Database         | PostgreSQL 16 (sqlx + migrations) | ✅     |
| Discord          | `discordgo` – slash commands      | ✅     |
| Event Sourcing   | Append-only event log             | ✅     |
| Observability    | OpenTelemetry (traces/metrics/logs)| ✅    |
| Health Checks    | `/healthz`, `/readyz`             | ✅     |
| Leader Election  | Kubernetes Lease API              | ✅     |
| Helm Chart       | Production-ready                  | ✅     |
| CI/CD            | GitHub Actions + GoReleaser       | ✅     |
| Docker           | Multi-stage alpine image          | ✅     |

**Key insight:** The Go rewrite is feature-complete. The remaining work is
infrastructure provisioning and data migration from MongoDB → PostgreSQL.

---

## 2  Target Architecture

```
┌──────────────────────────────────────────────────────────┐
│                    Hetzner Cloud                         │
│                                                          │
│  ┌──────────────────────────────────────────────────┐    │
│  │         Kubernetes (via Cluster API)              │    │
│  │                                                    │    │
│  │  ┌─────────┐  ┌─────────┐  ┌─────────┐           │    │
│  │  │ dkpbot  │  │ dkpbot  │  │ dkpbot  │  (3 pods) │    │
│  │  │ replica │  │ replica │  │ replica │           │    │
│  │  └────┬────┘  └────┬────┘  └────┬────┘           │    │
│  │       │  Leader Election (Lease)  │               │    │
│  │       └──────────┬───────────────┘               │    │
│  │                  ▼                                │    │
│  │  ┌──────────────────────────┐                     │    │
│  │  │   CloudNative-PG         │                     │    │
│  │  │   (PostgreSQL 16 HA)     │                     │    │
│  │  │   primary + 2 replicas   │                     │    │
│  │  └──────────────────────────┘                     │    │
│  │                                                    │    │
│  │  ┌──────────────────────────────────────────┐     │    │
│  │  │          Observability Stack              │     │    │
│  │  │  Grafana ─ Prometheus ─ Loki ─ Tempo     │     │    │
│  │  │  OTel Collector                          │     │    │
│  │  └──────────────────────────────────────────┘     │    │
│  └──────────────────────────────────────────────────┘    │
└──────────────────────────────────────────────────────────┘
```

---

## 3  Infrastructure — Hetzner Cloud via Cluster API

### 3.1 Why Cluster API + Hetzner?

- **Declarative cluster lifecycle** — create, upgrade, and scale Kubernetes
  clusters using Kubernetes resources themselves.
- **Hetzner provider (CAPH)** — first-class CAPI provider for Hetzner Cloud
  and Hetzner dedicated (Robot) servers.
- **Pivot pattern** — bootstrap with a local Kind cluster, then pivot CAPI
  management to the workload cluster itself.

### 3.2 Bootstrap Flow

```
1. Local machine (kind)      ── clusterctl init ──►  Management cluster
2. Apply Cluster manifests   ── CAPH creates VMs ──► Workload cluster
3. Pivot CAPI controllers    ── clusterctl move ──►  Workload cluster
4. Delete local kind cluster
```

### 3.3 Cluster Sizing (MVP)

| Role          | Hetzner Type | Count | Purpose               |
|---------------|-------------|-------|-----------------------|
| Control Plane | CPX21       | 3     | HA control plane      |
| Workers       | CPX21       | 2     | Bot + database + obs  |

> CPX21 = 3 vCPU, 4 GB RAM, 80 GB disk — ~€8/month each. Total: ~€40/month.

### 3.4 Manifests

See [`deploy/infrastructure/`](../deploy/infrastructure/) for:

- `clusterctl-settings.env` — CAPI environment variables
- `cluster.yaml` — HetznerCluster + Cluster resources
- `control-plane.yaml` — KubeadmControlPlane (3 replicas)
- `workers.yaml` — MachineDeployment for worker nodes
- `bootstrap.sh` — End-to-end bootstrap and pivot script

---

## 4  Database — CloudNative-PG

### 4.1 Why CloudNative-PG?

- **Kubernetes-native** PostgreSQL operator (CNCF Sandbox).
- **Automated failover** — promotes replicas on primary failure.
- **Declarative backups** to S3-compatible storage (Hetzner Object Storage).
- **WAL archiving** for point-in-time recovery.
- **No external dependencies** — runs entirely inside the cluster.

### 4.2 Cluster Definition

See [`deploy/cloudnative-pg/`](../deploy/cloudnative-pg/) for:

- `operator.yaml` — Install the CNPG operator
- `cluster.yaml` — 3-instance PostgreSQL cluster with backup to Hetzner S3
- `scheduled-backup.yaml` — Daily backup CronJob

### 4.3 Connection from DKP Bot

The CNPG operator creates a Secret `dkpbot-db-app` containing:
- `host`, `port`, `dbname`, `user`, `password`, `uri`

The Helm chart is updated to mount this Secret instead of inline credentials.

---

## 5  Observability

### 5.1 Stack Selection

| Concern | Tool              | Reason                               |
|---------|-------------------|--------------------------------------|
| Metrics | Prometheus        | De-facto standard, OTel compatible   |
| Logs    | Grafana Loki      | Lightweight, S3-backed               |
| Traces  | Grafana Tempo     | Pairs with Loki, low overhead        |
| Dashboards | Grafana        | Unified UI for metrics/logs/traces   |
| Collection | OTel Collector | Already used by the bot, CNCF native |

### 5.2 Deployment

See [`deploy/observability/`](../deploy/observability/) for:

- `namespace.yaml` — `observability` namespace
- `kube-prometheus-stack-values.yaml` — Helm values for Prometheus + Grafana
- `loki-values.yaml` — Helm values for Grafana Loki
- `tempo-values.yaml` — Helm values for Grafana Tempo
- `otel-collector.yaml` — OTel Collector DaemonSet
- `install.sh` — Script to install all components via Helm

### 5.3 Integration

The bot already emits OTLP traces, metrics, and logs. Configure:

```yaml
telemetry:
  otlp_endpoint: "otel-collector.observability.svc:4318"
  insecure: true
```

---

## 6  Data Migration (MongoDB → PostgreSQL)

### 6.1 Strategy

1. **Export** existing DKP data from MongoDB as JSON.
2. **Transform** into PostgreSQL-compatible INSERT statements.
3. **Import** into the CNPG cluster using `psql` or a migration script.
4. **Validate** totals match between old and new systems.

### 6.2 Data Mapping

| MongoDB Collection | PostgreSQL Table  | Notes                      |
|-------------------|-------------------|----------------------------|
| `players`         | `players`         | discord_id, character, dkp |
| `auctions`        | `auctions`        | item, status, winner       |
| `bids`            | (event store)     | Converted to bid events    |
| `guild_config`    | (config.yaml)     | Static configuration       |

---

## 7  Phased Rollout

### Phase 1 — Infrastructure (Week 1–2)

- [ ] Bootstrap Hetzner cluster with CAPI
- [ ] Install CloudNative-PG operator and create cluster
- [ ] Install observability stack
- [ ] Verify end-to-end connectivity

### Phase 2 — Deploy Bot (Week 2–3)

- [ ] Deploy DKP bot via Helm to Hetzner cluster
- [ ] Run database migrations
- [ ] Validate slash commands work end-to-end
- [ ] Set up OTLP pipeline and confirm dashboards

### Phase 3 — Data Migration (Week 3–4)

- [ ] Export MongoDB data
- [ ] Transform and import into PostgreSQL
- [ ] Run parallel validation (JS bot read-only, Go bot active)
- [ ] Switch Discord bot token to Go bot
- [ ] Decommission JS bot

### Phase 4 — Production Hardening (Week 4+)

- [ ] Enable scheduled CNPG backups to Hetzner S3
- [ ] Configure Grafana alerting (bot health, DKP anomalies)
- [ ] Enable leader election (3 replicas)
- [ ] Document runbooks for common operations

---

## 8  Cost Estimate (Monthly)

| Resource              | Hetzner Product   | Cost       |
|-----------------------|-------------------|------------|
| 3× Control Plane      | CPX21             | ~€24       |
| 2× Worker Nodes       | CPX21             | ~€16       |
| Object Storage (backups) | 100 GB         | ~€5        |
| **Total**             |                   | **~€45**   |

---

## 9  Risks & Mitigations

| Risk                                 | Mitigation                                |
|--------------------------------------|-------------------------------------------|
| CAPI provider immaturity             | CAPH is well-maintained; fallback to Terraform |
| Data loss during migration           | Full MongoDB backup before migration      |
| Bot downtime during switch           | Parallel run; instant rollback via token swap |
| Hetzner region outage                | Daily S3 backups; documented restore procedure |

---

## 10  Next Steps

1. Review and approve this plan.
2. Create Hetzner Cloud project and generate API token.
3. Run `deploy/infrastructure/bootstrap.sh` to provision the cluster.
4. Install database and observability stacks.
5. Deploy the Go bot and begin data migration.
