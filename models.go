// Package client provides integration with SureSQL server through HTTP calls,
// implementing the ORM interface to provide consistent access methods.
package client

import (
	"net/http"
	"os"
	"strconv"
	"sync"
	"time"

	utils "github.com/medatechnology/goutil"
	"github.com/medatechnology/goutil/object"
	orm "github.com/medatechnology/simpleorm"
	"github.com/medatechnology/suresql"
)

//-----------------------------------------------------------------------------
// Original constants
//-----------------------------------------------------------------------------

const (
	// Default HTTP timeouts
	DEFAULT_CONNECTION_TIMEOUT            = 60 * time.Second
	DEFAULT_TIMEOUT                       = 60 * time.Second
	DEFAULT_DIAL_TIMEOUT                  = 60 * time.Second
	DEFAULT_KEEP_ALIVE                    = 30 * time.Second
	DEFAULT_TLS_HANDSHAKE_TIMEOUT         = 30 * time.Second
	DEFAULT_RESPONSE_TIMEOUT              = 60 * time.Second
	DEFAULT_CONTINUE_TIMEOUT              = 5 * time.Second
	DEFAULT_MAX_IDLE_CONNECTIONS          = 100
	DEFAULT_MAX_IDLE_CONNECTIONS_PER_HOST = 100
	DEFAULT_MAX_CONNECTIONS_PER_HOST      = 1000
	DEFAULT_IDLE_CONNECTION_TIMEOUT       = 90 * time.Second

	//-----------------------------------------------------------------------------
	// Connection pool constants
	//-----------------------------------------------------------------------------
	// Default pool configuration values
	DEFAULT_MINIMUM_POOL_SIZE       = 5  // deprecated
	DEFAULT_MAXIMUM_POOL_SIZE       = 10 // for read pool operations
	DEFAULT_MAXIMUM_WRITE_POOL_SIZE = 1
	DEFAULT_SCALE_UP_TRESHOLD       = 10 // how many calls
	DEFAULT_IDLE_TIMEOUT            = 5 * time.Minute
	DEFAULT_SCALE_DOWN_INTERVAL     = 1 * time.Minute // frequencies for checking scale down needs
	DEFAULT_CONNECTION_TTL          = 1 * time.Hour
	DEFAULT_SCALE_UP_BATCH_SIZE     = 3
	DEFAULT_USAGE_WINDOW_SIZE       = 100

	// Request types
	RequestTypeQuery RequestType = iota
	RequestTypeSQLExec
	RequestTypeSQLQuery
	RequestTypeInsert

	// Response formats
	ResponseFormatSingleRecord ResponseFormat = iota
	ResponseFormatMultiRecord
	ResponseFormatSQLResult
	ResponseFormatMultiSQLResult
	ResponseFormatMultiRecordSets
	ResponseFormatSingleRecordOnly
)

//-----------------------------------------------------------------------------
// Original request type definitions
//-----------------------------------------------------------------------------

// RequestType defines the type of request being made
type RequestType int

// ResponseFormat defines the expected response format
type ResponseFormat int

//-----------------------------------------------------------------------------
// Connection pool configuration types
//-----------------------------------------------------------------------------

// PoolConfig defines configuration for the dynamic connection pool
type PoolConfig struct {
	MinPoolSize       int           // Minimum connections per node, deprecated, use ScaleUpBatchSize for minimum now!
	MaxPoolSize       int           // Maximum connections per node (from status.MaxPool)
	MaxWritePoolSize  int           // Maximum WRITE connections per node (from status.MaxWritePool, not yet implemented) for now take from the environment variables.
	ScaleUpThreshold  int           // Number of concurrent requests to trigger scaling up
	IdleTimeout       time.Duration // How long a connection can be idle before becoming eligible for removal
	ScaleDownInterval time.Duration // How often to check for idle connections to remove
	ConnectionTTL     time.Duration // Maximum lifetime of a connection before refresh/recreation
	ScaleUpBatchSize  int           // How many connections to add when scaling up AND also serves as minimum
	UsageWindowSize   int           // Size of the moving window for usage statistics
	// New field for HTTP client creation policy
	NodeUseMultiClient bool // If true, create one HTTP client per connection (original behavior)
	// If false, share one HTTP client per node (new optimized behavior)
}

