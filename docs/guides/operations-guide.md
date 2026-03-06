# Artstore Operations Guide

> **Language**: [English](operations-guide.md) | [Русский](operations-guide.ru.md)
>
> **Related guides**: [Admin Guide](admin-guide.md) | [Developer Guide](developer-guide.md)

---

## Table of Contents

- [1. Monitoring Overview](#1-monitoring-overview)
  - [1.1 Metrics Architecture](#11-metrics-architecture)
  - [1.2 Prometheus Metrics Reference](#12-prometheus-metrics-reference)
  - [1.3 Prometheus Scraping Configuration](#13-prometheus-scraping-configuration)
  - [1.4 Dependency Health Monitoring (topologymetrics)](#14-dependency-health-monitoring-topologymetrics)
  - [1.5 Built-in Monitoring in Admin UI](#15-built-in-monitoring-in-admin-ui)
- [2. Grafana Dashboards](#2-grafana-dashboards)
  - [2.1 Dashboard Provisioning](#21-dashboard-provisioning)
  - [2.2 Artstore Overview Dashboard](#22-artstore-overview-dashboard)
  - [2.3 Admin Module Dashboard](#23-admin-module-dashboard)
  - [2.4 Storage Element Dashboard](#24-storage-element-dashboard)
  - [2.5 Ingester Module Dashboard](#25-ingester-module-dashboard)
  - [2.6 Query Module Dashboard](#26-query-module-dashboard)
  - [2.7 Dependency Topology Dashboard](#27-dependency-topology-dashboard)
  - [2.8 Template Variables](#28-template-variables)
- [3. Alerting](#3-alerting)
  - [3.1 AlertManager Integration](#31-alertmanager-integration)
  - [3.2 Alert Reference](#32-alert-reference)
  - [3.3 Runbooks](#33-runbooks)
  - [3.4 Silencing and Inhibition](#34-silencing-and-inhibition)
  - [3.5 Notification Channels](#35-notification-channels)
- [4. Troubleshooting](#4-troubleshooting)
  - [4.1 Health Check Endpoints](#41-health-check-endpoints)
  - [4.2 Dependency Diagnostics via topologymetrics](#42-dependency-diagnostics-via-topologymetrics)
  - [4.3 Log Analysis (slog JSON)](#43-log-analysis-slog-json)
  - [4.4 Common Problems and Solutions](#44-common-problems-and-solutions)
- [5. Backup and Restore](#5-backup-and-restore)
  - [5.1 PostgreSQL](#51-postgresql)
  - [5.2 Storage Element Data (PVC Snapshots)](#52-storage-element-data-pvc-snapshots)
  - [5.3 Keycloak Realm Export](#53-keycloak-realm-export)
  - [5.4 Backup Schedule Recommendations](#54-backup-schedule-recommendations)
- [6. Scaling](#6-scaling)
  - [6.1 Horizontal Scaling (Stateless Modules)](#61-horizontal-scaling-stateless-modules)
  - [6.2 Adding Storage Elements](#62-adding-storage-elements)
  - [6.3 Resource Recommendations](#63-resource-recommendations)
  - [6.4 Performance Tuning](#64-performance-tuning)

---

## 1. Monitoring Overview

Artstore provides comprehensive observability through Prometheus metrics, structured logging (slog JSON), health check endpoints, and inter-service dependency monitoring via [topologymetrics](https://github.com/BigKAA/topologymetrics). Every module exposes a `/metrics` endpoint in the Prometheus exposition format.

### 1.1 Metrics Architecture

Each module exposes metrics with a module-specific prefix to avoid naming collisions:

| Module | Metric Prefix | Default Port | Metrics Path |
|--------|:-------------:|:------------:|:------------:|
| Admin Module | `am_` | 8000 | `/metrics` |
| Storage Element | `se_` | 8010 | `/metrics` |
| Ingester Module | `im_` | 8020 | `/metrics` |
| Query Module | `qm_` | 8030 | `/metrics` |

In addition, all modules export dependency health metrics with the `app_` prefix (from the topologymetrics SDK).

**Metric types used**:

- **Counter** (`_total`) — monotonically increasing values (requests, operations, errors)
- **Gauge** — values that can go up and down (active uploads, file counts, storage bytes)
- **Histogram** (`_seconds`, `_bytes`) — distribution of values (latency, file sizes)

### 1.2 Prometheus Metrics Reference

#### Common HTTP Metrics (all modules)

Every module registers these metrics with its own prefix (`am_`, `se_`, `im_`, `qm_`):

| Metric | Type | Labels | Description |
|--------|------|--------|-------------|
| `*_http_requests_total` | Counter | method, path, status | Total HTTP requests |
| `*_http_request_duration_seconds` | Histogram | method, path | HTTP request latency (seconds) |

> **Note**: The `path` label is normalized to reduce cardinality (e.g., `/api/v1/files/{id}` instead of `/api/v1/files/abc-123`).

#### Admin Module Metrics (`am_`)

| Metric | Type | Labels | Description |
|--------|------|--------|-------------|
| `am_http_requests_total` | Counter | method, path, status | AM HTTP requests |
| `am_http_request_duration_seconds` | Histogram | method, path | AM HTTP latency |

#### Storage Element Metrics (`se_`)

| Metric | Type | Labels | Description |
|--------|------|--------|-------------|
| `se_http_requests_total` | Counter | method, path, status | SE HTTP requests |
| `se_http_request_duration_seconds` | Histogram | method, path | SE HTTP latency |
| `se_files_total` | Gauge | — | Total file count |
| `se_storage_bytes` | Gauge | — | Total bytes stored |
| `se_operations_total` | Counter | operation, result | Operations count (upload, download, delete, update) |
| `se_gc_runs_total` | Counter | — | GC execution count |
| `se_gc_files_removed_total` | Counter | — | Files removed by GC |
| `se_gc_files_expired_total` | Counter | — | Files expired by GC |
| `se_gc_duration_seconds` | Histogram | — | GC execution time |
| `se_index_sync_runs_total` | Counter | — | Index rebuild count |
| `se_index_sync_errors_total` | Counter | — | Index rebuild errors |
| `se_index_sync_files_total` | Gauge | — | Files in the index |
| `se_index_sync_duration_seconds` | Histogram | — | Index rebuild time |
| `se_mode_sync_runs_total` | Counter | — | Mode sync checks |
| `se_mode_sync_changes_total` | Counter | — | Mode transitions detected |
| `se_mode_sync_errors_total` | Counter | — | Mode sync errors |
| `se_reconcile_runs_total` | Counter | — | Reconcile execution count |
| `se_reconcile_issues_total` | Counter | type | Reconcile issues found (orphaned, checksum mismatch) |
| `se_reconcile_duration_seconds` | Histogram | — | Reconcile execution time |

#### Ingester Module Metrics (`im_`)

| Metric | Type | Labels | Description |
|--------|------|--------|-------------|
| `im_http_requests_total` | Counter | method, path, status | IM HTTP requests |
| `im_http_request_duration_seconds` | Histogram | method, path | IM HTTP latency |
| `im_uploads_total` | Counter | retention_policy, status | Upload count by retention and outcome |
| `im_upload_duration_seconds` | Histogram | retention_policy | Full upload pipeline time (0.5s–600s buckets) |
| `im_upload_size_bytes` | Histogram | retention_policy | Upload file size distribution (1KB–1GB buckets) |
| `im_se_upload_duration_seconds` | Histogram | — | SE transfer time only |
| `im_active_uploads` | Gauge | — | Currently in-progress uploads |
| `im_retry_total` | Counter | reason | Upload retry statistics |
| `im_se_selection_total` | Counter | result | SE selection outcomes (success, no_storage, error) |

#### Query Module Metrics (`qm_`)

| Metric | Type | Labels | Description |
|--------|------|--------|-------------|
| `qm_http_requests_total` | Counter | method, path, status | QM HTTP requests |
| `qm_http_request_duration_seconds` | Histogram | method, path | QM HTTP latency |
| `qm_search_total` | Counter | — | Total search requests |
| `qm_search_duration_seconds` | Histogram | — | Search latency (0.01s–60s buckets) |
| `qm_downloads_total` | Counter | status | Downloads by outcome |
| `qm_download_duration_seconds` | Histogram | — | Download latency (0.1s–300s buckets) |
| `qm_download_bytes_total` | Counter | — | Total bytes transferred |
| `qm_active_downloads` | Gauge | — | Currently in-progress downloads |
| `qm_cache_hits_total` | Counter | — | File cache hits |
| `qm_cache_misses_total` | Counter | — | File cache misses |
| `qm_hard_delete_total` | Counter | — | 404-triggered hard deletes (AM + DB + cache) |

#### Dependency Health Metrics (`app_`) — topologymetrics

| Metric | Type | Labels | Description |
|--------|------|--------|-------------|
| `app_dependency_health` | Gauge | service, target, group | Dependency health: 1=OK, 0=fail |
| `app_dependency_latency_seconds` | Histogram | service, target, group | Dependency check latency |
| `app_dependency_status` | Gauge | service, target, group, status_category | Status categories: ok, degraded, fail, unknown |
| `app_dependency_status_detail` | Gauge | service, target, group, status_detail | Detailed status information |

### 1.3 Prometheus Scraping Configuration

#### Pod Annotations (recommended)

All Artstore Helm charts include Prometheus pod annotations by default. This enables automatic scraping when Prometheus is configured with `kubernetes_sd_configs` and annotation-based relabeling.

Annotations added to each pod:

```yaml
prometheus.io/scrape: "true"
prometheus.io/port: "<module-port>"
prometheus.io/path: "/metrics"
```

To verify annotations are present:

```bash
# Check annotations for all Artstore pods
kubectl get pods -n artstore -o jsonpath='{range .items[*]}{.metadata.name}{"\t"}{.metadata.annotations.prometheus\.io/scrape}{"\n"}{end}'
```

#### ServiceMonitor (Prometheus Operator)

If using the Prometheus Operator, deploy ServiceMonitor resources for fine-grained scraping control:

```yaml
apiVersion: monitoring.coreos.com/v1
kind: ServiceMonitor
metadata:
  name: artstore
  namespace: artstore
  labels:
    release: prometheus  # must match Prometheus operator selector
spec:
  namespaceSelector:
    matchNames:
      - artstore
  selector:
    matchLabels:
      app.kubernetes.io/part-of: artstore
  endpoints:
    - port: http
      path: /metrics
      interval: 30s
      scrapeTimeout: 10s
```

#### Verifying Scraping

Test that metrics are being collected:

```bash
# Direct metric check from a pod
kubectl port-forward -n artstore svc/admin-module 8000:8000
curl -s http://localhost:8000/metrics | head -20

# Check Prometheus targets
curl -s http://prometheus:9090/api/v1/targets | jq '.data.activeTargets[] | select(.labels.job | test("artstore"))'
```

### 1.4 Dependency Health Monitoring (topologymetrics)

Artstore uses the [topologymetrics](https://github.com/BigKAA/topologymetrics) SDK to monitor inter-service dependencies in real time. Each module registers its critical dependencies and periodically checks their health.

**Dependency map**:

| Module (group) | Dependencies (targets) | Criticality |
|-----------------|----------------------|:-----------:|
| admin-module | PostgreSQL, Keycloak | Critical |
| ingester-module | Admin Module, Keycloak | Critical |
| query-module | PostgreSQL, Admin Module | Critical |
| storage-element | Admin Module | Critical |

> **Note**: Storage Elements are NOT registered as dynamic dependencies of AM/IM/QM in topologymetrics. SE health is tracked separately through AM's periodic health checks and reflected in the Admin UI.

**How it works**:

1. Each module creates a `dephealth.Service` on startup with its dependency list
2. A background goroutine periodically pings each dependency (HTTP health check or TCP connect)
3. Results are exported as `app_dependency_health`, `app_dependency_latency_seconds`, and `app_dependency_status` metrics
4. The Admin Module UI subscribes to SSE events for real-time updates

**Interpreting health values**:

| `app_dependency_health` value | Meaning |
|:-----------------------------:|---------|
| 1 | Dependency is healthy |
| 0 | Dependency is unreachable or unhealthy |

### 1.5 Built-in Monitoring in Admin UI

The Admin Module includes a built-in Monitoring page accessible at `https://<domain>/admin/monitoring`. It provides real-time visibility without requiring an external monitoring stack.

![Admin UI: Monitoring](images/ui-monitoring.png)
*Figure 1.1 — Admin UI built-in monitoring page*

**Features**:

- **Dependency status cards** — real-time health of PostgreSQL and Keycloak (via SSE)
- **Storage Element status** — current mode, capacity, file count for each SE
- **Background task status** — file sync, SA sync, and dephealth check intervals with last run time
- **Auto-generated alerts** — SE offline/degraded warnings, SE capacity > 80%, dependency failures
- **Latency charts** — inter-service latency over time (requires Prometheus configuration in AM; selectable periods: 1h, 6h, 24h, 7d)

> **Tip**: The built-in monitoring is useful for quick checks, but for production operations use Grafana dashboards for richer visualization and historical analysis.

---

## 2. Grafana Dashboards

Artstore ships with 6 pre-built Grafana dashboards covering system overview, per-module metrics, and dependency topology. Dashboard JSON files are located in `charts/artstore/dashboards/`.

### 2.1 Dashboard Provisioning

#### Manual Import

1. Open Grafana → **Dashboards** → **Import**
2. Upload the JSON file or paste its contents
3. Select the Prometheus data source
4. Click **Import**

#### Kubernetes ConfigMap (Grafana Sidecar)

When using the Grafana Helm chart with the sidecar enabled, provision dashboards automatically:

```yaml
# ConfigMap for Grafana dashboard provisioning
apiVersion: v1
kind: ConfigMap
metadata:
  name: artstore-dashboards
  namespace: monitoring
  labels:
    grafana_dashboard: "1"  # label for sidecar discovery
data:
  overview.json: |
    <contents of charts/artstore/dashboards/overview.json>
  admin-module.json: |
    <contents of charts/artstore/dashboards/admin-module.json>
  # ... repeat for all 6 dashboards
```

Alternatively, use the umbrella Helm chart (see [Admin Guide §3](admin-guide.md#3-installation)) which creates this ConfigMap automatically when `monitoring.dashboards.enabled: true`.

#### Dashboard List

| Dashboard | File | UID | Description |
|-----------|------|-----|-------------|
| Artstore Overview | `overview.json` | `artstore-overview` | System health, request rate, latency, errors, storage |
| Admin Module | `admin-module.json` | `artstore-admin-module` | AM HTTP metrics, latency distribution, dependencies |
| Storage Element | `storage-element.json` | `artstore-storage-element` | Files, storage, operations, GC, reconcile, index sync |
| Ingester Module | `ingester-module.json` | `artstore-ingester-module` | Uploads, SE selection, file sizes, retries |
| Query Module | `query-module.json` | `artstore-query-module` | Search, download, cache hit rate, throughput |
| Dependency Topology | `dependency-topology.json` | `artstore-dependency-topology` | Health matrix, latency, status map, Node Graph |

### 2.2 Artstore Overview Dashboard

The overview dashboard provides a single-pane-of-glass view of the entire Artstore system.

**Rows and panels**:

| Row | Panel | What to Watch |
|-----|-------|---------------|
| **System Health** | Module status (stat panels) | All 4 modules should show `1` (up). Any `0` triggers `ArtstoreServiceDown` alert |
| **Dependency Health** | Health Matrix (table) | All dependencies should be `1`. Check `group` label for the affected module |
| **Request Rate** | Stacked RPS by module | Baseline traffic patterns. Sudden spikes or drops indicate anomalies |
| **Request Rate** | Error Rate (% 5xx) | Should stay below 5%. Sustained elevation triggers `ArtstoreHighErrorRate` |
| **Latency** | p50 / p95 / p99 | p95 < 500ms typical. p99 > 2s for AM or > 3s for QM search triggers alerts |
| **Storage** | File Count | Monitor total file count per SE. Unexpected drops may indicate GC issues |
| **Storage** | Storage Usage | Track growth trend. Plan SE additions before reaching capacity |
| **Storage** | SE Operations (stacked) | Breakdown: upload, download, delete, update. Upload drops may indicate SE issues |

**Use cases**:

- **Morning check**: verify all modules are up, error rate near zero, no alert annotations
- **Incident triage**: start here to identify which module/layer is affected, then drill into module-specific dashboard
- **Capacity planning**: track storage growth trend over weeks/months

### 2.3 Admin Module Dashboard

Focused on Admin Module HTTP performance and dependency health.

**Key panels**:

| Row | Panel | Description |
|-----|-------|-------------|
| **HTTP Overview** | RPS by Path | Top-traffic endpoints. `/api/v1/files` and `/api/v1/storage-elements` dominate |
| **HTTP Overview** | Latency by Path (p95) | Per-endpoint latency. UI routes (`/admin/*`) are typically faster than API |
| **HTTP Overview** | Error Rate by Path | Identify endpoints with elevated error rates |
| **HTTP Overview** | RPS by Status Code | Healthy: 200/204 majority. Watch for 401 (auth), 500 (server errors) |
| **Latency Distribution** | p50 / p95 / p99 | Overall AM latency percentiles |
| **Latency Distribution** | Request Duration Heatmap | Visual distribution of request times |
| **Dependency Health** | Dependencies Status | PostgreSQL and Keycloak health from topologymetrics |
| **Dependency Health** | Dependency Latency | Time to reach PG and KC. High latency = slower AM responses |

**Typical investigation flow**: Error rate spike → check RPS by Status Code → identify 5xx or 4xx → check Dependency Health → if dependency degraded, check that service.

### 2.4 Storage Element Dashboard

The most detailed dashboard, covering files, storage, operations, GC, reconcile, and index/mode sync.

**Key panels**:

| Row | Panel | Description |
|-----|-------|-------------|
| **Storage Overview** | Active / Deleted / Index Files, Storage Used | High-level counters as stat panels |
| **Files & Storage** | Files by Status (per instance) | Track file distribution across SE instances |
| **Files & Storage** | Storage Usage by Instance | Identify which SE is filling up |
| **Operations** | Operations Rate | Upload/download/delete/update rates |
| **Operations** | Operations by Result | success vs error breakdown per operation type |
| **HTTP Metrics** | RPS by Path, Latency p50/p95/p99 | SE API performance |
| **GC & Reconcile** | GC Duration (p95) | Normal: < 1s. High: check disk I/O |
| **GC & Reconcile** | GC Activity | Runs, files deleted, files expired rates |
| **GC & Reconcile** | Reconcile Issues | Orphaned files, checksum mismatches. Should be near zero |
| **Index & Mode Sync** | Index Sync | Rebuild count, errors, file count in index |
| **Index & Mode Sync** | Mode Sync | Mode check runs, changes detected, errors |

**Important**: Use the `$instance` variable to filter by specific SE when investigating individual SE issues.

**Warning signs**:

- `se_reconcile_issues_total` increasing → data integrity concern, check SE logs
- `se_gc_duration_seconds` p95 > 10s → disk I/O bottleneck
- `se_index_sync_errors_total` > 0 → index rebuild failing, files may not be discoverable
- Operations `result="error"` growing → check SE disk space and filesystem health

### 2.5 Ingester Module Dashboard

Focused on file upload pipeline, SE selection, and retry behavior.

**Key panels**:

| Row | Panel | Description |
|-----|-------|-------------|
| **Upload Overview** | Active Uploads, Upload RPS, Error Rate, No-Storage Rate | Quick health indicators |
| **Upload Metrics** | Uploads by Retention & Status | Breakdown: temporary vs permanent, success vs error |
| **Upload Metrics** | Upload Duration (p50/p95/p99) | Full pipeline time including AM registration |
| **Upload Metrics** | Upload File Size (p50/p95) | Track typical file sizes for capacity planning |
| **Upload Metrics** | SE Selection & Retries | `no_storage` results and retry reasons |
| **HTTP Metrics** | RPS by Status Code | Overall HTTP status distribution |
| **HTTP Metrics** | HTTP Latency p50/p95/p99 | End-to-end latency including file transfer |
| **Dependency Health** | Dependencies Status | AM and Keycloak health |

**Key indicators**:

- `im_se_selection_total{result="no_storage"}` growing → no SE available for writes, check SE modes
- `im_retry_total` increasing → transient SE failures, check SE health
- Upload duration p95 > 30s for small files → network issues between IM and SE

### 2.6 Query Module Dashboard

Focused on search performance, download throughput, and cache efficiency.

**Key panels**:

| Row | Panel | Description |
|-----|-------|-------------|
| **Overview** | Active Downloads, Search RPS, Download RPS, Cache Hit Rate, Bytes Transferred, Hard Delete (404) | Summary stats |
| **Search** | Search Rate | Queries per second over time |
| **Search** | Search Duration (p50/p95/p99) | PostgreSQL FTS performance |
| **Download** | Downloads by Status | success, error, not_found breakdown |
| **Download** | Download Duration (p50/p95/p99) | End-to-end download time |
| **Download** | Download Throughput | Bytes/second transferred |
| **Cache** | Cache Hits vs Misses | Absolute counts over time |
| **Cache** | Cache Hit Rate (%) | Target: > 50% for repeat-access workloads |
| **HTTP Metrics** | RPS by Endpoint, Error Rate by Endpoint | Per-endpoint breakdown |
| **Dependency Health** | Dependencies Status | PostgreSQL and AM health |

**Key indicators**:

- Cache Hit Rate < 30% → consider increasing `QM_CACHE_MAX_SIZE` or `QM_CACHE_TTL`
- Search p99 > 3s → check PostgreSQL FTS indexes (`files_fts_idx`), query complexity
- `qm_hard_delete_total` increasing → files missing on SE, QM performs hard delete (AM + DB + cache invalidation)
- Download errors increasing → check SE availability and AM file registry

### 2.7 Dependency Topology Dashboard

Visualizes inter-service dependency health using topologymetrics data. Requires Grafana 10+ for the Node Graph panel.

**Key panels**:

| Row | Panel | Description |
|-----|-------|-------------|
| **Health Matrix** | All Dependencies Health (stat) | Color-coded health: green=OK, red=fail |
| **Health Matrix** | Critical Dependencies Table | Table with group, target, health status, and latency |
| **Dependency Latency** | Average Dependency Latency | Trend over time per dependency |
| **Dependency Latency** | Dependency Latency p95 | 95th percentile latency per dependency |
| **Dependency Latency** | Latency Heatmap | Distribution of dependency check times |
| **Status Details** | Status Categories Table | Detailed status: ok, degraded, fail, unknown |
| **Topology Graph** | Node Graph | Visual graph showing modules as nodes, dependencies as edges |
| **Health History** | Health Over Time (Status Map) | Timeline view showing when dependencies changed state |

**Typical use case**: During an incident, this dashboard answers "what is affected and what depends on what?" — helps identify the root cause quickly by following the dependency chain.

### 2.8 Template Variables

All dashboards support template variables for filtering:

| Variable | Dashboards | Values | Purpose |
|----------|-----------|--------|---------|
| `$namespace` | All | Kubernetes namespaces | Filter by deployment namespace |
| `$interval` | All | 1m, 5m, 15m, 30m, 1h | Aggregation interval for `rate()` |
| `$instance` | Storage Element | SE pod names | Filter by specific SE instance |
| `$group` | Dependency Topology | Module names (multi-select) | Filter by dependency group |

> **Tip**: When investigating a specific SE, select it in `$instance` to see only that SE's metrics on the Storage Element dashboard.

---

## 3. Alerting

Artstore provides a set of PrometheusRule alerts covering service availability, error rates, latency, and storage health. Alert rules are defined in `charts/artstore/alerts/artstore-alerts.yaml`.

### 3.1 AlertManager Integration

#### Installing Alert Rules

**As a Kubernetes CRD** (requires Prometheus Operator):

```bash
kubectl apply -f charts/artstore/alerts/artstore-alerts.yaml
```

**Via Helm** (umbrella chart):

```yaml
# values.yaml
monitoring:
  alerts:
    enabled: true
```

**Standalone Prometheus** (without Operator):

Copy the alert expressions into your `prometheus.yml` under the `rule_files` section:

```yaml
rule_files:
  - /etc/prometheus/rules/artstore-alerts.yaml
```

#### Alert Labels

All Artstore alerts include:

| Label | Values | Purpose |
|-------|--------|---------|
| `team` | `artstore` | Route alerts to the Artstore operations team |
| `severity` | `critical`, `warning` | Alert priority |
| `module` | `admin-module`, `ingester-module`, `query-module`, `storage-element` | Identify affected module |

### 3.2 Alert Reference

#### Availability Alerts

| Alert | Severity | Condition | For | Description |
|-------|:--------:|-----------|:---:|-------------|
| **ArtstoreServiceDown** | critical | `up{job=~".*<module>.*"} == 0` | 2m | Module is not responding to Prometheus scrape |
| **SENoEditAvailable** | warning | No active SE files detected | 5m | No Storage Element available for writing |

#### Error Rate Alerts

| Alert | Severity | Condition | For | Modules |
|-------|:--------:|-----------|:---:|---------|
| **ArtstoreHighErrorRate** | warning | 5xx error rate > 5% (5m window) | 5m | AM, IM, QM, SE (per instance) |

#### Latency Alerts

| Alert | Severity | Condition | For | Description |
|-------|:--------:|-----------|:---:|-------------|
| **ArtstoreHighLatency** (AM) | warning | p99 latency > 2s | 5m | Admin Module response time degraded |
| **ArtstoreHighLatency** (QM search) | warning | p99 search latency > 3s | 5m | Query Module search performance degraded |
| **DependencyLatencyHigh** | warning | Avg dependency latency > 1s | 5m | Inter-service communication slowdown |

#### Storage Alerts

| Alert | Severity | Condition | For | Description |
|-------|:--------:|-----------|:---:|-------------|
| **SEReconcileIssuesHigh** | warning | Reconcile issues rate > 0.1/s | 15m | Data integrity concerns on SE |
| **SEIndexSyncErrors** | warning | Index sync error rate > 0 | 10m | SE index rebuild failing |
| **IMNoStorageAvailable** | critical | SE selection failure > 10% | 5m | File uploads may be blocked |

### 3.3 Runbooks

#### ArtstoreServiceDown

**Severity**: critical | **Threshold**: Module unreachable for > 2 minutes

**Diagnosis**:

```bash
# 1. Check pod status
kubectl get pods -n artstore -l app.kubernetes.io/name=<module-name>

# 2. Check pod events
kubectl describe pod -n artstore <pod-name>

# 3. Check recent logs
kubectl logs -n artstore <pod-name> --tail=100 | jq '.'

# 4. Check resource usage
kubectl top pod -n artstore <pod-name>
```

**Common causes and fixes**:

| Cause | Symptoms | Fix |
|-------|----------|-----|
| OOMKilled | `OOMKilled` in pod status | Increase memory limits in Helm values |
| CrashLoopBackOff | Repeated restarts | Check logs for panic/fatal errors |
| Liveness probe failed | `Unhealthy` events | Check `/health/live` endpoint, verify dependencies |
| Image pull failure | `ImagePullBackOff` | Verify image tag and registry access |
| PVC mount failure (SE) | `ContainerCreating` stuck | Check PVC status, storage class, NFS server |

---

#### SENoEditAvailable

**Severity**: warning | **Threshold**: No active SE detected for > 5 minutes

**Diagnosis**:

```bash
# 1. Check SE pods
kubectl get pods -n artstore -l app.kubernetes.io/name=storage-element

# 2. Check SE modes via AM API
curl -s -H "Authorization: Bearer $TOKEN" \
  "https://artstore.example.com/api/v1/storage-elements" | jq '.[] | {name, mode, status}'

# 3. Check if any SE is in edit mode
curl -s -H "Authorization: Bearer $TOKEN" \
  "https://artstore.example.com/api/v1/storage-elements" | jq '[.[] | select(.mode == "edit")] | length'
```

**Common causes and fixes**:

| Cause | Fix |
|-------|-----|
| All SE in `ro` or `ar` mode | Transition an SE to `rw` mode or deploy a new `edit` SE |
| SE pods down | Restart SE pods, check PVC availability |
| AM lost contact with SE | Check network connectivity, trigger manual SE sync from Admin UI |

---

#### ArtstoreHighErrorRate

**Severity**: warning | **Threshold**: > 5% of 5xx responses over 5 minutes

**Diagnosis**:

```bash
# 1. Identify the module from alert label
# module: admin-module | ingester-module | query-module | storage-element

# 2. Check error distribution
kubectl logs -n artstore <module-pod> --tail=500 | \
  jq 'select(.level == "ERROR") | {time, msg, error}' | head -20

# 3. Check dependencies
kubectl logs -n artstore <module-pod> --tail=100 | \
  jq 'select(.msg | test("dependency|health|connect"))' | head -10
```

**Common causes by module**:

| Module | Common 5xx Causes |
|--------|-------------------|
| AM | PostgreSQL connection errors, Keycloak JWKS fetch failure |
| IM | SE unavailable, AM unreachable, timeout during file upload |
| QM | PostgreSQL query timeout, SE unreachable during download, AM API errors |
| SE | Disk full (507), filesystem errors, WAL corruption |

---

#### ArtstoreHighLatency

**Severity**: warning | **AM threshold**: p99 > 2s | **QM search threshold**: p99 > 3s

**Diagnosis**:

```bash
# 1. Check slow queries (AM/QM — PostgreSQL)
kubectl exec -n artstore <postgres-pod> -- psql -U artstore -c "
  SELECT pid, now() - pg_stat_activity.query_start AS duration, query
  FROM pg_stat_activity
  WHERE state != 'idle'
  ORDER BY duration DESC LIMIT 5;"

# 2. Check dependency latency
curl -s http://localhost:8000/metrics | grep 'app_dependency_latency_seconds'

# 3. Check resource pressure
kubectl top pod -n artstore
```

**Common causes and fixes**:

| Cause | Fix |
|-------|-----|
| PostgreSQL slow queries | Check `EXPLAIN ANALYZE`, rebuild indexes if needed |
| Keycloak token validation slow | Verify JWKS cache (`AM_JWKS_CACHE_TTL`), check KC pod resources |
| Network latency to remote SE | Check WAN connectivity, consider priority adjustment |
| CPU throttling | Increase CPU limits in Helm values |

---

#### DependencyLatencyHigh

**Severity**: warning | **Threshold**: Average latency > 1 second

**Diagnosis**:

```bash
# 1. Check which dependency is slow (from alert labels: group → target)
curl -s http://localhost:8000/metrics | grep 'app_dependency_latency' | sort

# 2. Test connectivity directly
kubectl exec -n artstore <pod> -- curl -w "%{time_total}\n" -o /dev/null -s http://<target-host>:<port>/health/live
```

**Common causes**: Network congestion, target service overloaded, DNS resolution issues, firewall rules changed.

---

#### SEReconcileIssuesHigh

**Severity**: warning | **Threshold**: > 0.1 issues/second for 15 minutes

**Diagnosis**:

```bash
# 1. Check reconcile logs
kubectl logs -n artstore <se-pod> --tail=500 | \
  jq 'select(.msg | test("reconcile|orphan|checksum"))' | head -20

# 2. Check reconcile metrics
curl -s http://localhost:8010/metrics | grep 'se_reconcile_issues_total'
```

**Common causes**:

| Issue Type | Meaning | Fix |
|------------|---------|-----|
| orphaned | File on disk without `attr.json` | SE GC will clean up automatically; verify WAL is intact |
| checksum_mismatch | `attr.json` checksum differs from file | Possible disk corruption; verify filesystem integrity |
| missing_file | `attr.json` exists but file is absent | Possible incomplete upload or disk failure |

---

#### SEIndexSyncErrors

**Severity**: warning | **Threshold**: Any errors for > 10 minutes

**Diagnosis**:

```bash
# 1. Check SE logs for index errors
kubectl logs -n artstore <se-pod> --tail=200 | \
  jq 'select(.msg | test("index|sync|rebuild"))' | head -20

# 2. Check filesystem (SE data directory)
kubectl exec -n artstore <se-pod> -- ls -la /data/
kubectl exec -n artstore <se-pod> -- df -h /data/
```

**Fix**: If filesystem is intact, restart the SE pod to trigger a fresh index rebuild. If disk errors are present, replace the PVC.

---

#### IMNoStorageAvailable

**Severity**: critical | **Threshold**: > 10% SE selection failures for 5 minutes

**Diagnosis**:

```bash
# 1. Check IM logs for SE selection
kubectl logs -n artstore <im-pod> --tail=200 | \
  jq 'select(.msg | test("selection|no_storage|retry"))' | head -20

# 2. Check available SEs through AM
curl -s -H "Authorization: Bearer $TOKEN" \
  "https://artstore.example.com/api/v1/storage-elements" | \
  jq '.[] | select(.mode == "edit" or .mode == "rw") | {name, mode, status}'

# 3. Check SE health from IM perspective
kubectl logs -n artstore <im-pod> --tail=100 | \
  jq 'select(.msg | test("health|se_list"))' | head -10
```

**Common causes and fixes**:

| Cause | Fix |
|-------|-----|
| No SE in edit/rw mode | Deploy new SE or transition existing SE to rw/edit |
| All SE at capacity (507) | Add more SE or increase storage volume |
| AM unreachable from IM | Check network policies, AM pod health |
| SE health check failing | Check SE pods, restart if needed |

### 3.4 Silencing and Inhibition

#### Silencing Alerts

During planned maintenance, silence alerts to avoid noise:

```bash
# Silence all Artstore alerts for 2 hours
amtool silence add --alertmanager.url=http://alertmanager:9093 \
  --comment="Planned maintenance" \
  --duration=2h \
  team=artstore

# Silence specific module during upgrade
amtool silence add --alertmanager.url=http://alertmanager:9093 \
  --comment="AM upgrade in progress" \
  --duration=30m \
  module=admin-module
```

#### Inhibition Rules

Configure AlertManager to suppress lower-severity alerts when critical alerts fire:

```yaml
# alertmanager.yml
inhibit_rules:
  - source_matchers:
      - severity = critical
    target_matchers:
      - severity = warning
    equal:
      - team
      - module
```

This ensures that when `ArtstoreServiceDown` (critical) fires for a module, the corresponding `ArtstoreHighErrorRate` (warning) is suppressed — since errors are expected when a service is down.

### 3.5 Notification Channels

Configure AlertManager routing for Artstore alerts:

```yaml
# alertmanager.yml
route:
  group_by: ['team', 'module']
  group_wait: 30s
  group_interval: 5m
  repeat_interval: 4h
  routes:
    - matchers:
        - team = artstore
        - severity = critical
      receiver: artstore-critical
    - matchers:
        - team = artstore
        - severity = warning
      receiver: artstore-warning

receivers:
  - name: artstore-critical
    # Configure: PagerDuty, OpsGenie, phone call, etc.
    webhook_configs:
      - url: 'https://hooks.example.com/artstore-critical'
  - name: artstore-warning
    # Configure: Slack, email, Telegram, etc.
    slack_configs:
      - channel: '#artstore-ops'
        send_resolved: true
```

---

## 4. Troubleshooting

### 4.1 Health Check Endpoints

Every Artstore module exposes two health check endpoints:

| Endpoint | Purpose | Expected Response |
|----------|---------|:-----------------:|
| `GET /health/live` | Liveness probe — process is running | `200 OK` |
| `GET /health/ready` | Readiness probe — ready to serve traffic | `200 OK` |

**Quick health check for all modules**:

```bash
# Check all modules via API Gateway
for path in "api/v1/health" "upload/health/live" "query/health/live"; do
  echo -n "$path: "
  curl -s -o /dev/null -w "%{http_code}" "https://artstore.example.com/$path"
  echo
done

# Direct pod health check (via port-forward)
kubectl port-forward -n artstore svc/admin-module 8000:8000 &
curl -s http://localhost:8000/health/live | jq '.'
curl -s http://localhost:8000/health/ready | jq '.'
```

**Readiness check response** (example):

```json
{
  "status": "ok",
  "checks": {
    "postgresql": "ok",
    "keycloak": "ok"
  }
}
```

If any dependency fails, readiness returns `503 Service Unavailable` with the failing check name.

### 4.2 Dependency Diagnostics via topologymetrics

Use Prometheus queries to diagnose dependency chains:

```promql
# All failing dependencies
app_dependency_health == 0

# Dependencies of a specific module
app_dependency_health{group="admin-module"}

# Average latency to each dependency
rate(app_dependency_latency_seconds_sum[5m]) / rate(app_dependency_latency_seconds_count[5m])

# Dependency status details
app_dependency_status{status_category!="ok"}
```

**Tracing a dependency chain** (example: file upload failure):

1. Check IM dependencies: `app_dependency_health{group="ingester-module"}` → if AM is failing, uploads cannot register files
2. Check AM dependencies: `app_dependency_health{group="admin-module"}` → if PostgreSQL is failing, AM cannot register files
3. Check SE health: `up{job=~".*storage-element.*"}` → if SE is down, file transfer fails

### 4.3 Log Analysis (slog JSON)

All Artstore modules use Go's `slog` package with JSON output. This enables powerful log analysis with `jq`.

**Log format** (all modules):

```json
{
  "time": "2026-03-03T10:15:30.123Z",
  "level": "ERROR",
  "msg": "failed to register file",
  "error": "connection refused",
  "request_id": "abc-123",
  "method": "POST",
  "path": "/api/v1/files"
}
```

**Common log queries**:

```bash
# Recent errors from a specific module
kubectl logs -n artstore deploy/admin-module --tail=500 | \
  jq 'select(.level == "ERROR")' | head -20

# Errors in the last 5 minutes
kubectl logs -n artstore deploy/admin-module --since=5m | \
  jq 'select(.level == "ERROR") | {time, msg, error}'

# Search for specific error message
kubectl logs -n artstore deploy/ingester-module --tail=1000 | \
  jq 'select(.msg | test("se_selection|no_storage"))'

# Count errors by message
kubectl logs -n artstore deploy/query-module --tail=5000 | \
  jq -r 'select(.level == "ERROR") | .msg' | sort | uniq -c | sort -rn

# Slow requests (if duration is logged)
kubectl logs -n artstore deploy/admin-module --tail=1000 | \
  jq 'select(.duration_ms > 2000) | {time, method, path, duration_ms}'

# All logs for a specific request ID
kubectl logs -n artstore deploy/admin-module --tail=5000 | \
  jq 'select(.request_id == "abc-123")'

# Multi-pod log aggregation
kubectl logs -n artstore -l app.kubernetes.io/name=storage-element --all-containers --tail=200 | \
  jq 'select(.level == "ERROR")' 2>/dev/null
```

### 4.4 Common Problems and Solutions

#### Module Cannot Start (CrashLoopBackOff)

| Symptom | Likely Cause | Diagnostic | Fix |
|---------|--------------|------------|-----|
| `dial tcp: connection refused` in logs | PostgreSQL not ready | `kubectl get pods -n artstore -l app=postgresql` | Wait for PG pod, check PG service |
| `JWKS fetch failed` | Keycloak not reachable | Check KC pod, verify `AM_KEYCLOAK_URL` | Wait for KC, check KC service URL |
| `migration failed` | DB migration error | Check log for SQL errors | Fix migration, or drop and recreate DB for dev |
| `bind: address already in use` | Port conflict | Check for duplicate deployments | Delete conflicting pod/service |

#### File Upload Fails (IM → SE → AM)

```
Client → IM: 500 Internal Server Error
```

**Diagnostic steps**:

```bash
# 1. Check IM logs
kubectl logs -n artstore deploy/ingester-module --tail=200 | \
  jq 'select(.level == "ERROR")'

# 2. Check if IM can reach AM
kubectl exec -n artstore deploy/ingester-module -- \
  curl -s http://admin-module:8000/health/live

# 3. Check if IM has valid SA token
kubectl logs -n artstore deploy/ingester-module --tail=100 | \
  jq 'select(.msg | test("token"))'

# 4. Check SE availability from IM
kubectl logs -n artstore deploy/ingester-module --tail=100 | \
  jq 'select(.msg | test("se_selection|se_list"))'
```

**Common causes**: IM cannot obtain SA token (Keycloak issue), AM is down, no SE in edit mode, SE disk full.

#### Search Returns No Results (QM)

```bash
# 1. Verify file exists in AM registry
curl -s -H "Authorization: Bearer $TOKEN" \
  "https://artstore.example.com/api/v1/files?search=<query>" | jq '.total'

# 2. Check QM logs for search errors
kubectl logs -n artstore deploy/query-module --tail=200 | \
  jq 'select(.msg | test("search"))'

# 3. Check PostgreSQL FTS index
kubectl exec -n artstore <postgres-pod> -- psql -U artstore -c "
  SELECT indexname FROM pg_indexes WHERE tablename = 'files' AND indexname LIKE '%fts%';"
```

**Common causes**: FTS index not created (run migrations), file not yet synced to QM database, search syntax error.

#### Download Fails with 404 or 502

```bash
# 1. Check if file exists in registry
curl -s -H "Authorization: Bearer $TOKEN" \
  "https://artstore.example.com/api/v1/files/<file-id>" | jq '{id, status, se_id}'

# 2. Check if file's SE is reachable
curl -s -H "Authorization: Bearer $TOKEN" \
  "https://artstore.example.com/api/v1/storage-elements/<se-id>" | jq '{name, status, mode}'

# 3. Check QM logs
kubectl logs -n artstore deploy/query-module --tail=200 | \
  jq 'select(.msg | test("download|stream|proxy"))'
```

**Common causes**: File is on an archived SE (`mode=ar` → 410 Gone), SE is offline, file deleted from SE but registry not updated.

#### Admin UI Not Loading

```bash
# 1. Check AM pod health
kubectl get pods -n artstore -l app.kubernetes.io/name=admin-module

# 2. Check AM readiness
curl -s "https://artstore.example.com/api/v1/health" | jq '.'

# 3. Check Keycloak accessibility (UI needs KC for login)
curl -s "https://artstore.example.com/realms/artstore/.well-known/openid-configuration" | jq '.issuer'

# 4. Check Gateway routing
kubectl get httproute -n artstore -o yaml | grep -A5 "admin"
```

**Common causes**: Keycloak unreachable (OIDC redirect fails), Gateway misconfiguration, AM cookie encryption key changed (sessions invalidated).

#### SE Leader Election Issues (Replicated Mode)

```bash
# 1. Check SE logs for leader election
kubectl logs -n artstore <se-pod> --tail=200 | \
  jq 'select(.msg | test("leader|follower|flock"))'

# 2. Check NFS mount
kubectl exec -n artstore <se-pod> -- ls -la /data/.leader.lock /data/.leader.info

# 3. Check if leader info is stale
kubectl exec -n artstore <se-pod> -- cat /data/.leader.info
```

**Common causes**: NFS server unreachable, stale lock file (previous leader crashed without releasing lock), NFS `flock()` not supported (must be NFS v4+).

---

## 5. Backup and Restore

Artstore stores data in three locations that require backup: PostgreSQL (file registry, user data), Storage Element PVCs (actual files), and Keycloak realm configuration. Automated backup is not yet built into Artstore — the sections below describe manual procedures and recommendations.

### 5.1 PostgreSQL

PostgreSQL stores the file registry (AM), search index (QM), and module configuration. It is the most critical component to back up.

#### Backup with pg_dump

```bash
# Full database dump (compressed, custom format)
kubectl exec -n artstore <postgres-pod> -- \
  pg_dump -U artstore -Fc artstore > artstore-db-$(date +%Y%m%d).dump

# Schema-only dump (for disaster recovery planning)
kubectl exec -n artstore <postgres-pod> -- \
  pg_dump -U artstore --schema-only artstore > artstore-schema.sql

# Specific tables (file registry only)
kubectl exec -n artstore <postgres-pod> -- \
  pg_dump -U artstore -t files -t storage_elements -Fc artstore > artstore-registry.dump
```

#### Restore with pg_restore

```bash
# Restore from custom format dump
kubectl exec -i -n artstore <postgres-pod> -- \
  pg_restore -U artstore -d artstore --clean --if-exists < artstore-db-20260303.dump

# Restore specific tables
kubectl exec -i -n artstore <postgres-pod> -- \
  pg_restore -U artstore -d artstore -t files -t storage_elements < artstore-registry.dump
```

> **Important**: After restoring, restart all modules to re-initialize database connections and clear internal caches.

#### Continuous Archiving (WAL-based)

For production environments, configure PostgreSQL with continuous WAL archiving for point-in-time recovery (PITR):

```yaml
# postgresql.conf (or Helm values)
wal_level: replica
archive_mode: "on"
archive_command: "cp %p /backup/wal/%f"
```

### 5.2 Storage Element Data (PVC Snapshots)

Storage Elements keep files on persistent volumes. Backup strategies depend on the storage backend.

#### CSI VolumeSnapshot (recommended)

If your CSI driver supports snapshots (e.g., Longhorn, Rook-Ceph, AWS EBS CSI):

```yaml
apiVersion: snapshot.storage.k8s.io/v1
kind: VolumeSnapshot
metadata:
  name: se-edit-01-backup-20260303
  namespace: artstore
spec:
  volumeSnapshotClassName: csi-snapshotter
  source:
    persistentVolumeClaimName: data-se-edit-01-0
```

```bash
# Create snapshot
kubectl apply -f se-snapshot.yaml

# Verify snapshot
kubectl get volumesnapshot -n artstore
```

#### Manual File Copy

For environments without CSI snapshot support:

```bash
# Copy SE data directory (stop writes first by switching SE to ro mode)
kubectl exec -n artstore <se-pod> -- tar czf - /data | \
  gzip > se-edit-01-backup-$(date +%Y%m%d).tar.gz
```

> **Warning**: File copy during active writes may result in inconsistent backup. Either switch SE to `ro` mode first or use filesystem-level snapshots.

#### Restore from Snapshot

```yaml
apiVersion: v1
kind: PersistentVolumeClaim
metadata:
  name: data-se-edit-01-restored
  namespace: artstore
spec:
  dataSource:
    name: se-edit-01-backup-20260303
    kind: VolumeSnapshot
    apiGroup: snapshot.storage.k8s.io
  accessModes:
    - ReadWriteOnce
  resources:
    requests:
      storage: 50Gi
```

### 5.3 Keycloak Realm Export

Keycloak stores realm configuration (clients, roles, groups, scopes, mappers) in its own database. Export the realm for disaster recovery.

#### Export via CLI

```bash
# Export realm (without users)
kubectl exec -n artstore <keycloak-pod> -- \
  /opt/keycloak/bin/kc.sh export \
  --realm artstore \
  --dir /tmp/export \
  --users skip

# Copy export from pod
kubectl cp artstore/<keycloak-pod>:/tmp/export/artstore-realm.json \
  artstore-realm-export-$(date +%Y%m%d).json
```

#### Import on New Installation

```bash
# Import during Keycloak startup (Helm values)
# keycloak.extraEnvVars:
#   KEYCLOAK_IMPORT: /opt/keycloak/data/import/artstore-realm.json

# Or manual import via CLI
kubectl exec -n artstore <keycloak-pod> -- \
  /opt/keycloak/bin/kc.sh import \
  --file /tmp/artstore-realm.json \
  --override true
```

> **Note**: Realm export does not include user passwords. Users will need to reset passwords after import. For full user backup, use Keycloak's database-level backup.

### 5.4 Backup Schedule Recommendations

| Component | Frequency | Retention | Method |
|-----------|:---------:|:---------:|--------|
| PostgreSQL (full dump) | Daily | 30 days | `pg_dump -Fc` |
| PostgreSQL (WAL archive) | Continuous | 7 days | WAL archiving |
| SE PVC snapshots | Weekly | 4 snapshots | VolumeSnapshot |
| Keycloak realm export | After changes | 5 versions | `kc.sh export` |

> **Future work**: Automated backup via CronJob and/or dedicated backup operator (Velero, Stash) is planned for a future release.

---

## 6. Scaling

### 6.1 Horizontal Scaling (Stateless Modules)

**Ingester Module** and **Query Module** are stateless — they can be scaled horizontally by increasing the replica count.

```yaml
# Helm values for horizontal scaling
ingester-module:
  replicaCount: 3

query-module:
  replicaCount: 3
```

Or via kubectl:

```bash
kubectl scale deployment -n artstore ingester-module --replicas=3
kubectl scale deployment -n artstore query-module --replicas=3
```

**Admin Module** should remain at **1 replica** in most deployments. It manages synchronization tasks (file sync, SA sync, dephealth checks) that are not designed for multi-instance operation. Running multiple AM instances may cause duplicate sync operations.

> **Note**: The API Gateway (Envoy) automatically load-balances across multiple replicas of IM and QM.

**When to scale**:

| Metric | Threshold | Action |
|--------|-----------|--------|
| IM upload latency p95 > 10s | Sustained load | Add IM replicas |
| QM search latency p95 > 2s | Sustained load | Add QM replicas |
| QM `qm_active_downloads` > 50 | High concurrent downloads | Add QM replicas |
| IM `im_active_uploads` > 100 | High concurrent uploads | Add IM replicas |

### 6.2 Adding Storage Elements

To increase storage capacity, deploy additional Storage Elements:

**In Kubernetes**:

```yaml
# Helm values — add a new SE instance
storage-element:
  instances:
    - name: se-rw-03
      mode: rw
      priority: 30
      storage: 100Gi
```

**Remote Docker SE**:

```bash
docker run -d \
  --name se-remote-01 \
  -p 8010:8010 \
  -v /data/se-remote-01:/data \
  -e SE_MODE=rw \
  -e SE_ADMIN_MODULE_URL=https://artstore.example.com \
  -e SE_ADMIN_MODULE_TOKEN_URL=https://keycloak.example.com/realms/artstore/protocol/openid-connect/token \
  -e SE_TLS_CERT=/certs/tls.crt \
  -e SE_TLS_KEY=/certs/tls.key \
  harbor.kryukov.lan/library/storage-element:v0.1.0
```

After deploying, register the SE via Admin UI or API (see [Admin Guide §5.3](admin-guide.md#53-registering-a-storage-element)).

**SE priority and fill order**: Ingester Module uses the Sequential Fill algorithm — lower priority number is filled first. When adding new SE, assign a higher priority number to fill existing SE before the new one:

| SE Name | Priority | Fill Order |
|---------|:--------:|:----------:|
| se-rw-01 | 10 | 1st |
| se-rw-02 | 20 | 2nd |
| se-rw-03 (new) | 30 | 3rd |

### 6.3 Resource Recommendations

#### Development / Testing

| Module | CPU Request | CPU Limit | Memory Request | Memory Limit | Replicas |
|--------|:----------:|:---------:|:--------------:|:------------:|:--------:|
| Admin Module | 100m | 500m | 128Mi | 256Mi | 1 |
| Storage Element | 100m | 500m | 128Mi | 256Mi | 1 per mode |
| Ingester Module | 100m | 500m | 128Mi | 256Mi | 1 |
| Query Module | 100m | 500m | 128Mi | 256Mi | 1 |
| PostgreSQL | 200m | 1000m | 256Mi | 512Mi | 1 |
| Keycloak | 200m | 1000m | 512Mi | 1Gi | 1 |

**Total minimum**: ~4 CPU cores, ~4 GB RAM (fits on a single minikube/kind node).

#### Production

| Module | CPU Request | CPU Limit | Memory Request | Memory Limit | Replicas |
|--------|:----------:|:---------:|:--------------:|:------------:|:--------:|
| Admin Module | 250m | 1000m | 256Mi | 512Mi | 1 |
| Storage Element | 250m | 1000m | 256Mi | 512Mi | 2+ per mode |
| Ingester Module | 250m | 1000m | 256Mi | 512Mi | 2+ |
| Query Module | 250m | 1000m | 256Mi | 512Mi | 2+ |
| PostgreSQL | 500m | 2000m | 1Gi | 4Gi | 1 (or HA) |
| Keycloak | 500m | 2000m | 1Gi | 2Gi | 1 (or HA) |

**Total recommended**: ~8-16 CPU cores, ~16-32 GB RAM (3-node K8s cluster).

### 6.4 Performance Tuning

#### PostgreSQL Tuning

For file registries with 100K+ files:

```sql
-- Ensure FTS index exists and is up to date
REINDEX INDEX files_fts_idx;

-- Check query performance
EXPLAIN ANALYZE SELECT * FROM files WHERE to_tsvector('english', name) @@ to_tsquery('test');
```

Key PostgreSQL parameters for Artstore workloads:

| Parameter | Default | Recommendation | Impact |
|-----------|---------|----------------|--------|
| `shared_buffers` | 128MB | 25% of RAM | Query cache performance |
| `work_mem` | 4MB | 64MB | FTS and sort operations |
| `max_connections` | 100 | 200 | AM + QM + IM connections |
| `effective_cache_size` | 4GB | 75% of RAM | Query planner decisions |

#### Query Module Cache Tuning

| Parameter | Env Variable | Default | Tuning Guide |
|-----------|-------------|---------|--------------|
| Cache size | `QM_CACHE_MAX_SIZE` | 1000 | Increase for repeat-access workloads |
| Cache TTL | `QM_CACHE_TTL` | 5m | Increase for rarely-changing files |

Monitor cache effectiveness:

```promql
# Cache hit rate
100 * rate(qm_cache_hits_total[5m]) / (rate(qm_cache_hits_total[5m]) + rate(qm_cache_misses_total[5m]))
```

Target cache hit rate > 50% for typical workloads. If consistently below 30%, increase cache size.

#### Ingester Module Tuning

| Parameter | Env Variable | Default | Tuning Guide |
|-----------|-------------|---------|--------------|
| Max upload size | `IM_MAX_UPLOAD_SIZE` | 1GB | Adjust based on expected file sizes |
| Upload timeout | `IM_UPLOAD_TIMEOUT` | 300s | Increase for large files over slow networks |
| SE retry count | `IM_SE_RETRY_COUNT` | 3 | Increase if SE failures are transient |

---

*Last updated: 2026-03-03*
