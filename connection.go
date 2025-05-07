package client

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"time"

	"github.com/medatechnology/goutil/object"
	"github.com/medatechnology/suresql"
)

// helper function to return map to be used in Request Body for login
func userCredentialsFromConfig(config *ClientConfig) map[string]string {
	// Login information needed for /connect
	return map[string]string{
		"username": config.Username,
		"password": config.Password,
	}
}

func NewHTTPClient(config *HTTPClientConfig) *http.Client {
	// Use config's HTTP client configuration or create a default one
	if config == nil {
		config = NewHTTPClientConfig()
	}
	timeout := config.Timeout
	if timeout == 0 {
		timeout = DEFAULT_TIMEOUT
	}
	return &http.Client{
		Timeout: timeout,
		Transport: &http.Transport{
			Dial: (&net.Dialer{
				Timeout:   config.DialTimeout,
				KeepAlive: config.KeepAlive,
			}).Dial,
			TLSHandshakeTimeout:   config.TLSHandshakeTimeout,
			ResponseHeaderTimeout: config.ResponseHeaderTimeout,
			ExpectContinueTimeout: config.ExpectContinueTimeout,
			MaxIdleConns:          config.MaxIdleConns,
			MaxIdleConnsPerHost:   config.MaxIdleConnsPerHost,
			MaxConnsPerHost:       config.MaxConnsPerHost,
			IdleConnTimeout:       config.IdleConnTimeout,
		}}
}

// Create new connection object, not yet connected to the url
func NewConnection(config *ClientConfig, url, nodeID, mode string, leader bool, token suresql.TokenTable) *Connection {
	// Use config's HTTP client configuration or create a default one
	client := NewHTTPClient(config.HTTPClientConfig)
	return NewConnectionWithClient(config, url, nodeID, mode, leader, token, client)
}

// NewConnectionWithClient creates a connection with a shared HTTP client
func NewConnectionWithClient(config *ClientConfig, url, nodeID, mode string,
	leader bool, token suresql.TokenTable,
	httpClient *http.Client) *Connection {
	// default values if none, that means it's for the leader
	if nodeID == "" {
		nodeID = "0" // leader
	}
	if mode == "" {
		mode = "rw" // read-write
	}
	if url == "" {
		url = config.ServerURL
	}
	now := time.Now()

	return &Connection{
		URL:         url,
		HTTPClient:  httpClient,
		Token:       token,
		IsLeader:    leader,
		Mode:        mode,
		NodeID:      nodeID,
		Created:     now,
		LastUsed:    now,
		LastRefresh: now,
	}
}

// getOrCreateNodeHTTPClient gets or creates an HTTP client for a node
func (c *Client) getOrCreateNodeHTTPClient(nodeID string) *http.Client {
	// Use the client pool mutex to ensure thread safety
	c.readPool.mutex.Lock()
	defer c.readPool.mutex.Unlock()

	// Check if we already have a client for this node
	if client, exists := c.readPool.nodeHTTPClients[nodeID]; exists {
		return client
	}

	// Create a new HTTP client with the specified configuration
	client := NewHTTPClient(nil)

	// Store the client for future use
	if c.readPool.nodeHTTPClients == nil {
		c.readPool.nodeHTTPClients = make(map[string]*http.Client)
	}
	c.readPool.nodeHTTPClients[nodeID] = client

	// Also set in write pool for consistency
	c.writePool.mutex.Lock()
	if c.writePool.nodeHTTPClients == nil {
		c.writePool.nodeHTTPClients = make(map[string]*http.Client)
	}
	c.writePool.nodeHTTPClients[nodeID] = client
	c.writePool.mutex.Unlock()

	return client
}

// Create new connection then connect it (to get token)
func (c *Client) createAndConnectNewConnection(url, nodeID, mode string, leader bool) (*Connection, error) {
	var conn *Connection

	if c.PoolConfig.NodeUseMultiClient {
		// Original behavior - create a new HTTP client for each connection
		conn = NewConnection(&c.Config, url, nodeID, mode, leader, suresql.TokenTable{})
	} else {
		// New behavior - share HTTP client by node
		httpClient := c.getOrCreateNodeHTTPClient(nodeID)
		conn = NewConnectionWithClient(&c.Config, url, nodeID, mode, leader, suresql.TokenTable{}, httpClient)
	}
	// conn := NewConnection(&c.Config, url, nodeID, mode, leader, suresql.TokenTable{})
	// fmt.Println("Creating new connection: ", url, nodeID, mode, leader)
	err := conn.newOrRefreshToken(&c.Config, CALL_CONNECT)
	if err != nil {
		return nil, err
	}
	return conn, nil
}

//------------------------------------------------------------------
// CORE HTTP AND RESPONSE FUNCTIONS
//------------------------------------------------------------------

