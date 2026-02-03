package client

import (
	"errors"
	"fmt"
	"os"
	"time"

	utils "github.com/medatechnology/goutil"
	"github.com/medatechnology/goutil/object"
	orm "github.com/medatechnology/simpleorm"
	"github.com/medatechnology/suresql"
)

// Migrate runs schema migrations from the specified directory
func (c *Client) Migrate(dir string) error {
	ms := NewMigrationService(c)
	return ms.Migrate(dir)
}

const (
	DEFAULT_ENVIRONMENT_FILE = ".env.client"
	DEFAULT_AUTO_REFRESH     = true

	// For readibility of code
	WITH_TOKEN      = true  // for calling API with token, most API need token
	NO_TOKEN        = false // for calling API without token (like for /connect and /refresh)
	FALLBACK_LEADER = true  // for calling with connection, but if failed, use leader
	NO_FALLBACK     = false // for calling with connection, if failed then return error
	AUTO_REFRESH    = true  // for calling API, then token expired, auto refresh
	NO_REFRESH      = false // for calling API, then token expired, then just return error
	IS_WRITE        = true  // for write operation, used to find the connection from writePools
	IS_READ         = false // for read operation, used to find connection from readPools
	CALL_REFRESH    = true  // for calling /refresh on function newOrRefreshToken
	CALL_CONNECT    = false // for calling /connect on function newOrRefreshToken
)

// Initialized the client package, loading environment file(s)
func init() {
	_, err := os.Stat(DEFAULT_ENVIRONMENT_FILE)
	if err == nil {
		utils.ReloadEnvEach(DEFAULT_ENVIRONMENT_FILE)
	}
	// serverURL := utils.GetEnv("SURESQL_SERVER_URL", "http://localhost:8080")
	// apiKey := utils.GetEnv("SURESQL_API_KEY", "development_api_key")
	// clientID := utils.GetEnv("SURESQL_CLIENT_ID", "development_client_id")
	// username := utils.GetEnv("SURESQL_USERNAME", "admin")
	// password := utils.GetEnv("SURESQL_PASSWORD", "admin")
}

// This should be the first function to be called after creating a new SureSQL client object
// Connect authenticates with the SureSQL server and initializes the connection pool
// This has to use leader connection
func (c *Client) Connect(username, password string) error {
	// Just in case it is being recalled again
	if c.Connected {
		return errors.New("already connected, no need to call again")
	}

	// Use leader connection. TODO: make DEFAULT_AUTO_REFRESH more dynamic, maybe from environment variable
	data, err := c.sendRequestToLeader("POST", "/db/connect", c.userCredentialsDefault(username, password), NO_TOKEN, DEFAULT_AUTO_REFRESH)
	if err != nil {
		return err
	}
	// Extract token from response
	tokenObj, err := convertDataToToken(data)
	if err != nil {
		return err
	}

	// save the token
	c.leaderConn.Token = tokenObj
	c.leaderConn.LastRefresh = time.Now()
	c.Connected = true

	fmt.Println("going to call initialize pool")
	// Initialize the connection pool
	return c.InitializePool()
}

// GetRefreshToken updates the access token using the refresh token
// Since we are using connection pool, for now, this only checks for the LeaderConn token!
func (c *Client) GetRefreshToken() error {
	return c.leaderConn.tryRefreshAndRenew(&c.Config)
}

// GetSchema returns the database schema
func (c *Client) GetSchema(hideSQL bool, hideSureSQL bool) []orm.SchemaStruct {

	// Since schema returns array of SchemaStruct, first we process as []interface{}
	data, err := c.sendRequestToLeader("GET", "/db/api/getschema", nil, true, false)
	// data, err := c.executeWithConnectionOrFallback("GET", "/db/api/getschema", nil, true)
	if err != nil {
		return []orm.SchemaStruct{}
	}

	// Process schema data
	var schemaItems []orm.SchemaStruct
	// Try to handle as direct array first
	schemaArray, ok := data.([]interface{})
	if ok {
		// Process each schema item
		for _, item := range schemaArray {
			schemaMap, ok := item.(map[string]interface{})
			if !ok {
				continue // skip if not a map, shouldn't happens. QUESTION: maybe need to add error log here?
			}
			// Convert map to SchemaStruct using object.MapToStructSlow
			schemaItem := object.MapToStructSlow[orm.SchemaStruct](schemaMap)
			schemaItems = append(schemaItems, schemaItem)
		}
	}
	return schemaItems
}

