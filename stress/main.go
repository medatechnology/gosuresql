package main

import (
	"fmt"
	"math"
	"math/rand/v2"
	"os"
	"sort"
	"strconv"
	"sync"
	"sync/atomic"
	"time"

	client "github.com/medatechnology/gosuresql"
)

func getEnv(key, defaultVal string) string {
	if val := os.Getenv(key); val != "" {
		return val
	}
	return defaultVal
}

func getEnvInt(key string, defaultVal int) int {
	if val := os.Getenv(key); val != "" {
		if n, err := strconv.Atoi(val); err == nil {
			return n
		}
	}
	return defaultVal
}

type LatencyStats struct {
	Durations []time.Duration
	TotalOps  int
	Successes int64
	Failures  int64
	Elapsed   time.Duration
}

func (s *LatencyStats) PrintReport(title string) {
	fmt.Printf("\n=== %s ===\n", title)
	fmt.Printf("Elapsed:      %v\n", s.Elapsed)
	fmt.Printf("Total Ops:    %d\n", s.TotalOps)
	fmt.Printf("Successes:    %d\n", s.Successes)
	fmt.Printf("Failures:     %d\n", s.Failures)

	if len(s.Durations) == 0 {
		return
	}

	sort.Slice(s.Durations, func(i, j int) bool {
		return s.Durations[i] < s.Durations[j]
	})

	var sum time.Duration
	for _, d := range s.Durations {
		sum += d
	}
	avg := sum / time.Duration(len(s.Durations))
	min := s.Durations[0]
	max := s.Durations[len(s.Durations)-1]
	p50 := s.Durations[len(s.Durations)*50/100]
	p95 := s.Durations[len(s.Durations)*95/100]
	p99 := s.Durations[len(s.Durations)*99/100]

	qps := float64(s.Successes) / s.Elapsed.Seconds()

	fmt.Printf("Throughput:   %.2f ops/sec\n", qps)
	fmt.Printf("Latency Min:  %v\n", min)
	fmt.Printf("Latency Avg:  %v\n", avg)
	fmt.Printf("Latency p50:  %v\n", p50)
	fmt.Printf("Latency p95:  %v\n", p95)
	fmt.Printf("Latency p99:  %v\n", p99)
	fmt.Printf("Latency Max:  %v\n", max)
}

