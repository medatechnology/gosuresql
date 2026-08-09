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

// decode the response envelope; on success returns the raw JSON bytes of
// `data` so callers decode ONCE into their concrete type (no intermediate
// map[string]interface{} + re-marshal round trip).
func (c *Connection) getAndCheckResponseData(resp *http.Response) ([]byte, error) {
	defer resp.Body.Close()
	var env struct {
		Status  int             `json:"status"`
		Message string          `json:"message"`
		Data    json.RawMessage `json:"data"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&env); err != nil {
		return nil, fmt.Errorf("failed to decode response: %w", err)
	}

	if env.Status != http.StatusOK {
		return nil, fmt.Errorf("request error: %s", env.Message)
	}

	return env.Data, nil
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

//------------------------------------------------------------------
// CONNECTION MANAGEMENT
//------------------------------------------------------------------

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
	return nil
}

// convertDataToToken unmarshals the raw `data` JSON bytes into a TokenTable.
func convertDataToToken(data []byte) (suresql.TokenTable, error) {
	var tokenObj suresql.TokenTable
	if err := json.Unmarshal(data, &tokenObj); err != nil {
		return tokenObj, fmt.Errorf("unexpected response format: %w", err)
	}
	if tokenObj.Token == "" || tokenObj.Refresh == "" {
		return tokenObj, errors.New("token not found in response")
	}
	return tokenObj, nil
}
