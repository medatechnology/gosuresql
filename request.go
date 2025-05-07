package client

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"

	"github.com/medatechnology/suresql"
)

// Username and password in the body of request. Mainly use to connect/login/refresh etc
func (c *Client) userCredentialsDefault(username, password string) map[string]string {
	// If empty get from config
	if username == "" || password == "" {
		return userCredentialsFromConfig(&c.Config)
	}
	return map[string]string{
		"username": username,
		"password": password,
	}
}

//------------------------------------------------------------------
// DIRECT CLIENT REQUESTS
//------------------------------------------------------------------

// send Request using leader connection, if not exist create it
// return is standardResponse.Data which is of type interface{}
func (c *Client) sendRequestToLeader(method, endpoint string, body interface{}, withToken, autorefresh bool) (interface{}, error) {
	// if this is called for the first time, maybe from connect, but it shouldn't be because the newClient will create this
	if c.leaderConn == nil {
		// c.leaderConn = &Connection{
		// 	URL:        c.Config.ServerURL,
		// 	IsLeader:   true,
		// 	HTTPClient: &http.Client{Timeout: c.Config.HTTPTimeout},
		// 	Created:    time.Now(),
		// 	Mode:       "rw", // QUESTION: default?
		// 	NodeID:     "0",  // QUESTION: default?
		// }
		c.leaderConn = NewConnection(&c.Config, "", "", "", true, suresql.TokenTable{})
	}
	return c.sendRequestToPool(c.leaderConn, method, endpoint, body, withToken, autorefresh, NO_FALLBACK)
}

// This will send http call with option of autorefresh
// return is standardResponse.Data which is of type interface{}
func (c *Client) sendRequestToPool(conn *Connection, method, endpoint string, body interface{}, withToken, autorefresh, fallback bool) (interface{}, error) {
	// double check connection is there
	if conn == nil {
		return nil, errors.New("no DB connection")
	}
	// Cannot set autorefresh (token) when withToken is false
	if !withToken && autorefresh {
		autorefresh = false
	}

	if err := conn.getAndCheckToken(withToken); err != nil {
		return nil, err
	}

	resp, err := conn.sendHttpRequest(method, endpoint, body, &c.Config, withToken)
	if err != nil {
		// AutoRefresh logic, if it's on, make sure the error is UnAuthorized (which is token expires)
		// NOTE: neede to check resp!= nil first, sometimes it is nil and create panic
		// resp.Body.Close()
		if autorefresh && resp != nil && resp.StatusCode == http.StatusUnauthorized && withToken {
			err = conn.tryRefreshAndRenew(&c.Config)
			if err == nil {
				// 2nd try if auto-refresh
				resp, err = conn.sendHttpRequest(method, endpoint, body, &c.Config, withToken)
				if err != nil {
					resp.Body.Close()
					return nil, fmt.Errorf("api-call failed, after refresh success, err: %w", err)
				}
			}
		}
		// other error or auto-refresh=false + other error, check if there is fallback to leader (and current connection is not already leader!)
		// NOTE: this err!= nil is important because it could be carry over error from refresh and 2nd try sendRequest
		if err != nil {
			if fallback && conn != c.leaderConn {
				// could also return c.sendRequestToLeader but the error won't say this is the leader fallback
				data, err := c.sendRequestToLeader(method, endpoint, body, withToken, autorefresh)
				if err != nil {
					// resp.Body.Close()
					return nil, fmt.Errorf("api-call fallback to leader failed, err:%w", err)
				}
				return data, err
			} else {
				return nil, fmt.Errorf("api-call failed, err: %w", err)
			}
		}
	}
	// process the response and return only the Data part
	return conn.getAndCheckResponseData(resp)
}

//------------------------------------------------------------------
// CONNECTION SELECTION AND EXECUTION
//------------------------------------------------------------------

// send Request using leader connection, if not exist create it
// func (c *Client) sendReadRequest(method, endpoint string, body interface{}, withToken, autorefresh, fallback bool) (interface{}, error) {
// 	conn, err := c.getReadConnection()
// 	if err != nil {
// 		return c.sendRequestToLeader(method, endpoint, body, withToken, autorefresh)
// 	}
// 	// Track the request metrics
// 	defer c.markRequestComplete(conn)

// 	// Execute with the read connection`
// 	return c.sendRequestToPool(conn, method, endpoint, body, withToken, autorefresh, fallback)
// }

// Generic sendRequest wrapper to call Connection.sendHttpRequest
// write=true means it's write operation (insert/update/delete) because we use different connection pool
// write=false means it's read operation (insert/update/delete)
// return is of type T which is generics, can be set from caller to be
// orm.NodeStatusStruct
// orm.SchemaStruct
// suresql.QueryResponse     - all singular query (basically is Records)
// suresql.QueryResponseSQL
// suresql.SQLResponse
// Converted using json.Marshal and json.Unmarshal to the generic types from  standardResponse.Data which is of type interface{}
// This function always requires token, which is connection essentially
func sendRequest[T any](c *Client, method, endpoint string, body interface{}, isWrite, autorefresh, fallback bool) (T, error) {
	var conn *Connection
	var err error
	var typedResp T
	var ok bool

	if isWrite {
		conn, err = c.getWriteConnection()
	} else {
		conn, err = c.getReadConnection()
	}
	if err != nil {
		// If no connection found, and not falling back, return error!
		if !fallback {
			return typedResp, err
		}
		// Fall back to direct request if no read connections
		fmt.Println("fallback to leader right away")
		conn = c.leaderConn
	}
	defer c.markRequestComplete(conn, isWrite)
	// fmt.Println("DEBUG: calling request to Pool")
	rawData, err := c.sendRequestToPool(conn, method, endpoint, body, WITH_TOKEN, autorefresh, fallback)
	if err != nil {
		return typedResp, err
	}

	// Convert to SQLResponse
	typedResp, ok = rawData.(T)
	if !ok {
		// If direct conversion failed, try marshal/unmarshal
		jsonData, errL := json.Marshal(rawData)
		if errL != nil {
			// return typedResp, fmt.Errorf("failed to marshal SQL response data: %w", err)
			err = fmt.Errorf("failed to marshal SQL response data: %w", errL)
		} else {
			if err = json.Unmarshal(jsonData, &typedResp); err != nil {
				// return typedResp, fmt.Errorf("failed to unmarshal SQL response: %w", err)
				err = fmt.Errorf("failed to unmarshal SQL response: %w", err)
			}
		}
	}
	return typedResp, err
}

