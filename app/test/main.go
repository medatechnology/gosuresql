package main

import (
	"fmt"
	"log"
	"sync"
	"time"

	client "github.com/medatechnology/gosuresql"
	orm "github.com/medatechnology/simpleorm"

	"github.com/joho/godotenv"
)

// UserModel for testing table struct operations
type UserModel struct {
	ID        int       `json:"id,omitempty"           db:"id"`
	Username  string    `json:"username,omitempty"     db:"username"`
	Email     string    `json:"email,omitempty"        db:"email"`
	Active    bool      `json:"active,omitempty"       db:"active"`
	CreatedAt time.Time `json:"created_at,omitempty"   db:"created_at"`
}

// TableName implementation for the TableStruct interface
func (u UserModel) TableName() string {
	return "users"
}

func main() {
	// Initialize the client
	c, err := initClient()
	if err != nil {
		log.Fatalf("Failed to initialize client: %v", err)
	}

	// Ensure we can connect to the server
	fmt.Println("▶️ Testing connection status")
	testIsConnected(c)

	// Test status methods
	fmt.Println("\n▶️ Testing status methods")
	testStatus(c)

	// Clean up first before testing
	fmt.Println("\n▶️ Cleaning up all testing tables before begin")
	testCleanup(c)

	// Create test tables
	fmt.Println("\n▶️ Creating test tables")
	createTestTables(c)

	// Test basic SQL execution
	fmt.Println("\n▶️ Testing SQL execution methods")
	testSQLExecution(c)

	// Test parameterized SQL execution
	fmt.Println("\n▶️ Testing parameterized SQL execution")
	testParameterizedSQLExecution(c)

	// Test simple queries
	fmt.Println("\n▶️ Testing simple query methods")
	testSimpleQueries(c)

	// Test conditional queries
	fmt.Println("\n▶️ Testing conditional query methods")
	testConditionalQueries(c)

	// Test SQL queries
	fmt.Println("\n▶️ Testing SQL query methods")
	testSQLQueries(c)

	// Test parameterized SQL queries
	fmt.Println("\n▶️ Testing parameterized SQL queries")
	testParameterizedSQLQueries(c)

	// Test insert operations
	fmt.Println("\n▶️ Testing insert operations")
	testInsertOperations(c)

	// Test struct operations
	fmt.Println("\n▶️ Testing struct operations")
	testStructOperations(c)

	// Test struct operations
	fmt.Println("\n▶️ Testing load test")
	runLoadTest(c, 1000)

	// Cleaning up test
	fmt.Println("\n▶️ Cleaning up all testing tables")
	testCleanup(c)

	fmt.Println("\n✅ All tests completed")
}

func initClient() (*client.Client, error) {
	LoadEnvironment(".env.client", "./app/test/.env.client")
	// Read configuration from environment variables or use defaults

	// serverURL := getEnv("SURESQL_SERVER_URL", "http://localhost:8080")
	// apiKey := getEnv("SURESQL_API_KEY", "development_api_key")
	// clientID := getEnv("SURESQL_CLIENT_ID", "development_client_id")
	// username := getEnv("SURESQL_USERNAME", "admin")
	// password := getEnv("SURESQL_PASSWORD", "admin")

	// config := client.ClientConfig{
	// 	ServerURL:   serverURL,
	// 	APIKey:      apiKey,
	// 	ClientID:    clientID,
	// 	Username:    username,
	// 	Password:    password,
	// 	HTTPTimeout: 30 * time.Second,
	// }
	fmt.Println("-end")
	config := client.NewClientConfig()
	fmt.Println("Config: ", config)
	c, err := client.NewClient(config)
	if err != nil {
		return nil, fmt.Errorf("failed to create client: %w", err)
	}

	return c, nil
}

// func getEnv(key, fallback string) string {
// 	if value, exists := os.LookupEnv(key); exists {
// 		return value
// 	}
// 	return fallback
// }

func testIsConnected(c *client.Client) {
	err := c.Connect("", "")
	if err != nil {
		fmt.Println("❌ Cannot connect to SureSQL server")
	}
	if c.IsConnected() {
		fmt.Println("✅ Successfully connected to SureSQL server")
	} else {
		fmt.Println("❌ Not connected to SureSQL server")
	}
}