// HTTPClientConfig defines configuration for HTTP client settings
type HTTPClientConfig struct {
	Timeout               time.Duration
	DialTimeout           time.Duration
	KeepAlive             time.Duration
	TLSHandshakeTimeout   time.Duration
	ResponseHeaderTimeout time.Duration
	ExpectContinueTimeout time.Duration
	MaxIdleConns          int
	MaxIdleConnsPerHost   int
	MaxConnsPerHost       int
	IdleConnTimeout       time.Duration
}

//-----------------------------------------------------------------------------
// Connection and pool metrics types
//-----------------------------------------------------------------------------

// Connection represents a single connection to a SureSQL node. Basically in the client-api calls relation
// There is no "open" connection, it's just a pool of tokens so we do not have to call connect again to get
// token and also it will automatically refresh or reconnect if necessary.
type Connection struct {
	URL         string             // Node URL
	Token       suresql.TokenTable // Connection-specific tokens
	NodeID      string             // Unique node identifier
	IsLeader    bool               // Whether this node is the leader
	Mode        string             // "r", "w", or "rw"
	HTTPClient  *http.Client       // Connection-specific HTTP client
	LastUsed    time.Time          // When this connection was last used
	LastRefresh time.Time          // When token was last refreshed
	Created     time.Time          // When this connection was created
}

// ConnectionStats tracks usage statistics for a specific node
type ConnectionStats struct {
	NodeID             string
	CurrentConnections int
	ActiveRequests     int         // Requests currently in progress
	LastScaleUp        time.Time   // When we last scaled up
	LastScaleDown      time.Time   // When we last scaled down
	UsageHistory       []time.Time // Recent request timestamps
	HistoryWindow      int         // Size of the usage history window
	HistoryMutex       sync.Mutex  // Protect usage history during updates
	LastCleanup        time.Time   // Last time we checked for idle connections
	ScaleUpEvents      int         // Counter for scale-up events
	ScaleDownEvents    int         // Counter for scale-down events
}

// ConnectionPool manages a pool of connections with node-level round-robin support
type ConnectionPool struct {
	nodeConnections       map[string][]*Connection // Connections organized by node ID
	nodeRoundRobinIndices map[string]int           // Current index for round-robin within each node
	nodeOrder             []string                 // Order of nodes for true node-level round-robin
	nodeOrderIndex        int                      // Current node index for node-level round-robin
	mutex                 sync.RWMutex             // Mutex for thread safety
	isWritePool           bool                     // Pool type (read or write)
	maxPool               int                      // Max read pool
	maxWritePool          int                      // Max write pool (usually 1 for atomic)
	nodeHTTPClients       map[string]*http.Client  // New field for HTTP client management
}

// PoolMetrics provides statistics for the connection pool
type PoolMetrics struct {
	TotalConnections   int                        // Total connections across all nodes
	ActiveRequests     int                        // Requests currently in progress
	ConnectionsPerNode map[string]NodePoolMetrics // Per-node metrics
	ScaleUpEvents      int                        // Number of scale-up events since start
	ScaleDownEvents    int                        // Number of scale-down events since start
	RequestsPerSecond  float64                    // Approximate RPS based on recent history
}

// NodePoolMetrics provides statistics for a single node's connection pool
type NodePoolMetrics struct {
	NodeID             string
	URL                string
	Mode               string
	CurrentConnections int
	ActiveRequests     int
	IdleConnections    int
	RecentRequests     int // Requests in the last minute
	LastScaleUp        time.Time
	LastScaleDown      time.Time
	ScaleUpEvents      int
	ScaleDownEvents    int
}

