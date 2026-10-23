package client

import (
	"math"
	"testing"
)

func TestVectorBlobRoundtrip(t *testing.T) {
	orig := []float32{0.1, -0.5, 3.14159, 100.0, 0.0}

	blob := VectorToBlob(orig)
	if len(blob) != len(orig)*4 {
		t.Fatalf("expected blob length %d, got %d", len(orig)*4, len(blob))
	}

	decoded, err := BlobToVector(blob)
	if err != nil {
		t.Fatalf("BlobToVector failed: %v", err)
	}

	if len(decoded) != len(orig) {
		t.Fatalf("expected decoded length %d, got %d", len(orig), len(decoded))
	}

	for i := range orig {
		if math.Abs(float64(orig[i]-decoded[i])) > 1e-6 {
			t.Fatalf("mismatch at index %d: %f != %f", i, orig[i], decoded[i])
		}
	}
}

func TestVectorDistances(t *testing.T) {
	a := []float32{1.0, 0.0, 0.0}
	b := []float32{1.0, 0.0, 0.0}
	c := []float32{0.0, 1.0, 0.0}

	// Cosine distance of identical vectors is 0
	distIdentical, err := VectorDistanceCosine(a, b)
	if err != nil || math.Abs(float64(distIdentical)) > 1e-6 {
		t.Fatalf("expected cosine distance 0, got %f (err: %v)", distIdentical, err)
	}

	// Cosine distance of orthogonal vectors is 1
	distOrtho, err := VectorDistanceCosine(a, c)
	if err != nil || math.Abs(float64(distOrtho-1.0)) > 1e-6 {
		t.Fatalf("expected cosine distance 1, got %f (err: %v)", distOrtho, err)
	}

	// L2 distance of identical vectors is 0
	l2Zero, err := VectorDistanceL2(a, b)
	if err != nil || math.Abs(float64(l2Zero)) > 1e-6 {
		t.Fatalf("expected L2 distance 0, got %f (err: %v)", l2Zero, err)
	}

	// L2 distance of [1,0,0] and [0,1,0] is sqrt(2) ≈ 1.4142
	l2Sqrt2, err := VectorDistanceL2(a, c)
	expectedL2 := float32(math.Sqrt(2))
	if err != nil || math.Abs(float64(l2Sqrt2-expectedL2)) > 1e-5 {
		t.Fatalf("expected L2 distance %f, got %f", expectedL2, l2Sqrt2)
	}
}
