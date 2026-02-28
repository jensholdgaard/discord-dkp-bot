# DKP Bot Migration Plan: JavaScript → Go on Hetzner Cloud

## Executive Summary

This document outlines the migration of the Discord DKP Bot from the legacy
JavaScript/Node.js implementation to the new Go implementation, deployed on
Hetzner Cloud using Cluster API (CAPI) for Kubernetes lifecycle management,
FluxCD for GitOps self-management, CloudNative-PG for PostgreSQL, and a
lightweight CNCF-aligned observability stack.

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
│  │         Self-managed by FluxCD (GitOps)           │    │
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
│  │  │   (PostgreSQL 16.6 HA)   │                     │    │
│  │  │   primary + 2 replicas   │                     │    │
│  │  └──────────────────────────┘                     │    │
│  │                                                    │    │
│  │  ┌──────────────────────────────────────────┐     │    │
│  │  │          Observability Stack              │     │    │
│  │  │  Grafana ─ Prometheus ─ Loki ─ Tempo     │     │    │
│  │  │  OTel Collector                          │     │    │
│  │  └──────────────────────────────────────────┘     │    │
│  │                                                    │    │
│  │  ┌──────────────────────────────────────────┐     │    │
│  │  │          FluxCD                           │     │    │
│  │  │  Watches Git repo → reconciles all above │     │    │
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
4. Install FluxCD            ── flux bootstrap ───►  Self-managing cluster
5. Delete local kind cluster
```

After step 4, FluxCD watches this Git repository and reconciles all
services (CNPG, observability, the bot itself) automatically. Pushing
a change to `main` triggers a cluster-wide reconciliation.

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
- `bootstrap.sh` — End-to-end bootstrap, pivot, and Flux install script

---

## 4  GitOps — FluxCD

### 4.1 Why FluxCD?

- **Self-managing cluster** — after the CAPI pivot, Flux reconciles every
  service from Git. No more manual `kubectl apply` or `helm install`.
- **Dependency ordering** — Flux Kustomizations express dependencies
  (e.g. CNPG operator before database cluster, database before bot).
- **Drift detection** — any manual change in the cluster is reverted to
  match the Git source of truth.
- **Safe upgrades** — update a Helm chart version or values file in Git,
  Flux rolls it out with automatic remediation on failure.

### 4.2 Reconciliation Flow

```
Git push → FluxCD detects change → Reconcile Kustomizations/HelmReleases
                                      │
                ┌─────────────────────┼─────────────────────┐
                ▼                     ▼                     ▼
         CNPG Operator        Observability Stack      DKP Bot
         (HelmRelease)        (HelmReleases +          (HelmRelease)
              │                raw manifests)
              ▼
         CNPG Cluster
         (Kustomization)
```

### 4.3 Manifests

See [`deploy/flux/`](../deploy/flux/) for:

- `flux-system.yaml` — GitRepository pointing at this repo
- `kustomizations/cnpg-operator.yaml` — CNPG operator HelmRelease
- `kustomizations/cnpg-cluster.yaml` — CNPG database Kustomization
- `kustomizations/observability.yaml` — Full observability HelmReleases
- `kustomizations/dkpbot.yaml` — DKP bot HelmRelease
- `kustomizations/helm-values-configmaps.yaml` — Helm values as ConfigMaps

### 4.4 Secrets Management

Sensitive values (Discord token, S3 credentials) are **not** stored in Git.
They are created as Kubernetes Secrets on the cluster:

```bash
# Discord bot credentials
kubectl -n flux-system create secret generic dkpbot-secrets \
  --from-literal=config.discord.token=YOUR_TOKEN \
  --from-literal=config.discord.guild_id=YOUR_GUILD_ID

# S3 backup credentials (in dkpbot namespace)
kubectl -n dkpbot create secret generic backup-s3-credentials \
  --from-literal=ACCESS_KEY_ID=YOUR_KEY \
  --from-literal=ACCESS_SECRET_KEY=YOUR_SECRET