//-----------------------------------------------------------------------------
// Original client configuration types - enhanced with pool config
//-----------------------------------------------------------------------------

// ClientConfig holds configuration for a SureSQL client
type ClientConfig struct {
	ServerURL        string
	APIKey           string
	ClientID         string
	Username         string
	Password         string
	HTTPTimeout      time.Duration
	PoolConfig       *PoolConfig       // Optional pool configuration
	HTTPClientConfig *HTTPClientConfig // Optional HTTP client configuration
}

//-----------------------------------------------------------------------------
// Client struct - enhanced with connection pool fields
//-----------------------------------------------------------------------------

// Client represents a SureSQL client that connects to a SureSQL server via HTTP
// with support for connection pooling and dynamic scaling
// Client represents a SureSQL client that connects to a SureSQL server via HTTP
// with support for connection pooling and dynamic scaling
type Client struct {
	Config    ClientConfig
	Connected bool

	// Leader connection for initial setup and fallback
	leaderConn *Connection

	// Connection pools
	readPool  *ConnectionPool
	writePool *ConnectionPool

	// Dynamic pool scaling fields
	PoolConfig        PoolConfig
	statsPerNodeRead  map[string]*ConnectionStats
	statsPerNodeWrite map[string]*ConnectionStats
	scalingMutex      sync.Mutex

	// Cached cluster status information
	status *orm.NodeStatusStruct

	// Cleanup timer for idle connections
	cleanupTimer *time.Timer
	cleanupDone  chan struct{}
}

//-----------------------------------------------------------------------------
// Original request data type
//-----------------------------------------------------------------------------

// requestData is a generic container for different request payloads
// type requestData struct {
// 	// Query related fields
// 	QueryReq      *suresql.QueryRequest
// 	QueryReqSQL   *suresql.QueryRequestSQL
// 	SQLReq        *suresql.SQLRequest
// 	InsertReq     *suresql.InsertRequest
// 	ConnectReq    map[string]string
// 	RefreshReq    map[string]string
// 	RequestType   RequestType
// 	ExpectResults bool
// }

//-----------------------------------------------------------------------------
// Pool configuration helper functions
//-----------------------------------------------------------------------------

// PoolConfigOption defines a function that can modify a PoolConfig
type PoolConfigOption func(*PoolConfig)

// WithMinPoolSize sets the minimum pool size
func WithMinPoolSize(size int) PoolConfigOption {
	return func(config *PoolConfig) {
		config.MinPoolSize = size
	}
}

// WithMinPoolSize sets the minimum pool size
func WithMaxPoolSize(size int) PoolConfigOption {
	return func(config *PoolConfig) {
		config.MaxPoolSize = size
	}
}

// WithMaxWritePoolSize sets the maximum size for the write connection pool
func WithMaxWritePoolSize(size int) PoolConfigOption {
	return func(config *PoolConfig) {
		config.MaxWritePoolSize = size
	}
}

// WithScaleUpThreshold sets the threshold for scaling up
func WithScaleUpThreshold(threshold int) PoolConfigOption {
	return func(config *PoolConfig) {
		config.ScaleUpThreshold = threshold
	}
}

// WithIdleTimeout sets the idle timeout
func WithIdleTimeout(timeout time.Duration) PoolConfigOption {
	return func(config *PoolConfig) {
		config.IdleTimeout = timeout
	}
}

// WithScaleDownInterval sets the scale-down check interval
func WithScaleDownInterval(interval time.Duration) PoolConfigOption {
	return func(config *PoolConfig) {
		config.ScaleDownInterval = interval
	}
}

// WithConnectionTTL sets the maximum lifetime for a connection
func WithConnectionTTL(ttl time.Duration) PoolConfigOption {
	return func(config *PoolConfig) {
		config.ConnectionTTL = ttl
	}
}

// WithScaleUpBatchSize sets how many connections to add at once
func WithScaleUpBatchSize(size int) PoolConfigOption {
	return func(config *PoolConfig) {
		config.ScaleUpBatchSize = size
	}
}

