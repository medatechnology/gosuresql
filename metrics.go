package client

import (
	"time"
)

// GetPoolMetrics returns current metrics for the connection pool
func (c *Client) GetPoolMetrics() PoolMetrics {
	// Begin with an empty metrics structure
	metrics := PoolMetrics{
		ConnectionsPerNode: make(map[string]NodePoolMetrics),
	}

	// Get all node IDs from both pools
	nodeIDs := make(map[string]bool)

	// Add leader connection nodeID if it exists
	if c.leaderConn != nil {
		nodeIDs[c.leaderConn.NodeID] = true
	}

	// Add node IDs from read pool
	for _, conn := range c.readPool.GetAllConnections() {
		nodeIDs[conn.NodeID] = true
	}

	// Add node IDs from write pool
	for _, conn := range c.writePool.GetAllConnections() {
		nodeIDs[conn.NodeID] = true
	}

	// Calculate total connections
	metrics.TotalConnections = 0
	if c.leaderConn != nil {
		metrics.TotalConnections++
	}
	metrics.TotalConnections += c.readPool.Size()
	metrics.TotalConnections += c.writePool.Size()

	// Accumulate scale events
	totalScaleUpEvents := 0
	totalScaleDownEvents := 0

	// Calculate per-node metrics
	now := time.Now()

	for nodeID := range nodeIDs {

		// Get node info
		var url, mode string
		if c.status != nil && nodeID == c.status.NodeID {
			url = c.Config.ServerURL
			mode = c.status.Mode
		} else if c.status != nil {
			for _, peer := range c.status.Peers {
				if peer.NodeID == nodeID {
					url = peer.URL
					mode = peer.Mode
					break
				}
			}
		}

		// Get connections for this node from both pools
		readConns := c.readPool.GetAllConnectionsForNode(nodeID)
		writeConns := c.writePool.GetAllConnectionsForNode(nodeID)
		allConns := append(readConns, writeConns...)

		// Count idle connections
		idleCount := 0
		for _, conn := range allConns {
			if now.Sub(conn.LastUsed) > c.PoolConfig.IdleTimeout {
				idleCount++
			}
		}

		// Count recent requests
		recentRequests := 0
		statsRead := c.getOrCreateNodeStats(nodeID, IS_READ)
		statsWrite := c.getOrCreateNodeStats(nodeID, IS_WRITE)

		statsRead.HistoryMutex.Lock()
		statsWrite.HistoryMutex.Lock()

		for _, timestamp := range statsRead.UsageHistory {
			if now.Sub(timestamp) < time.Minute {
				recentRequests++
			}
		}
		for _, timestamp := range statsWrite.UsageHistory {
			if now.Sub(timestamp) < time.Minute {
				recentRequests++
			}
		}

		// Add scale events to total
		totalScaleUpEvents += statsRead.ScaleUpEvents + statsWrite.ScaleUpEvents
		totalScaleDownEvents += statsRead.ScaleDownEvents + statsWrite.ScaleDownEvents

		nodeMetrics := NodePoolMetrics{
			NodeID:             nodeID,
			URL:                url,
			Mode:               mode,
			CurrentConnections: len(allConns),
			ActiveRequests:     statsRead.ActiveRequests + statsWrite.ActiveRequests,
			IdleConnections:    idleCount,
			RecentRequests:     recentRequests,
			LastScaleUp:        statsRead.LastScaleUp,
			LastScaleDown:      statsRead.LastScaleDown,
			ScaleUpEvents:      statsRead.ScaleUpEvents + statsWrite.ScaleUpEvents,
			ScaleDownEvents:    statsRead.ScaleDownEvents + statsWrite.ScaleDownEvents,
		}

		statsRead.HistoryMutex.Unlock()
		statsWrite.HistoryMutex.Unlock()

		metrics.ConnectionsPerNode[nodeID] = nodeMetrics
		metrics.ActiveRequests += nodeMetrics.ActiveRequests
	}

	// Calculate approximate requests per second
	totalRecentRequests := 0
	for _, nodeMetrics := range metrics.ConnectionsPerNode {
		totalRecentRequests += nodeMetrics.RecentRequests
	}
	metrics.RequestsPerSecond = float64(totalRecentRequests) / 60.0
	metrics.ScaleUpEvents = totalScaleUpEvents
	metrics.ScaleDownEvents = totalScaleDownEvents

	return metrics
}