// sendHttpRequest is the core HTTP function that all other methods build upon
// It can work with either the client's HTTPClient or a connection's HTTPClient
// func (c *Client) sendHttpRequest(httpClient *http.Client, url string, method string,
// 	endpoint string, data interface{}, authToken string) (*http.Response, error) {
// 	var body io.Reader
// 	if data != nil {
// 		jsonData, err := json.Marshal(data)
// 		if err != nil {
// 			return nil, fmt.Errorf("failed to marshal request data: %w", err)
// 		}
// 		body = bytes.NewBuffer(jsonData)
// 	}

// 	fullUrl := url + endpoint
// 	req, err := http.NewRequest(method, fullUrl, body)
// 	if err != nil {
// 		return nil, fmt.Errorf("failed to create request: %w", err)
// 	}

// 	// Set common headers
// 	req.Header.Set("Content-Type", "application/json")
// 	req.Header.Set("API_KEY", c.Config.APIKey)
// 	req.Header.Set("CLIENT_ID", c.Config.ClientID)

// 	// Set authorization if token provided
// 	if authToken != "" {
// 		req.Header.Set("Authorization", "Bearer "+authToken)
// 	}

// 	// Do the actual HTTP request
// 	return httpClient.Do(req)
// }

// Preparing standard request, using APIKEY and CLIENTID
func (c *Connection) createHttpRequest(method, endpoint string, data interface{}, config *ClientConfig) (*http.Request, error) {
	var body io.Reader
	if data != nil {
		jsonData, err := json.Marshal(data)
		if err != nil {
			return nil, fmt.Errorf("failed to marshal request data: %w", err)
		}
		body = bytes.NewBuffer(jsonData)
	}

	fullUrl := c.URL + endpoint
	req, err := http.NewRequest(method, fullUrl, body)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	// Set common headers
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("API_KEY", config.APIKey)
	req.Header.Set("CLIENT_ID", config.ClientID)
	return req, err
}

// Making HTTP call
func (c *Connection) sendHttpRequest(method, endpoint string, data interface{}, config *ClientConfig, withToken bool) (*http.Response, error) {
	// prepare standard request
	req, err := c.createHttpRequest(method, endpoint, data, config)
	if err != nil {
		return nil, err
	}

	// Set authorization if token provided
	if withToken && c.Token.Token != "" {
		req.Header.Set("Authorization", "Bearer "+c.Token.Token)
	}
	// Do the actual HTTP request
	return c.HTTPClient.Do(req)
}

// decode the response into StandardResponse which has status and then check if it's not OK
// If it's OK then return just the Data part.
func (c *Connection) getAndCheckResponseData(resp *http.Response) (interface{}, error) {
	defer resp.Body.Close()
	// if resp.StatusCode != http.StatusOK {
	// 	return nil, fmt.Errorf("request error: %s", resp.Status)
	// }
	var result suresql.StandardResponse
	err := json.NewDecoder(resp.Body).Decode(&result)
	if err != nil {
		return nil, fmt.Errorf("failed to decode response: %w", err)
	}

	if result.Status != http.StatusOK {
		return nil, fmt.Errorf("request error: %s", result.Message)
	}

	return result.Data, nil
}

// Just repetitive check for sending http request withToken==true, then it will check first if token exist
func (c *Connection) getAndCheckToken(withToken bool) error {
	if withToken {
		if c.Token.Token == "" {
			return fmt.Errorf("authentication required but no token available for %s", c.NodeID)
		}
	}
	return nil
}

// NOTE: might not needed.
// func getAndCheckResponseData(resp *http.Response) (interface{}, error) {
// 	defer resp.Body.Close()
// 	// if resp.StatusCode != http.StatusOK {
// 	// 	return nil, fmt.Errorf("request error: %s", resp.Status)
// 	// }
// 	var result suresql.StandardResponse
// 	err := json.NewDecoder(resp.Body).Decode(&result)
// 	if err != nil {
// 		return nil, fmt.Errorf("failed to decode response: %w", err)
// 	}

// 	if result.Status != http.StatusOK {
// 		return nil, fmt.Errorf("request error: %s", result.Message)
// 	}

// 	return result.Data, nil
// }

// decodeResponse processes an HTTP response into the standard format
// This centralizes all response decoding in one place, returning only result.Data
// func decodeResponse(resp *http.Response) (interface{}, error) {
// 	defer resp.Body.Close()

// 	var result suresql.StandardResponse
// 	err := json.NewDecoder(resp.Body).Decode(&result)
// 	if err != nil {
// 		return nil, fmt.Errorf("failed to decode response: %w", err)
// 	}

// 	if result.Status != http.StatusOK {
// 		return nil, fmt.Errorf("request error: %s", result.Message)
// 	}

// 	return result.Data, nil
// }

