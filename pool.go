package client

import (
	"errors"
	"fmt"
	"net/http"
	"time"

	"github.com/medatechnology/suresql"
)

//-----------------------------------------------------------------------------
// ConnectionPool implementation
//-----------------------------------------------------------------------------

// NewConnectionPool creates a new connection pool
func NewConnectionPool(isWritePool bool, maxRead, maxWrite int) *ConnectionPool {
	return &ConnectionPool{
		nodeConnections:       make(map[string][]*Connection),
		nodeRoundRobinIndices: make(map[string]int),
		nodeOrder:             make([]string, 0),
		nodeOrderIndex:        0,
		isWritePool:           isWritePool,
		maxPool:               maxRead,
		maxWritePool:          maxWrite,
		nodeHTTPClients:       make(map[string]*http.Client),
	}
}

// Size returns the total number of connections in the pool
func (p *ConnectionPool) Size() int {
	p.mutex.RLock()
	defer p.mutex.RUnlock()

	count := 0
	for _, conns := range p.nodeConnections {
		count += len(conns)
	}
	return count
}

// SizeForNode returns the number of connections for a specific node
func (p *ConnectionPool) SizeForNode(nodeID string) int {
	p.mutex.RLock()
	defer p.mutex.RUnlock()

	if conns, exists := p.nodeConnections[nodeID]; exists {
		return len(conns)
	}
	return 0
}

// Add adds a connection to the pool
func (p *ConnectionPool) Add(conn *Connection) {
	p.mutex.Lock()
	defer p.mutex.Unlock()

	nodeID := conn.NodeID

	// Initialize node connections if needed
	if _, exists := p.nodeConnections[nodeID]; !exists {
		p.nodeConnections[nodeID] = make([]*Connection, 0)
		p.nodeRoundRobinIndices[nodeID] = 0

		// Add to node order for node-level round-robin
		p.nodeOrder = append(p.nodeOrder, nodeID)
	}

	// Add connection to the node's pool
	p.nodeConnections[nodeID] = append(p.nodeConnections[nodeID], conn)
}

// AddBatch adds multiple connections to the pool in a single operation
func (p *ConnectionPool) AddBatch(conns []*Connection) {
	if len(conns) == 0 {
		return
	}

	p.mutex.Lock()
	defer p.mutex.Unlock()

	// Group connections by node ID
	connsByNode := make(map[string][]*Connection)
	nodeIDs := make(map[string]bool)

	for _, conn := range conns {
		nodeID := conn.NodeID
		nodeIDs[nodeID] = true
		connsByNode[nodeID] = append(connsByNode[nodeID], conn)
	}

	// Add connections to their respective node pools
	for nodeID, nodeConns := range connsByNode {
		if _, exists := p.nodeConnections[nodeID]; !exists {
			p.nodeConnections[nodeID] = make([]*Connection, 0)
			p.nodeRoundRobinIndices[nodeID] = 0

			// Add to node order for node-level round-robin
			p.nodeOrder = append(p.nodeOrder, nodeID)
		}

		p.nodeConnections[nodeID] = append(p.nodeConnections[nodeID], nodeConns...)
	}
}

// Remove removes a specific connection from the pool
func (p *ConnectionPool) Remove(conn *Connection) bool {
	p.mutex.Lock()
	defer p.mutex.Unlock()

	nodeID := conn.NodeID
	nodeConns, exists := p.nodeConnections[nodeID]
	if !exists {
		return false
	}

	// Find and remove the connection
	for i, c := range nodeConns {
		if c == conn {
			// Remove the connection
			p.nodeConnections[nodeID] = append(nodeConns[:i], nodeConns[i+1:]...)

			// If this node has no more connections, remove it from the node order
			if len(p.nodeConnections[nodeID]) == 0 {
				delete(p.nodeConnections, nodeID)
				delete(p.nodeRoundRobinIndices, nodeID)

				// Clean up HTTP client if this is the last connection
				// Note: Only delete if we're the write pool (to avoid race conditions)
				if p.isWritePool {
					delete(p.nodeHTTPClients, nodeID)
				}
				for i, id := range p.nodeOrder {
					if id == nodeID {
						p.nodeOrder = append(p.nodeOrder[:i], p.nodeOrder[i+1:]...)

						// Adjust nodeOrderIndex if needed
						if p.nodeOrderIndex >= len(p.nodeOrder) && len(p.nodeOrder) > 0 {
							p.nodeOrderIndex = 0
						}
						break
					}
				}
			}

			return true
		}
	}

	return false
}