// ConnectionStats returns statistics about the connection pool in map format
// This is for backward compatibility with the previous implementation
func (c *Client) ConnectionStats() map[string]interface{} {
	stats := make(map[string]interface{})

	// Leader connection info
	if c.leaderConn != nil {
		stats["leader"] = map[string]interface{}{
			"url":          c.leaderConn.URL,
			"node_id":      c.leaderConn.NodeID,
			"mode":         c.leaderConn.Mode,
			"last_used":    c.leaderConn.LastUsed,
			"created":      c.leaderConn.Created,
			"last_refresh": c.leaderConn.LastRefresh,
		}
	}

	// Overall pool info
	stats["total_read_pool_size"] = c.readPool.Size()
	stats["total_write_pool_size"] = c.writePool.Size()

	// Per-node pool info
	nodeStats := make(map[string]interface{})

	// Get all node IDs from both pools
	nodeIDs := make(map[string]bool)

	if c.leaderConn != nil {
		nodeIDs[c.leaderConn.NodeID] = true
	}

	for _, conn := range c.readPool.GetAllConnections() {
		nodeIDs[conn.NodeID] = true
	}

	for _, conn := range c.writePool.GetAllConnections() {
		nodeIDs[conn.NodeID] = true
	}

	for nodeID := range nodeIDs {
		readConns := c.readPool.GetAllConnectionsForNode(nodeID)
		writeConns := c.writePool.GetAllConnectionsForNode(nodeID)

		// Build pool info for read connections
		readPoolInfo := make([]map[string]interface{}, 0, len(readConns))
		for _, conn := range readConns {
			readPoolInfo = append(readPoolInfo, map[string]interface{}{
				"url":          conn.URL,
				"node_id":      conn.NodeID,
				"mode":         conn.Mode,
				"last_used":    conn.LastUsed,
				"created":      conn.Created,
				"last_refresh": conn.LastRefresh,
			})
		}

		// Build pool info for write connections
		writePoolInfo := make([]map[string]interface{}, 0, len(writeConns))
		for _, conn := range writeConns {
			writePoolInfo = append(writePoolInfo, map[string]interface{}{
				"url":          conn.URL,
				"node_id":      conn.NodeID,
				"mode":         conn.Mode,
				"last_used":    conn.LastUsed,
				"created":      conn.Created,
				"last_refresh": conn.LastRefresh,
			})
		}

		// Get node usage stats. TODO: add the write as well!
		usage := make(map[string]interface{})
		if stats, exists := c.statsPerNodeRead[nodeID]; exists {
			stats.HistoryMutex.Lock()
			usage = map[string]interface{}{
				"active_requests":   stats.ActiveRequests,
				"last_scale_up":     stats.LastScaleUp,
				"last_scale_down":   stats.LastScaleDown,
				"scale_up_events":   stats.ScaleUpEvents,
				"scale_down_events": stats.ScaleDownEvents,
			}
			stats.HistoryMutex.Unlock()
		}

		nodeStats[nodeID] = map[string]interface{}{
			"read_pool_size":    len(readConns),
			"write_pool_size":   len(writeConns),
			"read_connections":  readPoolInfo,
			"write_connections": writePoolInfo,
			"usage":             usage,
		}
	}

	stats["node_pools"] = nodeStats

	// Add pool configuration
	stats["pool_config"] = map[string]interface{}{
		"min_pool_size":       c.PoolConfig.MinPoolSize,
		"scale_up_threshold":  c.PoolConfig.ScaleUpThreshold,
		"idle_timeout":        c.PoolConfig.IdleTimeout.String(),
		"scale_down_interval": c.PoolConfig.ScaleDownInterval.String(),
		"connection_ttl":      c.PoolConfig.ConnectionTTL.String(),
		"scale_up_batch_size": c.PoolConfig.ScaleUpBatchSize,
		"usage_window_size":   c.PoolConfig.UsageWindowSize,
	}

	return stats
}