// Status returns the database status with connection pooling
func (c *Client) Status() (orm.NodeStatusStruct, error) {
	// If we already have a connection, use it
	// conn := c.getAnyConnection()
	fmt.Println("Calling status")
	return sendRequest[orm.NodeStatusStruct](c, "GET", "/db/api/status", nil, IS_READ, NO_REFRESH, FALLBACK_LEADER)
	// if conn != nil {
	// 	// Use the connection
	// 	data, err := c.sendConnectionRequest(conn, "GET", "/db/api/status", nil, true)
	// 	fmt.Println("Conn exist calling /api/status, data:", data)
	// 	if err != nil {
	// 		// Fall back to direct request
	// 		data, err = c.sendDirectRequest("GET", "/db/api/status", nil, true)
	// 		if err != nil {
	// 			return orm.NodeStatusStruct{}, err
	// 		}
	// 	}

	// 	statusData, ok := data.(map[string]interface{})
	// 	if !ok {
	// 		return orm.NodeStatusStruct{}, fmt.Errorf("error: unexpected response format")
	// 	}
	// 	fmt.Println("After calling /api/status, statusData:", statusData)

	// 	return object.MapToStruct[orm.NodeStatusStruct](statusData), nil
	// }

	// // No connections available, use direct request
	// return c.getStatusWithoutLock()
}

// New helper method to get status without using the existing connections
// or acquiring the mutex lock
func (c *Client) getStatusWithoutLock() (orm.NodeStatusStruct, error) {
	// Use direct request to get status
	data, err := c.sendRequestToLeader("GET", "/db/api/status", nil, WITH_TOKEN, NO_REFRESH)
	if err != nil {
		return orm.NodeStatusStruct{}, err
	}

	// need to assert the data of type interface{} into map[string]interface
	statusData, ok := data.(map[string]interface{})
	if !ok {
		return orm.NodeStatusStruct{}, fmt.Errorf("error: unexpected response format")
	}

	// return as struct
	return object.MapToStruct[orm.NodeStatusStruct](statusData), nil
}

//------------------------------------------------------------------
// ORM QUERY METHODS
//------------------------------------------------------------------

// SelectOne selects a single record from the table
func (c *Client) SelectOne(tableName string) (orm.DBRecord, error) {
	req := &suresql.QueryRequest{
		Table:     tableName,
		SingleRow: true,
	}

	response, err := sendRequest[suresql.QueryResponse](c, "POST", "/db/api/query", req, IS_READ, AUTO_REFRESH, FALLBACK_LEADER)
	// response, err := c.executeReadQueryRequest("/db/api/query", req)
	if err != nil {
		return orm.DBRecord{}, err
	}
	// let user know this is not error, just no rows found
	if len(response.Records) == 0 {
		return orm.DBRecord{}, orm.ErrSQLNoRows
	}

	return response.Records[0], nil
}

// SelectMany selects multiple records from the table
func (c *Client) SelectMany(tableName string) (orm.DBRecords, error) {
	req := &suresql.QueryRequest{
		Table:     tableName,
		SingleRow: false,
	}

	// response, err := c.executeReadQueryRequest("/db/api/query", req)
	response, err := sendRequest[suresql.QueryResponse](c, "POST", "/db/api/query", req, IS_READ, AUTO_REFRESH, FALLBACK_LEADER)
	if err != nil {
		return nil, err
	}
	// let user know this is not error, just no rows found
	if len(response.Records) == 0 {
		return nil, orm.ErrSQLNoRows
	}

	return response.Records, nil
}

// SelectOneWithCondition selects a single record with a condition
func (c *Client) SelectOneWithCondition(tableName string, condition *orm.Condition) (orm.DBRecord, error) {
	req := &suresql.QueryRequest{
		Table:     tableName,
		Condition: condition,
		SingleRow: true,
	}

	// response, err := c.executeReadQueryRequest("/db/api/query", req)
	response, err := sendRequest[suresql.QueryResponse](c, "POST", "/db/api/query", req, IS_READ, AUTO_REFRESH, FALLBACK_LEADER)
	if err != nil {
		return orm.DBRecord{}, err
	}
	// let user know this is not error, just no rows found
	if len(response.Records) == 0 {
		return orm.DBRecord{}, orm.ErrSQLNoRows
	}
	return response.Records[0], nil
}

