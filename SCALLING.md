# Connection Pool Scaling Guide

The gosuresql package implements a sophisticated dynamic connection pool that automatically scales up and down based on actual usage patterns. This document explains how the scaling system works and how to configure it for different environments.

## How Scaling Works

### Automatic Scaling Mechanism

The connection pool starts with a minimal set of connections and adapts to your application's traffic patterns:

1. **Initial Connections**: 
   - Each node starts with `MinPoolSize` connections (default: 5)
   - This includes both the leader node and any read-only peer nodes

2. **Scale-Up Mechanism**:
   - When concurrent active requests exceed `ScaleUpThreshold` (default: 10)
   - Adds `ScaleUpBatchSize` (default: 3) new connections
   - Waits at least 10 seconds between scale-up events to prevent oscillation
   - Never exceeds `MaxPool` (determined by node settings)

3. **Scale-Down Mechanism**:
   - Periodically checks (every `ScaleDownInterval`, default: 1 minute)
   - Removes connections not used for `IdleTimeout` (default: 5 minutes)
   - Always maintains at least `MinPoolSize` connections per node
   - Prioritizes keeping the most recently used connections

### Connection Lifecycle

Each connection in the pool goes through a complete lifecycle:

- **Creation**: 
  - At pool initialization (MinPoolSize connections)
  - During scale-up events when traffic increases
  - When replacing failed connections

- **Usage**: 
  - Selected via round-robin for read operations
  - Each request updates the LastUsed timestamp
  - Tokens refreshed when approaching `ConnectionTTL`

- **Removal**:
  - When idle for IdleTimeout duration
  - When connection fails and can't be refreshed
  - When pool is shut down

## Connection Needs for Different Traffic Levels

### Low Traffic (10-20 requests/second)
- **Concurrent Requests**: ~1-2 (assuming 100ms per request)
- **Expected Pool Size**: Stays at `MinPoolSize` (5)
- **Why**: Concurrent requests never exceed threshold
- **Recommendation**: MinPoolSize=3, IdleTimeout=10min

### Medium Traffic (100-200 requests/second)
- **Concurrent Requests**: ~10-20
- **Expected Pool Size**: ~8-12 connections
- **Why**: Will trigger 1-2 scale-up events
- **Behavior**: Will scale up during traffic spikes, then scale down after 5 minutes of lower activity
- **Recommendation**: MinPoolSize=5, ScaleUpThreshold=10

### High Traffic (1,000 requests/second)
- **Concurrent Requests**: ~100
- **Expected Pool Size**: Will reach MaxPool (25)
- **Why**: Will quickly trigger multiple scale-up events
- **Timeline**: Reaches maximum in ~70 seconds (7 scale-up events)
- **Recommendation**: MinPoolSize=10, MaxPool=30, ScaleUpBatchSize=5

### Very High Traffic (10,000+ requests/second)
- **Concurrent Requests**: 1,000+
- **Expected Pool Size**: MaxPool (25) on all nodes
- **Why**: Maximum connections will be continuously used
- **Recommendation**: Add more read nodes for horizontal scaling

## Scaling Formula

The number of concurrent requests can be approximated as:

```
Concurrent Requests ≈ Requests Per Second × Average Request Duration
```

For example:
- 100 RPS with 100ms average duration = ~10 concurrent requests
- 1000 RPS with 250ms average duration = ~250 concurrent requests

## Configuration Recommendations

### For Typical Web Application (100-500 RPS):
```go
poolConfig := NewPoolConfig(
    WithMinPoolSize(5),         // Base connection count
    WithScaleUpThreshold(15),   // Start scaling at 15 concurrent requests
    WithIdleTimeout(5 * time.Minute),
    WithScaleUpBatchSize(3),    // Add 3 connections at a time
)
```

### For High-Performance API (1000+ RPS):
```go
poolConfig := NewPoolConfig(
    WithMinPoolSize(10),        // Higher base connection count
    WithScaleUpThreshold(20),   // Higher threshold for stability
    WithIdleTimeout(3 * time.Minute),
    WithScaleUpBatchSize(5),    // Add more connections at once
)
```

### For Low-Resource Environment:
```go
poolConfig := NewPoolConfig(
    WithMinPoolSize(3),         // Minimize idle connections
    WithScaleUpThreshold(10),   // Standard threshold
    WithIdleTimeout(2 * time.Minute), // Reclaim resources faster
    WithScaleUpBatchSize(2),    // Smaller batch size
)
```

## Advanced Configuration

Full configuration options:

| Parameter | Description | Default | Recommendation |
|-----------|-------------|---------|---------------|
| MinPoolSize | Minimum connections per node | 5 | Lower for resource constraints, higher for consistent latency |
| ScaleUpThreshold | Concurrent requests before scaling | 10 | Higher for stable traffic, lower for variable traffic |
| IdleTimeout | Time before removing unused connections | 5 min | Lower for resource constraints, higher for spiky traffic |
| ScaleDownInterval | Frequency of cleanup checks | 1 min | Adjust based on traffic volatility |
| ConnectionTTL | Maximum connection lifetime | 1 hour | Based on token expiration policies |
| ScaleUpBatchSize | Connections added per scale event | 3 | Higher for rapidly increasing traffic |
| UsageWindowSize | History size for usage tracking | 100 | Larger for more accurate trend detection |

## Monitoring Pool Behavior

You can monitor the pool's behavior using:

```go
// Get detailed metrics
metrics := client.GetPoolMetrics()
fmt.Printf("Total connections: %d\n", metrics.TotalConnections)
fmt.Printf("Active requests: %d\n", metrics.ActiveRequests)
fmt.Printf("Requests per second: %.2f\n", metrics.RequestsPerSecond)

// Get a quick health check
health := client.GetPoolHealth()
fmt.Printf("Pool health: %+v\n", health)
```

This helps you fine-tune your configuration based on actual usage patterns.