# Artstore Admin Guide

> **Language**: [English](admin-guide.md) | [Русский](admin-guide.ru.md)

---

## Table of Contents

- [1. Architecture Overview](#1-architecture-overview)
  - [1.1 System Components](#11-system-components)
  - [1.2 System Topology](#12-system-topology)
  - [1.3 Data Flows](#13-data-flows)
  - [1.4 Deployment Models](#14-deployment-models)
- [2. Requirements](#2-requirements)
  - [2.1 Software Requirements](#21-software-requirements)
  - [2.2 Hardware Resources](#22-hardware-resources)
  - [2.3 Network Requirements](#23-network-requirements)
  - [2.4 Storage Requirements](#24-storage-requirements)
- [3. Installation](#3-installation)
  - [3.1 Deployment Order](#31-deployment-order)
  - [3.2 Quick Start (docker-compose)](#32-quick-start-docker-compose)
  - [3.3 Kubernetes Production Deployment](#33-kubernetes-production-deployment)
  - [3.4 Individual Module Installation](#34-individual-module-installation)
  - [3.5 Remote Storage Element (Docker)](#35-remote-storage-element-docker)
  - [3.6 API Gateway Configuration](#36-api-gateway-configuration)
- [4. Configuration](#4-configuration)
  - [4.1 Admin Module](#41-admin-module)
  - [4.2 Storage Element](#42-storage-element)
  - [4.3 Ingester Module](#43-ingester-module)
  - [4.4 Query Module](#44-query-module)
  - [4.5 PostgreSQL (Shared Database)](#45-postgresql-shared-database)
  - [4.6 TLS Configuration](#46-tls-configuration)
- [5. Storage Element Management](#5-storage-element-management)
  - [5.1 SE Lifecycle](#51-se-lifecycle)
  - [5.2 SE Internal Architecture](#52-se-internal-architecture)
  - [5.3 Registering a Storage Element](#53-registering-a-storage-element)
  - [5.4 Replication (High Availability)](#54-replication-high-availability)
  - [5.5 Capacity Planning](#55-capacity-planning)
  - [5.6 Geo-Distributed Storage Elements](#56-geo-distributed-storage-elements)
- [6. Admin UI](#6-admin-ui)
  - [6.1 Authentication Flow](#61-authentication-flow)
  - [6.2 Dashboard](#62-dashboard)
  - [6.3 Storage Elements Page](#63-storage-elements-page)
  - [6.4 Files Page](#64-files-page)
  - [6.5 Monitoring Page](#65-monitoring-page)
  - [6.6 Access Management](#66-access-management)
  - [6.7 Settings Page](#67-settings-page)
- [7. Keycloak Configuration](#7-keycloak-configuration)
  - [7.1 Realm Settings](#71-realm-settings)
  - [7.2 Realm Roles and Groups](#72-realm-roles-and-groups)
  - [7.3 Client Scopes](#73-client-scopes)
  - [7.4 Clients](#74-clients)
  - [7.5 The client_id Mapper (Critical)](#75-the-client_id-mapper-critical)
  - [7.6 Two-URL Pattern for Kubernetes](#76-two-url-pattern-for-kubernetes)
  - [7.7 Custom Keycloak Theme](#77-custom-keycloak-theme)
  - [7.8 Creating a New Client (Step by Step)](#78-creating-a-new-client-step-by-step)
- [8. Upgrading](#8-upgrading)
  - [8.1 Upgrade Order](#81-upgrade-order)
  - [8.2 Database Migrations](#82-database-migrations)
  - [8.3 Version Compatibility](#83-version-compatibility)
  - [8.4 Backup and Recovery](#84-backup-and-recovery)

---

## 1. Architecture Overview

Artstore is a distributed file storage system with a microservices architecture. It provides secure file storage, upload, search, and download functionality with fine-grained access control via Keycloak (OpenID Connect / OAuth 2.0).

### 1.1 System Components

The system consists of four independently deployable modules:

| Module | Default Port | Responsibility |
|--------|:------------:|----------------|
| **Admin Module (AM)** | 8000 | Control center: SE registry, file registry, RBAC, Service Accounts, Admin UI |
| **Storage Element (SE)** | 8010 | Physical file storage with WAL, attribute-based metadata, replication |
| **Ingester Module (IM)** | 8020 | File upload endpoint (stateless), SE selection via Sequential Fill |
| **Query Module (QM)** | 8030 | File search (PostgreSQL FTS) and download proxy with LRU cache |

**Key architectural principle**: Admin Module is *not* an authentication server. All authentication (JWT issuance, user management) is delegated to **Keycloak**. Modules only validate JWT tokens for authorization decisions.

![C4 Context Diagram](images/c4-context.png)
*Figure 1.1 — C4 Context: Artstore system boundary, external actors and systems*

### 1.2 System Topology

All control-plane modules (AM, IM, QM) run inside a Kubernetes cluster behind an API Gateway. Storage Elements can be deployed anywhere — inside K8s, on remote Docker hosts, or on bare-metal servers across data centers.

![C4 Container Diagram](images/c4-container.png)
*Figure 1.2 — C4 Container: modules, databases, and communication flows*

**Inter-module communication**:

| From | To | Protocol | Purpose |
|------|----|----------|---------|
| API Gateway | AM, IM, QM | HTTP | Route external requests, TLS termination |
| IM | Keycloak | HTTP | Obtain SA token (client_credentials grant) |
| IM | AM | HTTP + JWT | Get SE list, register uploaded files |
| IM | SE | HTTPS + JWT | Upload files |
| QM | PostgreSQL | TCP | Search file registry (FTS) |
| QM | AM | HTTP + JWT | Get SE URLs for download |
| QM | SE | HTTPS + JWT | Proxy file download |
| AM | Keycloak | HTTP | Sync Service Accounts, JWKS |
| AM | SE | HTTPS + JWT | Health checks, file sync, mode transitions |
| SE | Keycloak | HTTP | JWKS endpoint (JWT validation) |

Health and metrics endpoints (`/health/*`, `/metrics`) are accessed directly on pod IPs, bypassing the API Gateway — used by Kubernetes probes and Prometheus.

### 1.3 Data Flows

**File Upload** — Client uploads a file via Ingester Module, which selects a target Storage Element using the Sequential Fill algorithm, stores the file on SE, and registers it in Admin Module.

![Data Flow: Upload](images/dataflow-upload.png)
*Figure 1.3 — Data Flow: file upload through IM → SE → AM*

**File Download** — Client requests a file via Query Module, which looks up the file location in Admin Module, streams the file from the appropriate Storage Element, and caches the result.

![Data Flow: Download](images/dataflow-download.png)
*Figure 1.4 — Data Flow: file download through QM → AM → SE*

**File Search** — Client searches for files via Query Module, which queries the PostgreSQL full-text search index directly (shared database with Admin Module).

![Data Flow: Search](images/dataflow-search.png)
*Figure 1.5 — Data Flow: file search via QM → PostgreSQL FTS*

For detailed sequence diagrams, see:
- [File Upload Sequence](images/seq-file-upload.png)
- [File Download Sequence](images/seq-file-download.png)
- [JWT Authentication Sequence](images/seq-jwt-auth.png)
- [SE Registration Sequence](images/seq-se-registration.png)

### 1.4 Deployment Models

Artstore supports two primary deployment models:

**Full Kubernetes** — All modules, including Storage Elements, run inside a Kubernetes cluster. SE uses StatefulSet with PersistentVolumeClaims (PVC) for data storage.

![Deployment: Kubernetes](images/deploy-k8s.png)
*Figure 1.6 — Deployment: all components within a Kubernetes cluster*

**Hybrid** — Control-plane modules (AM, IM, QM) and databases (PostgreSQL, Keycloak) run in Kubernetes. Storage Elements run as Docker containers on remote hosts or VMs in other data centers, connected over WAN with TLS.

![Deployment: Hybrid](images/deploy-hybrid.png)
*Figure 1.7 — Deployment: K8s control plane + remote Docker SE instances*

The hybrid model is particularly useful for:
- Geo-distributed storage across multiple data centers
- Using existing infrastructure with large local disks
- Compliance requirements mandating data locality
- Gradual migration from on-premise to cloud

---

## 2. Requirements

### 2.1 Software Requirements

| Component | Minimum Version | Notes |
|-----------|:---------------:|-------|
| Kubernetes | 1.28+ | For AM, IM, QM deployment |
| Docker | 24+ | For remote SE deployment |
| PostgreSQL | 17 | Shared between AM and QM |
| Keycloak | 26.1+ | With custom `artstore` realm |
| Helm | 3.12+ | Chart deployment |
| cert-manager | 1.13+ | TLS certificate management (recommended) |
| Gateway API | 1.0+ | API Gateway (Envoy Gateway recommended) |

**Kubernetes features required**:
- Gateway API CRDs (primary) *or* Ingress controller (nginx, alternative)
- LoadBalancer service support (MetalLB for bare-metal clusters)
- cert-manager with a configured `ClusterIssuer` (for automatic TLS)
- PersistentVolume provisioner (for SE data storage in K8s mode)

### 2.2 Hardware Resources

#### Development / Testing

| Component | CPU | Memory | Storage |
|-----------|:---:|:------:|:-------:|
| Admin Module | 100m | 128Mi | — |
| Storage Element | 100m | 128Mi | 1Gi+ (PVC) |
| Ingester Module | 100m | 128Mi | — |
| Query Module | 100m | 128Mi | — |
| PostgreSQL | 250m | 256Mi | 1Gi |
| Keycloak | 500m | 512Mi | — |
| **Total** | ~1.2 CPU | ~1.3Gi | ~3Gi |

A development environment can run on **minikube** or **kind** with 4 GB RAM.

#### Production (Recommended)

| Component | CPU | Memory | Storage | Replicas |
|-----------|:---:|:------:|:-------:|:--------:|
| Admin Module | 500m | 512Mi | — | 1 |
| Storage Element | 500m–1000m | 512Mi–1Gi | 100Gi+ (PVC) | 1+ per mode |
| Ingester Module | 500m | 256Mi | — | 2 (HA) |
| Query Module | 500m | 512Mi | — | 2 (HA) |
| PostgreSQL | 1000m | 2Gi | 50Gi | 1 (or HA cluster) |
| Keycloak | 1000m | 1Gi | — | 1 (or HA cluster) |

SE resources depend on file sizes and throughput requirements. For large files (>100 MB), increase SE memory to buffer streaming operations.

### 2.3 Network Requirements

- **API Gateway** must have a public or internal LoadBalancer IP accessible to clients
- **Storage Elements** must be reachable via HTTP/HTTPS from the Kubernetes cluster (for IM upload, QM download, AM health checks)
- **Keycloak** must be accessible from all locations where JWT validation occurs: API Gateway, SE instances, and client applications
- All connections to SE over WAN **must** use TLS (configure `SE_TLS_CERT` and `SE_TLS_KEY`)
- Firewall rules: allow HTTPS (443) from K8s cluster to remote SE hosts
- DNS resolution required for all module endpoints

**Bandwidth considerations for hybrid deployments**:
- Upload throughput depends on the WAN link between K8s (IM) and remote SE
- Download throughput depends on the WAN link between K8s (QM) and remote SE
- Replication bandwidth between SE replicas depends on the shared NFS link

### 2.4 Storage Requirements

Each Storage Element requires:

| Directory | Environment Variable | Purpose |
|-----------|---------------------|---------|
| Data directory | `SE_DATA_DIR` | File data and `*.attr.json` metadata |
| WAL directory | `SE_WAL_DIR` | Write-Ahead Log for atomicity |

**For replicated mode** (`SE_REPLICA_MODE=replicated`):
- NFS v4+ shared filesystem mounted on all replicas
- Both `SE_DATA_DIR` and `SE_WAL_DIR` must be on the shared NFS volume
- Leader election uses `flock()` on `.leader.lock` in the data directory

**Capacity planning**:
- Each file has an overhead of ~1 KB for the corresponding `*.attr.json` metadata file
- WAL entries are temporary and cleaned up after commit
- `SE_MAX_CAPACITY` must be set to the usable disk capacity (excluding OS and other data)
- Reserve 10% of disk space for WAL and filesystem overhead

---

## 3. Installation

### 3.1 Deployment Order

Components must be deployed in the following order:

1. **PostgreSQL** — database for Admin Module and Query Module
2. **Keycloak** — configure realm `artstore`, create clients, groups, and initial admin user
3. **API Gateway** — configure TLS termination, JWT validation, and path routing
4. **Admin Module** — connects to PostgreSQL and Keycloak, applies DB migrations automatically
5. **Storage Elements** — register with Admin Module after startup
6. **Ingester Module** and **Query Module** — connect to AM and SE via API Gateway

> **Note**: Admin Module must be running and accessible before starting IM, QM, or registering SE instances. IM and QM depend on AM for SE discovery and file registry operations.

### 3.2 Quick Start (docker-compose)

The fastest way to run Artstore locally is via `docker-compose`:

```bash
# Clone the repository
git clone https://github.com/bigkaa/goartstore.git
cd goartstore

# Start all services
docker-compose up -d

# Verify all services are running
docker-compose ps
```

This starts PostgreSQL, Keycloak (with pre-configured realm), Admin Module, one Storage Element (edit mode), Ingester Module, and Query Module.

Default endpoints:
- Admin UI: `http://localhost:8000/admin/`
- Upload API: `http://localhost:8020/api/v1/upload`
- Search API: `http://localhost:8030/api/v1/search`
- Download API: `http://localhost:8030/api/v1/files/{id}/download`

Default credentials:
- Admin user: `admin` / `admin`

> **Note**: docker-compose is intended for development and testing only. For production deployments, use Kubernetes with the umbrella Helm chart.

### 3.3 Kubernetes Production Deployment

Use the umbrella Helm chart for a full Kubernetes deployment:

```bash
# Add the Artstore Helm repository (or use local charts)
helm repo add artstore https://charts.example.com/artstore

# Install with production profile
helm install artstore charts/artstore/ \
  -f charts/artstore/values-production.yaml \
  -n artstore --create-namespace

# Verify all pods are running
kubectl get pods -n artstore
```

The umbrella chart includes all modules as sub-charts with two predefined profiles:
- `values-dev.yaml` — minimal resources for development/testing
- `values-production.yaml` — recommended resources, HA replicas, TLS

See [§4. Configuration](#4-configuration) for detailed settings.

### 3.4 Individual Module Installation

Each module has its own Helm chart in `src/<module>/charts/<module>/` and can be deployed independently:

```bash
# Deploy Admin Module
helm install admin-module src/admin-module/charts/admin-module/ \
  -n artstore --create-namespace \
  -f my-admin-values.yaml

# Deploy Storage Element
helm install se-edit-01 src/storage-element/charts/storage-element/ \
  -n artstore \
  -f my-se-values.yaml

# Deploy Ingester Module
helm install ingester-module src/ingester-module/charts/ingester-module/ \
  -n artstore \
  -f my-ingester-values.yaml

# Deploy Query Module
helm install query-module src/query-module/charts/query-module/ \
  -n artstore \
  -f my-query-values.yaml
```

### 3.5 Remote Storage Element (Docker)

For hybrid deployments, run Storage Element as a Docker container on a remote host:

```bash
docker run -d \
  --name se-remote-01 \
  --restart unless-stopped \
  -p 8010:8010 \
  -v /data/artstore/storage:/data \
  -v /data/artstore/wal:/wal \
  -v /etc/artstore/certs:/certs:ro \
  -e SE_STORAGE_ID=se-remote-01 \
  -e SE_DATA_DIR=/data \
  -e SE_WAL_DIR=/wal \
  -e SE_MODE=rw \
  -e SE_MAX_CAPACITY=107374182400 \
  -e SE_MAX_FILE_SIZE=1073741824 \
  -e SE_TLS_CERT=/certs/tls.crt \
  -e SE_TLS_KEY=/certs/tls.key \
  -e SE_JWKS_URL=http://keycloak.example.com/realms/artstore/protocol/openid-connect/certs \
  harbor.kryukov.lan/library/storage-element:v0.6.0
```

After starting the remote SE, register it in Admin Module:

```bash
# Get admin token
TOKEN=$(curl -s -X POST \
  "https://keycloak.example.com/realms/artstore/protocol/openid-connect/token" \
  -d "grant_type=client_credentials" \
  -d "client_id=artstore-admin-module" \
  -d "client_secret=<secret>" | jq -r '.access_token')

# Register SE
curl -X POST "https://artstore.example.com/api/v1/storage-elements" \
  -H "Authorization: Bearer $TOKEN" \
  -H "Content-Type: application/json" \
  -d '{
    "name": "se-remote-01",
    "url": "https://se-remote-01.example.com:8010",
    "priority": 10
  }'
```

> **Important**: The SE URL must be reachable from the Kubernetes cluster (from IM and QM pods). Ensure firewall rules and DNS resolution are properly configured.

### 3.6 API Gateway Configuration

Artstore requires an API Gateway for TLS termination, JWT validation, and path-based routing.

**Path Routing Rules**:

| Path Prefix | Backend Service | Port | Strip Prefix |
|-------------|----------------|:----:|:------------:|
| `/admin/*` | admin-module | 8000 | No |
| `/api/*` | admin-module | 8000 | No |
| `/upload/*` | ingester-module | 8020 | Yes (`/upload` → `/api/v1`) |
| `/query/*` | query-module | 8030 | Yes (`/query` → `/`) |

**Envoy Gateway (recommended for Kubernetes)**:

```yaml
apiVersion: gateway.networking.k8s.io/v1
kind: HTTPRoute
metadata:
  name: artstore
  namespace: artstore
spec:
  parentRefs:
    - name: eg
      namespace: envoy-gateway-system
  hostnames:
    - "artstore.example.com"
  rules:
    - matches:
        - path:
            type: PathPrefix
            value: /admin
      backendRefs:
        - name: admin-module
          port: 8000
    - matches:
        - path:
            type: PathPrefix
            value: /api
      backendRefs:
        - name: admin-module
          port: 8000
    - matches:
        - path:
            type: PathPrefix
            value: /upload
      backendRefs:
        - name: ingester-module
          port: 8020
      filters:
        - type: URLRewrite
          urlRewrite:
            path:
              type: ReplacePrefixMatch
              replacePrefixMatch: /api/v1
    - matches:
        - path:
            type: PathPrefix
            value: /query
      backendRefs:
        - name: query-module
          port: 8030
      filters:
        - type: URLRewrite
          urlRewrite:
            path:
              type: ReplacePrefixMatch
              replacePrefixMatch: /
```

**JWT Validation (Envoy Gateway)**:

```yaml
apiVersion: gateway.envoyproxy.io/v1alpha1
kind: SecurityPolicy
metadata:
  name: artstore-jwt
  namespace: artstore
spec:
  targetRefs:
    - group: gateway.networking.k8s.io
      kind: HTTPRoute
      name: artstore
  jwt:
    providers:
      - name: keycloak
        issuer: "https://artstore.example.com/realms/artstore"
        remoteJWKS:
          uri: "http://keycloak.artstore.svc:8080/realms/artstore/protocol/openid-connect/certs"
```

**Alternative: Nginx Ingress** — If using Nginx Ingress controller instead of Gateway API, configure JWT validation via `auth_request` to a sidecar or `lua-resty-jwt`.

---

## 4. Configuration

All modules are configured via environment variables. Each module uses a distinct prefix to avoid collisions.

### 4.1 Admin Module

Environment variable prefix: `AM_`

**Server**:

| Variable | Required | Default | Description |
|----------|:--------:|---------|-------------|
| `AM_PORT` | No | `8000` | HTTP server port |
| `AM_LOG_LEVEL` | No | `info` | Log level: `debug`, `info`, `warn`, `error` |
| `AM_LOG_FORMAT` | No | `json` | Log format: `json` (production), `text` (development) |

**PostgreSQL**:

| Variable | Required | Default | Description |
|----------|:--------:|---------|-------------|
| `AM_DB_HOST` | Yes | — | PostgreSQL host |
| `AM_DB_PORT` | No | `5432` | PostgreSQL port |
| `AM_DB_NAME` | Yes | — | Database name |
| `AM_DB_USER` | Yes | — | Database user |
| `AM_DB_PASSWORD` | Yes | — | Database password |
| `AM_DB_SSL_MODE` | No | `disable` | SSL mode: `disable`, `require`, `verify-ca`, `verify-full` |

**Keycloak**:

| Variable | Required | Default | Description |
|----------|:--------:|---------|-------------|
| `AM_KEYCLOAK_URL` | Yes | — | Keycloak base URL (e.g., `https://keycloak.example.com`) |
| `AM_KEYCLOAK_REALM` | No | `artstore` | Keycloak realm name |
| `AM_KEYCLOAK_CLIENT_ID` | Yes | — | Client ID for Keycloak Admin API (e.g., `artstore-admin-module`) |
| `AM_KEYCLOAK_CLIENT_SECRET` | Yes | — | Client secret for Keycloak Admin API |
| `AM_KEYCLOAK_SA_PREFIX` | No | `sa_` | Prefix to identify Service Account clients in Keycloak |

**JWT**:

| Variable | Required | Default | Description |
|----------|:--------:|---------|-------------|
| `AM_JWT_ISSUER` | No | *(derived from KC URL)* | Expected `iss` claim in JWT |
| `AM_JWT_JWKS_URL` | No | *(derived from KC URL)* | JWKS URL for JWT validation |
| `AM_JWT_ROLES_CLAIM` | No | `realm_access.roles` | JSON path to roles in JWT claims |
| `AM_JWT_GROUPS_CLAIM` | No | `groups` | JSON path to groups in JWT claims |

**Synchronization**:

| Variable | Required | Default | Description |
|----------|:--------:|---------|-------------|
| `AM_SYNC_INTERVAL` | No | `1h` | SE file registry sync interval |
| `AM_SYNC_PAGE_SIZE` | No | `1000` | Page size during file sync |
| `AM_SA_SYNC_INTERVAL` | No | `15m` | Service Account sync interval with Keycloak |
| `AM_SE_CA_CERT_PATH` | No | — | CA certificate path for TLS connections to SE |
| `AM_DEPHEALTH_CHECK_INTERVAL` | No | `15s` | Dependency health check interval |

**Role Mapping**:

| Variable | Required | Default | Description |
|----------|:--------:|---------|-------------|
| `AM_ROLE_ADMIN_GROUPS` | No | `artstore-admins` | Keycloak groups mapped to `admin` role |
| `AM_ROLE_READONLY_GROUPS` | No | `artstore-viewers` | Keycloak groups mapped to `readonly` role |

**Admin UI Session**:

| Variable | Required | Default | Description |
|----------|:--------:|---------|-------------|
| `AM_UI_SESSION_SECRET` | No | *(auto-generated)* | 32-byte key for AES-GCM encrypted session cookies |

> **Note**: In production, always set `AM_UI_SESSION_SECRET` explicitly. An auto-generated key changes on pod restart, invalidating all active sessions.

### 4.2 Storage Element

Environment variable prefix: `SE_`

**Server**:

| Variable | Required | Default | Description |
|----------|:--------:|---------|-------------|
| `SE_PORT` | No | `8010` | HTTP server port |
| `SE_STORAGE_ID` | Yes | — | Unique SE identifier (e.g., `se-moscow-01`) |
| `SE_LOG_LEVEL` | No | `info` | Log level |
| `SE_LOG_FORMAT` | No | `json` | Log format |

**Storage**:

| Variable | Required | Default | Description |
|----------|:--------:|---------|-------------|
| `SE_DATA_DIR` | Yes | — | Path to file data directory |
| `SE_WAL_DIR` | Yes | — | Path to WAL directory |
| `SE_MODE` | No | `edit` | Initial mode: `edit`, `rw`, `ro`, `ar` |
| `SE_MAX_FILE_SIZE` | No | `1073741824` | Maximum file size in bytes (default: 1 GB) |
| `SE_MAX_CAPACITY` | Yes | — | Configured capacity limit in bytes |

**Maintenance**:

| Variable | Required | Default | Description |
|----------|:--------:|---------|-------------|
| `SE_GC_INTERVAL` | No | `1h` | Garbage collection interval |
| `SE_RECONCILE_INTERVAL` | No | `6h` | Auto-reconciliation interval (attr.json vs filesystem) |

**Security**:

| Variable | Required | Default | Description |
|----------|:--------:|---------|-------------|
| `SE_JWKS_URL` | Yes | — | JWKS endpoint URL for JWT validation |
| `SE_TLS_CERT` | Yes | — | Path to TLS certificate |
| `SE_TLS_KEY` | Yes | — | Path to TLS private key |

**Replication** (optional):

| Variable | Required | Default | Description |
|----------|:--------:|---------|-------------|
| `SE_REPLICA_MODE` | No | `standalone` | `standalone` or `replicated` |
| `SE_INDEX_REFRESH_INTERVAL` | No | `30s` | Follower index refresh interval |

**Health**:

| Variable | Required | Default | Description |
|----------|:--------:|---------|-------------|
| `SE_DEPHEALTH_CHECK_INTERVAL` | No | `15s` | Dependency health check interval |

### 4.3 Ingester Module

Environment variable prefix: `IM_`

**Server**:

| Variable | Required | Default | Description |
|----------|:--------:|---------|-------------|
| `IM_PORT` | No | `8020` | HTTP server port |
| `IM_LOG_LEVEL` | No | `info` | Log level |
| `IM_LOG_FORMAT` | No | `json` | Log format |

**JWT**:

| Variable | Required | Default | Description |
|----------|:--------:|---------|-------------|
| `IM_JWKS_URL` | Yes | — | JWKS endpoint for validating incoming JWT |
| `IM_JWT_ISSUER` | No | `""` | Expected JWT issuer |

**Service Account**:

| Variable | Required | Default | Description |
|----------|:--------:|---------|-------------|
| `IM_CLIENT_ID` | Yes | — | SA client_id for AM and SE requests |
| `IM_CLIENT_SECRET` | Yes | — | SA client_secret |
| `IM_TOKEN_URL` | No | `""` | Keycloak token endpoint (direct, not via AM) |

**Upload**:

| Variable | Required | Default | Description |
|----------|:--------:|---------|-------------|
| `IM_ADMIN_URL` | Yes | — | Admin Module base URL |
| `IM_MAX_FILE_SIZE` | No | `1073741824` | Maximum file size (1 GB) |
| `IM_DEFAULT_TTL_DAYS` | No | `30` | Default TTL for temporary files |
| `IM_MAX_RETRIES` | No | `3` | Max retries on SE 507 (Insufficient Storage) |
| `IM_SE_UPLOAD_TIMEOUT` | No | `10m` | Upload timeout to SE |
| `IM_SE_CA_CERT_PATH` | No | `""` | CA certificate for TLS connections to SE |
| `IM_HTTP_WRITE_TIMEOUT` | No | `600s` | HTTP write timeout (large to support big uploads) |

**Role Mapping**:

| Variable | Required | Default | Description |
|----------|:--------:|---------|-------------|
| `IM_ROLE_ADMIN_GROUPS` | No | `artstore-admins` | Keycloak groups for admin role |
| `IM_ROLE_READONLY_GROUPS` | No | `artstore-viewers` | Keycloak groups for readonly role |

### 4.4 Query Module

Environment variable prefix: `QM_`

**Server**:

| Variable | Required | Default | Description |
|----------|:--------:|---------|-------------|
| `QM_PORT` | No | `8030` | HTTP server port |
| `QM_LOG_LEVEL` | No | `info` | Log level |
| `QM_LOG_FORMAT` | No | `json` | Log format |

**PostgreSQL**:

| Variable | Required | Default | Description |
|----------|:--------:|---------|-------------|
| `QM_DB_HOST` | Yes | — | PostgreSQL host (same instance as AM) |
| `QM_DB_PORT` | No | `5432` | PostgreSQL port |
| `QM_DB_NAME` | Yes | — | Database name (same as AM) |
| `QM_DB_USER` | Yes | — | Database user (can be read-only) |
| `QM_DB_PASSWORD` | Yes | — | Database password |
| `QM_DB_SSL_MODE` | No | `disable` | SSL mode |

**Service Account**:

| Variable | Required | Default | Description |
|----------|:--------:|---------|-------------|
| `QM_JWKS_URL` | Yes | — | JWKS endpoint for JWT validation |
| `QM_ADMIN_URL` | Yes | — | Admin Module base URL |
| `QM_CLIENT_ID` | Yes | — | SA client_id |
| `QM_CLIENT_SECRET` | Yes | — | SA client_secret |
| `QM_TOKEN_URL` | No | `""` | Keycloak token endpoint |

**Cache**:

| Variable | Required | Default | Description |
|----------|:--------:|---------|-------------|
| `QM_CACHE_TTL` | No | `60s` | LRU cache entry TTL |
| `QM_CACHE_MAX_SIZE` | No | `10000` | Maximum number of cached entries |

**Timeouts**:

| Variable | Required | Default | Description |
|----------|:--------:|---------|-------------|
| `QM_ADMIN_TIMEOUT` | No | `10s` | Admin Module request timeout |
| `QM_SE_DOWNLOAD_TIMEOUT` | No | `5m` | SE proxy download timeout |
| `QM_SE_CA_CERT_PATH` | No | — | CA certificate for TLS connections to SE |

### 4.5 PostgreSQL (Shared Database)

Admin Module and Query Module share a single PostgreSQL instance:

- **Admin Module** owns the schema: creates tables (`file_registry`, `storage_elements`, `service_accounts`, `role_overrides`, `sync_state`, `ui_settings`) and applies migrations on startup
- **Query Module** adds read-optimized indexes (GIN/FTS) to the `file_registry` table and applies its own migrations (indexes only) on startup
- Both modules use `golang-migrate` with embedded migrations — no manual migration steps required

**Database setup**:

```sql
-- Create database and user
CREATE DATABASE artstore;
CREATE USER artstore_app WITH PASSWORD 'secure_password';
GRANT ALL PRIVILEGES ON DATABASE artstore TO artstore_app;

-- For Query Module read-only user (optional, recommended for production)
CREATE USER artstore_reader WITH PASSWORD 'reader_password';
GRANT CONNECT ON DATABASE artstore TO artstore_reader;
GRANT USAGE ON SCHEMA public TO artstore_reader;
GRANT SELECT ON ALL TABLES IN SCHEMA public TO artstore_reader;
ALTER DEFAULT PRIVILEGES IN SCHEMA public GRANT SELECT ON TABLES TO artstore_reader;
-- Note: QM also needs CREATE INDEX permission for its migrations
GRANT CREATE ON SCHEMA public TO artstore_reader;
```

### 4.6 TLS Configuration

**For SE in Kubernetes** — Use cert-manager to automatically provision TLS certificates:

```yaml
apiVersion: cert-manager.io/v1
kind: Certificate
metadata:
  name: se-tls
  namespace: artstore
spec:
  secretName: se-tls-secret
  issuerRef:
    name: dev-ca-issuer
    kind: ClusterIssuer
  dnsNames:
    - "se-edit-01.artstore.svc"
    - "se-rw-01.artstore.svc"
```

Mount the certificate secret in the SE pod and set `SE_TLS_CERT` and `SE_TLS_KEY`.

**For remote SE** — Obtain a TLS certificate from your CA (or use Let's Encrypt) and mount it in the Docker container:

```bash
# Generate with Let's Encrypt (example)
certbot certonly --standalone -d se-remote-01.example.com

# Or use your internal CA
openssl req -x509 -newkey rsa:4096 -keyout tls.key -out tls.crt \
  -days 365 -nodes -subj "/CN=se-remote-01.example.com"
```

**CA Trust** — When using internal CA certificates, configure all modules to trust the SE CA:
- Admin Module: `AM_SE_CA_CERT_PATH=/path/to/ca.crt`
- Ingester Module: `IM_SE_CA_CERT_PATH=/path/to/ca.crt`
- Query Module: `QM_SE_CA_CERT_PATH=/path/to/ca.crt`

---

## 5. Storage Element Management

### 5.1 SE Lifecycle

Storage Elements follow a strict lifecycle with mode transitions:

```
  ┌──────────────────────────────────────────────────────┐
  │  edit (full access, temporary files)                 │
  │  - Isolated lifecycle, no transitions to/from others │
  └──────────────────────────────────────────────────────┘

  ┌──────┐     ┌──────┐     ┌──────┐
  │  rw  │ ──→ │  ro  │ ──→ │  ar  │
  └──────┘     └──────┘     └──────┘
  read-write   read-only    archive
               ← (with confirm:true)
```

**Mode capabilities**:

| Mode | Upload | Download | Update | Delete | List/Metadata | Use Case |
|------|:------:|:--------:|:------:|:------:|:-------------:|----------|
| `edit` | Yes | Yes | Yes | Yes | Yes | Temporary files, drafts |
| `rw` | Yes | Yes | Yes | No | Yes | Permanent file storage |
| `ro` | No | Yes | No | No | Yes | Decommissioning, migration |
| `ar` | No | No | No | No | Yes | Cold storage (metadata only) |

**Two separate lifecycles**:
1. **`edit`** — completely isolated, no transitions to other modes (and vice versa)
2. **`rw` → `ro` → `ar`** — one-way progression for permanent storage. The only allowed reverse transition is `ro` → `rw` (requires explicit `confirm: true`)

**Retention policies**:

| Policy | SE Mode | TTL | Description |
|--------|---------|-----|-------------|
| `temporary` | edit | 1–365 days (default 30) | Auto-deleted by SE garbage collector |
| `permanent` | rw | None | Stored indefinitely until SE mode change |

### 5.2 SE Internal Architecture

![C4 Component: Storage Element](images/c4-component-se.png)
*Figure 5.1 — C4 Component diagram: SE internal structure*

Key components:
- **WAL (Write-Ahead Log)** — ensures atomicity of file operations. Sequence: WAL entry → file write → `attr.json` creation → WAL commit
- **attr.json Handler** — manages per-file metadata files. `*.attr.json` is the single source of truth for file metadata
- **GC (Garbage Collector)** — runs periodically (`SE_GC_INTERVAL`), removes expired temporary files
- **Reconcile Engine** — periodically (`SE_RECONCILE_INTERVAL`) syncs attr.json metadata with the actual filesystem state
- **In-Memory Index** — fast file lookup by ID, rebuilt on startup from attr.json files
- **Health Check** — exposes `/health/live` and `/health/ready` endpoints
- **Prometheus Metrics** — exposes `/metrics` endpoint

### 5.3 Registering a Storage Element

**In Kubernetes** — SE pods register automatically when discovered by Admin Module. The Admin UI provides a "Discover" button to scan for new SE services in the namespace.

**Via API** (for remote SE):

```bash
curl -X POST "https://artstore.example.com/api/v1/storage-elements" \
  -H "Authorization: Bearer $TOKEN" \
  -H "Content-Type: application/json" \
  -d '{
    "name": "se-remote-datacenter-01",
    "url": "https://se-dc01.example.com:8010",
    "priority": 10
  }'
```

**Via Admin UI** — Navigate to Storage Elements page → click "Add" → enter name, URL, and priority.

After registration, Admin Module performs:
1. Health check to verify SE is reachable
2. Full file sync to import existing files into the registry
3. Periodic health checks (`AM_DEPHEALTH_CHECK_INTERVAL`)

**Priority** — Lower value = higher priority. Used by Ingester Module's Sequential Fill algorithm to determine which SE to fill first.

### 5.4 Replication (High Availability)

When `SE_REPLICA_MODE=replicated`, multiple SE instances share an NFS v4+ filesystem using a Leader/Follower pattern.

**Leader election**: via `flock()` file lock on `.leader.lock` in the shared NFS directory. No external dependencies (no etcd, no Redis).

**Role distribution**:

| Operation | Leader | Follower |
|-----------|:------:|:--------:|
| Upload | Yes | Proxies to leader |
| Download | Yes | Yes |
| Delete | Yes | Proxies to leader |
| Update metadata | Yes | Proxies to leader |
| List / metadata | Yes | Yes |
| GC | Yes | No |
| Reconcile | Yes | No |
| WAL | Yes | No |
| Mode transition | Yes | Proxies to leader |

**Follower behavior**:
- Discovers leader address via `.leader.info` file on NFS
- Rebuilds in-memory index at startup and refreshes every `SE_INDEX_REFRESH_INTERVAL` (default 30s)
- Metadata on follower may lag behind leader by up to `SE_INDEX_REFRESH_INTERVAL`
- On leader failure, a follower acquires the lock and becomes the new leader automatically

### 5.5 Capacity Planning

**Sequential Fill algorithm** — Ingester Module fills SE instances in order of priority (ascending). When one SE reaches capacity (returns HTTP 507), IM retries with the next available SE.

Recommendations:
- Provision at least one `edit` SE and one `rw` SE for normal operations
- Monitor `se_storage_bytes` / `SE_MAX_CAPACITY` ratio — alert at 80%, critical at 90%
- When adding new SE, set lower priority (higher number) to fill existing SE first
- For large deployments, use multiple SE with similar capacity for balanced distribution

### 5.6 Geo-Distributed Storage Elements

For hybrid deployments with SE in remote data centers:

1. **Network**: Ensure stable WAN connection between K8s cluster and remote SE. Latency impacts upload/download speed but not system stability
2. **TLS**: Always enable TLS for WAN connections (`SE_TLS_CERT`, `SE_TLS_KEY`)
3. **CA Trust**: Configure AM, IM, QM with the remote SE CA certificate
4. **DNS**: Remote SE must be resolvable from K8s pods
5. **Firewall**: Allow HTTPS (SE port) from K8s cluster to remote SE
6. **Priority**: Assign higher priority (lower number) to local SE for faster access; lower priority (higher number) to remote SE

**Replication across data centers**: Not directly supported via NFS (requires low-latency shared filesystem). For geo-replication, deploy separate standalone SE instances and use Admin Module's file registry to track file locations.

---

## 6. Admin UI

Admin UI is a web application embedded in the Admin Module binary. It uses Templ (Go templates), HTMX for dynamic content, Alpine.js for client-side interactivity, and Tailwind CSS for styling.

Access the Admin UI at: `https://<domain>/admin/`

### 6.1 Authentication Flow

Admin UI uses Keycloak's Authorization Code flow with PKCE (S256):

1. User navigates to `/admin/` in a browser
2. Admin Module detects no active session → redirects to Keycloak login page
3. User enters credentials on the Keycloak login page (customized with the `artstore` theme)
4. Keycloak redirects back to Admin Module with an authorization code
5. Admin Module exchanges the code for JWT tokens via the Keycloak token endpoint
6. JWT is stored in an AES-GCM encrypted HTTP-only cookie

![Keycloak Login](images/kc-login.png)
*Figure 6.1 — Keycloak login page with custom Artstore theme*

### 6.2 Dashboard

The Dashboard provides a real-time overview of the system:

- Metric cards: SE count by status, files by SE mode, total storage used/available
- Dependency status: PostgreSQL and Keycloak connectivity
- SE list with mode indicators, status, and storage capacity bars
- Storage usage charts (ApexCharts)

![Admin UI: Dashboard](images/ui-dashboard.png)
*Figure 6.2 — Admin UI Dashboard with system overview*

### 6.3 Storage Elements Page

Manage all registered Storage Elements:

- Table with columns: name, mode, status, capacity (progress bar), file count, latency, last sync time
- Actions: discover new SE, register SE manually, edit SE settings, trigger manual sync, delete SE
- Mode transition: change SE mode through the lifecycle (`rw` → `ro` → `ar`, or reverse `ro` → `rw` with confirmation)

![Admin UI: SE List](images/ui-se-list.png)
*Figure 6.3 — Storage Elements list with status and capacity*

![Admin UI: SE Details](images/ui-se-details.png)
*Figure 6.4 — Storage Element detail view*

![Admin UI: SE Mode Change](images/ui-se-mode-change.png)
*Figure 6.5 — SE mode transition dialog*

### 6.4 Files Page

Browse and manage the file registry:

- File table with metadata: name, size, content type, SE location, retention policy, status, upload date
- Filters: by status (active/deleted/expired), retention policy, SE, content type
- Search: by filename (substring match)
- File detail modal with full metadata
- Soft delete (admin role only): marks file as deleted in the registry

![Admin UI: Files List](images/ui-files-list.png)
*Figure 6.6 — File registry with filters*

![Admin UI: File Details](images/ui-file-details.png)
*Figure 6.7 — File detail view*

### 6.5 Monitoring Page

Built-in monitoring without external tools:

- Dependency health: status of PostgreSQL, Keycloak, and each SE
- Latency charts for dependency checks
- Background task status: file sync and SA sync progress

![Admin UI: Monitoring](images/ui-monitoring.png)
*Figure 6.8 — Monitoring: dependency health and latency charts*

### 6.6 Access Management

**Users tab** — View users from Keycloak with their roles. Admin can elevate a user's role locally (e.g., `readonly` → `admin`). Role elevation cannot be demoted — effective role = max(IdP role, local override).

**Service Accounts tab** — Manage machine-to-machine (M2M) Service Accounts:
- View SA list with scopes and sync status
- Create new SA (creates corresponding Keycloak client with `sa_` prefix)
- Edit SA scopes
- Rotate SA secret (displayed once after generation)
- Delete SA
- Sync with Keycloak (manual trigger)

![Admin UI: Service Accounts](images/ui-service-accounts.png)
*Figure 6.9 — Service Accounts management*

### 6.7 Settings Page

Configure Admin Module runtime settings (admin role required):

- Prometheus URL and enable/disable toggle
- Query timeout and retention period settings

![Admin UI: Settings](images/ui-settings.png)
*Figure 6.10 — Settings page*

---

## 7. Keycloak Configuration

Artstore uses Keycloak as the Identity Provider (IdP) for all authentication and authorization. This section provides detailed configuration instructions for the `artstore` realm.

### 7.1 Realm Settings

Create a new realm named `artstore` with the following settings:

**General**:

| Setting | Value | Notes |
|---------|-------|-------|
| Realm name | `artstore` | Isolated realm for the project |
| Display name | `Artstore` | Shown on the login page |
| Login theme | `artstore` | Custom theme (see [§7.7](#77-custom-keycloak-theme)) |
| Default Signature Algorithm | `RS256` | All modules validate RS256 JWT |

![Keycloak: Realm General](images/kc-realm-general.png)
*Figure 7.1 — Realm general settings*

**Login**:

| Setting | Value | Notes |
|---------|-------|-------|
| User registration | Disabled | Users created by admin only |
| Forgot password | Disabled | Not needed for initial deployment |
| Remember me | Enabled | Convenience for Admin UI users |
| Login with email | Disabled | Login by username only |

![Keycloak: Realm Login](images/kc-realm-login.png)
*Figure 7.2 — Realm login settings*

**Sessions**:

| Setting | Value |
|---------|-------|
| SSO Session Idle | 1800 seconds (30 min) |
| SSO Session Max | 36000 seconds (10 hours) |

![Keycloak: Realm Sessions](images/kc-realm-sessions.png)
*Figure 7.3 — Realm session settings*

**Tokens**:

| Setting | Value | Notes |
|---------|-------|-------|
| Access Token Lifespan | 300 seconds (5 min) | Short-lived for security |
| Brute Force Protection | Enabled | 5 attempts, 900s permanent lockout |

![Keycloak: Realm Tokens](images/kc-realm-tokens.png)
*Figure 7.4 — Realm token settings*

### 7.2 Realm Roles and Groups

**Roles** — Create two realm roles:

| Role | Description |
|------|-------------|
| `admin` | Full access: CRUD operations, manage SE, SA, and settings |
| `readonly` | Read-only access: view files, SE, and monitoring data |

![Keycloak: Realm Roles](images/kc-realm-roles.png)
*Figure 7.5 — Realm roles*

**Groups** — Create two groups with role mappings:

| Group | Mapped Role | Purpose |
|-------|-------------|---------|
| `artstore-admins` | `admin` | Members get admin role via group membership |
| `artstore-viewers` | `readonly` | Members get readonly role via group membership |

![Keycloak: Group artstore-admins](images/kc-group-admins.png)
*Figure 7.6 — Group artstore-admins with admin role mapping*

![Keycloak: Group artstore-viewers](images/kc-group-viewers.png)
*Figure 7.7 — Group artstore-viewers with readonly role mapping*

Role assignment works via group membership. Modules map groups to roles using environment variables (e.g., `AM_ROLE_ADMIN_GROUPS=artstore-admins`).

### 7.3 Client Scopes

Client scopes define fine-grained permissions for Service Accounts. Each scope includes an `oidc-audience-mapper` to add the audience claim to the access token.

**Business scopes**:

| Scope | Description | Used By |
|-------|-------------|---------|
| `files:read` | Read file metadata and download | IM, QM, AM |
| `files:write` | Upload, update, and delete files | IM, AM |
| `storage:read` | Read SE information | IM, QM, AM |
| `storage:write` | Manage SE (sync, mode transition) | AM |
| `admin:read` | Read admin data (users, SA) | AM |
| `admin:write` | Manage users and SA | AM |

![Keycloak: Client Scopes List](images/kc-client-scopes-list.png)
*Figure 7.8 — Client scopes list*

**Example: `files:read` scope**:

![Keycloak: Scope files:read](images/kc-scope-files-read.png)
*Figure 7.9 — Scope files:read settings*

Each scope has an audience mapper that adds the scope name to the `aud` claim:

![Keycloak: Scope files:read Mappers](images/kc-scope-files-read-mappers.png)
*Figure 7.10 — Scope files:read audience mapper*

**Special scope: `groups`** — This scope uses a `group-membership-mapper` to include group names in the JWT:

| Mapper Setting | Value |
|----------------|-------|
| Mapper type | Group Membership |
| Claim name | `groups` |
| Full group path | Off (short names without `/` prefix) |

![Keycloak: Groups Mapper](images/kc-scope-groups-mapper.png)
*Figure 7.11 — Groups scope with group membership mapper*

The `groups` claim is essential for role-based authorization. Modules use it to determine user roles via group-to-role mapping.

### 7.4 Clients

Artstore uses four Keycloak clients:

#### 7.4.1 artstore-admin-module — Admin Module Service Account

| Setting | Value |
|---------|-------|
| Client type | Confidential (client-secret) |
| Authentication flow | Client Credentials |
| Service Account | Enabled |
| Standard flow | Disabled |

**Client Scopes** (assigned): `files:read`, `files:write`, `storage:read`, `storage:write`, `admin:read`, `admin:write`, `groups`

**Service Account Realm Management Roles**: `view-users`, `manage-clients`, `view-clients`, `manage-users`, `view-realm`, `query-users`, `query-clients`

These roles allow Admin Module to manage Keycloak resources: sync Service Accounts, read user information, and manage SA clients.

![Keycloak: AM Client Settings](images/kc-client-am-settings.png)
*Figure 7.12 — Admin Module client settings*

![Keycloak: AM Client Credentials](images/kc-client-am-credentials.png)
*Figure 7.13 — Admin Module client credentials*

![Keycloak: AM Client Scopes](images/kc-client-am-scopes.png)
*Figure 7.14 — Admin Module assigned scopes*

![Keycloak: AM Client SA Roles](images/kc-client-am-sa-roles.png)
*Figure 7.15 — Admin Module Service Account realm management roles*

#### 7.4.2 artstore-ingester — Ingester Module Service Account

| Setting | Value |
|---------|-------|
| Client type | Confidential (client-secret) |
| Authentication flow | Client Credentials |
| Service Account | Enabled |

**Client Scopes** (assigned): `files:read`, `files:write`, `storage:read`

Ingester Module needs to read SE information (for upload target selection), write files (upload to SE), and register files in Admin Module.

![Keycloak: IM Client Settings](images/kc-client-im-settings.png)
*Figure 7.16 — Ingester Module client settings*

![Keycloak: IM Client Scopes](images/kc-client-im-scopes.png)
*Figure 7.17 — Ingester Module assigned scopes*

#### 7.4.3 artstore-query — Query Module Service Account

| Setting | Value |
|---------|-------|
| Client type | Confidential (client-secret) |
| Authentication flow | Client Credentials |
| Service Account | Enabled |

**Client Scopes** (assigned): `files:read`, `storage:read`

Query Module needs to read file metadata (for search and location lookup) and SE information (for download proxy).

![Keycloak: QM Client Settings](images/kc-client-qm-settings.png)
*Figure 7.18 — Query Module client settings*

#### 7.4.4 artstore-admin-ui — Admin UI Browser Client

| Setting | Value |
|---------|-------|
| Client type | Public (no client secret) |
| Authentication flow | Authorization Code + PKCE (S256) |
| Standard flow | Enabled |
| Direct access grants | Disabled |
| Redirect URIs | `https://<domain>/*` |

**Default scopes**: `openid`, `profile`, `email`, `groups`

This client is used by the Admin UI for browser-based authentication. It uses PKCE to secure the authorization code flow without a client secret.

![Keycloak: UI Client Settings](images/kc-client-ui-settings.png)
*Figure 7.19 — Admin UI client settings*

![Keycloak: UI Client Scopes](images/kc-client-ui-scopes.png)
*Figure 7.20 — Admin UI assigned scopes*

### 7.5 The client_id Mapper (Critical)

> **This is a critical configuration element. Without it, inter-service authorization will not work.**

All Service Account clients (`artstore-admin-module`, `artstore-ingester`, `artstore-query`) **must** have a protocol mapper that adds the `client_id` claim to the access token.

| Mapper Setting | Value |
|----------------|-------|
| Name | `client_id` |
| Mapper type | User Session Note |
| User Session Note | `client_id` |
| Token Claim Name | `client_id` |
| Claim JSON type | String |
| Add to access token | On |
| Add to ID token | Off |

![Keycloak: client_id Mapper](images/kc-mapper-client-id.png)
*Figure 7.21 — The client_id mapper configuration*

**Why is this needed?** — Admin Module uses the `client_id` claim in SA tokens to identify which Service Account is making the request. This claim is matched against the `service_accounts` table to determine the SA's permissions (scopes). Without this mapper, the access token will not contain `client_id`, and Admin Module will reject SA requests.

**How to add**: In each SA client → Client Scopes → Dedicated scope → Add mapper → By configuration → User Session Note → configure as shown above.

### 7.6 Two-URL Pattern for Kubernetes

In Kubernetes deployments, Keycloak typically has two URLs:

| URL Type | Protocol | Purpose | Example |
|----------|----------|---------|---------|
| Internal | HTTP | Module-to-KC requests (JWKS, token endpoint) | `http://keycloak.artstore.svc:8080` |
| External | HTTPS | JWT issuer (`iss` claim), browser authentication | `https://artstore.example.com` |

**Configuration**:
- JWT `iss` claim must match the **external** URL (what the client application sees)
- `JWKS_URL` and `TOKEN_URL` in module config should use the **internal** URL (for performance, avoiding TLS overhead and external routing)
- Admin UI redirect uses the **external** URL (browser navigates to Keycloak)

Example for Ingester Module:
```
IM_JWT_ISSUER=https://artstore.example.com/realms/artstore
IM_JWKS_URL=http://keycloak.artstore.svc:8080/realms/artstore/protocol/openid-connect/certs
IM_TOKEN_URL=http://keycloak.artstore.svc:8080/realms/artstore/protocol/openid-connect/token
```

### 7.7 Custom Keycloak Theme

Artstore includes a custom Keycloak login theme (`artstore`) that matches the Admin UI design:

- CSS-only customization (parent: `keycloak.v2`)
- Dark green color scheme: accent `#22c55e`, background `#0a0f0a`
- Fonts: Inter (main), JetBrains Mono (monospace)
- Localization: English and Russian
- Docker image: `harbor.kryukov.lan/library/keycloak-artstore:v26.1-1`
- Source: `deploy/keycloak/Dockerfile`

To use the custom theme, deploy the pre-built Keycloak image and set the login theme to `artstore` in realm settings.

### 7.8 Creating a New Client (Step by Step)

To create a new Service Account client (e.g., for a custom integration):

**Step 1: General Settings**

- Click "Create client" in the Clients section
- Enter Client ID (e.g., `sa_my-integration`)
- Enter a description

![Keycloak: Wizard Step 1](images/kc-wizard-step1-general.png)
*Figure 7.22 — Create Client wizard: General settings*

**Step 2: Capability Configuration**

- Enable "Client authentication" (confidential)
- Enable "Service accounts roles"
- Disable "Standard flow" and "Direct access grants"

![Keycloak: Wizard Step 2](images/kc-wizard-step2-capability.png)
*Figure 7.23 — Create Client wizard: Capability configuration*

**Step 3: Login Settings**

- Leave redirect URIs empty (not needed for Client Credentials flow)
- Click "Save"

![Keycloak: Wizard Step 3](images/kc-wizard-step3-login.png)
*Figure 7.24 — Create Client wizard: Login settings*

**Step 4: Assign Client Scopes**

- Go to the new client → "Client scopes" tab
- Add the required scopes (e.g., `files:read`, `storage:read`)

**Step 5: Add the client_id Mapper**

- Go to "Client scopes" → client's dedicated scope
- Click "Add mapper" → "By configuration"
- Select "User Session Note"
- Configure as described in [§7.5](#75-the-client_id-mapper-critical)

![Keycloak: Add Mapper](images/kc-add-mapper-step.png)
*Figure 7.25 — Adding a mapper to the client's dedicated scope*

**Step 6: Copy Credentials**

- Go to the "Credentials" tab
- Copy the client secret — it will be used as the Service Account password

**Step 7: Register in Admin Module**

- The SA will be auto-discovered by Admin Module if the client ID starts with the configured prefix (`AM_KEYCLOAK_SA_PREFIX`, default: `sa_`)
- Alternatively, create the SA manually via Admin UI → Access Management → Service Accounts

---

## 8. Upgrading

### 8.1 Upgrade Order

When upgrading Artstore components, follow this order to minimize downtime and avoid compatibility issues:

1. **PostgreSQL** — apply any required version upgrades (rare, typically backward-compatible)
2. **Keycloak** — upgrade and verify realm configuration is intact
3. **Admin Module** — upgrade first among application modules (applies DB migrations)
4. **Storage Elements** — upgrade one by one (rolling update in K8s)
5. **Ingester Module** — upgrade (stateless, zero-downtime with multiple replicas)
6. **Query Module** — upgrade (minimal downtime with multiple replicas)

> **Important**: Always upgrade Admin Module before IM and QM. AM applies database migrations that IM/QM may depend on.

### 8.2 Database Migrations

All migrations are applied automatically on module startup using `golang-migrate` with embedded migration files:

- **Admin Module**: creates and updates tables (`file_registry`, `storage_elements`, `service_accounts`, etc.)
- **Query Module**: creates and updates read-optimized indexes on `file_registry`

**Migration table separation**:
- AM uses `schema_migrations` (default)
- QM uses `schema_migrations_qm` (separate table to avoid conflicts)

No manual migration steps are required. Simply deploy the new version — migrations run automatically during startup.

**Rollback**: If a migration fails, the module will not start. Check logs for the specific migration error. Migrations are designed to be forward-compatible; rollback requires restoring a database backup.

### 8.3 Version Compatibility

Artstore follows semantic versioning (`0.Y.Z` during development):

- **Patch versions** (`0.1.0` → `0.1.1`): bug fixes, backward-compatible. Safe to upgrade without coordination
- **Minor versions** (`0.1.0` → `0.2.0`): new features, may include DB migrations. Upgrade AM first, then other modules
- **Major versions** (`0.x.y` → `1.0.0`): potentially breaking changes. Follow release notes

**Inter-module compatibility**: All modules within the same minor version are compatible. When upgrading across minor versions, always upgrade AM first to apply any schema changes.

### 8.4 Backup and Recovery

**PostgreSQL**:
- Use `pg_dump` for logical backups
- Use `VolumeSnapshot` for PVC-based backups in Kubernetes
- Schedule regular backups (recommended: daily full + hourly WAL archiving)

**Keycloak**:
- Export realm configuration: `kcadm.sh export --realm artstore`
- The realm JSON includes: settings, clients, groups, roles, scopes (not user passwords)
- Store realm export in version control for reproducible deployments

**Storage Elements**:
- Data directory contains files and `*.attr.json` metadata
- Use filesystem-level backups or `VolumeSnapshot` for PVC
- In hybrid deployments, use standard server backup tools (rsync, borgbackup, etc.)

**Recovery procedure**:

1. Restore PostgreSQL from backup
2. Start Admin Module — it will apply any missing migrations
3. Service Accounts — recovered automatically from Keycloak on next sync cycle
4. File registry — recovered automatically via full sync when each SE connects
5. Local role overrides — **only recoverable from PostgreSQL backup** (not stored externally)
6. System fully operational after all SE sync completes

> **Note**: The file registry in PostgreSQL is a secondary index. If the database is lost and no backup exists, it can be completely rebuilt by triggering a manual sync for each registered Storage Element. `attr.json` files on SE are the primary source of truth.