func testStatus(c *client.Client) {
	// Test leader and peers
	leader, err := c.Leader()
	if err != nil {
		fmt.Printf("❌ Failed to get leader: %v\n", err)
	} else {
		fmt.Printf("✅ Leader: %s\n", leader)
	}

	peers, err := c.Peers()
	if err != nil {
		fmt.Printf("❌ Failed to get peers: %v\n", err)
	} else {
		fmt.Printf("✅ Peers: %v\n", peers)
	}

	// Test full status
	status, err := c.Status()
	if err != nil {
		fmt.Printf("❌ Failed to get status: %v\n", err)
	} else {
		// fmt.Printf("✅ Status: %+v\n", status)
		status.PrintPretty()
	}

	// Test schema
	schema := c.GetSchema(true, true)
	fmt.Printf("✅ Schema retrieved, count: %d\n", len(schema))
}

func createTestTables(c *client.Client) {
	// Create users table
	createUserTableSQL := `
	CREATE TABLE IF NOT EXISTS users (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		username TEXT NOT NULL UNIQUE,
		email TEXT NOT NULL,
		active BOOLEAN DEFAULT TRUE,
		created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
	)`

	result := c.ExecOneSQL(createUserTableSQL)
	if result.Error != nil {
		fmt.Printf("❌ Failed to create users table: %v\n", result.Error)
	} else {
		fmt.Println("✅ Users table created successfully")
	}

	// Create products table
	createProductsTableSQL := `
	CREATE TABLE IF NOT EXISTS products (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		name TEXT NOT NULL,
		description TEXT,
		price DECIMAL(10,2) NOT NULL,
		stock INTEGER DEFAULT 0
	)`

	result = c.ExecOneSQL(createProductsTableSQL)
	if result.Error != nil {
		fmt.Printf("❌ Failed to create products table: %v\n", result.Error)
	} else {
		fmt.Println("✅ Products table created successfully")
	}
}

func testSQLExecution(c *client.Client) {
	// Test ExecOneSQL
	insertSQL := "INSERT INTO users (username, email) VALUES ('john_doe', 'john@example.com')"
	result := c.ExecOneSQL(insertSQL)
	if result.Error != nil {
		fmt.Printf("❌ ExecOneSQL failed: %v\n", result.Error)
	} else {
		fmt.Printf("✅ ExecOneSQL succeeded: %d rows affected, last ID: %d\n",
			result.RowsAffected, result.LastInsertID)
	}

	// Test ExecManySQL
	insertStatements := []string{
		"INSERT INTO users (username, email) VALUES ('jane_doe', 'jane@example.com')",
		"INSERT INTO users (username, email) VALUES ('bob_smith', 'bob@example.com')",
	}

	results, err := c.ExecManySQL(insertStatements)
	if err != nil {
		fmt.Printf("❌ ExecManySQL failed: %v\n", err)
	} else {
		fmt.Printf("✅ ExecManySQL succeeded, executed %d statements\n", len(results))
		for i, res := range results {
			fmt.Printf("  - Statement %d: %d rows affected, last ID: %d\n",
				i+1, res.RowsAffected, res.LastInsertID)
		}
	}
}

func testParameterizedSQLExecution(c *client.Client) {
	// Test ExecOneSQLParameterized
	paramSQL := orm.ParametereizedSQL{
		Query:  "INSERT INTO users (username, email) VALUES (?, ?)",
		Values: []interface{}{"alice_wonder", "alice@example.com"},
	}

	result := c.ExecOneSQLParameterized(paramSQL)
	if result.Error != nil {
		fmt.Printf("❌ ExecOneSQLParameterized failed: %v\n", result.Error)
	} else {
		fmt.Printf("✅ ExecOneSQLParameterized succeeded: %d rows affected, last ID: %d\n",
			result.RowsAffected, result.LastInsertID)
	}

	// Test ExecManySQLParameterized
	paramSQLs := []orm.ParametereizedSQL{
		{
			Query:  "INSERT INTO products (name, description, price, stock) VALUES (?, ?, ?, ?)",
			Values: []interface{}{"Product 1", "Description 1", 9.99, 100},
		},
		{
			Query:  "INSERT INTO products (name, description, price, stock) VALUES (?, ?, ?, ?)",
			Values: []interface{}{"Product 2", "Description 2", 19.99, 50},
		},
	}

	results, err := c.ExecManySQLParameterized(paramSQLs)
	if err != nil {
		fmt.Printf("❌ ExecManySQLParameterized failed: %v\n", err)
	} else {
		fmt.Printf("✅ ExecManySQLParameterized succeeded, executed %d statements\n", len(results))
		for i, res := range results {
			fmt.Printf("  - Statement %d: %d rows affected, last ID: %d\n",
				i+1, res.RowsAffected, res.LastInsertID)
		}
	}
}

