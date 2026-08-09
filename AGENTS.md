# AGENTS.md — gosuresql (Go client SDK)

> **Reference doc:** `DESCRIPTION.md` — read this first for what gosuresql is
> and how client applications use it.

## Purpose
This folder owns the **official Go client SDK** for SureSQL — the library a
member's application embeds to talk to their provisioned engine instance
(node 1 R/W + replicas). It turns the engine's REST API into a turnkey client:
token auth + auto-refresh, dynamic connection pooling, read/write separation,
ORM helpers, raw/parameterized SQL, schema/status, and migrations.

## Ownership
- Product tier: **client SDK** (4th project in the SureSQL family — see the
  shared terminology in `../suresql/DESCRIPTION.md` / this folder's
  `DESCRIPTION.md`)
- Consumes: the suresql engine's `/db/*` REST API (contract in
  `../suresql/API.md`). This repo must stay in lockstep with the engine's
  endpoints and response envelope (`suresql.StandardResponse`).
- The *consumer* is an external client application, not our SaaS. API
  stability and clean behavior (no log noise, no panics, clear errors) are
  product requirements.

## Local Contracts
- **Package layout**: root package is named `client` (module
  `github.com/medatechnology/gosuresql`). Do NOT create a `client/` subpackage —
  the import path for users is `client "github.com/medatechnology/gosuresql"`.
- **No stdout noise**: library code must not `fmt.Println`/`Printf` debug
  output — it lands in the client app's logs. The only exceptions are the
  `Migrate()` progress prints and the pool's connection-failure warning.
  Never print tokens or credentials anywhere.
- **Reuse over duplication**: ORM types (`orm.DBRecord`, `orm.Condition`,
  `orm.ParametereizedSQL`, `orm.TableStruct`, `orm.BasicSQLResult`) and request
  types (`suresql.QueryRequest`, `suresql.SQLRequest`, `suresql.InsertRequest`,
  `suresql.TokenTable`, `suresql.StandardResponse`) come from
  `medatechnology/simpleorm` and `medatechnology/suresql` — never redefine them
  here.
- **Read/write split stays in the pool**: writes (`/db/api/sql`, `/db/api/insert`)
  must use the write pool (leader, node 1); reads (`/db/api/query`,
  `/db/api/querysql`, `/db/api/getschema`, `/db/api/status`) use the read pool
  with leader fallback. Do not bypass the pool for individual queries.
- **Headers**: every request sends `API_KEY` and `CLIENT_ID`; token-required
  calls add `Authorization: Bearer <token>`. Matches
  `../suresql/server/middleware.go`.
- **Config is env-first**: `NewClientConfig()` reads `SURESQL_*` env vars /
  `.env.client`; programmatic options only override. Keep the env variable
  names stable (documented in `DESCRIPTION.md`).
- **No engine changes here**: any endpoint change must first land in
  `../suresql` (engine + `API.md`), then be mirrored here.

## Work Guidance
- Connect flow: `NewClient(config)` → `Connect(user, pass)` (or `Connect("","")`
  for config creds) → token on leader → `InitializePool()` discovers nodes from
  `/db/api/status` and seeds read/write pools.
- **Leader URL is always `Config.ServerURL`**, never `status.URL` — a local-dev
  engine advertises `0.0.0.0:8080` (unreachable from the client). Only peer
  (nodes 2..N) URLs come from status. See `pool.go InitializePool`.
- **Lifecycle**: always `defer client.Close()`. `Close()` sends `/db/api/disconnect`
  for the leader token and every pool connection token (best-effort, synchronous)
  so the engine frees its pool slots immediately, then clears local pools. The
  engine endpoint lives in `../suresql/server/handler.go` (`HandleDisconnect`).
- Token lifecycle: `/db/refresh` with the refresh token first; on failure
  reconnect via `/db/connect`. Retry-once semantics live in
  `request.go`/`connection.go`.
- Adding a new engine call: add the endpoint constant + typed request in
  `suresql.go` (or the right file), route through `sendRequest[T]` with the
  correct `IS_READ`/`IS_WRITE` + `FALLBACK_LEADER` flags, and document it in
  `README.md` API reference.
- Keep the README's Quick Start executable: any new required step (e.g. a new
  method that must be called) must appear in the Quick Start too.
- The stress/bench tool no longer lives here — it moved to
  `../suresqlctl/cmd/suresqlbench` (the old `gosuresql/bench` deadlocked above
  4096 ops on a full latency channel; the rewrite drains concurrently).
  `suresqlc` (sqlite3-style client) also lives in `../suresqlctl/cmd/suresqlc`
  and reuses this SDK.

## Verification
- `go build ./...` and `go vet ./...` from this folder must pass.
- `grep -n "fmt.Println\|fmt.Printf" *.go` must only show the allowed
  exceptions (migration progress + pool warning).
- Manual smoke test against a local engine: `cd app/test && go run .` with
  `app/test/.env.client` pointing at a running suresql instance.
- After changing request/response shapes, confirm the README examples still
  compile (they are the client's documentation).

## Child DOX Index
No child AGENTS.md files — this is a flat library.