// WithUsageWindowSize sets the size of the usage history window
func WithUsageWindowSize(size int) PoolConfigOption {
	return func(config *PoolConfig) {
		config.UsageWindowSize = size
	}
}

// WithNodeUseMultiClient sets whether to use multiple HTTP clients per node
func WithNodeUseMultiClient(useMulti bool) PoolConfigOption {
	return func(config *PoolConfig) {
		config.NodeUseMultiClient = useMulti
	}
}

// NewPoolConfig creates a pool configuration with the specified options
func NewPoolConfig(options ...PoolConfigOption) *PoolConfig {
	timeout := utils.GetEnvInt("SURESQL_POOL_IDLE_TIMEOUT", 0)
	interval := utils.GetEnvInt("SURESQL_SCALE_DOWN_INTERVAL", 0)
	ttl := utils.GetEnvInt("SURESQL_CONNECTION_TTL", 0)
	tmpBool, _ := strconv.ParseBool(os.Getenv("SURESQL_NODE_USE_MULTI_CLIENT"))

	config := PoolConfig {
		MinPoolSize:       utils.GetEnvInt("SURESQL_POOL_MINIMUM", DEFAULT_MINIMUM_POOL_SIZE),
		MaxPoolSize:       utils.GetEnvInt("SURESQL_POOL_MAXIMUM", DEFAULT_MAXIMUM_POOL_SIZE),
		MaxWritePoolSize:  utils.GetEnvInt("SURESQL_WRITE_POOL_MAXIMUM", DEFAULT_MAXIMUM_WRITE_POOL_SIZE),
		ScaleUpThreshold:  utils.GetEnvInt("SURESQL_SCALE_UP_THRESHOLD", DEFAULT_SCALE_UP_TRESHOLD),
		IdleTimeout:       ValueOrDefault(time.Duration(timeout)*time.Minute, DEFAULT_IDLE_TIMEOUT, DurationBiggerThanZero),
		ScaleDownInterval: ValueOrDefault(time.Duration(interval)*time.Minute, DEFAULT_SCALE_DOWN_INTERVAL, DurationBiggerThanZero),
		ConnectionTTL:     ValueOrDefault(time.Duration(ttl)*time.Minute, DEFAULT_CONNECTION_TTL, DurationBiggerThanZero),
		ScaleUpBatchSize:  utils.GetEnvInt("SURESQL_SCALE_UP_BATCH", DEFAULT_SCALE_UP_BATCH_SIZE),
		UsageWindowSize:   utils.GetEnvInt("SURESQL_USAGE_WINDOW", DEFAULT_USAGE_WINDOW_SIZE),
		NodeUseMultiClient: tmpBool,
	}
	for _, option := range options {
		option(&config)
	}
	return &config
}

//-----------------------------------------------------------------------------
// Client configuration helper functions
//-----------------------------------------------------------------------------

type ClientConfigOption func(*ClientConfig)

func NewClientConfig(options ...ClientConfigOption) ClientConfig {
	// tmpBool, _ := strconv.ParseBool(os.Getenv("DB_SSL"))
	tmpTimeout, _ := strconv.ParseInt(os.Getenv("SURESQL_HTTP_TIMEOUT"), 10, 64)

	config := ClientConfig{
		ServerURL:   utils.GetEnv("SURESQL_SERVER_URL", "http://localhost:8080"),
		APIKey:      utils.GetEnv("SURESQL_API_KEY", "development_api_key"),
		ClientID:    utils.GetEnv("SURESQL_CLIENT_ID", "development_client_id"),
		Username:    utils.GetEnv("SURESQL_USERNAME", "admin"),
		Password:    utils.GetEnv("SURESQL_PASSWORD", "admin"),
		HTTPTimeout: ValueOrDefault(time.Duration(tmpTimeout)*time.Second, DEFAULT_TIMEOUT, DurationBiggerThanZero),
		// PoolConfig: NewPoolConfig(),
	}
	for _, option := range options {
		option(&config)
	}
	return config
}