//------------------------------------------------------------------
// CONNECTION MANAGEMENT
//------------------------------------------------------------------

// connectToPeer establishes a connection to a peer node
// func (c *Client) connectToPeer(peer orm.StatusStruct) (*Connection, error) {
// 	client := &http.Client{Timeout: c.HTTPClient.Timeout}

// 	resp, err := c.sendHttpRequest(client, peer.URL, "POST", "/db/connect", c.userCredentials("", ""), "")
// 	if err != nil {
// 		return nil, fmt.Errorf("connection to peer failed: %w", err)
// 	}

// 	// Process response
// 	data, err := decodeResponse(resp)
// 	if err != nil {
// 		return nil, fmt.Errorf("failed to decode peer connect response: %w", err)
// 	}

// 	// Extract token from response
// 	tokenData, ok := data.(map[string]interface{})
// 	if !ok {
// 		return nil, errors.New("unexpected response format from peer")
// 	}

// 	tokenObj := object.MapToStructSlow[suresql.TokenTable](tokenData)

// 	if tokenObj.Token == "" {
// 		return nil, errors.New("token not found in peer response")
// 	}

// 	now := time.Now()
// 	conn := &Connection{
// 		URL:         peer.URL,
// 		Token:       tokenObj,
// 		NodeID:      peer.NodeID,
// 		IsLeader:    peer.IsLeader,
// 		Mode:        peer.Mode,
// 		HTTPClient:  client,
// 		LastUsed:    now,
// 		Created:     now,
// 		LastRefresh: now,
// 	}

// 	return conn, nil
// }

// For existing connection (maybe when call send it failed) try to renew the token by:
// 1. First try to refresh using refresh token, if succeed then exit.
// 2. If refresh failed, try to renew by calling /connect
func (c *Connection) tryRefreshAndRenew(config *ClientConfig) error {
	err := c.newOrRefreshToken(config, true)
	if err != nil {
		// this means refresh failed, then re-connect again
		err = c.newOrRefreshToken(config, false)
	}
	return err
}

// Can be used to get new token (using /connect) or refresh token (using /refresh)
// for existing connection. It can be new connection, or existing but make sure it already
// have information such as URL
// If refresh==true then it's refresh, if refresh==false then it's creating new token
// refreshConnection attempts to refresh a connection's token
func (c *Connection) newOrRefreshToken(config *ClientConfig, refresh bool) error {
	var resp *http.Response
	var err error

	if refresh {
		// if refresh called /db/refresh
		if c.Token.Refresh == "" {
			return errors.New("no refresh token available for connection")
		}
		refreshReq := map[string]string{
			"refresh_token": c.Token.Refresh,
		}

		resp, err = c.sendHttpRequest("POST", "/db/refresh", refreshReq, config, NO_TOKEN)
		if err != nil {
			resp.Body.Close()
			return fmt.Errorf("refresh request failed: %w", err)
		}
	} else {
		// if new token called /db/connect
		resp, err = c.sendHttpRequest("POST", "/db/connect", userCredentialsFromConfig(config), config, NO_TOKEN)
		if err != nil {
			// resp.Body.Close()
			return fmt.Errorf("connect (new token) request failed: %w", err)
		}
	}

	// Process response (and also check)
	data, err := c.getAndCheckResponseData(resp)
	if err != nil {
		// any error, wether server error or unautorized, try again by using connect
		// return fmt.Errorf("failed to decode refresh response: %w", err)
		return err // already have err message from getAndCheckResponseData
	}

	// Extract token from response
	tokenObj, err := convertDataToToken(data)
	if err != nil {
		return err
	}

	c.Token = tokenObj
	c.LastRefresh = time.Now()
	fmt.Printf("Connection: %s=%s, get new token:%s\n", c.NodeID, c.URL, c.Token.Token)
	return nil
}

// Data is already extracted from StandardResponse.Data , convert to map first then to struct
func convertDataToToken(data interface{}) (suresql.TokenTable, error) {
	// Extract token from response
	tokenData, ok := data.(map[string]interface{})
	if !ok {
		return suresql.TokenTable{}, errors.New("unexpected response format")
	}

	tokenObj := object.MapToStructSlow[suresql.TokenTable](tokenData)

	if tokenObj.Token == "" || tokenObj.Refresh == "" {
		return tokenObj, errors.New("token not found in response")
	}
	return tokenObj, nil
}

// refreshConnection attempts to refresh a connection's token
// func (c *Client) refreshConnection(conn *Connection) error {
// 	if conn.Token.Refresh == "" {
// 		return errors.New("no refresh token available for connection")
// 	}

// 	refreshReq := map[string]string{
// 		"refresh_token": conn.Token.Refresh,
// 	}

