package client

import (
	"time"
)

// startCleanupTimer starts the periodic cleanup routine
func (c *Client) startCleanupTimer() {
	c.cleanupDone = make(chan struct{})
	c.cleanupTimer = time.NewTimer(c.PoolConfig.ScaleDownInterval)

	go func() {
		for {
			select {
			case <-c.cleanupTimer.C:
				c.cleanupIdleConnections()
				c.cleanupTimer.Reset(c.PoolConfig.ScaleDownInterval)
			case <-c.cleanupDone:
				if !c.cleanupTimer.Stop() {
					select {
					case <-c.cleanupTimer.C:
					default:
					}
				}
				return
			}
		}
	}()
}

// getOrCreateNodeStats gets or creates stats tracking for a node
func (c *Client) getOrCreateNodeStats(nodeID string, isWrite bool) *ConnectionStats {
	c.scalingMutex.Lock()
	defer c.scalingMutex.Unlock()

	stats, exists := c.statsPerNodeRead[nodeID]
	if isWrite {
		stats, exists = c.statsPerNodeWrite[nodeID]
	}
	if !exists {
		stats = &ConnectionStats{
			NodeID:        nodeID,
			HistoryWindow: c.PoolConfig.UsageWindowSize,
			UsageHistory:  make([]time.Time, 0, c.PoolConfig.UsageWindowSize),
			LastCleanup:   time.Now(),
		}
		if isWrite {
			c.statsPerNodeWrite[nodeID] = stats
		} else {
			c.statsPerNodeRead[nodeID] = stats
		}
	}
	return stats
}

// recordNodeUsage records a usage event for a node
func (c *Client) recordNodeUsage(nodeID string, isWrite bool) {
	stats := c.getOrCreateNodeStats(nodeID, isWrite)

	stats.HistoryMutex.Lock()
	defer stats.HistoryMutex.Unlock()

	// Add current time to usage history
	now := time.Now()
	stats.UsageHistory = append(stats.UsageHistory, now)

	// Trim history to window size
	if len(stats.UsageHistory) > stats.HistoryWindow {
		stats.UsageHistory = stats.UsageHistory[len(stats.UsageHistory)-stats.HistoryWindow:]
	}
}

// beginRequest increments the active request counter for a node
func (c *Client) beginRequest(conn *Connection, isWrite bool) {
	stats := c.getOrCreateNodeStats(conn.NodeID, isWrite)

	stats.HistoryMutex.Lock()
	defer stats.HistoryMutex.Unlock()

	stats.ActiveRequests++

	// Check if we need to scale up
	if stats.ActiveRequests >= c.PoolConfig.ScaleUpThreshold {
		// Avoid frequent scale-ups
		if time.Since(stats.LastScaleUp) > 10*time.Second {
			if isWrite {
				go c.scaleUpNode(conn, isWrite)
			} else {
				go c.scaleUpNode(conn, isWrite)
			}
			stats.LastScaleUp = time.Now()
		}
	}
}

// endRequest decrements the active request counter for a node
func (c *Client) endRequest(nodeID string, isWrite bool) {
	stats := c.getOrCreateNodeStats(nodeID, isWrite)

	stats.HistoryMutex.Lock()
	defer stats.HistoryMutex.Unlock()

	if stats.ActiveRequests > 0 {
		stats.ActiveRequests--
	}
}

// Get maxPool (read) then maxWritePool (write) by NodeID from status
func (c *Client) findMaxPoolsByNodeID(nodeID string) int {
	if nodeID == c.status.NodeID {
		return c.status.MaxPool
	}
	for _, p := range c.status.Peers {
		if nodeID == p.NodeID {
			return p.MaxPool
		}
	}
	return c.PoolConfig.MaxPoolSize
}

// scaleUpNode adds connections to both read and write pools for a node if needed
func (c *Client) scaleUpNode(conn *Connection, isWrite bool) {
	// Get node info from connection
	maxPool := c.findMaxPoolsByNodeID(conn.NodeID)
	pool := c.readPool
	if isWrite {
		maxPool = c.readPool.maxWritePool
		pool = c.writePool
	}

	currentSize := pool.SizeForNode(conn.NodeID)
	// Calculate how many connections we can add
	addCount := min(c.PoolConfig.ScaleUpBatchSize, maxPool-currentSize)
	if addCount <= 0 {
		return
	}

	connections := c.createPoolConnections(conn.URL, conn.NodeID, conn.Mode, conn.IsLeader, addCount)

	// Add connections to pool if any were created
	if len(connections) > 0 {
		pool.AddBatch(connections)

		// Update stats
		stats := c.getOrCreateNodeStats(conn.NodeID, isWrite)
		stats.HistoryMutex.Lock()
		stats.CurrentConnections += len(connections)
		stats.ScaleUpEvents++
		stats.HistoryMutex.Unlock()
	}
}

// markRequestComplete indicates a request is complete on a connection
func (c *Client) markRequestComplete(conn *Connection, isWrite bool) {
	go c.endRequest(conn.NodeID, isWrite)
}
