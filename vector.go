package client

import (
	"encoding/binary"
	"fmt"
	"math"
)

// VectorToBlob converts a slice of float32 embeddings into a raw byte slice (binary BLOB).
// This matches the binary representation expected by sqlite-vec and SQLite BLOB parameters.
func VectorToBlob(vec []float32) []byte {
	buf := make([]byte, len(vec)*4)
	for i, f := range vec {
		binary.LittleEndian.PutUint32(buf[i*4:], math.Float32bits(f))
	}
	return buf
}

// BlobToVector decodes a raw binary BLOB into a slice of float32 embeddings.
func BlobToVector(buf []byte) ([]float32, error) {
	if len(buf)%4 != 0 {
		return nil, fmt.Errorf("invalid vector byte length %d: must be a multiple of 4", len(buf))
	}
	count := len(buf) / 4
	vec := make([]float32, count)
	for i := range count {
		bits := binary.LittleEndian.Uint32(buf[i*4:])
		vec[i] = math.Float32frombits(bits)
	}
	return vec, nil
}

// VectorDistanceCosine computes the cosine distance between two float32 vectors.
// Returns value in [0, 2] where 0 is identical direction.
func VectorDistanceCosine(a, b []float32) (float32, error) {
	if len(a) != len(b) {
		return 0, fmt.Errorf("vector length mismatch: %d != %d", len(a), len(b))
	}

	var dot, normA, normB float32
	for i := range a {
		dot += a[i] * b[i]
		normA += a[i] * a[i]
		normB += b[i] * b[i]
	}

	denom := float32(math.Sqrt(float64(normA)) * math.Sqrt(float64(normB)))
	if denom == 0 {
		return 0, nil
	}

	return 1.0 - (dot / denom), nil
}

// VectorDistanceL2 computes Euclidean (L2) distance between two float32 vectors.
func VectorDistanceL2(a, b []float32) (float32, error) {
	if len(a) != len(b) {
		return 0, fmt.Errorf("vector length mismatch: %d != %d", len(a), len(b))
	}

	var sum float32
	for i := range a {
		diff := a[i] - b[i]
		sum += diff * diff
	}

	return float32(math.Sqrt(float64(sum))), nil
}