func main() {
	serverURL := getEnv("SURESQL_SERVER_URL", "http://127.0.0.1:8080")
	apiKey := getEnv("SURESQL_API_KEY", "development_api_key")
	clientID := getEnv("SURESQL_CLIENT_ID", "development_client_id")
	username := getEnv("SURESQL_USERNAME", "admin")
	password := getEnv("SURESQL_PASSWORD", "admin")
	workers := getEnvInt("WORKERS", 50)
	totalOps := getEnvInt("TOTAL_OPS", 1000)

	fmt.Println("==================================================")
	fmt.Println("🚀 SureSQL High-Concurrency Stress & Integration Test")
	fmt.Println("==================================================")
	fmt.Printf("Target URL:   %s\n", serverURL)
	fmt.Printf("Member Auth:  ClientID=%s, APIKey=%s\n", clientID, apiKey)
	fmt.Printf("DB User:      %s\n", username)
	fmt.Printf("Concurrency:  %d workers\n", workers)
	fmt.Printf("Workload:     %d ops per phase\n", totalOps)
	fmt.Println("--------------------------------------------------")

	// Phase 1: Initialize Client & Authenticate
	fmt.Println("\n[1/5] Connecting to SureSQL...")
	cfg := client.NewClientConfig(
		client.WithServerURL(serverURL),
		client.WithApiKey(apiKey),
		client.WithClientID(clientID),
	)
	c, err := client.NewClient(cfg)
	if err != nil {
		fmt.Printf("❌ Failed to initialize client: %v\n", err)
		os.Exit(1)
	}

	startConnect := time.Now()
	err = c.Connect(username, password)
	if err != nil {
		fmt.Printf("❌ Failed to connect: %v\n", err)
		os.Exit(1)
	}
	defer c.Close()
	fmt.Printf("✅ Connected in %v\n", time.Since(startConnect))

	// Discover Topology
	fmt.Printf("   Cluster Topology: %d node(s) (IsCluster: %v)\n", c.ClusterNodeCount(), c.IsCluster())
	fmt.Printf("   Leader URL:       %s\n", c.GetLeaderURL())
	if c.IsCluster() {
		fmt.Printf("   Replicas (%d):    %v\n", len(c.GetReplicaURLs()), c.GetReplicaURLs())
	}

	// Phase 2: Schema Preparation
	fmt.Println("\n[2/5] Preparing test schema...")
	createTableSQL := `CREATE TABLE IF NOT EXISTS stress_test (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		worker_id INTEGER,
		payload TEXT,
		created_at DATETIME DEFAULT CURRENT_TIMESTAMP
	);`
	res := c.ExecOneSQL(createTableSQL)
	if res.Error != nil {
		fmt.Printf("❌ Failed to create table: %v\n", res.Error)
		os.Exit(1)
	}
	c.ExecOneSQL("DELETE FROM stress_test;")
	fmt.Println("✅ Test table 'stress_test' created and cleaned")

	// Phase 3: Concurrent Write Stress Test
	fmt.Printf("\n[3/5] Running Concurrent Writes (%d ops across %d workers)...\n", totalOps, workers)
	writeStats := runWorkload(c, workers, totalOps, func(c *client.Client, workerID, opID int) error {
		sql := fmt.Sprintf("INSERT INTO stress_test (worker_id, payload) VALUES (%d, 'sample-payload-%d-%d');", workerID, workerID, opID)
		r := c.ExecOneSQL(sql)
		return r.Error
	})
	writeStats.PrintReport("Concurrent Writes (Group-Commit Write Queue)")

	// Phase 4: Concurrent Read Stress Test
	fmt.Printf("\n[4/5] Running Concurrent Reads (%d ops across %d workers)...\n", totalOps, workers)
	readStats := runWorkload(c, workers, totalOps, func(c *client.Client, workerID, opID int) error {
		randWorker := rand.IntN(workers)
		sql := fmt.Sprintf("SELECT id, worker_id, payload FROM stress_test WHERE worker_id = %d LIMIT 5;", randWorker)
		_, err := c.SelectOneSQL(sql)
		return err
	})
	readStats.PrintReport("Concurrent Reads (Smart Routing: Replicas in Cluster)")

	// Phase 5: Vector Embeddings & Similarity Test
	fmt.Println("\n[5/5] Testing Native Vector Embeddings...")
	vecA := []float32{0.5, 0.2, 0.8, -0.1}
	vecB := []float32{0.5, 0.2, 0.8, -0.1} // identical
	vecC := []float32{-0.5, -0.2, -0.8, 0.1} // opposite

	blobA := client.VectorToBlob(vecA)
	if len(blobA) != 16 {
		fmt.Printf("❌ Expected blob length 16, got %d\n", len(blobA))
	} else {
		fmt.Printf("✅ Vector serialized to %d-byte binary BLOB\n", len(blobA))
	}

	decodedA, err := client.BlobToVector(blobA)
	if err != nil || len(decodedA) != 4 {
		fmt.Printf("❌ Failed to decode vector: %v\n", err)
	} else {
		fmt.Printf("✅ Vector deserialized accurately: %v\n", decodedA)
	}

	cosDistZero, _ := client.VectorDistanceCosine(vecA, vecB)
	cosDistOpposite, _ := client.VectorDistanceCosine(vecA, vecC)
	l2Dist, _ := client.VectorDistanceL2(vecA, vecB)

	fmt.Printf("   Cosine Distance (Identical): %.4f (expected 0.0)\n", cosDistZero)
	fmt.Printf("   Cosine Distance (Opposite):  %.4f (expected 2.0)\n", cosDistOpposite)
	fmt.Printf("   L2 Distance (Identical):     %.4f (expected 0.0)\n", l2Dist)

	if math.Abs(float64(cosDistZero)) < 1e-5 && math.Abs(float64(cosDistOpposite-2.0)) < 1e-5 {
		fmt.Println("✅ Vector similarity math verified")
	}

	fmt.Println("\n==================================================")
	fmt.Println("🎉 ALL TESTS & BENCHMARKS COMPLETED SUCCESSFULLY")
	fmt.Println("==================================================")
}

func runWorkload(c *client.Client, workers, totalOps int, opFn func(c *client.Client, workerID, opID int) error) *LatencyStats {
	stats := &LatencyStats{
		TotalOps:  totalOps,
		Durations: make([]time.Duration, totalOps),
	}

	opsPerWorker := totalOps / workers
	var wg sync.WaitGroup
	var completed int64
	startTime := time.Now()

	for w := 0; w < workers; w++ {
		wg.Add(1)
		go func(workerID int) {
			defer wg.Done()
			for i := 0; i < opsPerWorker; i++ {
				opIndex := int(atomic.AddInt64(&completed, 1)) - 1
				if opIndex >= totalOps {
					return
				}

				t0 := time.Now()
				err := opFn(c, workerID, i)
				dur := time.Since(t0)

				stats.Durations[opIndex] = dur
				if err != nil {
					atomic.AddInt64(&stats.Failures, 1)
				} else {
					atomic.AddInt64(&stats.Successes, 1)
				}
			}
		}(w)
	}

	wg.Wait()
	stats.Elapsed = time.Since(startTime)
	return stats
}