// GetPoolHealth returns a simplified health status of the connection pool
func (c *Client) GetPoolHealth() map[string]interface{} {
	health := make(map[string]interface{})

	// Check if we have a leader connection
	health["has_leader"] = c.leaderConn != nil

	// Check if we have read and write connections
	health["read_connections_count"] = c.readPool.Size()
	health["write_connections_count"] = c.writePool.Size()
	health["has_read_connections"] = c.readPool.Size() > 0
	health["has_write_connections"] = c.writePool.Size() > 0

	// Calculate active requests
	activeRequests := 0
	for _, stats := range c.statsPerNodeRead {
		stats.HistoryMutex.Lock()
		activeRequests += stats.ActiveRequests
		stats.HistoryMutex.Unlock()
	}
	health["active_requests"] = activeRequests

	// Check connection ages
	now := time.Now()
	oldestConnection := time.Time{}

	if c.leaderConn != nil {
		oldestConnection = c.leaderConn.Created
	}

	// Check read pool for old connections
	for _, conn := range c.readPool.GetAllConnections() {
		if oldestConnection.IsZero() || conn.Created.Before(oldestConnection) {
			oldestConnection = conn.Created
		}
	}

	// Check write pool for old connections
	for _, conn := range c.writePool.GetAllConnections() {
		if oldestConnection.IsZero() || conn.Created.Before(oldestConnection) {
			oldestConnection = conn.Created
		}
	}

	if !oldestConnection.IsZero() {
		health["oldest_connection_age"] = now.Sub(oldestConnection).String()
	}

	// Calculate node coverage
	nodeCount := 0
	if c.status != nil {
		nodeCount = 1 + len(c.status.Peers) // Leader + peers

		// Get unique nodes with connections
		nodesWithConnections := make(map[string]bool)

		for _, conn := range c.readPool.GetAllConnections() {
			nodesWithConnections[conn.NodeID] = true
		}

		for _, conn := range c.writePool.GetAllConnections() {
			nodesWithConnections[conn.NodeID] = true
		}

		health["node_count"] = nodeCount
		health["nodes_with_connections"] = len(nodesWithConnections)
		health["full_node_coverage"] = len(nodesWithConnections) == nodeCount
	}

	return health
}

// GetNodePoolMetrics returns detailed metrics for a specific node
func (c *Client) GetNodePoolMetrics(nodeID string) (NodePoolMetrics, bool) {
	// Check if we have any connections for this node
	readConns := c.readPool.GetAllConnectionsForNode(nodeID)
	writeConns := c.writePool.GetAllConnectionsForNode(nodeID)

	if len(readConns) == 0 && len(writeConns) == 0 {
		return NodePoolMetrics{}, false
	}

	stats := c.getOrCreateNodeStats(nodeID, IS_READ)

	// Get node info
	var url, mode string
	if c.status != nil && nodeID == c.status.NodeID {
		url = c.Config.ServerURL
		mode = c.status.Mode
	} else if c.status != nil {
		for _, peer := range c.status.Peers {
			if peer.NodeID == nodeID {
				url = peer.URL
				mode = peer.Mode
				break
			}
		}
	}

	// Count idle connections
	now := time.Now()
	idleCount := 0
	allConns := append(readConns, writeConns...)

	for _, conn := range allConns {
		if now.Sub(conn.LastUsed) > c.PoolConfig.IdleTimeout {
			idleCount++
		}
	}

	// Count recent requests
	recentRequests := 0
	stats.HistoryMutex.Lock()

	for _, timestamp := range stats.UsageHistory {
		if now.Sub(timestamp) < time.Minute {
			recentRequests++
		}
	}

	metrics := NodePoolMetrics{
		NodeID:             nodeID,
		URL:                url,
		Mode:               mode,
		CurrentConnections: len(allConns),
		ActiveRequests:     stats.ActiveRequests,
		IdleConnections:    idleCount,
		RecentRequests:     recentRequests,
		LastScaleUp:        stats.LastScaleUp,
		LastScaleDown:      stats.LastScaleDown,
		ScaleUpEvents:      stats.ScaleUpEvents,
		ScaleDownEvents:    stats.ScaleDownEvents,
	}

	stats.HistoryMutex.Unlock()

	return metrics, true
}