// SelectManyWithCondition selects multiple records with a condition
func (c *Client) SelectManyWithCondition(tableName string, condition *orm.Condition) ([]orm.DBRecord, error) {
	req := &suresql.QueryRequest{
		Table:     tableName,
		Condition: condition,
		SingleRow: false,
	}

	// response, err := c.executeReadQueryRequest("/db/api/query", req)
	response, err := sendRequest[suresql.QueryResponse](c, "POST", "/db/api/query", req, IS_READ, AUTO_REFRESH, FALLBACK_LEADER)
	if err != nil {
		return nil, err
	}
	// let user know this is not error, just no rows found
	if len(response.Records) == 0 {
		return nil, orm.ErrSQLNoRows
	}
	return response.Records, nil
}

//------------------------------------------------------------------
// ORM SQL QUERY METHODS
//------------------------------------------------------------------

// SelectOneSQL executes a single SQL query that can return multiple rows
func (c *Client) SelectOneSQL(sql string) (orm.DBRecords, error) {
	req := &suresql.SQLRequest{
		Statements: []string{sql},
		SingleRow:  false,
	}

	// response, err := c.executeReadSQLQueryRequest("/db/api/querysql", req)
	response, err := sendRequest[suresql.QueryResponseSQL](c, "POST", "/db/api/querysql", req, IS_READ, AUTO_REFRESH, FALLBACK_LEADER)
	if err != nil {
		return nil, err
	}
	// let user know this is not error, just no rows found
	if len(response) == 0 || len(response[0].Records) == 0 {
		return nil, orm.ErrSQLNoRows
	}
	return response[0].Records, nil
}

// SelectManySQL executes multiple SQL queries, each returning a set of records
func (c *Client) SelectManySQL(sqlStatements []string) ([]orm.DBRecords, error) {
	req := &suresql.SQLRequest{
		Statements: sqlStatements,
		SingleRow:  false,
	}

	// response, err := c.executeReadSQLQueryRequest("/db/api/querysql", req)
	response, err := sendRequest[suresql.QueryResponseSQL](c, "POST", "/db/api/querysql", req, IS_READ, AUTO_REFRESH, FALLBACK_LEADER)
	if err != nil {
		return nil, err
	}
	// let user know this is not error, just no rows found
	if len(response) == 0 {
		return nil, orm.ErrSQLNoRows
	}

	// Convert QueryREsponseSQL into []orm.DBRecords
	var allRecords []orm.DBRecords
	for _, resp := range response {
		allRecords = append(allRecords, resp.Records)
	}
	return allRecords, nil
}

// SelectOnlyOneSQL executes a SQL query that should return only one row
func (c *Client) SelectOnlyOneSQL(sql string) (orm.DBRecord, error) {
	req := &suresql.SQLRequest{
		Statements: []string{sql},
		SingleRow:  true,
	}

	// response, err := c.executeReadSQLQueryRequest("/db/api/querysql", req)
	response, err := sendRequest[suresql.QueryResponseSQL](c, "POST", "/db/api/querysql", req, IS_READ, AUTO_REFRESH, FALLBACK_LEADER)
	if err != nil {
		return orm.DBRecord{}, err
	}
	// let user know this is not error, just no rows found
	if len(response) == 0 || len(response[0].Records) == 0 {
		return orm.DBRecord{}, orm.ErrSQLNoRows
	}
	// Because this function is meant to check if it's only 1 row return
	if len(response[0].Records) > 1 {
		return orm.DBRecord{}, orm.ErrSQLMoreThanOneRow
	}

	return response[0].Records[0], nil
}

// SelectOneSQLParameterized executes a single parameterized SQL query
func (c *Client) SelectOneSQLParameterized(paramSQL orm.ParametereizedSQL) (orm.DBRecords, error) {
	req := &suresql.SQLRequest{
		ParamSQL:  []orm.ParametereizedSQL{paramSQL},
		SingleRow: false,
	}

	// response, err := c.executeReadSQLQueryRequest("/db/api/querysql", req)
	response, err := sendRequest[suresql.QueryResponseSQL](c, "POST", "/db/api/querysql", req, IS_READ, AUTO_REFRESH, FALLBACK_LEADER)
	if err != nil {
		return nil, err
	}
	// let user know this is not error, just no rows found
	if len(response) == 0 || len(response[0].Records) == 0 {
		return nil, orm.ErrSQLNoRows
	}
	return response[0].Records, nil
}