// GetConnection gets the next connection using true node-level round-robin
func (p *ConnectionPool) GetConnection() (*Connection, error) {
	p.mutex.Lock()
	defer p.mutex.Unlock()

	if len(p.nodeOrder) == 0 {
		return nil, errors.New("no connections available in pool")
	}

	// Start from current node index and try to find an available node
	startNodeIdx := p.nodeOrderIndex
	for i := 0; i < len(p.nodeOrder); i++ {
		nodeIdx := (startNodeIdx + i) % len(p.nodeOrder)
		nodeID := p.nodeOrder[nodeIdx]

		nodeConns := p.nodeConnections[nodeID]
		if len(nodeConns) > 0 {
			// Get connection from this node using round-robin
			connIdx := p.nodeRoundRobinIndices[nodeID]
			conn := nodeConns[connIdx]

			// Update round-robin indices
			p.nodeRoundRobinIndices[nodeID] = (connIdx + 1) % len(nodeConns)
			p.nodeOrderIndex = (nodeIdx + 1) % len(p.nodeOrder)

			// Update last used time
			conn.LastUsed = time.Now()

			return conn, nil
		}
	}

	return nil, errors.New("no connections available in pool despite having nodes")
}

// GetConnectionForNode gets a connection for a specific node
func (p *ConnectionPool) GetConnectionForNode(nodeID string) (*Connection, error) {
	p.mutex.Lock()
	defer p.mutex.Unlock()

	nodeConns, exists := p.nodeConnections[nodeID]
	if !exists || len(nodeConns) == 0 {
		return nil, fmt.Errorf("no connections available for node %s", nodeID)
	}

	// Get connection using round-robin
	connIdx := p.nodeRoundRobinIndices[nodeID]
	conn := nodeConns[connIdx]

	// Update round-robin index
	p.nodeRoundRobinIndices[nodeID] = (connIdx + 1) % len(nodeConns)

	// Update last used time
	conn.LastUsed = time.Now()

	return conn, nil
}

// GetAllConnections returns all connections in the pool
func (p *ConnectionPool) GetAllConnections() []*Connection {
	p.mutex.RLock()
	defer p.mutex.RUnlock()

	result := make([]*Connection, 0)
	for _, conns := range p.nodeConnections {
		result = append(result, conns...)
	}

	return result
}

// GetAllConnectionsForNode returns all connections for a specific node
func (p *ConnectionPool) GetAllConnectionsForNode(nodeID string) []*Connection {
	p.mutex.RLock()
	defer p.mutex.RUnlock()

	if conns, exists := p.nodeConnections[nodeID]; exists {
		// Return a copy to avoid race conditions
		result := make([]*Connection, len(conns))
		copy(result, conns)
		return result
	}

	return []*Connection{}
}

// GetIdleConnections returns connections that have been idle longer than the specified duration
func (p *ConnectionPool) GetIdleConnections(idleTimeout time.Duration) []*Connection {
	p.mutex.RLock()
	defer p.mutex.RUnlock()

	now := time.Now()
	idle := make([]*Connection, 0)

	for _, conns := range p.nodeConnections {
		for _, conn := range conns {
			if now.Sub(conn.LastUsed) > idleTimeout {
				idle = append(idle, conn)
			}
		}
	}

	return idle
}