func testSimpleQueries(c *client.Client) {
	// Test SelectOne
	userRecord, err := c.SelectOne("users")
	if err != nil {
		fmt.Printf("❌ SelectOne failed: %v\n", err)
	} else {
		fmt.Printf("✅ SelectOne succeeded, got user: %v\n", userRecord.Data["username"])
	}

	// Test SelectMany
	userRecords, err := c.SelectMany("users")
	if err != nil {
		fmt.Printf("❌ SelectMany failed: %v\n", err)
	} else {
		fmt.Printf("✅ SelectMany succeeded, got %d users\n", len(userRecords))
		for i, user := range userRecords {
			if i < 3 { // Just show the first 3 to keep output manageable
				fmt.Printf("  - User %d: %v (%v)\n", i+1, user.Data["username"], user.Data["email"])
			}
		}
	}
}

func testConditionalQueries(c *client.Client) {
	// Test SelectOneWithCondition
	condition := &orm.Condition{
		Field:    "username",
		Operator: "=",
		Value:    "john_doe",
	}

	user, err := c.SelectOneWithCondition("users", condition)
	if err != nil {
		fmt.Printf("❌ SelectOneWithCondition failed: %v\n", err)
	} else {
		fmt.Printf("✅ SelectOneWithCondition succeeded, got user: %v (%v)\n",
			user.Data["username"], user.Data["email"])
	}

	// Test SelectManyWithCondition
	condition = &orm.Condition{
		OrderBy: []string{"id DESC"},
		Limit:   2,
	}

	users, err := c.SelectManyWithCondition("users", condition)
	if err != nil {
		fmt.Printf("❌ SelectManyWithCondition failed: %v\n", err)
	} else {
		fmt.Printf("✅ SelectManyWithCondition succeeded, got %d users\n", len(users))
		for i, user := range users {
			fmt.Printf("  - User %d: %v (%v)\n", i+1, user.Data["username"], user.Data["email"])
		}
	}

	// Test complex condition
	complexCondition := &orm.Condition{
		Logic: "OR",
		Nested: []orm.Condition{
			{
				Field:    "username",
				Operator: "LIKE",
				Value:    "%doe",
			},
			{
				Field:    "email",
				Operator: "LIKE",
				Value:    "%@example.com",
			},
		},
		OrderBy: []string{"username ASC"},
	}

	users, err = c.SelectManyWithCondition("users", complexCondition)
	if err != nil {
		fmt.Printf("❌ Complex SelectManyWithCondition failed: %v\n", err)
	} else {
		fmt.Printf("✅ Complex SelectManyWithCondition succeeded, got %d users\n", len(users))
		for i, user := range users {
			fmt.Printf("  - User %d: %v (%v)\n", i+1, user.Data["username"], user.Data["email"])
		}
	}
}

func testSQLQueries(c *client.Client) {
	// Test SelectOneSQL
	sql := "SELECT * FROM users ORDER BY id DESC LIMIT 3"
	records, err := c.SelectOneSQL(sql)
	if err != nil {
		fmt.Printf("❌ SelectOneSQL failed: %v\n", err)
	} else {
		fmt.Printf("✅ SelectOneSQL succeeded, got %d records\n", len(records))
		for i, record := range records {
			fmt.Printf("  - Record %d: %v (%v)\n", i+1, record.Data["username"], record.Data["email"])
		}
	}

	// Test SelectManySQL
	sqlStatements := []string{
		"SELECT * FROM users WHERE username LIKE '%doe%'",
		"SELECT * FROM users WHERE email LIKE '%@example.com' LIMIT 2",
	}

	recordSets, err := c.SelectManySQL(sqlStatements)
	if err != nil {
		fmt.Printf("❌ SelectManySQL failed: %v\n", err)
	} else {
		fmt.Printf("✅ SelectManySQL succeeded, got %d record sets\n", len(recordSets))
		for i, recordSet := range recordSets {
			fmt.Printf("  - Record set %d: %d records\n", i+1, len(recordSet))
			for j, record := range recordSet {
				if j < 2 { // Just show the first 2 records per set
					fmt.Printf("    - Record %d: %v (%v)\n", j+1, record.Data["username"], record.Data["email"])
				}
			}
		}
	}

	// Test SelectOnlyOneSQL
	singleSQL := "SELECT * FROM users WHERE username = 'john_doe'"
	record, err := c.SelectOnlyOneSQL(singleSQL)
	if err != nil {
		fmt.Printf("❌ SelectOnlyOneSQL failed: %v\n", err)
	} else {
		fmt.Printf("✅ SelectOnlyOneSQL succeeded, got user: %v (%v)\n",
			record.Data["username"], record.Data["email"])
	}
}