// SelectManySQLParameterized executes multiple parameterized SQL queries
func (c *Client) SelectManySQLParameterized(paramSQLs []orm.ParametereizedSQL) ([]orm.DBRecords, error) {
	req := &suresql.SQLRequest{
		ParamSQL:  paramSQLs,
		SingleRow: false,
	}

	// response, err := c.executeReadSQLQueryRequest("/db/api/querysql", req)
	response, err := sendRequest[suresql.QueryResponseSQL](c, "POST", "/db/api/querysql", req, IS_READ, AUTO_REFRESH, FALLBACK_LEADER)
	if err != nil {
		return nil, err
	}
	// let user know this is not error, just no rows found
	if len(response) == 0 {
		return nil, orm.ErrSQLNoRows
	}

	// Convert QueryREsponseSQL into []orm.DBRecords
	var allRecords []orm.DBRecords
	for _, resp := range response {
		allRecords = append(allRecords, resp.Records)
	}
	return allRecords, nil
}

// SelectOnlyOneSQLParameterized executes a parameterized SQL query that should return only one row
func (c *Client) SelectOnlyOneSQLParameterized(paramSQL orm.ParametereizedSQL) (orm.DBRecord, error) {
	req := &suresql.SQLRequest{
		ParamSQL:  []orm.ParametereizedSQL{paramSQL},
		SingleRow: true,
	}

	// response, err := c.executeReadSQLQueryRequest("/db/api/querysql", req)
	response, err := sendRequest[suresql.QueryResponseSQL](c, "POST", "/db/api/querysql", req, IS_READ, AUTO_REFRESH, FALLBACK_LEADER)
	if err != nil {
		return orm.DBRecord{}, err
	}
	// let user know this is not error, just no rows found
	if len(response) == 0 || len(response[0].Records) == 0 {
		return orm.DBRecord{}, orm.ErrSQLNoRows
	}

	// Because this function is meant to check if it's only 1 row return
	if len(response[0].Records) > 1 {
		return orm.DBRecord{}, orm.ErrSQLMoreThanOneRow
	}
	return response[0].Records[0], nil
}

//------------------------------------------------------------------
// ORM SQL EXECUTION METHODS
//------------------------------------------------------------------

// ExecOneSQL executes a single SQL statement
func (c *Client) ExecOneSQL(sql string) orm.BasicSQLResult {
	req := &suresql.SQLRequest{
		Statements: []string{sql},
	}

	// response, err := c.executeWriteSQLRequest("/db/api/sql", req)
	response, err := sendRequest[suresql.SQLResponse](c, "POST", "/db/api/sql", req, IS_WRITE, AUTO_REFRESH, FALLBACK_LEADER)
	if err != nil {
		return orm.BasicSQLResult{Error: err}
	}

	if len(response.Results) == 0 {
		return orm.BasicSQLResult{Error: errors.New("no results returned")}
	}

	return response.Results[0]
}

// ExecOneSQLParameterized executes a single parameterized SQL statement
func (c *Client) ExecOneSQLParameterized(paramSQL orm.ParametereizedSQL) orm.BasicSQLResult {
	req := &suresql.SQLRequest{
		ParamSQL: []orm.ParametereizedSQL{paramSQL},
	}

	// response, err := c.executeWriteSQLRequest("/db/api/sql", req)
	response, err := sendRequest[suresql.SQLResponse](c, "POST", "/db/api/sql", req, IS_WRITE, AUTO_REFRESH, FALLBACK_LEADER)
	if err != nil {
		return orm.BasicSQLResult{Error: err}
	}

	if len(response.Results) == 0 {
		return orm.BasicSQLResult{Error: errors.New("no results returned")}
	}

	return response.Results[0]
}

// ExecManySQL executes multiple SQL statements
func (c *Client) ExecManySQL(sqlStatements []string) ([]orm.BasicSQLResult, error) {
	req := &suresql.SQLRequest{
		Statements: sqlStatements,
	}

	// response, err := c.executeWriteSQLRequest("/db/api/sql", req)
	response, err := sendRequest[suresql.SQLResponse](c, "POST", "/db/api/sql", req, IS_WRITE, AUTO_REFRESH, FALLBACK_LEADER)
	if err != nil {
		return nil, err
	}

	if len(response.Results) == 0 {
		return nil, errors.New("no results returned")
	}

	return response.Results, nil
}

// ExecManySQLParameterized executes multiple parameterized SQL statements
func (c *Client) ExecManySQLParameterized(paramSQLs []orm.ParametereizedSQL) ([]orm.BasicSQLResult, error) {
	req := &suresql.SQLRequest{
		ParamSQL: paramSQLs,
	}

	// response, err := c.executeWriteSQLRequest("/db/api/sql", req)
	response, err := sendRequest[suresql.SQLResponse](c, "POST", "/db/api/sql", req, IS_WRITE, AUTO_REFRESH, FALLBACK_LEADER)
	if err != nil {
		return nil, err
	}

	if len(response.Results) == 0 {
		return nil, errors.New("no results returned")
	}

	return response.Results, nil
}

