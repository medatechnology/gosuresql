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
// return is the raw JSON bytes of standardResponse.Data.
func (c *Client) sendRequestToLeader(method, endpoint string, body interface{}, withToken, autorefresh bool) ([]byte, error) {
	// if this is called for the first time, maybe from connect, but it shouldn't be because the newClient will create this
	if c.leaderConn == nil {
		c.leaderConn = NewConnection(&c.Config, "", "", "", true, suresql.TokenTable{})
	}
	return c.sendRequestToPool(c.leaderConn, method, endpoint, body, withToken, autorefresh, NO_FALLBACK)
}

// This will send http call with option of autorefresh
// return is the raw JSON bytes of standardResponse.Data.
func (c *Client) sendRequestToPool(conn *Connection, method, endpoint string, body interface{}, withToken, autorefresh, fallback bool) ([]byte, error) {
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
		// AutoRefresh logic: only for 401 (token expired). resp can be nil on
		// transport errors — check before touching it.
		if autorefresh && resp != nil && resp.StatusCode == http.StatusUnauthorized && withToken {
			err = conn.tryRefreshAndRenew(&c.Config)
			if err == nil {
				// 2nd try if auto-refresh
				resp, err = conn.sendHttpRequest(method, endpoint, body, &c.Config, withToken)
				if err != nil {
					if resp != nil {
						resp.Body.Close()
					}
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
		conn = c.leaderConn
	}
	defer c.markRequestComplete(conn, isWrite)
	// rawData is the raw JSON bytes of `data` — decode ONCE straight into T
	// (no intermediate map + marshal round trip).
	rawData, err := c.sendRequestToPool(conn, method, endpoint, body, WITH_TOKEN, autorefresh, fallback)
	if err != nil {
		return typedResp, err
	}
	if err = json.Unmarshal(rawData, &typedResp); err != nil {
		err = fmt.Errorf("failed to unmarshal response: %w", err)
	}
	return typedResp, err
}

// Type-specific request helpers were collapsed into the generic sendRequest[T]
// wrapper above (rawData → JSON marshal/unmarshal → T); no per-endpoint
// duplication remains.