// executeReadRequest gets a read connection and executes the request
// func (c *Client) executeReadRequest(method, endpoint string, data interface{}, requiresAuth bool) (interface{}, error) {
// 	conn, err := c.getReadConnection()
// 	if err != nil {
// 		// Fall back to direct request if no read connections
// 		return c.sendDirectRequest(method, endpoint, data, requiresAuth)
// 	}

// 	// Track the request metrics
// 	defer c.markRequestComplete(conn)

// 	// Execute with the read connection
// 	return c.sendConnectionRequest(conn, method, endpoint, data, requiresAuth)
// }

// executeWriteRequest gets the write connection and executes the request
// func (c *Client) executeWriteRequest(method, endpoint string, data interface{}, requiresAuth bool) (interface{}, error) {
// 	conn, err := c.getWriteConnection()
// 	if err != nil {
// 		// Fall back to direct request if no write connection
// 		return c.sendDirectRequest(method, endpoint, data, requiresAuth)
// 	}

// 	// Track the request metrics
// 	defer c.markRequestComplete(conn)

// 	// Execute with the write connection
// 	return c.sendConnectionRequest(conn, method, endpoint, data, requiresAuth)
// }

//------------------------------------------------------------------
// TYPE-SPECIFIC READ REQUESTS
//------------------------------------------------------------------

// executeReadQueryRequest executes a read request and returns QueryResponse directly
// func (c *Client) executeReadQueryRequest(endpoint string, data interface{}) (suresql.QueryResponse, error) {
// 	// Make the request
// 	rawData, err := c.executeReadRequest("POST", endpoint, data, true)
// 	if err != nil {
// 		return suresql.QueryResponse{}, err
// 	}

// 	// Convert to QueryResponse
// 	queryResp, ok := rawData.(suresql.QueryResponse)
// 	if ok {
// 		return queryResp, nil
// 	}

// 	// If direct conversion failed, try marshal/unmarshal
// 	jsonData, err := json.Marshal(rawData)
// 	if err != nil {
// 		return suresql.QueryResponse{}, fmt.Errorf("failed to marshal response data: %w", err)
// 	}

// 	var typedResp suresql.QueryResponse
// 	if err := json.Unmarshal(jsonData, &typedResp); err != nil {
// 		return suresql.QueryResponse{}, fmt.Errorf("failed to unmarshal response: %w", err)
// 	}

// 	return typedResp, nil
// }

// executeReadSQLQueryRequest executes a SQL read request and returns QueryResponseSQL directly
// func (c *Client) executeReadSQLQueryRequest(endpoint string, data interface{}) (suresql.QueryResponseSQL, error) {
// 	// Make the request
// 	rawData, err := c.executeReadRequest("POST", endpoint, data, true)
// 	if err != nil {
// 		return nil, err
// 	}

// 	// Convert to QueryResponseSQL
// 	queryResp, ok := rawData.(suresql.QueryResponseSQL)
// 	if ok {
// 		return queryResp, nil
// 	}

// 	// If direct conversion failed, try marshal/unmarshal
// 	jsonData, err := json.Marshal(rawData)
// 	if err != nil {
// 		return nil, fmt.Errorf("failed to marshal SQL query response data: %w", err)
// 	}

// 	var typedResp suresql.QueryResponseSQL
// 	if err := json.Unmarshal(jsonData, &typedResp); err != nil {
// 		return nil, fmt.Errorf("failed to unmarshal SQL query response: %w", err)
// 	}

// 	return typedResp, nil
// }

//------------------------------------------------------------------
// TYPE-SPECIFIC WRITE REQUESTS
//------------------------------------------------------------------

// executeWriteSQLRequest executes a write SQL request and returns SQLResponse directly
// func (c *Client) executeWriteSQLRequest(endpoint string, data interface{}) (suresql.SQLResponse, error) {
// 	// Make the request
// 	rawData, err := c.executeWriteRequest("POST", endpoint, data, true)
// 	if err != nil {
// 		return suresql.SQLResponse{}, err
// 	}

// 	// Convert to SQLResponse
// 	sqlResp, ok := rawData.(suresql.SQLResponse)
// 	if ok {
// 		return sqlResp, nil
// 	}

// 	// If direct conversion failed, try marshal/unmarshal
// 	jsonData, err := json.Marshal(rawData)
// 	if err != nil {
// 		return suresql.SQLResponse{}, fmt.Errorf("failed to marshal SQL response data: %w", err)
// 	}

// 	var typedResp suresql.SQLResponse
// 	if err := json.Unmarshal(jsonData, &typedResp); err != nil {
// 		return suresql.SQLResponse{}, fmt.Errorf("failed to unmarshal SQL response: %w", err)
// 	}

// 	return typedResp, nil
// }
