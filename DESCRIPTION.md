# DESCRIPTION — gosuresql (Go client SDK)

> **Read this first.** Everything about how client applications talk to a
> SureSQL instance through gosuresql. Details live in `AGENTS.md`,
> `ARCHITECTURE.md`, `README.md`, `SCALING.md`.

## Shared Terminology (use these everywhere)

> These terms are identical across the four project description files
> (`suresql/DESCRIPTION.md`, `suresqlctl/DESCRIPTION.md`,
> `suresql-saas/DESCRIPTION.md`, `gosuresql/DESCRIPTION.md`).

| Term | Meaning |
|------|---------|
| **SureSQL** | The product: managed SQL databases (Supabase-like), sold via the SaaS |
| **engine** | The suresql repo — one containerized DB gateway (RQLite/PostgreSQL) |
| **cloudNode** | A VPS in *our* fleet that hosts customer instances (managed by suresqlctl; has WireGuard + Caddy + daemon) |
| **controller** | The suresqlctl instance on the *dashboard VPS* — mesh hub: registry, placement, routing, DNS, audit. Hosts no instances, no Caddy |
| **instance** | One customer's database product = 1..N *member nodes* of the engine |
| **member node** | Customer-POV node number (1..N, per instance, NOT a cloudNode). Node 1 = R/W, 2..N = read-only replicas. Each member node lives on a *different* cloudNode |
| **member** | A customer of the SaaS |
| **SaaS / dashboard** | The suresql-saas web product members use; runs on the dashboard VPS next to the controller |
| **client / SDK** | This repo — `gosuresql`, the official Go client applications use to talk to an instance |

## What It Is

`gosuresql` is the **official Go client SDK** for SureSQL. A member's
application embeds it to talk to their provisioned instance over HTTPS/REST —
no direct database driver, no credentials in the app beyond the instance's own
API key / client ID / user credentials.

It provides:

- **Authentication**: API key + client ID headers plus JWT access/refresh token
  flow (`/db/connect`, `/db/refresh`) — fully automatic
- **Connection pool**: dynamic, per-node pools with round-robin; scales up/down
  with traffic
- **Read/Write separation**: writes go to node 1 (R/W leader), reads are
  distributed across read replicas (nodes 2..N) with automatic leader fallback
- **Token lifecycle**: auto-refresh on expiry, auto-reconnect if refresh fails
- **ORM-style helpers**: `SelectOne` / `SelectMany` / `InsertOne` /
  `InsertMany` over `orm.DBRecord` and `orm.TableStruct`
- **Raw SQL**: single, batched, and parameterized queries + statements
- **Schema & cluster status**: `GetSchema`, `Status`, `Leader`, `Peers`
- **Migrations**: run schema SQL files from a directory on connect
- **Metrics**: pool/connection health for monitoring

```
┌──────────────┐  HTTPS/REST    ┌───────────────────┐   SQL    ┌──────────┐
│  App (Go)    │ ─────────────▶ │  gosuresql (SDK)  │ ───────▶ │ SureSQL  │
│  (member's   │ ◀───────────── │  token + pool +   │ ◀─────── │  engine  │
│  application)│   response     │  read/write split │          │ (node 1) │
└──────────────┘                └───────────────────┘          └────┬─────┘
                                                                   │ replicas
                                                             ┌─────▼─────┐
                                                             │ nodes 2..N│
                                                             └───────────┘
```

## Where It Sits in the Product

| Tier | Project | Role |
|------|---------|------|
| Engine | **suresql** | One containerized DB gateway per member node (RQLite/PostgreSQL) |
| Controller | **suresqlctl** | Provisions + manages engine instances on the fleet, mesh, DNS |
| SaaS | **suresql-saas** | Customer dashboard; talks to the controller |
| **Client SDK** | **gosuresql (this repo)** | What the member's application uses to query their instance |

A member creates one *instance* = 1..N engine nodes (1 R/W + N−1 read
replicas), each on a different cloudNode with its own domain. The SaaS hands
the member the connection details; the member puts them in `gosuresql`.

## Connecting to an Instance (the member's point of view)

1. The member creates an instance in the **SaaS dashboard** (or via the
   controller API): picks size (`limit_mb`), node count, initial SQL structure
   (migrations), and their own `admin_user` / `admin_password`.
2. The SaaS (or the instance page) provides the member with:
   - **Server URL**: `https://{app}-{slug}-{node}.{domain}` (node 1 is R/W)
   - **API key** + **client ID**: the instance's API credentials (headers)
   - **Username / password**: the member's `admin_user` / `admin_password`
3. The member configures `gosuresql` with those values — either via
   environment variables / `.env.client` (recommended) or an explicit
   `client.ClientConfig`.

```
ServerURL   = https://blog-a1b2c3-1.suresql.app   ← node 1 (read/write)
APIKey      = <instance API key>
ClientID    = <instance client ID>
Username    = admin                                ← member's admin_user
Password    = <member's admin_password>
```

`gosuresql` handles the rest: `Connect` → token → pool → queries → auto-refresh
→ reconnect, with reads spread across replicas and writes pinned to the leader.