```

For a fully GitOps approach, consider adding Mozilla SOPS or Sealed
Secrets to encrypt secrets in Git.

---

## 5  PostgreSQL Version Consistency

All environments use **PostgreSQL 16.6** to ensure identical behaviour
across development, testing, and production:

| Environment       | Image                                          |
|-------------------|------------------------------------------------|
| Production (CNPG) | `ghcr.io/cloudnative-pg/postgresql:16.6-bookworm` |
| Local dev (Compose)| `postgres:16.6-alpine`                        |
| Integration tests  | `postgres:16.6-alpine` (testcontainers)       |

> The base OS differs (bookworm vs alpine) but the PostgreSQL version
> is identical. This avoids subtle behavioural differences in SQL
> processing, extensions, and default parameters.

---

## 6  Database — CloudNative-PG

### 6.1 Why CloudNative-PG?

- **Kubernetes-native** PostgreSQL operator (CNCF Sandbox).
- **Automated failover** — promotes replicas on primary failure.
- **Declarative backups** to S3-compatible storage (Hetzner Object Storage).
- **WAL archiving** for point-in-time recovery.
- **No external dependencies** — runs entirely inside the cluster.

### 6.2 Cluster Definition

See [`deploy/cloudnative-pg/`](../deploy/cloudnative-pg/) for:

- `operator.yaml` — Install the CNPG operator
- `cluster.yaml` — 3-instance PostgreSQL cluster with backup to Hetzner S3
- `scheduled-backup.yaml` — Daily backup CronJob

### 6.3 Connection from DKP Bot

The CNPG operator creates a Secret `dkpbot-db-app` containing:
- `host`, `port`, `dbname`, `user`, `password`, `uri`

The Helm chart is updated to mount this Secret instead of inline credentials.

---

## 7  Observability

### 7.1 Stack Selection

| Concern | Tool              | Reason                               |
|---------|-------------------|--------------------------------------|
| Metrics | Prometheus        | De-facto standard, OTel compatible   |
| Logs    | Grafana Loki      | Lightweight, S3-backed               |
| Traces  | Grafana Tempo     | Pairs with Loki, low overhead        |
| Dashboards | Grafana        | Unified UI for metrics/logs/traces   |
| Collection | OTel Collector | Already used by the bot, CNCF native |

### 7.2 Deployment

See [`deploy/observability/`](../deploy/observability/) for:

- `namespace.yaml` — `observability` namespace
- `kube-prometheus-stack-values.yaml` — Helm values for Prometheus + Grafana
- `loki-values.yaml` — Helm values for Grafana Loki
- `tempo-values.yaml` — Helm values for Grafana Tempo
- `otel-collector.yaml` — OTel Collector DaemonSet
- `install.sh` — Script to install all components via Helm

### 7.3 Integration

The bot already emits OTLP traces, metrics, and logs. Configure:

```yaml
telemetry:
  otlp_endpoint: "otel-collector.observability.svc:4318"
  insecure: true
```

---

## 8  Data Migration (MongoDB → PostgreSQL)

### 8.1 Strategy

1. **Export** existing DKP data from MongoDB as JSON.
2. **Transform** into PostgreSQL-compatible INSERT statements.
3. **Import** into the CNPG cluster using `psql` or a migration script.
4. **Validate** totals match between old and new systems.

### 8.2 Data Mapping

| MongoDB Collection | PostgreSQL Table  | Notes                      |
|-------------------|-------------------|----------------------------|
| `players`         | `players`         | discord_id, character, dkp |
| `auctions`        | `auctions`        | item, status, winner       |
| `bids`            | (event store)     | Converted to bid events    |
| `guild_config`    | (config.yaml)     | Static configuration       |

---

## 9  Phased Rollout

### Phase 1 — Infrastructure (Week 1–2)

- [ ] Bootstrap Hetzner cluster with CAPI
- [ ] Pivot CAPI to the workload cluster
- [ ] Install FluxCD (automated by bootstrap.sh)
- [ ] Verify Flux reconciles CNPG, observability, and bot
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

## 10  Cost Estimate (Monthly)

| Resource              | Hetzner Product   | Cost       |
|-----------------------|-------------------|------------|
| 3× Control Plane      | CPX21             | ~€24       |
| 2× Worker Nodes       | CPX21             | ~€16       |
| Object Storage (backups) | 100 GB         | ~€5        |
| **Total**             |                   | **~€45**   |

---

## 11  Risks & Mitigations

| Risk                                 | Mitigation                                |
|--------------------------------------|-------------------------------------------|
| CAPI provider immaturity             | CAPH is well-maintained; fallback to Terraform |
| Data loss during migration           | Full MongoDB backup before migration      |
| Bot downtime during switch           | Parallel run; instant rollback via token swap |
| Hetzner region outage                | Daily S3 backups; documented restore procedure |
| Flux drift from manual changes       | Flux auto-reverts drift; use `flux suspend` for maintenance |

---

## 12  Next Steps

1. Review and approve this plan.
2. Create Hetzner Cloud project and generate API token.
3. Create GitHub PAT with `repo` scope for FluxCD.
4. Run `deploy/infrastructure/bootstrap.sh` to provision the cluster and install Flux.
5. Flux auto-deploys database, observability, and bot from Git.
6. Populate secrets (Discord token, S3 credentials) on the cluster.
7. Begin data migration from MongoDB.