// Set the server URL for client
func WithServerURL(val string) ClientConfigOption {
	return func(config *ClientConfig) {
		config.ServerURL = val
	}
}

// Set the server URL for client
func WithApiKey(val string) ClientConfigOption {
	return func(config *ClientConfig) {
		config.APIKey = val
	}
}

// Set the server URL for client
func WithClientID(val string) ClientConfigOption {
	return func(config *ClientConfig) {
		config.ClientID = val
	}
}

// Set the server URL for client
func WithUsername(val string) ClientConfigOption {
	return func(config *ClientConfig) {
		config.Username = val
	}
}

// Set the server URL for client
func WithPassword(val string) ClientConfigOption {
	return func(config *ClientConfig) {
		config.Password = val
	}
}

// Set the server URL for client
func WithHttpTimeout(val time.Duration) ClientConfigOption {
	return func(config *ClientConfig) {
		config.HTTPTimeout = val
	}
}

// Set the server URL for client
func WithPoolConfig(val *PoolConfig) ClientConfigOption {
	return func(config *ClientConfig) {
		config.PoolConfig = val
	}
}

// Set the HTTP client configuration
func WithHTTPClientConfig(val *HTTPClientConfig) ClientConfigOption {
	return func(config *ClientConfig) {
		config.HTTPClientConfig = val
	}
}

//-----------------------------------------------------------------------------
// Client initialization function - enhanced with pool setup
//-----------------------------------------------------------------------------

// NewClient creates a new SureSQL client with the provided config and connection pooling
func NewClient(config ClientConfig) (*Client, error) {
	if config.HTTPTimeout == 0 {
		config.HTTPTimeout = DEFAULT_TIMEOUT
	}

	// Initialize pool config with defaults or provided values
	poolConfig := NewPoolConfig()

	// Override with user-provided values if any
	if config.PoolConfig != nil {
		poolConfig.MinPoolSize = ValueOrDefault(config.PoolConfig.MinPoolSize, poolConfig.MinPoolSize, IntBiggerThanZero)
		poolConfig.MaxPoolSize = ValueOrDefault(config.PoolConfig.MaxPoolSize, poolConfig.MaxPoolSize, IntBiggerThanZero)
		poolConfig.MaxWritePoolSize = ValueOrDefault(config.PoolConfig.MaxWritePoolSize, poolConfig.MaxWritePoolSize, IntBiggerThanZero)
		poolConfig.ScaleUpThreshold = ValueOrDefault(config.PoolConfig.ScaleUpThreshold, poolConfig.ScaleUpThreshold, IntBiggerThanZero)
		poolConfig.IdleTimeout = ValueOrDefault(config.PoolConfig.IdleTimeout, poolConfig.IdleTimeout, DurationBiggerThanZero)
		poolConfig.ScaleDownInterval = ValueOrDefault(config.PoolConfig.ScaleDownInterval, poolConfig.ScaleDownInterval, DurationBiggerThanZero)
		poolConfig.ConnectionTTL = ValueOrDefault(config.PoolConfig.ConnectionTTL, poolConfig.ConnectionTTL, DurationBiggerThanZero)
		poolConfig.ScaleUpBatchSize = ValueOrDefault(config.PoolConfig.ScaleUpBatchSize, poolConfig.ScaleUpBatchSize, IntBiggerThanZero)
		poolConfig.UsageWindowSize = ValueOrDefault(config.PoolConfig.UsageWindowSize, poolConfig.UsageWindowSize, IntBiggerThanZero)
	}

	// Initialize HTTP client config if not provided
	if config.HTTPClientConfig == nil {
		config.HTTPClientConfig = NewHTTPClientConfig()
	}

	client := &Client{
		// URL:           config.ServerURL,
		// HTTPClient:    &http.Client{Timeout: timeout},
		// leaderConn: &Connection{
		// 	URL:        config.ServerURL,
		// 	HTTPClient: &http.Client{Timeout: config.HTTPTimeout},
		// 	IsLeader:   true,
		// 	Created:    time.Now(),
		// 	Mode:       "rw", // QUESTION: default?
		// 	NodeID:     "1",  // QUESTION: default?
		// },
		Config:            config,
		leaderConn:        NewConnection(&config, "", "", "", true, suresql.TokenTable{}),
		readPool:          NewConnectionPool(IS_READ, poolConfig.MaxPoolSize, poolConfig.MaxWritePoolSize),  // Read pool
		writePool:         NewConnectionPool(IS_WRITE, poolConfig.MaxPoolSize, poolConfig.MaxWritePoolSize), // Write pool
		statsPerNodeRead:  make(map[string]*ConnectionStats),
		statsPerNodeWrite: make(map[string]*ConnectionStats),
		PoolConfig:        *poolConfig,
	}
	// Connect to server to get a token
	// if config.Username != "" && config.Password != "" {
	// 	err := client.Connect(config.Username, config.Password)
	// 	if err != nil {
	// 		return nil, err
	// 	}
	// }
	return client, nil
}