> **Credential tiers — what the client touches:** the SDK only ever uses the
> member-facing pair (API key + client ID headers, plus the member's
> `admin_user`/`admin_password` — credential pair 4 of the instance's model).
> The engine maps that session to its OWN rqlite connection for the query —
> rqlite user B (`rqlite_app`), never the internal admin user A
> (`rqlite_admin`) and never the internal-API credentials — so the client never
> sees or holds any rqlite/internal credential.

## Connection Model (leader, pool, lifecycle)

- **Leader**: the node the client was configured with (`ServerURL`) — node 1
  (read/write). All writes go here, and it is the fallback for reads.
  ⚠️ The leader connection is always built from `Config.ServerURL`, NEVER from
  the engine's advertised `status.URL` (a local-dev engine advertises
  `0.0.0.0:8080`, unreachable from the client). Peers (nodes 2..N) come from
  status.
- **Read pool**: `SURESQL_POOL_MAXIMUM` connections (default 10) across the
  leader + read replicas, round-robin, with leader fallback on failure.
- **Write pool**: `SURESQL_WRITE_POOL_MAXIMUM` connections (default 1) pinned
  to the leader for write atomicity. AUDIT VERIFIED: the pool is leader-only —
  peers are NEVER scaled into the write pool. A write sent to a replica makes
  rqlite 301-redirect to the leader, and Go's http.Client rewrites a 301 on
  POST to GET, failing the write; leader-only pinning prevents that.
- **Scale**: pools start at `SURESQL_SCALE_UP_BATCH` (default 3) connections
  and scale up under load (`SURESQL_SCALE_UP_THRESHOLD`, default 10 active
  requests), idle-scale down via `SURESQL_POOL_IDLE_TIMEOUT`.
- **Lifecycle**: `Connect(username, password)` → token + pool; every operation
  auto-refreshes the token; `Close()` sends `/db/api/disconnect` for every
  token it holds so the engine frees its pool slots immediately, then clears
  local state. Always defer `Close()` (short-lived CLI clients otherwise leak
  engine slots until the 24h token TTL).
- **Response decoding**: the envelope's `data` is returned as raw JSON and
  unmarshaled ONCE into the concrete type (`sendRequest[T]`,
  `getStatusWithoutLock`, `GetSchema`, token conversion) — no intermediate
  `map[string]interface{}` + re-marshal round trip per call.
- **Known limitation**: `GetSchema()` calls `/db/api/getschema`, which the
  engine intentionally does NOT expose publicly (schema is available via the
  internal `/suresql/schema` with basic auth). `GetSchema()` therefore returns
  an empty slice today; the SDK method exists for when/if a public schema
  endpoint is added.

## Configuration Surface

Environment variables (loaded from `.env.client` or the process env by
`client.NewClientConfig()`):

| Variable | Default | Meaning |
|----------|---------|---------|
| `SURESQL_SERVER_URL` | `http://localhost:8080` | Instance node-1 URL |
| `SURESQL_API_KEY` | `development_api_key` | Instance API key (`API_KEY` header) |
| `SURESQL_CLIENT_ID` | `development_client_id` | Instance client ID (`CLIENT_ID` header) |
| `SURESQL_USERNAME` | `admin` | Member user (`admin_user`) |
| `SURESQL_PASSWORD` | `admin` | Member password (`admin_password`) |
| `SURESQL_HTTP_*` | defaults | HTTP client tuning (timeouts, keep-alive, max conns) |
| `SURESQL_POOL_*` | defaults | Pool sizing (min/max, scale-up threshold/batch, TTL, idle) |
| `SURESQL_WRITE_POOL_MAXIMUM` | `1` | Max write (leader) connections — usually 1 for atomicity |
| `SURESQL_NODE_USE_MULTI_CLIENT` | `false` | One shared HTTP client per node vs one per connection |

The same values can be set programmatically via
`client.ClientConfig{...}` and `client.NewPoolConfig(...)` /
`client.NewHTTPClientConfig(...)` with their `With*` options.

## Reuse Standard (MANDATORY for all future tasks)

> Same rule as the engine: generic logic lives in the shared libraries and is
> CALLED from here, never re-implemented.

1. **Env / parsing**: `goutil` (`utils.GetEnv*`, `utils.ShortText`); no custom
   env readers.
2. **SQL command splitting**: `simpleorm.ConvertSQLCommands` (quote-aware) —
   never `strings.Split` on `;`.
3. **ORM types**: `simpleorm` (records, conditions, results) and
   `medatechnology/suresql` (request/response models) — do not define parallel
   types.
4. **Routing**: writes → leader (node 1) via the leader-only write pool; reads
   → round-robin across the read pool. Do not add peers to the write pool.
5. **No per-request goroutines**: stats updates are inline mutex ops; only the
   deferred scale-up spawns a goroutine.
6. **Module wiring**: `go.mod` carries `replace` → `../goutil`,
   `../simpleorm`, `../suresql` (monorepo dev, mirror of suresqlctl).

## Repo Facts

- GitHub: `github.com/medatechnology/gosuresql` (branch `main`)
- Go module `github.com/medatechnology/gosuresql`; root package name is `client`
- Dependencies: `medatechnology/simpleorm` (ORM types), `medatechnology/suresql`
  (request/response types), `medatechnology/goutil` (utils)
- Verification: `go build ./...`, `go vet ./...`
- Test app: `app/test/main.go` (connect, CRUD, struct ops, load test) with
  `app/test/.env.client`