func testParameterizedSQLQueries(c *client.Client) {
	// Test SelectOneSQLParameterized
	paramSQL := orm.ParametereizedSQL{
		Query:  "SELECT * FROM users WHERE username LIKE ?",
		Values: []interface{}{"%doe%"},
	}

	records, err := c.SelectOneSQLParameterized(paramSQL)
	if err != nil {
		fmt.Printf("❌ SelectOneSQLParameterized failed: %v\n", err)
	} else {
		fmt.Printf("✅ SelectOneSQLParameterized succeeded, got %d records\n", len(records))
		for i, record := range records {
			if i < 3 { // Just show up to 3 records
				fmt.Printf("  - Record %d: %v (%v)\n", i+1, record.Data["username"], record.Data["email"])
			}
		}
	}

	// Test SelectManySQLParameterized
	paramSQLs := []orm.ParametereizedSQL{
		{
			Query:  "SELECT * FROM users WHERE username = ?",
			Values: []interface{}{"john_doe"},
		},
		{
			Query:  "SELECT * FROM users WHERE username = ?",
			Values: []interface{}{"jane_doe"},
		},
	}

	recordSets, err := c.SelectManySQLParameterized(paramSQLs)
	if err != nil {
		fmt.Printf("❌ SelectManySQLParameterized failed: %v\n", err)
	} else {
		fmt.Printf("✅ SelectManySQLParameterized succeeded, got %d record sets\n", len(recordSets))
		for i, recordSet := range recordSets {
			fmt.Printf("  - Record set %d: %d records\n", i+1, len(recordSet))
			for j, record := range recordSet {
				fmt.Printf("    - Record %d: %v (%v)\n", j+1, record.Data["username"], record.Data["email"])
			}
		}
	}

	// Test SelectOnlyOneSQLParameterized
	singleParamSQL := orm.ParametereizedSQL{
		Query:  "SELECT * FROM users WHERE username = ?",
		Values: []interface{}{"alice_wonder"},
	}

	record, err := c.SelectOnlyOneSQLParameterized(singleParamSQL)
	if err != nil {
		fmt.Printf("❌ SelectOnlyOneSQLParameterized failed: %v\n", err)
	} else {
		fmt.Printf("✅ SelectOnlyOneSQLParameterized succeeded, got user: %v (%v)\n",
			record.Data["username"], record.Data["email"])
	}
}

func testInsertOperations(c *client.Client) {
	// Test InsertOneDBRecord
	userRecord := orm.DBRecord{
		TableName: "users",
		Data: map[string]interface{}{
			"username": "charlie_brown",
			"email":    "charlie@example.com",
			"active":   true,
		},
	}

	result := c.InsertOneDBRecord(userRecord, false)
	if result.Error != nil {
		fmt.Printf("❌ InsertOneDBRecord failed: %v\n", result.Error)
	} else {
		fmt.Printf("✅ InsertOneDBRecord succeeded: %d rows affected, last ID: %d\n",
			result.RowsAffected, result.LastInsertID)
	}

	// Test InsertManyDBRecords
	records := []orm.DBRecord{
		{
			TableName: "users",
			Data: map[string]interface{}{
				"username": "david_clark",
				"email":    "david@example.com",
				"active":   true,
			},
		},
		{
			TableName: "products",
			Data: map[string]interface{}{
				"name":        "Product 3",
				"description": "Description 3",
				"price":       29.99,
				"stock":       75,
			},
		},
	}

	results, err := c.InsertManyDBRecords(records, false)
	if err != nil {
		fmt.Printf("❌ InsertManyDBRecords failed: %v\n", err)
	} else {
		fmt.Printf("✅ InsertManyDBRecords succeeded, inserted %d records\n", len(results))
		for i, res := range results {
			fmt.Printf("  - Record %d: %d rows affected, last ID: %d\n",
				i+1, res.RowsAffected, res.LastInsertID)
		}
	}

	// Test InsertManyDBRecordsSameTable
	userRecords := []orm.DBRecord{
		{
			TableName: "users",
			Data: map[string]interface{}{
				"username": "emma_stone",
				"email":    "emma@example.com",
				"active":   true,
			},
		},
		{
			TableName: "users",
			Data: map[string]interface{}{
				"username": "frank_miller",
				"email":    "frank@example.com",
				"active":   false,
			},
		},
	}

	results, err = c.InsertManyDBRecordsSameTable(userRecords, false)
	if err != nil {
		fmt.Printf("❌ InsertManyDBRecordsSameTable failed: %v\n", err)
	} else {
		fmt.Printf("✅ InsertManyDBRecordsSameTable succeeded, inserted %d records\n", len(results))
		for i, res := range results {
			fmt.Printf("  - Record %d: %d rows affected, last ID: %d\n",
				i+1, res.RowsAffected, res.LastInsertID)
		}
	}
}