// 	resp, err := c.sendHttpRequest(conn.HTTPClient, conn.URL, "POST", "/db/refresh", refreshReq, "")
// 	if err != nil {
// 		return fmt.Errorf("refresh request failed: %w", err)
// 	}

// 	// Process response
// 	data, err := decodeResponse(resp)
// 	if err != nil {
// 		return fmt.Errorf("failed to decode refresh response: %w", err)
// 	}

// 	// Extract token from response
// 	tokenData, ok := data.(map[string]interface{})
// 	if !ok {
// 		return errors.New("unexpected response format")
// 	}

// 	tokenObj := object.MapToStructSlow[suresql.TokenTable](tokenData)

// 	if tokenObj.Token == "" {
// 		return errors.New("token not found in response")
// 	}

// 	conn.Token = tokenObj
// 	conn.LastRefresh = time.Now()
// 	return nil
// }

// createNewConnection creates a new connection to a node
// func (c *Client) createNewConnection(nodeURL, nodeID string, isLeader bool, mode string) (*Connection, error) {
// 	// Create a new HTTP client
// 	client := &http.Client{Timeout: c.Config.HTTPTimeout}
// 	if client.Timeout == 0 {
// 		client.Timeout = DefaultTimeout
// 	}

// 	resp, err := c.sendHttpRequest(client, nodeURL, "POST", "/db/connect", c.userCredentials("", ""), "")
// 	if err != nil {
// 		return nil, fmt.Errorf("connection failed: %w", err)
// 	}

// 	// Process response
// 	data, err := decodeResponse(resp)
// 	if err != nil {
// 		return nil, fmt.Errorf("failed to decode connect response: %w", err)
// 	}

// 	// Extract token from response
// 	tokenData, ok := data.(map[string]interface{})
// 	if !ok {
// 		return nil, errors.New("unexpected response format")
// 	}

// 	tokenObj := object.MapToStructSlow[suresql.TokenTable](tokenData)

// 	if tokenObj.Token == "" {
// 		return nil, errors.New("token not found in response")
// 	}

// 	now := time.Now()
// 	conn := &Connection{
// 		URL:         nodeURL,
// 		Token:       tokenObj,
// 		NodeID:      nodeID,
// 		IsLeader:    isLeader,
// 		Mode:        mode,
// 		HTTPClient:  client,
// 		LastUsed:    now,
// 		Created:     now,
// 		LastRefresh: now,
// 	}

// 	return conn, nil
// }

// To simplify other code that need this checks.
// Usage:
//
// token, err := getAndCheckToken(requiresAuth, conn)
//
//	if err != nil {
//	  return nil, err
//	}

// sendConnectionRequest sends a request using a specific connection
// It handles connection-specific token refresh and failures
// func (c *Client) sendConnectionRequest(conn *Connection, method, endpoint string,
// 	data interface{}, requiresAuth bool) (interface{}, error) {
// 	if conn == nil {
// 		return nil, errors.New("no DB connection")
// 	}

// 	// Determine token to use
// 	token, err := conn.getAndCheckToken(requiresAuth)
// 	if err != nil {
// 		return nil, err
// 	}

// 	// Send the request
// 	resp, err := c.sendHttpRequest(conn.HTTPClient, conn.URL, method, endpoint, data, token)
// 	if err != nil || resp.StatusCode == http.StatusUnauthorized {
// 		// Handle connection failure
// 		newConn, refreshErr := c.handleConnectionFailure(conn,
// 			method != "POST" || (endpoint != "/db/api/sql" && endpoint != "/db/api/insert"))
// 		if refreshErr != nil {
// 			return nil, fmt.Errorf("request failed and couldn't recover connection: %w", err)
// 		}

// 		// Retry with new connection
// 		return c.sendConnectionRequest(newConn, method, endpoint, data, requiresAuth)
// 	}

// 	// Handle 401 Unauthorized by refreshing token and retrying
// 	if resp.StatusCode == http.StatusUnauthorized && requiresAuth {
// 		resp.Body.Close()

// 		// Try to refresh the connection's token
// 		err = c.refreshConnection(conn)
// 		if err != nil {
// 			// If refresh fails, try to create a new connection
// 			newConn, refreshErr := c.handleConnectionFailure(conn,
// 				method != "POST" || (endpoint != "/db/api/sql" && endpoint != "/db/api/insert"))
// 			if refreshErr != nil {
// 				return nil, fmt.Errorf("token refresh failed and couldn't recover connection: %w", err)
// 			}

// 			// Retry with new connection
// 			return c.sendConnectionRequest(newConn, method, endpoint, data, requiresAuth)
// 		}

// 		// Retry with refreshed token
// 		return c.sendConnectionRequest(conn, method, endpoint, data, requiresAuth)
// 	}

// 	// Update the last used time
// 	conn.LastUsed = time.Now()

// 	// Process response
// 	return decodeResponse(resp)
// }