type ComparisonFunction[T comparable] func(a, b T) bool

func IntBiggerThanZero(a, b int) bool {
	return a > 0
}

func DurationBiggerThanZero(a, b time.Duration) bool {
	return a > 0
}

func ValueOrDefault[T comparable](value, preset T, compF ComparisonFunction[T]) T {
	if compF(value, preset) {
		return value
	}
	return preset
}

//-----------------------------------------------------------------------------
// HTTP Client Config initialization function - enhanced with pool setup
//-----------------------------------------------------------------------------

// NewHTTPClientConfig creates an HTTP client configuration with the specified options
func NewHTTPClientConfig(options ...HTTPClientConfigOption) *HTTPClientConfig {
	timeout := object.IntPlus(utils.GetEnv("SURESQL_HTTP_TIMEOUT", ""), 0)
	dialTimeout := object.IntPlus(utils.GetEnv("SURESQL_HTTP_DIAL_TIMEOUT", ""), 0)
	keepAlive := object.IntPlus(utils.GetEnv("SURESQL_HTTP_KEEP_ALIVE", ""), 0)
	tlsTimeout := object.IntPlus(utils.GetEnv("SURESQL_HTTP_TLS_TIMEOUT", ""), 0)
	responseTimeout := object.IntPlus(utils.GetEnv("SURESQL_HTTP_RESPONSE_TIMEOUT", ""), 0)
	continueTimeout := object.IntPlus(utils.GetEnv("SURESQL_HTTP_CONTINUE_TIMEOUT", ""), 0)
	maxIdle := object.IntPlus(utils.GetEnv("SURESQL_HTTP_MAX_IDLE_CONNECTION", ""), 0)
	maxIdlePerHost := object.IntPlus(utils.GetEnv("SURESQL_HTTP_MAX_IDLE_CONNS_PER_HOST", ""), 0)
	maxConnsPerHost := object.IntPlus(utils.GetEnv("SURESQL_HTTP_MAX_CONNS_PER_HOST", ""), 0)
	idleConnTimeout := object.IntPlus(utils.GetEnv("SURESQL_HTTP_IDLE_CONN_TIMEOUT", ""), 0)

	config := HTTPClientConfig{
		Timeout:               ValueOrDefault(time.Duration(timeout)*time.Second, DEFAULT_TIMEOUT, DurationBiggerThanZero),
		DialTimeout:           ValueOrDefault(time.Duration(dialTimeout)*time.Second, DEFAULT_DIAL_TIMEOUT, DurationBiggerThanZero),
		KeepAlive:             ValueOrDefault(time.Duration(keepAlive)*time.Second, DEFAULT_KEEP_ALIVE, DurationBiggerThanZero),
		TLSHandshakeTimeout:   ValueOrDefault(time.Duration(tlsTimeout)*time.Second, DEFAULT_TLS_HANDSHAKE_TIMEOUT, DurationBiggerThanZero),
		ResponseHeaderTimeout: ValueOrDefault(time.Duration(responseTimeout)*time.Second, DEFAULT_RESPONSE_TIMEOUT, DurationBiggerThanZero),
		ExpectContinueTimeout: ValueOrDefault(time.Duration(continueTimeout)*time.Second, DEFAULT_CONTINUE_TIMEOUT, DurationBiggerThanZero),
		MaxIdleConns:          ValueOrDefault(maxIdle, DEFAULT_MAX_IDLE_CONNECTIONS, IntBiggerThanZero),
		MaxIdleConnsPerHost:   ValueOrDefault(maxIdlePerHost, DEFAULT_MAX_IDLE_CONNECTIONS_PER_HOST, IntBiggerThanZero),
		MaxConnsPerHost:       ValueOrDefault(maxConnsPerHost, DEFAULT_MAX_CONNECTIONS_PER_HOST, IntBiggerThanZero),
		IdleConnTimeout:       ValueOrDefault(time.Duration(idleConnTimeout)*time.Second, DEFAULT_IDLE_CONNECTION_TIMEOUT, DurationBiggerThanZero),
	}

	for _, option := range options {
		option(&config)
	}

	return &config
}

