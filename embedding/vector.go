package embedding

import (
	"fmt"
	"math"
)

// CosineSimilarity computes the cosine similarity between two vectors.
// Returns a value in [-1.0, 1.0] where 1.0 means identical direction.
// Returns 0.0 if either vector has zero magnitude.
func CosineSimilarity(a, b []float32) (float32, error) {
	if len(a) != len(b) {
		return 0, fmt.Errorf("vectors must have equal dimensions: %d != %d", len(a), len(b))
	}

	var dot, normA, normB float32
	for i := range a {
		dot += a[i] * b[i]
		normA += a[i] * a[i]
		normB += b[i] * b[i]
	}

	normA = float32(math.Sqrt(float64(normA)))
	normB = float32(math.Sqrt(float64(normB)))

	if normA == 0.0 || normB == 0.0 {
		return 0.0, nil
	}

	return dot / (normA * normB), nil
}

// EuclideanDistance computes the Euclidean (L2) distance between two vectors.
func EuclideanDistance(a, b []float32) (float32, error) {
	if len(a) != len(b) {
		return 0, fmt.Errorf("vectors must have equal dimensions: %d != %d", len(a), len(b))
	}

	var sumSquares float32
	for i := range a {
		diff := a[i] - b[i]
		sumSquares += diff * diff
	}

	return float32(math.Sqrt(float64(sumSquares))), nil
}

// DotProduct computes the dot product of two vectors.
func DotProduct(a, b []float32) (float32, error) {
	if len(a) != len(b) {
		return 0, fmt.Errorf("vectors must have equal dimensions: %d != %d", len(a), len(b))
	}

	var result float32
	for i := range a {
		result += a[i] * b[i]
	}
	return result, nil
}

// MeanPooling computes the element-wise mean of a collection of vectors.
// Returns an error if the input is empty or vectors have inconsistent dimensions.
func MeanPooling(vectors [][]float32) ([]float32, error) {
	if len(vectors) == 0 {
		return nil, fmt.Errorf("cannot compute mean pooling of empty vector slice")
	}

	dims := len(vectors[0])
	result := make([]float32, dims)

	for _, v := range vectors {
		if len(v) != dims {
			return nil, fmt.Errorf("all vectors must have equal dimensions: expected %d, got %d", dims, len(v))
		}
		for i := range v {
			result[i] += v[i]
		}
	}

	count := float32(len(vectors))
	for i := range result {
		result[i] /= count
	}

	return result, nil
}

// MaxPooling computes the element-wise maximum of a collection of vectors.
// Returns an error if the input is empty or vectors have inconsistent dimensions.
func MaxPooling(vectors [][]float32) ([]float32, error) {
	if len(vectors) == 0 {
		return nil, fmt.Errorf("cannot compute max pooling of empty vector slice")
	}

	dims := len(vectors[0])
	result := make([]float32, dims)
	for i := range result {
		result[i] = math.MaxFloat32 * -1
	}

	for _, v := range vectors {
		if len(v) != dims {
			return nil, fmt.Errorf("all vectors must have equal dimensions: expected %d, got %d", dims, len(v))
		}
		for i := range v {
			if v[i] > result[i] {
				result[i] = v[i]
			}
		}
	}

	return result, nil
}

// Normalize returns a normalized (unit) vector.
func Normalize(v []float32) ([]float32, error) {
	var norm float32
	for _, val := range v {
		norm += val * val
	}

	if norm == 0 {
		return nil, fmt.Errorf("cannot normalize zero vector")
	}

	norm = float32(math.Sqrt(float64(norm)))
	result := make([]float32, len(v))
	for i := range v {
		result[i] = v[i] / norm
	}

	return result, nil
}