// RemoveIdleConnections removes connections that have been idle longer than idleTimeout
// while respecting the minimum pool size
func (p *ConnectionPool) RemoveIdleConnections(idleTimeout time.Duration, minSizePerNode int) int {
	p.mutex.Lock()
	defer p.mutex.Unlock()

	now := time.Now()
	removed := 0

	for nodeID, conns := range p.nodeConnections {
		// Skip if already at or below minimum size
		if len(conns) <= minSizePerNode {
			continue
		}

		// Find idle connections
		var active []*Connection
		var idle []*Connection

		for _, conn := range conns {
			if now.Sub(conn.LastUsed) > idleTimeout {
				idle = append(idle, conn)
			} else {
				active = append(active, conn)
			}
		}

		// Determine how many we can remove while respecting min size
		canRemove := len(conns) - minSizePerNode
		willRemove := min(len(idle), canRemove)

		if willRemove <= 0 {
			continue
		}

		// Keep some idle connections if needed to maintain minimum size
		keepIdle := len(idle) - willRemove
		newConnList := active

		if keepIdle > 0 {
			newConnList = append(newConnList, idle[:keepIdle]...)
		}

		// Update the node's connection list
		p.nodeConnections[nodeID] = newConnList

		// Update round-robin index if needed
		if p.nodeRoundRobinIndices[nodeID] >= len(newConnList) {
			p.nodeRoundRobinIndices[nodeID] = 0
		}

		removed += willRemove

		// If node has no more connections, remove it from tracking
		if len(newConnList) == 0 {
			delete(p.nodeConnections, nodeID)
			delete(p.nodeRoundRobinIndices, nodeID)

			for i, id := range p.nodeOrder {
				if id == nodeID {
					p.nodeOrder = append(p.nodeOrder[:i], p.nodeOrder[i+1:]...)

					// Adjust nodeOrderIndex if needed
					if p.nodeOrderIndex >= len(p.nodeOrder) && len(p.nodeOrder) > 0 {
						p.nodeOrderIndex = 0
					}
					break
				}
			}
		}
	}

	return removed
}

// Clear removes all connections from the pool
func (p *ConnectionPool) Clear() {
	p.mutex.Lock()
	defer p.mutex.Unlock()

	p.nodeConnections = make(map[string][]*Connection)
	p.nodeRoundRobinIndices = make(map[string]int)
	p.nodeOrder = make([]string, 0)
	p.nodeOrderIndex = 0
	// Clear HTTP clients (they'll be garbage collected)
	p.nodeHTTPClients = make(map[string]*http.Client)
}

//-----------------------------------------------------------------------------
// Client pool management methods
//-----------------------------------------------------------------------------

// createPoolConnections creates a batch of connections for a pool
func (c *Client) createPoolConnections(nodeURL, nodeID, nodeMode string, isLeader bool, count int) []*Connection {
	if count <= 0 {
		return nil
	}

	connections := make([]*Connection, 0, count)

	for i := 0; i < count; i++ {
		// Always create a new connection with its own token
		// Never reuse tokens - each connection must have a unique token
		conn, err := c.createAndConnectNewConnection(nodeURL, nodeID, nodeMode, isLeader)
		if err != nil {
			fmt.Printf("Warning: Failed to create connection to %s: %v\n", nodeURL, err)
			continue
		}

		connections = append(connections, conn)
	}

	return connections
}

// InitializePool initializes connection pools based on node status. This should be called only from Connect()
func (c *Client) InitializePool() error {
	// Get status to discover nodes
	status, err := c.getStatusWithoutLock()
	if err != nil {
		return fmt.Errorf("failed to get status for pool initialization: %w", err)
	}

	fmt.Println("Status:", status)
	c.status = &status

	// If this is called from Connect() which should be only called once, all variables for readPool, writePool and statsPerNode
	// should be properly initialized (made)

	// Initialize self node pools (should be the leader)
	// Since the scaleUpNode take in form of connection (for node info like URL, Mode etc) we prepare the empty connection
	leaderConn := NewConnection(&c.Config, status.URL, status.NodeID, status.Mode, status.IsLeader, suresql.TokenTable{})
	c.scaleUpNode(leaderConn, IS_WRITE)
	c.scaleUpNode(leaderConn, IS_READ)
	// c.initializePoolForNode(status.URL, status.NodeID, status.Mode, status.IsLeader, status.MaxPool)

	// Initialize peer nodes pools
	for _, peer := range status.Peers {
		tmpConn := NewConnection(&c.Config, peer.URL, peer.NodeID, peer.Mode, peer.IsLeader, suresql.TokenTable{})
		c.scaleUpNode(tmpConn, IS_WRITE)
		c.scaleUpNode(tmpConn, IS_READ)
		// c.initializePoolForNode(peer.URL, peer.NodeID, peer.Mode, peer.IsLeader, peer.MaxPool)
	}

	// Start the cleanup timer if not already running
	if c.cleanupTimer == nil {
		c.startCleanupTimer()
	}

	return nil
}