//-----------------------------------------------------------------------------
// HTTP Client configuration helper functions
//-----------------------------------------------------------------------------

// HTTPClientConfigOption defines a function that can modify an HTTPClientConfig
type HTTPClientConfigOption func(*HTTPClientConfig)

// WithDialTimeout sets the dial timeout
func WithTimeout(timeout time.Duration) HTTPClientConfigOption {
	return func(config *HTTPClientConfig) {
		config.Timeout = timeout
	}
}

// WithDialTimeout sets the dial timeout
func WithDialTimeout(timeout time.Duration) HTTPClientConfigOption {
	return func(config *HTTPClientConfig) {
		config.DialTimeout = timeout
	}
}

// WithKeepAlive sets the keep-alive duration
func WithKeepAlive(keepAlive time.Duration) HTTPClientConfigOption {
	return func(config *HTTPClientConfig) {
		config.KeepAlive = keepAlive
	}
}

// WithTLSHandshakeTimeout sets the TLS handshake timeout
func WithTLSHandshakeTimeout(timeout time.Duration) HTTPClientConfigOption {
	return func(config *HTTPClientConfig) {
		config.TLSHandshakeTimeout = timeout
	}
}

// WithResponseHeaderTimeout sets the response header timeout
func WithResponseHeaderTimeout(timeout time.Duration) HTTPClientConfigOption {
	return func(config *HTTPClientConfig) {
		config.ResponseHeaderTimeout = timeout
	}
}

// WithExpectContinueTimeout sets the expect continue timeout
func WithExpectContinueTimeout(timeout time.Duration) HTTPClientConfigOption {
	return func(config *HTTPClientConfig) {
		config.ExpectContinueTimeout = timeout
	}
}

// WithMaxIdleConns sets the maximum idle connections
func WithMaxIdleConns(max int) HTTPClientConfigOption {
	return func(config *HTTPClientConfig) {
		config.MaxIdleConns = max
	}
}

// WithMaxIdleConnsPerHost sets the maximum idle connections per host
func WithMaxIdleConnsPerHost(max int) HTTPClientConfigOption {
	return func(config *HTTPClientConfig) {
		config.MaxIdleConnsPerHost = max
	}
}

// WithMaxConnsPerHost sets the maximum connections per host
func WithMaxConnsPerHost(max int) HTTPClientConfigOption {
	return func(config *HTTPClientConfig) {
		config.MaxConnsPerHost = max
	}
}

// WithIdleConnTimeout sets the idle connection timeout
func WithIdleConnTimeout(timeout time.Duration) HTTPClientConfigOption {
	return func(config *HTTPClientConfig) {
		config.IdleConnTimeout = timeout
	}
}
