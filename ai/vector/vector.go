package vector

import (
	"errors"
	"fmt"
	"math"
)

// ErrDimensionMismatch reports that two vectors have unequal lengths. It wraps the offending dimensions in the returned error and can be matched with [errors.Is].
var ErrDimensionMismatch = errors.New("vectors must have equal dimensions")

// ErrNonFinite reports that a vector carries a NaN or infinite component. Provider-supplied vectors are untrusted, so a non-finite component is rejected before it can produce a non-finite score that cannot be persisted as JSON or compared meaningfully. Match it with [errors.Is].
var ErrNonFinite = errors.New("vector contains a non-finite component")

// requireFinite returns [ErrNonFinite] on the first NaN or infinite component.
func requireFinite(v []float32) error {
	for _, x := range v {
		if f := float64(x); math.IsNaN(f) || math.IsInf(f, 0) {
			return ErrNonFinite
		}
	}
	return nil
}

// toFinite32 narrows a float64 accumulator to float32, returning [ErrNonFinite] if the result is NaN, infinite, or too large to represent as a finite float32. Pairwise operations accumulate in float64 and finalize through this, so finite inputs that would overflow float32 arithmetic (for example squared math.MaxFloat32 components) are rejected rather than silently returned as Inf/NaN.
func toFinite32(v float64) (float32, error) {
	if math.IsNaN(v) || math.IsInf(v, 0) || v > math.MaxFloat32 || v < -math.MaxFloat32 {
		return 0, ErrNonFinite
	}
	return float32(v), nil
}

// CosineSimilarity computes the cosine similarity between two vectors. Returns a value in [-1.0, 1.0] where 1.0 means identical direction. Returns 0.0 if either vector has zero magnitude. Returns [ErrDimensionMismatch] on unequal lengths and [ErrNonFinite] if either vector has a NaN or infinite component or the result is not a finite float32.
func CosineSimilarity(a, b []float32) (float32, error) {
	if len(a) != len(b) {
		return 0, fmt.Errorf("%w: %d != %d", ErrDimensionMismatch, len(a), len(b))
	}
	if err := requireFinite(a); err != nil {
		return 0, err
	}
	if err := requireFinite(b); err != nil {
		return 0, err
	}

	var dot, normA, normB float64
	for i := range a {
		av, bv := float64(a[i]), float64(b[i])
		dot += av * bv
		normA += av * av
		normB += bv * bv
	}

	normA = math.Sqrt(normA)
	normB = math.Sqrt(normB)
	if normA == 0.0 || normB == 0.0 {
		return 0.0, nil
	}

	return toFinite32(dot / (normA * normB))
}

// EuclideanDistance computes the Euclidean (L2) distance between two vectors. Returns [ErrDimensionMismatch] on unequal lengths and [ErrNonFinite] if either vector has a NaN or infinite component or the result is not a finite float32.
func EuclideanDistance(a, b []float32) (float32, error) {
	if len(a) != len(b) {
		return 0, fmt.Errorf("%w: %d != %d", ErrDimensionMismatch, len(a), len(b))
	}
	if err := requireFinite(a); err != nil {
		return 0, err
	}
	if err := requireFinite(b); err != nil {
		return 0, err
	}

	var sumSquares float64
	for i := range a {
		diff := float64(a[i]) - float64(b[i])
		sumSquares += diff * diff
	}

	return toFinite32(math.Sqrt(sumSquares))
}

// DotProduct computes the dot product of two vectors. Returns [ErrDimensionMismatch] on unequal lengths and [ErrNonFinite] if either vector has a NaN or infinite component or the result is not a finite float32.
func DotProduct(a, b []float32) (float32, error) {
	if len(a) != len(b) {
		return 0, fmt.Errorf("%w: %d != %d", ErrDimensionMismatch, len(a), len(b))
	}
	if err := requireFinite(a); err != nil {
		return 0, err
	}
	if err := requireFinite(b); err != nil {
		return 0, err
	}

	var result float64
	for i := range a {
		result += float64(a[i]) * float64(b[i])
	}
	return toFinite32(result)
}

// MeanPooling computes the element-wise mean of a collection of vectors. Returns an error if the input is empty or vectors have inconsistent dimensions.
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

// MaxPooling computes the element-wise maximum of a collection of vectors. Returns an error if the input is empty or vectors have inconsistent dimensions.
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