// getReadConnection gets the next available read connection using node-level round-robin
func (c *Client) getReadConnection() (*Connection, error) {
	// Try to initialize pool if it's empty
	if c.readPool.Size() == 0 {
		err := c.InitializePool()
		if err != nil || c.readPool.Size() == 0 {
			return nil, errors.New("no read connections available")
		}
	}

	conn, err := c.readPool.GetConnection()
	if err != nil {
		return nil, err
	}

	// Record usage outside the lock
	go c.recordNodeUsage(conn.NodeID, IS_READ)

	// Track that a request is beginning
	go c.beginRequest(conn, IS_READ)

	return conn, nil
}

// getWriteConnection gets the next available write connection
func (c *Client) getWriteConnection() (*Connection, error) {
	// Prioritize leader connection for writes
	// 	if c.leaderConn != nil && (c.leaderConn.Mode == "rw" || c.leaderConn.Mode == "w") {
	// 		// Ensure leader connection has a valid token
	// 		if c.leaderConn.Token.Token == "" {
	// 			err := c.leaderConn.newOrRefreshToken(&c.Config, CALL_CONNECT)
	// 			if err != nil {
	// 				// If leader connection can't be connected, try the write pool
	// 				goto useWritePool
	// 			}
	// 		}

	// 		c.leaderConn.LastUsed = time.Now()

	// 		// Record usage outside the lock
	// 		go c.recordNodeUsage(c.leaderConn.NodeID)

	// 		// Track that a request is beginning
	// 		go c.beginRequest(c.leaderConn.NodeID)

	// 		return c.leaderConn, nil
	// 	}

	// useWritePool:
	// If leader connection unavailable, use write pool. QUESTION: I don't think this is necessary, because
	// the code should never reached this function if the pool is not yet initialized
	if c.writePool.Size() == 0 {
		err := c.InitializePool()
		if err != nil || c.writePool.Size() == 0 {
			return nil, errors.New("no write connections available")
		}
	}

	conn, err := c.writePool.GetConnection()
	if err != nil {
		return nil, err
	}

	// Record usage outside the lock
	go c.recordNodeUsage(conn.NodeID, IS_WRITE)

	// Track that a request is beginning
	go c.beginRequest(conn, IS_WRITE)

	return conn, nil
}

// cleanupIdleConnections removes connections that have been idle longer than IdleTimeout
// while respecting the MinPoolSize configuration
func (c *Client) cleanupIdleConnections() {
	now := time.Now()

	// Check if we have status info
	if c.status == nil {
		return
	}

	// Make sure we have stats for the leader node
	// c.getOrCreateNodeStats(c.status.NodeID,IS_WRITE)

	// Process read pool
	readRemoved := c.readPool.RemoveIdleConnections(c.PoolConfig.IdleTimeout, c.PoolConfig.MinPoolSize)

	// Process write pool
	writeRemoved := c.writePool.RemoveIdleConnections(c.PoolConfig.IdleTimeout, c.PoolConfig.MinPoolSize)

	// Update stats if connections were removed
	if readRemoved > 0 {
		for nodeID := range c.statsPerNodeRead {
			stats := c.getOrCreateNodeStats(nodeID, IS_READ)
			stats.HistoryMutex.Lock()
			// stats.CurrentConnections = readCount + writeCount
			stats.CurrentConnections = c.writePool.SizeForNode(nodeID)
			stats.LastScaleDown = now
			stats.LastCleanup = now
			stats.ScaleDownEvents++
			stats.HistoryMutex.Unlock()
		}
	}
	if writeRemoved > 0 {
		for nodeID := range c.statsPerNodeRead {
			stats := c.getOrCreateNodeStats(nodeID, IS_READ)
			stats.HistoryMutex.Lock()
			stats.CurrentConnections = c.readPool.SizeForNode(nodeID)
			stats.LastScaleDown = now
			stats.LastCleanup = now
			stats.ScaleDownEvents++
			stats.HistoryMutex.Unlock()
		}
	}
}

// CloseConnections properly closes all connections
func (c *Client) CloseConnections() {
	// Stop the cleanup routine
	if c.cleanupTimer != nil {
		if !c.cleanupTimer.Stop() {
			select {
			case <-c.cleanupTimer.C:
			default:
			}
		}
		close(c.cleanupDone)
	}

	// Clear all connection references
	c.leaderConn = nil
	c.readPool.Clear()
	c.writePool.Clear()
	c.Connected = false
}
