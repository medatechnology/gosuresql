# GoSureSQL

<div align="center">

[![Go Reference](https://pkg.go.dev/badge/github.com/medatechnology/gosuresql.svg)](https://pkg.go.dev/github.com/medatechnology/gosuresql)
[![Go Report Card](https://goreportcard.com/badge/github.com/medatechnology/gosuresql)](https://goreportcard.com/report/github.com/medatechnology/gosuresql)
[![License: MIT](https://img.shields.io/badge/License-MIT-yellow.svg)](https://opensource.org/licenses/MIT)

*The official Go client for SureSQL — managed SQL databases (Supabase-like)*

</div>

---

## 📋 Overview

GoSureSQL (`gosuresql`) is the official Go client SDK for **SureSQL**, a
secure database middleware gateway. Your application talks to your SureSQL
instance over HTTPS/REST — the SDK handles authentication, token refresh,
connection pooling, read/write separation, and high availability, so you can
focus on your application logic.

### Key Features

- **🔌 Automatic Auth**: API key + client ID headers, JWT access/refresh tokens
- **🔄 Dynamic Connection Pool**: Automatically scales with traffic demands
- **⚡ Read/Write Separation**: Writes go to node 1 (leader), reads spread across replicas
- **♻️ Token Management**: Handles token refresh and reconnection automatically
- **🔍 ORM + SQL**: Type-safe helpers plus raw, batched, and parameterized SQL
- **📊 Detailed Metrics**: Real-time insights into connection usage and performance
- **🛠️ Highly Configurable**: Customize scaling behavior to your environment

## 📑 Table of Contents

- [Installation](#-installation)
- [Quick Start](#-quick-start)
- [Connecting to Your SureSQL Instance](#-connecting-to-your-suresql-instance)
- [Configuration](#-configuration)
- [Capacity & Tuning Guidelines](#-capacity--tuning-guidelines)
- [Connection Management](#-connection-management)
- [API Reference](#-api-reference)
- [Migrations](#-migrations)
- [Monitoring](#-monitoring)
- [Connection Pool Scaling](#-connection-pool-scaling)
- [Architecture](#-architecture)
- [License](#-license)

## 📦 Installation

```bash
go get github.com/medatechnology/gosuresql
```

The module's root package is named `client`. Import it like this:

```go
import client "github.com/medatechnology/gosuresql"
```

## 🚀 Quick Start

> **Important:** after creating the client you must call `Connect(...)` — the
> pool and auth token are initialized there. Querying without connecting fails
> with *"authentication required but no token available"*.

### Style A — Environment driven (recommended)

Put your instance credentials in a `.env.client` file (or export them), then
`NewClientConfig()` reads them automatically:

```go
package main

import (
	"fmt"
	"log"

	client "github.com/medatechnology/gosuresql"
)

func main() {
	// Reads SURESQL_* env vars / .env.client (see "Configuration" below)
	c, err := client.NewClient(client.NewClientConfig())
	if err != nil {
		log.Fatalf("failed to create client: %v", err)
	}
	defer c.Close()

	// Uses SURESQL_USERNAME / SURESQL_PASSWORD from the config.
	// You can also pass explicit credentials: c.Connect("admin", "admin123")
	if err := c.Connect("", ""); err != nil {
		log.Fatalf("failed to connect: %v", err)
	}

	records, err := c.SelectOneSQL("SELECT * FROM users LIMIT 10")
	if err != nil {
		log.Fatalf("query failed: %v", err)
	}
	for _, r := range records {
		fmt.Printf("User ID: %v, Name: %v\n", r.Data["id"], r.Data["name"])
	}
}
```

### Style B — Explicit configuration

```go
package main

import (
	"fmt"
	"log"
	"time"

	client "github.com/medatechnology/gosuresql"
	orm "github.com/medatechnology/simpleorm"
)

func main() {
	c, err := client.NewClient(client.ClientConfig{
		ServerURL:   "https://blog-a1b2c3-1.suresql.app",
		APIKey:      "your-api-key",
		ClientID:    "your-client-id",
		Username:    "admin",
		Password:    "admin123",
		HTTPTimeout: 30 * time.Second,
	})
	if err != nil {
		log.Fatalf("failed to create client: %v", err)
	}
	defer c.Close()

	if err := c.Connect("admin", "admin123"); err != nil {
		log.Fatalf("failed to connect: %v", err)
	}

	users, err := c.SelectManyWithCondition("users", &orm.Condition{
		Field:    "status",
		Operator: "=",
		Value:    "active",
	})
	if err != nil {
		log.Fatalf("query failed: %v", err)
	}
	fmt.Printf("Found %d active users\n", len(users))
}
```

## 🔌 Connecting to Your SureSQL Instance

Your instance is created in the **SureSQL SaaS dashboard** (or via the
controller API). The dashboard gives you everything the SDK needs:

| You get | Where it goes | Example |
|---------|---------------|---------|
| **Server URL** (node 1 = read/write) | `SURESQL_SERVER_URL` / `ServerURL` | `https://blog-a1b2c3-1.suresql.app` |
| **API key** | `SURESQL_API_KEY` / `APIKey` | sent as the `API_KEY` header |
| **Client ID** | `SURESQL_CLIENT_ID` / `ClientID` | sent as the `CLIENT_ID` header |
| **Username** (`admin_user`) | `SURESQL_USERNAME` / `Username` | `admin` |
| **Password** (`admin_password`) | `SURESQL_PASSWORD` / `Password` | your own secret |

Notes:

- The **Server URL is node 1** of your instance (read/write leader). The SDK
  discovers read replicas (nodes 2..N) automatically from `/db/api/status` and
  routes reads across them.
- The **API key / client ID / user credentials are unique to your instance** —
  they never leave your application and are never logged.
- If you only have one node, everything still works — reads simply go to the
  leader.

## 🛠️ Configuration

### Environment variables (`.env.client`)

`NewClientConfig()` reads these from the process environment or a `.env.client`
file in the working directory (also loaded by the package `init`):

```bash
# Connection
SURESQL_SERVER_URL=https://blog-a1b2c3-1.suresql.app
SURESQL_API_KEY=your-api-key
SURESQL_CLIENT_ID=your-client-id
SURESQL_USERNAME=admin
SURESQL_PASSWORD=admin123

# HTTP client tuning (timeouts in seconds)
SURESQL_HTTP_TIMEOUT=30
SURESQL_HTTP_DIAL_TIMEOUT=30
SURESQL_HTTP_MAX_IDLE_CONNECTION=10
SURESQL_NODE_USE_MULTI_CLIENT=false   # true = one HTTP client per connection

# Pool settings (timeouts in minutes)
SURESQL_POOL_MINIMUM=5
SURESQL_POOL_MAXIMUM=20
SURESQL_WRITE_POOL_MAXIMUM=1
SURESQL_SCALE_UP_THRESHOLD=10
SURESQL_POOL_IDLE_TIMEOUT=5
SURESQL_SCALE_DOWN_INTERVAL=5
SURESQL_CONNECTION_TTL=60
SURESQL_SCALE_UP_BATCH=3
SURESQL_USAGE_WINDOW=100
```

### Programmatic configuration

```go
// Custom pool configuration
poolConfig := client.NewPoolConfig(
	client.WithMinPoolSize(5),
	client.WithScaleUpThreshold(15),
	client.WithIdleTimeout(5 * time.Minute),
	client.WithScaleUpBatchSize(3),
	client.WithConnectionTTL(1 * time.Hour),
)

// Config options are available for every field:
config := client.NewClientConfig(
	client.WithServerURL("https://blog-a1b2c3-1.suresql.app"),
	client.WithApiKey("your-api-key"),
	client.WithClientID("your-client-id"),
	client.WithUsername("admin"),
	client.WithPassword("admin123"),
	client.WithHttpTimeout(30 * time.Second),
	client.WithPoolConfig(poolConfig),
)

c, err := client.NewClient(config)
```

## 🔄 Connection Management

GoSureSQL handles all aspects of connection management automatically:

### Token Lifecycle

- **Initial Authentication**: Obtained during `Connect()`
- **Automatic Refresh**: When a token expires during operations (`/db/refresh`)
- **Reconnection**: If refresh fails, automatically reconnects (`/db/connect`)
- **No Manual Management**: You never need to handle tokens directly

### Connection Pooling

- **Adaptive Scaling**: Pool grows during high traffic, shrinks during idle periods
- **Multiple Nodes**: Separate pools for the leader (writes) and read replicas
- **Fault Tolerance**: Automatically handles node failures, with leader fallback
- **Round-Robin Distribution**: Evenly distributes requests across connections

For detailed configuration options, see [SCALING.md](SCALING.md).

## 📏 Capacity & Tuning Guidelines

What to set in your `ClientConfig` for the number of users your app serves.
These are **client-side** knobs — you own them. Engine-side settings
(`max_pool`, `token_exp`, `log_level`, quota) are managed by the platform
(admin dashboard), not by SDK credentials.

### The model

- One SDK connection = one engine pool slot. The client pool **auto-scales**
  under load up to `MaxPoolSize`; each connection cycles requests in ~1–5 ms,
  so one connection serves ~200–1,000 req/s. Size the pool for **peak concurrent
  in-flight requests**, not total QPS.
- Your app shares the instance's engine pool with your other apps. Rule of
  thumb: **client `MaxPoolSize` ≈ your app's peak concurrent requests ÷ 2,
  and never more than ~½ the instance `max_pool`** (engine default 25; raise it
  via the platform dashboard if your tier needs more).
- Keep `MaxPoolSize` ≥ `MinPoolSize` ≥ 1; the pool scales down when idle.

### Recommended values by tier

| Tier | Users (your app) | Peak concurrent reqs | `MaxPoolSize` | HTTP timeout | Dial timeout | Connection TTL |
|---|---|---|---|---|---|---|
| Starter | ≤ 1K | ≤ 50 | 5–10 | 10s | 3s | 1h |
| Growth | 1K–10K | 50–500 | 20–50 | 10s | 3s | 1h |
| Pro | 10K–100K | 500–5K | 100–200 | 10–30s | 3s | 1h |
| Scale | 100K–1M | 5K–50K | 200–500 | 10–30s | 5s | 1h |
| Enterprise | 1M+ | 50K+ | ≤ engine pool share | 10–30s | 5s | 1h |

Notes:

- **HTTP timeout 10–30 s** beats the 60 s default — OLTP requests take 1–50 ms;
  a 60 s timeout hides failures. Slow exports/batch jobs should use their own
  longer-timeout client.
- **Connection TTL 1 h** rotates tokens and connections, spreading load and
  bounding how long a stuck connection holds a slot.
- **Writes** go to the leader and are serialized by the database (raft commit):
  keep `MaxWritePoolSize` small (default 1) — more write connections do not
  make rqlite commit faster.
- If reads matter, ask for **read replicas** (multi-node instance): the SDK
  discovers them from `/db/api/status` automatically and round-robins reads.
  Single-node read throughput caps around ~1.4k ops/s.

### Example — Growth tier (10K users, ~500 concurrent)

```go
poolConfig := client.NewPoolConfig(
	client.WithMinPoolSize(10),
	client.WithMaxPoolSize(50),
	client.WithMaxWritePoolSize(1),
	client.WithScaleUpThreshold(10),
	client.WithIdleTimeout(5*time.Minute),
	client.WithScaleDownInterval(5*time.Minute),
	client.WithConnectionTTL(1*time.Hour),
)

config := client.NewClientConfig(
	client.WithServerURL("https://blog-a1b2c3-1.suresql.app"),
	client.WithApiKey("your-api-key"),
	client.WithClientID("your-client-id"),
	client.WithUsername("admin"),
	client.WithPassword("admin123"),
	client.WithHttpTimeout(10*time.Second),
	client.WithPoolConfig(poolConfig),
)

c, err := client.NewClient(config)
defer c.Close() // frees the engine pool slots immediately
```

Always `defer c.Close()` — an unclosed client holds its engine pool slots until
the token expires.

## 📚 API Reference

### Connection Management

#### `Connect(username, password string) error`

Establishes a connection to the SureSQL server using the provided credentials.
Pass empty strings to use the credentials from your config:

- Creates the initial token for authentication
- Initializes the connection pool with `MinPoolSize` connections
- Discovers and connects to peer nodes

```go
err := c.Connect("admin", "admin123") // or c.Connect("", "") to use config
if err != nil {
	log.Fatalf("Failed to connect: %v", err)
}
```

#### `IsConnected() bool`

Verifies if the client is connected to the server and has a leader connection.

```go
if c.IsConnected() {
	fmt.Println("Connected to SureSQL server")
} else {
	fmt.Println("Not connected")
}
```

#### `Close()`

Properly shuts down the client, closing all connections and cleaning up resources.

```go
defer c.Close()
```

### Basic Queries

#### `SelectOne(tableName string) (orm.DBRecord, error)`

Retrieves a single record from a table. Useful for single-item lookups.

```go
settings, err := c.SelectOne("app_settings")
if err != nil {
	log.Fatal(err)
}
fmt.Printf("App Name: %v\n", settings.Data["app_name"])
```

#### `SelectMany(tableName string) (orm.DBRecords, error)`

Retrieves all records from a table. Use with caution on large tables.

```go
categories, err := c.SelectMany("categories")
if err != nil {
	log.Fatal(err)
}
fmt.Printf("Found %d categories\n", len(categories))
```

### Conditional Queries

#### `SelectOneWithCondition(tableName string, condition *orm.Condition) (orm.DBRecord, error)`

Retrieves a single record matching specific criteria.

```go
user, err := c.SelectOneWithCondition("users", &orm.Condition{
	Field:    "id",
	Operator: "=",
	Value:    42,
})
if err != nil {
	log.Fatal(err)
}
fmt.Printf("User: %v %v\n", user.Data["first_name"], user.Data["last_name"])
```

#### `SelectManyWithCondition(tableName string, condition *orm.Condition) ([]orm.DBRecord, error)`

Retrieves multiple records matching specific criteria.

```go
// Complex nested condition: WHERE (age > 23 AND (location = 'surabaya' OR job = 'teacher'))
condition := &orm.Condition{
	Logic: "AND",
	Nested: []orm.Condition{
		{Field: "age", Operator: ">", Value: 23},
		{
			Logic: "OR",
			Nested: []orm.Condition{
				{Field: "location", Operator: "=", Value: "surabaya"},
				{Field: "job", Operator: "=", Value: "teacher"},
			},
		},
	},
}
users, err := c.SelectManyWithCondition("users", condition)
if err != nil {
	log.Fatal(err)
}
```

**With OrderBy, GroupBy and Pagination:**

```go
condition := &orm.Condition{
	Field:    "registration_date",
	Operator: ">=",
	Value:    "2023-01-01",
	OrderBy:  []string{"registration_date DESC", "last_name ASC"},
	GroupBy:  []string{"city"},
	Limit:    20,
	Offset:   20, // Skip first 20 records (page 1)
}
users, err := c.SelectManyWithCondition("users", condition)
if err != nil {
	log.Fatal(err)
}
```

### SQL Queries

#### `SelectOneSQL(sql string) (orm.DBRecords, error)`

Executes a raw SQL query that can return multiple rows.

```go
records, err := c.SelectOneSQL(`
	SELECT users.id, users.name, departments.name as dept_name
	FROM users
	JOIN departments ON users.department_id = departments.id
	WHERE users.status = 'active'
	ORDER BY users.last_login DESC
	LIMIT 10
`)
if err != nil {
	log.Fatal(err)
}
for _, record := range records {
	fmt.Printf("User: %v, Department: %v\n", record.Data["name"], record.Data["dept_name"])
}
```

#### `SelectManySQL(sqlStatements []string) ([]orm.DBRecords, error)`

Executes multiple SQL queries in a single request.

```go
queries := []string{
	"SELECT id, name FROM users LIMIT 5",
	"SELECT id, title FROM posts WHERE status = 'published' LIMIT 5",
	"SELECT COUNT(*) as count FROM comments",
}
resultSets, err := c.SelectManySQL(queries)
if err != nil {
	log.Fatal(err)
}
fmt.Printf("Comment count: %v\n", resultSets[2][0].Data["count"])
```

#### `SelectOnlyOneSQL(sql string) (orm.DBRecord, error)`

Executes a SQL query that should return exactly one row. Errors if multiple
rows would be returned.

```go
record, err := c.SelectOnlyOneSQL("SELECT * FROM users WHERE id = 42")
if err != nil {
	log.Fatal(err)
}
fmt.Printf("User name: %v\n", record.Data["name"])
```

### Parameterized SQL Queries

> Parameterized queries protect against SQL injection. Use `?` placeholders.

#### `SelectOneSQLParameterized(paramSQL orm.ParametereizedSQL) (orm.DBRecords, error)`

```go
query := orm.ParametereizedSQL{
	Query:  "SELECT * FROM users WHERE role = ? AND join_date > ? ORDER BY join_date DESC",
	Values: []interface{}{"admin", "2023-01-01"},
}
records, err := c.SelectOneSQLParameterized(query)
if err != nil {
	log.Fatal(err)
}
```

#### `SelectManySQLParameterized(paramSQLs []orm.ParametereizedSQL) ([]orm.DBRecords, error)`

```go
queries := []orm.ParametereizedSQL{
	{Query: "SELECT * FROM users WHERE department_id = ?", Values: []interface{}{5}},
	{Query: "SELECT * FROM tasks WHERE assigned_to = ? AND status = ?", Values: []interface{}{42, "pending"}},
}
resultSets, err := c.SelectManySQLParameterized(queries)
if err != nil {
	log.Fatal(err)
}
```

#### `SelectOnlyOneSQLParameterized(paramSQL orm.ParametereizedSQL) (orm.DBRecord, error)`

```go
user, err := c.SelectOnlyOneSQLParameterized(orm.ParametereizedSQL{
	Query:  "SELECT * FROM users WHERE email = ?",
	Values: []interface{}{"john@example.com"},
})
if err != nil {
	log.Fatal(err)
}
fmt.Printf("Found user: %v\n", user.Data["name"])
```

### SQL Execution

#### `ExecOneSQL(sql string) orm.BasicSQLResult`

Executes a single SQL statement that doesn't return records (INSERT, UPDATE, DELETE...).

```go
result := c.ExecOneSQL("UPDATE users SET status = 'inactive' WHERE last_login < '2023-01-01'")
if result.Error != nil {
	log.Fatalf("Error: %v", result.Error)
}
fmt.Printf("Updated %d users to inactive status\n", result.RowsAffected)
```

#### `ExecOneSQLParameterized(paramSQL orm.ParametereizedSQL) orm.BasicSQLResult`

```go
result := c.ExecOneSQLParameterized(orm.ParametereizedSQL{
	Query:  "UPDATE users SET status = ? WHERE id = ?",
	Values: []interface{}{"suspended", 42},
})
if result.Error != nil {
	log.Fatal(result.Error)
}
```

#### `ExecManySQL(sqlStatements []string) ([]orm.BasicSQLResult, error)`

```go
statements := []string{
	"DELETE FROM sessions WHERE expires < NOW()",
	"UPDATE statistics SET value = 0 WHERE period = 'daily'",
}
results, err := c.ExecManySQL(statements)
if err != nil {
	log.Fatal(err)
}
for i, result := range results {
	fmt.Printf("Statement %d affected %d rows\n", i, result.RowsAffected)
}
```

#### `ExecManySQLParameterized(paramSQLs []orm.ParametereizedSQL) ([]orm.BasicSQLResult, error)`

```go
statements := []orm.ParametereizedSQL{
	{Query: "UPDATE products SET price = price * ? WHERE category_id = ?", Values: []interface{}{1.1, 5}},
	{Query: "INSERT INTO price_change_log (category_id, change_pct) VALUES (?, ?)", Values: []interface{}{5, 10.0}},
}
results, err := c.ExecManySQLParameterized(statements)
if err != nil {
	log.Fatal(err)
}
```

### Insert Operations

#### `InsertOneDBRecord(record orm.DBRecord, queue bool) orm.BasicSQLResult`

```go
user := orm.DBRecord{
	TableName: "users",
	Data: map[string]interface{}{
		"name":    "Jane Smith",
		"email":   "jane@example.com",
		"role":    "user",
		"status":  "active",
		"created": time.Now(),
	},
}
result := c.InsertOneDBRecord(user, false)
if result.Error != nil {
	log.Fatal(result.Error)
}
fmt.Printf("Created new user with ID: %d\n", result.LastInsertID)
```

#### `InsertManyDBRecords(records []orm.DBRecord, queue bool) ([]orm.BasicSQLResult, error)`

Inserts multiple records, potentially into different tables.

```go
records := []orm.DBRecord{
	{TableName: "users", Data: map[string]interface{}{"name": "John Doe", "email": "john@example.com"}},
	{TableName: "user_preferences", Data: map[string]interface{}{"user_id": 1, "theme": "dark"}},
}
results, err := c.InsertManyDBRecords(records, false)
if err != nil {
	log.Fatal(err)
}
```

#### `InsertManyDBRecordsSameTable(records []orm.DBRecord, queue bool) ([]orm.BasicSQLResult, error)`

Batch-inserts multiple records into the same table (more efficient).

```go
products := []orm.DBRecord{
	{TableName: "products", Data: map[string]interface{}{"name": "Smartphone X", "category_id": 1, "price": 599.99, "stock": 50}},
	{TableName: "products", Data: map[string]interface{}{"name": "Tablet Pro", "category_id": 1, "price": 799.99, "stock": 30}},
}
results, err := c.InsertManyDBRecordsSameTable(products, false)
if err != nil {
	log.Fatal(err)
}
```

### TableStruct Operations

Define a struct that implements `orm.TableStruct` (a `TableName() string`
method + `db` field tags) and insert it directly.

```go
type User struct {
	ID     int    `db:"id"`
	Name   string `db:"name"`
	Email  string `db:"email"`
	Status string `db:"status"`
}

func (u User) TableName() string { return "users" }
```

#### `InsertOneTableStruct(record orm.TableStruct, queue bool) orm.BasicSQLResult`

```go
newUser := User{Name: "Alice Brown", Email: "alice@example.com", Status: "active"}
result := c.InsertOneTableStruct(newUser, false)
if result.Error != nil {
	log.Fatal(result.Error)
}
fmt.Printf("Created new user with ID: %d\n", result.LastInsertID)
```

#### `InsertManyTableStructs(records []orm.TableStruct, queue bool) ([]orm.BasicSQLResult, error)`

Inserts structs that may belong to different tables.

```go
results, err := c.InsertManyTableStructs([]orm.TableStruct{
	Product{Name: "Gaming Laptop", Price: 1299.99},
	Category{Name: "Electronics"},
}, false)
if err != nil {
	log.Fatal(err)
}
```

### Schema & Status Methods

#### `GetSchema(hideSQL bool, hideSureSQL bool) []orm.SchemaStruct`

Retrieves the database schema (tables, views, indices). Use `hideSureSQL=true`
to skip internal `_`-prefixed tables.

```go
schema := c.GetSchema(false, true)
for _, item := range schema {
	if item.ObjectType == "table" {
		fmt.Printf("- %s\n", item.TableName)
	}
}
```

#### `Status() (orm.NodeStatusStruct, error)`

Gets detailed status about the database cluster: leader, peers, version, uptime.

```go
status, err := c.Status()
if err != nil {
	log.Fatal(err)
}
fmt.Printf("DB Version: %s\n", status.Version)
fmt.Printf("Leader: %s\n", status.Leader)
for id, peer := range status.Peers {
	fmt.Printf("- Node %d: %s (Mode: %s)\n", id, peer.URL, peer.Mode)
}
```

#### `Leader() (string, error)`

```go
leader, err := c.Leader()
if err != nil {
	log.Fatal(err)
}
fmt.Printf("Current leader node: %s\n", leader)
```

#### `Peers() ([]string, error)`

```go
peers, err := c.Peers()
if err != nil {
	log.Fatal(err)
}
fmt.Printf("Cluster has %d peer nodes\n", len(peers))
```

## 🗂️ Migrations

Run schema migration files (SQL) from a directory — for example the initial
structure you provided when creating the instance:

```go
if err := c.Migrate("migrations/"); err != nil {
	log.Fatalf("migration failed: %v", err)
}
```

Each `.sql` file is executed in filename order. Progress is printed per file.
For your instance's initial structure you can also create tables directly with
`ExecOneSQL` / `ExecManySQL`.

## 📊 Monitoring

GoSureSQL provides comprehensive metrics for monitoring your connection pool:

```go
metrics := c.GetPoolMetrics()
fmt.Printf("Total connections: %d\n", metrics.TotalConnections)
fmt.Printf("Active requests: %d\n", metrics.ActiveRequests)
fmt.Printf("Requests per second: %.2f\n", metrics.RequestsPerSecond)
fmt.Printf("Scale-up events: %d\n", metrics.ScaleUpEvents)
fmt.Printf("Scale-down events: %d\n", metrics.ScaleDownEvents)

for nodeID, node := range metrics.ConnectionsPerNode {
	fmt.Printf("\nNode: %s (%s)\n", nodeID, node.URL)
	fmt.Printf("  Connections: %d (%d active, %d idle)\n",
		node.CurrentConnections, node.ActiveRequests, node.IdleConnections)
}

health := c.GetPoolHealth()
fmt.Printf("\nHealth Summary: %+v\n", health)
```

## 🔄 Connection Pool Scaling

The dynamic connection pool automatically adapts to your traffic patterns:

```
Initial connections (MinPoolSize: 5)
          ↓
Traffic increases → ActiveRequests > ScaleUpThreshold
          ↓
Pool grows (adds ScaleUpBatchSize connections)
          ↓
Traffic decreases → Connections idle > IdleTimeout
          ↓
Pool shrinks (removes idle connections, keeps MinPoolSize)
```

For detailed configuration options, see [SCALING.md](SCALING.md).

## 📐 Architecture

For details on the internal architecture, see [ARCHITECTURE.md](ARCHITECTURE.md).
For the product context (engine, controller, SaaS), see
[DESCRIPTION.md](DESCRIPTION.md).

## 📄 License

This project is licensed under the MIT License — see the LICENSE file for details.

## 🤝 Contributing

Contributions are welcome! Please feel free to submit a Pull Request.
