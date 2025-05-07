# GoSureSQL

<div align="center">

[![Go Reference](https://pkg.go.dev/badge/github.com/yourusername/gosuresql.svg)](https://pkg.go.dev/github.com/yourusername/gosuresql)
[![Go Report Card](https://goreportcard.com/badge/github.com/yourusername/gosuresql)](https://goreportcard.com/report/github.com/yourusername/gosuresql)
[![License: MIT](https://img.shields.io/badge/License-MIT-yellow.svg)](https://opensource.org/licenses/MIT)

*A high-performance Go client for SureSQL DB Server with dynamic connection pooling*

</div>

<p align="center">
  <img src="docs/images/logo.png" alt="GoSureSQL Logo" width="200"/>
</p>

---

## 📋 Overview

GoSureSQL is a robust client for SureSQL Database Server providing a complete ORM interface with intelligent connection management. The library automatically handles token refresh, connection pooling, scaling, and high availability - so you can focus on your application logic.

### Key Features

- **🔄 Dynamic Connection Pool**: Automatically scales with traffic demands
- **⚡ Read/Write Separation**: Routes queries to appropriate nodes for optimal performance
- **♻️ Token Management**: Handles token refresh and reconnection automatically
- **🔍 Comprehensive ORM**: Full SQL capabilities with type-safe interfaces
- **📊 Detailed Metrics**: Real-time insights into connection usage and performance
- **🛠️ Highly Configurable**: Customize scaling behavior to your environment

## 📦 Installation

```bash
go get github.com/skoeswanto/gosuresql
```

## 🚀 Quick Start

```go
package main

import (
    "fmt"
    "time"
    "github.com/yourusername/gosuresql/client"
)

func main() {
    // Initialize client
    sureSQL, err := client.NewClient(client.ClientConfig{
        ServerURL:   "http://suresql-server:8080",
        APIKey:      "your-api-key",
        ClientID:    "your-client-id",
        Username:    "username",
        Password:    "password",
        HTTPTimeout: 30 * time.Second,
    })
    
    if err != nil {
        panic(err)
    }
    defer sureSQL.Close()
    
    // Execute a simple query
    users, err := sureSQL.SelectOneSQL("SELECT * FROM users LIMIT 10")
    if err != nil {
        panic(err)
    }
    
    // Process results
    for _, user := range users {
        fmt.Printf("User ID: %v, Name: %v\n", 
            user.Data["id"], 
            user.Data["name"])
    }
}
```

## 🔄 Connection Management

GoSureSQL handles all aspects of connection management automatically:

### Token Lifecycle

- **Initial Authentication**: Obtained during `Connect()`
- **Automatic Refresh**: When a token expires during operations
- **Reconnection**: If refresh token expires, automatically reconnects
- **No Manual Management**: You never need to handle tokens directly

### Connection Pooling

- **Adaptive Scaling**: Pool grows during high traffic, shrinks during idle periods
- **Multiple Nodes**: Maintains separate pools for leader and read replicas
- **Fault Tolerance**: Automatically handles node failures
- **Round-Robin Distribution**: Evenly distributes requests across connections

For detailed configuration options, see [SCALING.md](SCALING.md).

## 🛠️ Configuration

### Basic Client Setup

```go
client, err := client.NewClient(client.ClientConfig{
    ServerURL:   "http://suresql-server:8080",
    APIKey:      "your-api-key",
    ClientID:    "your-client-id",
    Username:    "username",
    Password:    "password",
    HTTPTimeout: 30 * time.Second,
})
```

### Custom Pool Configuration

```go
// Create custom pool configuration
poolConfig := client.NewPoolConfig(
    client.WithMinPoolSize(5),
    client.WithScaleUpThreshold(15),
    client.WithIdleTimeout(5 * time.Minute),
    client.WithScaleUpBatchSize(3),
    client.WithConnectionTTL(1 * time.Hour),
)

// Initialize client with custom pool config
client, err := client.NewClient(client.ClientConfig{
    // Basic configuration...
    PoolConfig: poolConfig,
})
```

## 📚 API Reference

### Connection Management

#### `Connect(username, password string) error`

Establishes a connection to the SureSQL server using the provided credentials.

- Creates the initial token for authentication
- Initializes the connection pool with `MinPoolSize` connections
- Discovers and connects to peer nodes

```go
err := client.Connect("admin", "password123")
if err != nil {
    log.Fatalf("Failed to connect: %v", err)
}
```

#### `IsConnected() bool`

Verifies if the client is connected to the server and has active connections in the pool.

```go
if client.IsConnected() {
    fmt.Println("Connected to SureSQL server")
} else {
    fmt.Println("Not connected")
}
```

#### `Close()`

Properly shuts down the client, closing all connections and cleaning up resources.

```go
defer client.Close()
```

### Basic Queries

#### `SelectOne(tableName string) (orm.DBRecord, error)`

Retrieves a single record from a table. Useful for selecting default configuration entries or single items when only one record exists.

**Returns:**
- `orm.DBRecord`: A single record with table name and data map
- `error`: Error if no records found or other query issues

```go
// Get application settings
settings, err := client.SelectOne("app_settings")
if err != nil {
    log.Fatal(err)
}

// Access data fields
fmt.Printf("App Name: %v\n", settings.Data["app_name"])
fmt.Printf("Version: %v\n", settings.Data["version"])
```

#### `SelectMany(tableName string) (orm.DBRecords, error)`

Retrieves all records from a table. Use with caution on large tables.

**Returns:**
- `orm.DBRecords`: Slice of records, each with table name and data map
- `error`: Error if no records found or other query issues

```go
// Get all categories
categories, err := client.SelectMany("categories")
if err != nil {
    log.Fatal(err)
}

fmt.Printf("Found %d categories\n", len(categories))
for _, category := range categories {
    fmt.Printf("- %v\n", category.Data["name"])
}
```

### Conditional Queries

#### `SelectOneWithCondition(tableName string, condition *orm.Condition) (orm.DBRecord, error)`

Retrieves a single record matching specific criteria. Ideal for fetching unique records by ID or other unique field.

**Returns:**
- `orm.DBRecord`: Single matching record
- `error`: Error if no records found, multiple records found, or other query issues

```go
// Simple condition - find user by ID
condition := &orm.Condition{
    Field:    "id",
    Operator: "=",
    Value:    42,
}
user, err := client.SelectOneWithCondition("users", condition)
if err != nil {
    log.Fatal(err)
}

fmt.Printf("User: %v %v\n", user.Data["first_name"], user.Data["last_name"])
```

#### `SelectManyWithCondition(tableName string, condition *orm.Condition) ([]orm.DBRecord, error)`

Retrieves multiple records matching specific criteria. Great for filtering data based on various conditions.

**Returns:**
- `[]orm.DBRecord`: Slice of matching records
- `error`: Error if no records found or other query issues

```go
// Simple condition - find active users
condition := &orm.Condition{
    Field:    "status",
    Operator: "=",
    Value:    "active",
}
activeUsers, err := client.SelectManyWithCondition("users", condition)
if err != nil {
    log.Fatal(err)
}

fmt.Printf("Found %d active users\n", len(activeUsers))
```

**Complex Condition Example:**

```go
// Complex nested condition: WHERE (age > 23 AND (location = 'surabaya' OR job = 'teacher'))
condition := &orm.Condition{
    Logic: "AND",
    Nested: []orm.Condition{
        {
            Field:    "age",
            Operator: ">",
            Value:    23,
        },
        {
            Logic: "OR",
            Nested: []orm.Condition{
                {
                    Field:    "location",
                    Operator: "=",
                    Value:    "surabaya",
                },
                {
                    Field:    "job",
                    Operator: "=",
                    Value:    "teacher",
                },
            },
        },
    },
}

// Find matching users
users, err := client.SelectManyWithCondition("users", condition)
if err != nil {
    log.Fatal(err)
}

fmt.Printf("Found %d users who are over 23 and either from Surabaya or teachers\n", len(users))
```

**With OrderBy, GroupBy and Pagination:**

```go
// Find all users who registered in 2023, grouped by city, ordered by registration date
// and paginated (20 per page, page 2)
condition := &orm.Condition{
    Field:    "registration_date",
    Operator: ">=",
    Value:    "2023-01-01",
    OrderBy:  []string{"registration_date DESC", "last_name ASC"},
    GroupBy:  []string{"city"},
    Limit:    20,
    Offset:   20, // Skip first 20 records (page 1)
}

users, err := client.SelectManyWithCondition("users", condition)
if err != nil {
    log.Fatal(err)
}

// Group counts available after grouping
for _, user := range users {
    fmt.Printf("City: %v, Count: %v\n", user.Data["city"], user.Data["count"])
}
```

### SQL Queries

#### `SelectOneSQL(sql string) (orm.DBRecords, error)`

Executes a raw SQL query that can return multiple rows. Provides full SQL flexibility when the ORM condition API is insufficient.

**Returns:**
- `orm.DBRecords`: All records returned by the query
- `error`: Error if query fails or no records found

```go
// Complex query with joins
records, err := client.SelectOneSQL(`
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

// Process results
for _, record := range records {
    fmt.Printf("User: %v, Department: %v\n", 
        record.Data["name"], 
        record.Data["dept_name"])
}
```

#### `SelectManySQL(sqlStatements []string) ([]orm.DBRecords, error)`

Executes multiple SQL queries in a single request. Efficient when you need to fetch several different result sets.

**Returns:**
- `[]orm.DBRecords`: Slice of result sets, one per query
- `error`: Error if any query fails

```go
// Execute multiple queries in one request
queries := []string{
    "SELECT id, name FROM users LIMIT 5",
    "SELECT id, title FROM posts WHERE status = 'published' LIMIT 5",
    "SELECT COUNT(*) as count FROM comments",
}

resultSets, err := client.SelectManySQL(queries)
if err != nil {
    log.Fatal(err)
}

// Process each result set
fmt.Println("Users:")
for _, user := range resultSets[0] {
    fmt.Printf("- %v\n", user.Data["name"])
}

fmt.Println("Posts:")
for _, post := range resultSets[1] {
    fmt.Printf("- %v\n", post.Data["title"])
}

fmt.Printf("Comment count: %v\n", resultSets[2][0].Data["count"])
```

#### `SelectOnlyOneSQL(sql string) (orm.DBRecord, error)`

Executes a SQL query that should return exactly one row. Enforces the single-row constraint and returns an error if multiple rows would be returned.

**Returns:**
- `orm.DBRecord`: The single record returned by the query
- `error`: Error if no records found, multiple records found, or query fails

```go
// Get a single user by ID
record, err := client.SelectOnlyOneSQL("SELECT * FROM users WHERE id = 42")
if err != nil {
    log.Fatal(err)
}

fmt.Printf("User name: %v\n", record.Data["name"])
fmt.Printf("Email: %v\n", record.Data["email"])
```

### Parameterized SQL Queries

#### `SelectOneSQLParameterized(paramSQL orm.ParametereizedSQL) (orm.DBRecords, error)`

Executes a SQL query with parameters. Protects against SQL injection and properly handles various data types.

**Returns:**
- `orm.DBRecords`: All records returned by the query
- `error`: Error if query fails or no records found

```go
// Find users with a specific role who joined after a certain date
query := orm.ParametereizedSQL{
    Query: `
        SELECT * FROM users 
        WHERE role = ? 
        AND join_date > ? 
        ORDER BY join_date DESC
    `,
    Values: []interface{}{"admin", "2023-01-01"},
}

records, err := client.SelectOneSQLParameterized(query)
if err != nil {
    log.Fatal(err)
}

fmt.Printf("Found %d matching admins\n", len(records))
for _, user := range records {
    fmt.Printf("- %v (joined %v)\n", 
        user.Data["name"], 
        user.Data["join_date"])
}
```

#### `SelectManySQLParameterized(paramSQLs []orm.ParametereizedSQL) ([]orm.DBRecords, error)`

Executes multiple parameterized SQL queries. Combines the benefits of batching with SQL injection protection.

**Returns:**
- `[]orm.DBRecords`: Slice of result sets, one per query
- `error`: Error if any query fails

```go
// Multiple parameterized queries
queries := []orm.ParametereizedSQL{
    {
        Query:  "SELECT * FROM users WHERE department_id = ?",
        Values: []interface{}{5},
    },
    {
        Query:  "SELECT * FROM tasks WHERE assigned_to = ? AND status = ?",
        Values: []interface{}{42, "pending"},
    },
}

resultSets, err := client.SelectManySQLParameterized(queries)
if err != nil {
    log.Fatal(err)
}

// Process results from each query
fmt.Printf("Found %d users in department 5\n", len(resultSets[0]))
fmt.Printf("Found %d pending tasks for user 42\n", len(resultSets[1]))
```

#### `SelectOnlyOneSQLParameterized(paramSQL orm.ParametereizedSQL) (orm.DBRecord, error)`

Executes a parameterized SQL query that should return exactly one row. Combines the safety of parameterized queries with the single-row constraint.

**Returns:**
- `orm.DBRecord`: The single record returned by the query
- `error`: Error if no records found, multiple records found, or query fails

```go
// Find a user by email (unique field)
query := orm.ParametereizedSQL{
    Query:  "SELECT * FROM users WHERE email = ?",
    Values: []interface{}{"john@example.com"},
}

user, err := client.SelectOnlyOneSQLParameterized(query)
if err != nil {
    log.Fatal(err)
}

fmt.Printf("Found user: %v\n", user.Data["name"])
```

### SQL Execution

#### `ExecOneSQL(sql string) orm.BasicSQLResult`

Executes a single SQL statement that doesn't return records (INSERT, UPDATE, DELETE, etc.). 

**Returns:**
- `orm.BasicSQLResult`: Contains affected rows count, last insert ID, error, and timing info

```go
// Update multiple records
result := client.ExecOneSQL(`
    UPDATE users 
    SET status = 'inactive' 
    WHERE last_login < '2023-01-01'
`)

if result.Error != nil {
    log.Fatalf("Error: %v", result.Error)
}

fmt.Printf("Updated %d users to inactive status\n", result.RowsAffected)
fmt.Printf("Operation took %f seconds\n", result.Timing)
```

#### `ExecOneSQLParameterized(paramSQL orm.ParametereizedSQL) orm.BasicSQLResult`

Executes a parameterized SQL statement that doesn't return records. Provides SQL injection protection for data modification operations.

**Returns:**
- `orm.BasicSQLResult`: Contains affected rows count, last insert ID, error, and timing info

```go
// Update a user's status with parameter protection
update := orm.ParametereizedSQL{
    Query:  "UPDATE users SET status = ? WHERE id = ?",
    Values: []interface{}{"suspended", 42},
}

result := client.ExecOneSQLParameterized(update)
if result.Error != nil {
    log.Fatal(result.Error)
}

fmt.Printf("Updated user status. Rows affected: %d\n", result.RowsAffected)
```

#### `ExecManySQL(sqlStatements []string) ([]orm.BasicSQLResult, error)`

Executes multiple SQL statements in a single request. Perfect for batch operations that don't need parameters.

**Returns:**
- `[]orm.BasicSQLResult`: Results for each statement
- `error`: Error if the request fails completely

```go
// Batch database maintenance
statements := []string{
    "DELETE FROM sessions WHERE expires < NOW()",
    "UPDATE statistics SET value = 0 WHERE period = 'daily'",
    "TRUNCATE TABLE logs_archive",
}

results, err := client.ExecManySQL(statements)
if err != nil {
    log.Fatal(err)
}

// Check results of each statement
for i, result := range results {
    if result.Error != nil {
        fmt.Printf("Statement %d failed: %v\n", i, result.Error)
    } else {
        fmt.Printf("Statement %d affected %d rows\n", i, result.RowsAffected)
    }
}
```

#### `ExecManySQLParameterized(paramSQLs []orm.ParametereizedSQL) ([]orm.BasicSQLResult, error)`

Executes multiple parameterized SQL statements. Combines batch efficiency with SQL injection protection.

**Returns:**
- `[]orm.BasicSQLResult`: Results for each statement
- `error`: Error if the request fails completely

```go
// Batch operations with parameters
statements := []orm.ParametereizedSQL{
    {
        Query:  "UPDATE products SET price = price * ? WHERE category_id = ?",
        Values: []interface{}{1.1, 5}, // 10% price increase for category 5
    },
    {
        Query:  "INSERT INTO price_change_log (category_id, change_pct, change_date) VALUES (?, ?, NOW())",
        Values: []interface{}{5, 10.0},
    },
}

results, err := client.ExecManySQLParameterized(statements)
if err != nil {
    log.Fatal(err)
}

fmt.Printf("Updated %d products\n", results[0].RowsAffected)
fmt.Printf("Log entry ID: %d\n", results[1].LastInsertID)
```

### Insert Operations

#### `InsertOneDBRecord(record orm.DBRecord, queue bool) orm.BasicSQLResult`

Inserts a single record into a table. The `queue` parameter determines if the operation should be queued for batch processing.

**Returns:**
- `orm.BasicSQLResult`: Contains last insert ID, affected rows count, error, and timing info

```go
// Create a new user
user := orm.DBRecord{
    TableName: "users",
    Data: map[string]interface{}{
        "name":     "Jane Smith",
        "email":    "jane@example.com",
        "role":     "user",
        "status":   "active",
        "created":  time.Now(),
    },
}

result := client.InsertOneDBRecord(user, false)
if result.Error != nil {
    log.Fatal(result.Error)
}

fmt.Printf("Created new user with ID: %d\n", result.LastInsertID)
```

#### `InsertManyDBRecords(records []orm.DBRecord, queue bool) ([]orm.BasicSQLResult, error)`

Inserts multiple records into potentially different tables. Efficiently handles a variety of inserts in a single operation.

**Returns:**
- `[]orm.BasicSQLResult`: Results for each insert
- `error`: Error if the request fails completely

```go
// Insert records into different tables
records := []orm.DBRecord{
    {
        TableName: "users",
        Data: map[string]interface{}{
            "name":  "John Doe",
            "email": "john@example.com",
        },
    },
    {
        TableName: "user_preferences",
        Data: map[string]interface{}{
            "user_id":  1, // Will be replaced with the actual ID after first insert
            "theme":    "dark",
            "timezone": "UTC+7",
        },
    },
}

// Execute inserts
results, err := client.InsertManyDBRecords(records, false)
if err != nil {
    log.Fatal(err)
}

// Update preferences with the new user ID
if results[0].Error == nil {
    fmt.Printf("User created with ID: %d\n", results[0].LastInsertID)
    fmt.Printf("Preferences created for user\n")
}
```

#### `InsertManyDBRecordsSameTable(records []orm.DBRecord, queue bool) ([]orm.BasicSQLResult, error)`

Inserts multiple records into the same table. More efficient than individual inserts as it batches the operations.

**Returns:**
- `[]orm.BasicSQLResult`: Results for each batch of inserts
- `error`: Error if the request fails completely

```go
// Batch insert multiple products
products := []orm.DBRecord{
    {
        TableName: "products",
        Data: map[string]interface{}{
            "name":        "Smartphone X",
            "category_id": 1,
            "price":       599.99,
            "stock":       50,
        },
    },
    {
        TableName: "products",
        Data: map[string]interface{}{
            "name":        "Tablet Pro",
            "category_id": 1,
            "price":       799.99,
            "stock":       30,
        },
    },
    {
        TableName: "products",
        Data: map[string]interface{}{
            "name":        "Wireless Earbuds",
            "category_id": 2,
            "price":       129.99,
            "stock":       100,
        },
    },
}

results, err := client.InsertManyDBRecordsSameTable(products, false)
if err != nil {
    log.Fatal(err)
}

fmt.Printf("Inserted %d products\n", len(products))
fmt.Printf("First product ID: %d\n", results[0].LastInsertID)
```

### TableStruct Operations

#### `InsertOneTableStruct(record orm.TableStruct, queue bool) orm.BasicSQLResult`

Inserts a struct that implements the TableStruct interface. Automatically converts the struct to a DBRecord.

**Returns:**
- `orm.BasicSQLResult`: Contains last insert ID, affected rows count, error, and timing info

```go
// Define a struct that implements TableStruct
type User struct {
    ID     int    `db:"id"`
    Name   string `db:"name"`
    Email  string `db:"email"`
    Status string `db:"status"`
}

// Implement TableName method
func (u User) TableName() string {
    return "users"
}

// Create and insert a new user
newUser := User{
    Name:   "Alice Brown",
    Email:  "alice@example.com",
    Status: "active",
}

result := client.InsertOneTableStruct(newUser, false)
if result.Error != nil {
    log.Fatal(result.Error)
}

fmt.Printf("Created new user with ID: %d\n", result.LastInsertID)
```

#### `InsertManyTableStructs(records []orm.TableStruct, queue bool) ([]orm.BasicSQLResult, error)`

Inserts multiple structs that implement the TableStruct interface. Can insert into different tables based on each struct's TableName method.

**Returns:**
- `[]orm.BasicSQLResult`: Results for each insert
- `error`: Error if the request fails completely

```go
// Define structs for different tables
type Product struct {
    ID    int     `db:"id"`
    Name  string  `db:"name"`
    Price float64 `db:"price"`
}

func (p Product) TableName() string {
    return "products"
}

type Category struct {
    ID   int    `db:"id"`
    Name string `db:"name"`
}

func (c Category) TableName() string {
    return "categories"
}

// Create and insert multiple struct records
records := []orm.TableStruct{
    Product{Name: "Gaming Laptop", Price: 1299.99},
    Product{Name: "Wireless Mouse", Price: 49.99},
    Category{Name: "Electronics"},
}

results, err := client.InsertManyTableStructs(records, false)
if err != nil {
    log.Fatal(err)
}

fmt.Printf("Inserted %d records\n", len(results))
```

### Schema & Status Methods

#### `GetSchema(hideSQL bool, hideSureSQL bool) []orm.SchemaStruct`

Retrieves the database schema, including tables, views, and indices. Optional parameters can hide system tables.

**Returns:**
- `[]orm.SchemaStruct`: Array of schema objects with details about database objects

```go
// Get complete database schema
schema := client.GetSchema(false, false)

// Display tables
fmt.Println("Tables:")
for _, item := range schema {
    if item.ObjectType == "table" {
        fmt.Printf("- %s\n", item.TableName)
        fmt.Printf("  SQL: %s\n", item.SQLCommand)
    }
}

// Display indices
fmt.Println("\nIndices:")
for _, item := range schema {
    if item.ObjectType == "index" {
        fmt.Printf("- %s (on %s)\n", item.ObjectName, item.TableName)
    }
}
```

#### `Status() (orm.NodeStatusStruct, error)`

Gets detailed status information about the database cluster, including leader and peer nodes.

**Returns:**
- `orm.NodeStatusStruct`: Comprehensive status information
- `error`: Error if the request fails

```go
// Get cluster status
status, err := client.Status()
if err != nil {
    log.Fatal(err)
}

fmt.Printf("DB Version: %s\n", status.Version)
fmt.Printf("Cluster Size: %d nodes\n", status.Nodes)
fmt.Printf("Leader: %s\n", status.Leader)
fmt.Printf("Uptime: %s\n", status.Uptime)

// Display peer information
fmt.Println("Peers:")
for id, peer := range status.Peers {
    fmt.Printf("- Node %d: %s (Mode: %s)\n", id, peer.URL, peer.Mode)
}
```

#### `Leader() (string, error)`

Gets the current leader node of the cluster.

**Returns:**
- `string`: URL of the leader node
- `error`: Error if the request fails

```go
leader, err := client.Leader()
if err != nil {
    log.Fatal(err)
}

fmt.Printf("Current leader node: %s\n", leader)
```

#### `Peers() ([]string, error)`

Gets a list of peer nodes in the cluster.

**Returns:**
- `[]string`: URLs of peer nodes
- `error`: Error if the request fails

```go
peers, err := client.Peers()
if err != nil {
    log.Fatal(err)
}

fmt.Printf("Cluster has %d peer nodes:\n", len(peers))
for i, peer := range peers {
    fmt.Printf("%d. %s\n", i+1, peer)
}
```

## 📊 Monitoring

GoSureSQL provides comprehensive metrics for monitoring your connection pool:

```go
// Get detailed metrics
metrics := client.GetPoolMetrics()

fmt.Printf("Total connections: %d\n", metrics.TotalConnections)
fmt.Printf("Active requests: %d\n", metrics.ActiveRequests)
fmt.Printf("Requests per second: %.2f\n", metrics.RequestsPerSecond)
fmt.Printf("Scale-up events: %d\n", metrics.ScaleUpEvents)
fmt.Printf("Scale-down events: %d\n", metrics.ScaleDownEvents)

// Review per-node metrics
for nodeID, node := range metrics.ConnectionsPerNode {
    fmt.Printf("\nNode: %s (%s)\n", nodeID, node.URL)
    fmt.Printf("  Connections: %d (%d active, %d idle)\n", 
               node.CurrentConnections, node.ActiveRequests, node.IdleConnections)
    fmt.Printf("  Recent requests: %d\n", node.RecentRequests)
    fmt.Printf("  Last scale up: %s\n", node.LastScaleUp.Format(time.RFC3339))
}

// Quick health check
health := client.GetPoolHealth()
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

## 📄 License

This project is licensed under the MIT License - see the LICENSE file for details.

## 🤝 Contributing

Contributions are welcome! Please feel free to submit a Pull Request.