//------------------------------------------------------------------
// ORM INSERT METHODS
//------------------------------------------------------------------

// InsertOneDBRecord inserts a single record
func (c *Client) InsertOneDBRecord(record orm.DBRecord, queue bool) orm.BasicSQLResult {
	req := &suresql.InsertRequest{
		Records:   []orm.DBRecord{record},
		Queue:     queue,
		SameTable: true,
	}

	// response, err := c.executeWriteSQLRequest("/db/api/insert", req)
	response, err := sendRequest[suresql.SQLResponse](c, "POST", "/db/api/insert", req, IS_WRITE, AUTO_REFRESH, FALLBACK_LEADER)
	if err != nil {
		return orm.BasicSQLResult{Error: err}
	}

	if len(response.Results) == 0 {
		return orm.BasicSQLResult{Error: errors.New("no results returned")}
	}

	return response.Results[0]
}

// InsertManyDBRecords inserts multiple records
func (c *Client) InsertManyDBRecords(records []orm.DBRecord, queue bool) ([]orm.BasicSQLResult, error) {
	req := &suresql.InsertRequest{
		Records:   records,
		Queue:     queue,
		SameTable: false,
	}

	// response, err := c.executeWriteSQLRequest("/db/api/insert", req)
	response, err := sendRequest[suresql.SQLResponse](c, "POST", "/db/api/insert", req, IS_WRITE, AUTO_REFRESH, FALLBACK_LEADER)
	if err != nil {
		return nil, err
	}

	if len(response.Results) == 0 {
		return nil, errors.New("no results returned")
	}

	return response.Results, nil
}

// InsertManyDBRecordsSameTable inserts multiple records in the same table
func (c *Client) InsertManyDBRecordsSameTable(records []orm.DBRecord, queue bool) ([]orm.BasicSQLResult, error) {
	req := &suresql.InsertRequest{
		Records:   records,
		Queue:     queue,
		SameTable: true,
	}

	// response, err := c.executeWriteSQLRequest("/db/api/insert", req)
	response, err := sendRequest[suresql.SQLResponse](c, "POST", "/db/api/insert", req, IS_WRITE, AUTO_REFRESH, FALLBACK_LEADER)
	if err != nil {
		return nil, err
	}

	if len(response.Results) == 0 {
		return nil, errors.New("no results returned")
	}

	return response.Results, nil
}

// InsertOneTableStruct inserts a single table struct
func (c *Client) InsertOneTableStruct(record orm.TableStruct, queue bool) orm.BasicSQLResult {
	dbRecord, err := orm.TableStructToDBRecord(record)
	if err != nil {
		return orm.BasicSQLResult{Error: err}
	}

	return c.InsertOneDBRecord(dbRecord, queue)
}

// InsertManyTableStructs inserts multiple table structs
func (c *Client) InsertManyTableStructs(records []orm.TableStruct, queue bool) ([]orm.BasicSQLResult, error) {
	var dbRecords []orm.DBRecord

	for _, record := range records {
		dbRecord, err := orm.TableStructToDBRecord(record)
		if err != nil {
			return nil, err
		}
		dbRecords = append(dbRecords, dbRecord)
	}

	return c.InsertManyDBRecords(dbRecords, queue)
}

//------------------------------------------------------------------
// STATUS METHODS
//------------------------------------------------------------------

// IsConnected returns the connection status
func (c *Client) IsConnected() bool {
	// return c.Connected && (c.leaderConn != nil || len(c.readPool) > 0)
	return c.Connected && c.leaderConn != nil
}

// Leader returns the leader node of the cluster
func (c *Client) Leader() (string, error) {
	status, err := c.Status()
	if err != nil {
		return "", err
	}

	return status.Leader, nil
}

// Peers returns the peer nodes of the cluster
func (c *Client) Peers() ([]string, error) {
	status, err := c.Status()
	if err != nil {
		return nil, err
	}

	peers := make([]string, 0, len(status.Peers))
	for _, peer := range status.Peers {
		peers = append(peers, peer.URL)
	}

	return peers, nil
}

// Close properly cleans up resources and closes connections
func (c *Client) Close() {
	c.CloseConnections()
	c.Connected = false
}