func testStructOperations(c *client.Client) {
	// Test InsertOneTableStruct
	user := UserModel{
		Username:  "grace_hopper",
		Email:     "grace@example.com",
		Active:    true,
		CreatedAt: time.Now(),
	}

	result := c.InsertOneTableStruct(user, false)
	if result.Error != nil {
		fmt.Printf("❌ InsertOneTableStruct failed: %v\n", result.Error)
	} else {
		fmt.Printf("✅ InsertOneTableStruct succeeded: %d rows affected, last ID: %d\n",
			result.RowsAffected, result.LastInsertID)
	}

	// Test InsertManyTableStructs
	users := []orm.TableStruct{
		UserModel{
			Username:  "henry_ford",
			Email:     "henry@example.com",
			Active:    true,
			CreatedAt: time.Now(),
		},
		UserModel{
			Username:  "ida_lovelace",
			Email:     "ida@example.com",
			Active:    true,
			CreatedAt: time.Now(),
		},
	}

	results, err := c.InsertManyTableStructs(users, false)
	if err != nil {
		fmt.Printf("❌ InsertManyTableStructs failed: %v\n", err)
	} else {
		fmt.Printf("✅ InsertManyTableStructs succeeded, inserted %d records\n", len(results))
		for i, res := range results {
			fmt.Printf("  - Record %d: %d rows affected, last ID: %d\n",
				i+1, res.RowsAffected, res.LastInsertID)
		}
	}
}

func testCleanup(c *client.Client) {
	// Test ExecOneSQL
	// insertSQL := "INSERT INTO users (username, email) VALUES ('john_doe', 'john@example.com')"
	// result := c.ExecOneSQL(insertSQL)
	// if result.Error != nil {
	// 	fmt.Printf("❌ ExecOneSQL failed: %v\n", result.Error)
	// } else {
	// 	fmt.Printf("✅ ExecOneSQL succeeded: %d rows affected, last ID: %d\n",
	// 		result.RowsAffected, result.LastInsertID)
	// }

	// Test ExecManySQL
	insertStatements := []string{
		"DROP TABLE users",
		"DROP TABLE products",
	}

	results, err := c.ExecManySQL(insertStatements)
	if err != nil {
		fmt.Printf("❌ ExecManySQL failed: %v\n", err)
	} else {
		fmt.Printf("✅ ExecManySQL succeeded, executed %d statements\n", len(results))
		for i, res := range results {
			fmt.Printf("  - Statement %d: %d rows affected, last ID: %d\n",
				i+1, res.RowsAffected, res.LastInsertID)
		}
	}
}

// LoadEnvironment loads environment variables from .env file and optional additional files.
func LoadEnvironment(additionalFiles ...string) {

	// Load default .env file even if it doesn't exist
	godotenv.Overload()
	// if err := godotenv.Overload(); err != nil {
	// 	fmt.Print("No .env file found, proceeding without it")
	// }

	// NOTE: we can just do godotenv.Overload(additionalFiles) and it should be fine,
	//       but if we want to get the individual file that is not exist, we need to do below loop
	// Load additional environment files if provided
	for _, file := range additionalFiles {
		if err := godotenv.Overload(file); err != nil {
			fmt.Printf("Error loading %s: %v", file, err)
		}
	}
}

// Run a load test and analyze pool behavior
func runLoadTest(client *client.Client, requestCount int) {
	start := time.Now()

	// Before metrics
	beforeMetrics := client.GetPoolMetrics()

	// Run load test
	var wg sync.WaitGroup
	for i := 0; i < requestCount; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			client.SelectOneSQL("SELECT * FROM users LIMIT 1")
		}()
	}
	wg.Wait()

	// After metrics
	afterMetrics := client.GetPoolMetrics()
	elapsed := time.Since(start)

	// Analysis
	fmt.Printf("Load test completed in %v\n", elapsed)
	fmt.Printf("Starting connections: %d\n", beforeMetrics.TotalConnections)
	fmt.Printf("Ending connections: %d\n", afterMetrics.TotalConnections)
	fmt.Printf("Connections added: %d\n", afterMetrics.TotalConnections-beforeMetrics.TotalConnections)
	fmt.Printf("Scale up events: %d\n", afterMetrics.ScaleUpEvents-beforeMetrics.ScaleUpEvents)
	fmt.Printf("Requests per second: %.2f\n", afterMetrics.RequestsPerSecond)
}
