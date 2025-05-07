# GoSureSQL Architecture

This document provides an overview of the GoSureSQL client's internal architecture for developers interested in understanding, maintaining, or extending the codebase.

## Overview

GoSureSQL is organized using a layered architecture with clean separation of concerns:

1. **HTTP Layer** - Core HTTP request handling
2. **Response Processing** - Standardized response parsing
3. **Connection Management** - Creating, refreshing, and managing connections
4. **Request Execution** - Type-specific request handlers
5. **ORM Layer** - Public interface with database operations
6. **Dynamic Scaling** - Automatic pool size adjustment
7. **Metrics & Monitoring** - Performance and health tracking

## Component Diagram

```
┌─────────────────────────────────────────────────────────────────────┐
│                          ORM Methods                                │
└───────────┬─────────────────────┬────────────────────┬──────────────┘
            │                     │                    │
            ▼                     ▼                    ▼
┌───────────────────┐   ┌─────────────────┐   ┌────────────────────┐
│executeWithConnect-│   │executeRead      │   │executeWrite        │
│ionOrFallback      │   │Request          │   │Request             │
└────┬─────────┬────┘   └────────┬────────┘   └─────────┬──────────┘
     │         │                 │                      │
     ▼         │                 │                      │
┌──────────┐   │                 │                      │
│getAny    │   │                 │                      │
│Connection│   │                 │                      │
└──────────┘   │                 │                      │
               ▼                 ▼                      ▼
         ┌─────────────────┐  ┌────────────────┐  ┌────────────────┐
         │sendDirect       │  │sendConnection  │  │sendConnection  │
         │Request          │  │Request (Read)  │  │Request (Write) │
         └────────┬────────┘  └────────┬───────┘  └────────┬───────┘
                  │                    │                    │
                  │                    │                    │
                  ▼                    ▼                    ▼
         ┌───────────────────────────────────────────────────────────┐
         │                     sendHttpRequest                       │
         └───────────────────────────┬───────────────────────────────┘
                                     │
                                     ▼
         ┌───────────────────────────────────────────────────────────┐
         │                     decodeResponse                        │
         └───────────────────────────────────────────────────────────┘
```

## File Structure

The codebase is organized into these main files:

1. **models.go** - Core type definitions and configurations
2. **connection.go** - HTTP requests and connection management
3. **request.go** - Type-specific request execution
4. **pool.go** - Connection pool initialization and management
5. **scale.go** - Dynamic scaling logic
6. **metrics.go** - Pool statistics and monitoring
7. **suresql.go** - ORM interface implementations

## Key Components

### Client

The `Client` struct is the central component that coordinates all operations. It maintains:

- Connection pools for each node
- Authentication tokens
- Configuration settings
- Dynamic scaling parameters

### Connection

Each `Connection` in the pool is an independent entity with:

- Its own HTTP client
- Dedicated authentication tokens
- Connection-specific metadata
- Usage tracking

### Dynamic Scaling System

The scaling system consists of these key components:

1. **Usage Tracking**: Records request activity per node
2. **Scale Triggers**: Monitors thresholds to determine when to scale
3. **Connection Creation**: Adds new connections during high load
4. **Connection Cleanup**: Removes idle connections during quiet periods

## Request Flow

A typical request flows through these components:

1. Client calls an ORM method (e.g., `SelectOneSQL`)
2. The method calls the appropriate type-specific handler (`executeReadSQLQueryRequest`)
3. The handler gets a connection using `getReadConnection` (with round-robin)
4. The connection makes the HTTP request with `sendConnectionRequest`
5. The response is processed with `decodeResponse`
6. The result is returned to the caller with proper typing

## Type System

The client uses a sophisticated type system to ensure type safety:

1. Generic interface{} data from HTTP responses
2. Conversion to specific types (QueryResponse, SQLResponse)
3. Extraction of relevant parts (Records, Results)
4. Return of properly typed values to the caller

## Error Handling

Errors are handled at each layer:

1. HTTP errors in sendHttpRequest
2. Response errors in decodeResponse
3. Connection failures in sendConnectionRequest
4. Business logic errors in ORM methods

## Concurrency Model

The client uses several mechanisms for concurrency:

1. Mutex locks for critical sections
2. Background goroutines for non-blocking operations
3. Atomic operations for counters
4. Channels for shutdown signaling

## Key Design Decisions

1. **Separation of Read/Write Operations**:
   - Write operations always use the leader connection
   - Read operations use round-robin across all read connections
   - Ensures data consistency while maximizing performance

2. **Type-Specific Request Handlers**:
   - Separate handlers for each response type
   - Eliminates repetitive type conversion
   - Improves code readability and maintenance

3. **Dynamic Connection Creation**:
   - Connections created on-demand
   - Minimizes resource usage during quiet periods
   - Quickly adapts to traffic spikes

4. **Clean Layering**:
   - Each component focuses on a single responsibility
   - Clear boundaries between layers
   - Easy to modify or extend individual components

## Extending the Library

To add new functionality:

1. **New Request Type**: Add to request.go with appropriate type handling
2. **New ORM Method**: Add to suresql.go using the appropriate request handler
3. **New Configuration**: Add to models.go and update pool.go as needed
4. **New Metrics**: Add to metrics.go and update data collection in scale.go

## Performance Considerations

Key performance optimizations:

1. **Connection Reuse**: Connections are pooled to avoid creation overhead
2. **Round-Robin Distribution**: Evenly distributes load across connections
3. **Batch Connection Creation**: Adds multiple connections at once when scaling
4. **Efficient Cleanup**: Periodically removes idle connections in the background
5. **Token Refresh Strategy**: Refreshes tokens before they expire to avoid failures

## Future Improvements

Potential areas for enhancement:

1. **Advanced Load Balancing**: Weighted distribution based on node performance
2. **Connection Warmup**: Proactive connection creation based on traffic trends
3. **Query Routing**: Route queries to specific nodes based on content
4. **Connection Tagging**: Label connections for specialized purposes
5. **Circuit Breaker**: Automatically detect and isolate failing nodes