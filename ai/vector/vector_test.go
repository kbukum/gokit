package vector

import (
	"errors"
	"math"
	"testing"
)

func TestCosineSimilarity(t *testing.T) {
	tests := []struct {
		name      string
		a         []float32
		b         []float32
		wantSim   float32
		wantErr   bool
		tolerance float32
	}{
		{
			name:      "identical vectors",
			a:         []float32{1.0, 2.0, 3.0},
			b:         []float32{1.0, 2.0, 3.0},
			wantSim:   1.0,
			tolerance: 1e-6,
		},
		{
			name:      "orthogonal vectors",
			a:         []float32{1.0, 0.0},
			b:         []float32{0.0, 1.0},
			wantSim:   0.0,
			tolerance: 1e-6,
		},
		{
			name:      "opposite vectors",
			a:         []float32{1.0, 0.0},
			b:         []float32{-1.0, 0.0},
			wantSim:   -1.0,
			tolerance: 1e-6,
		},
		{
			name:      "zero vector",
			a:         []float32{1.0, 2.0},
			b:         []float32{0.0, 0.0},
			wantSim:   0.0,
			tolerance: 1e-6,
		},
		{
			name:    "mismatched dimensions",
			a:       []float32{1.0},
			b:       []float32{1.0, 2.0},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := CosineSimilarity(tt.a, tt.b)
			if (err != nil) != tt.wantErr {
				t.Errorf("CosineSimilarity() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if !tt.wantErr && math.Abs(float64(got-tt.wantSim)) > float64(tt.tolerance) {
				t.Errorf("CosineSimilarity() = %v, want %v", got, tt.wantSim)
			}
		})
	}
}

func TestEuclideanDistance(t *testing.T) {
	tests := []struct {
		name      string
		a         []float32
		b         []float32
		wantDist  float32
		wantErr   bool
		tolerance float32
	}{
		{
			name:      "same point",
			a:         []float32{1.0, 2.0, 3.0},
			b:         []float32{1.0, 2.0, 3.0},
			wantDist:  0.0,
			tolerance: 1e-6,
		},
		{
			name:      "known distance 3-4-5 triangle",
			a:         []float32{0.0, 0.0},
			b:         []float32{3.0, 4.0},
			wantDist:  5.0,
			tolerance: 1e-6,
		},
		{
			name:    "mismatched dimensions",
			a:       []float32{1.0},
			b:       []float32{1.0, 2.0},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := EuclideanDistance(tt.a, tt.b)
			if (err != nil) != tt.wantErr {
				t.Errorf("EuclideanDistance() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if !tt.wantErr && math.Abs(float64(got-tt.wantDist)) > float64(tt.tolerance) {
				t.Errorf("EuclideanDistance() = %v, want %v", got, tt.wantDist)
			}
		})
	}
}

func TestDotProduct(t *testing.T) {
	tests := []struct {
		name      string
		a         []float32
		b         []float32
		want      float32
		wantErr   bool
		tolerance float32
	}{
		{
			name:      "known dot product",
			a:         []float32{1.0, 2.0, 3.0},
			b:         []float32{4.0, 5.0, 6.0},
			want:      32.0,
			tolerance: 1e-6,
		},
		{
			name:      "orthogonal vectors",
			a:         []float32{1.0, 0.0},
			b:         []float32{0.0, 1.0},
			want:      0.0,
			tolerance: 1e-6,
		},
		{
			name:    "mismatched dimensions",
			a:       []float32{1.0},
			b:       []float32{1.0, 2.0},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := DotProduct(tt.a, tt.b)
			if (err != nil) != tt.wantErr {
				t.Errorf("DotProduct() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if !tt.wantErr && math.Abs(float64(got-tt.want)) > float64(tt.tolerance) {
				t.Errorf("DotProduct() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestMeanPooling(t *testing.T) {
	tests := []struct {
		name      string
		vectors   [][]float32
		want      []float32
		wantErr   bool
		tolerance float32
	}{
		{
			name:    "single vector",
			vectors: [][]float32{{2.0, 4.0}},
			want:    []float32{2.0, 4.0},
		},
		{
			name:      "multiple vectors",
			vectors:   [][]float32{{1.0, 3.0}, {3.0, 1.0}},
			want:      []float32{2.0, 2.0},
			tolerance: 1e-6,
		},
		{
			name:    "empty vectors",
			vectors: [][]float32{},
			wantErr: true,
		},
		{
			name:    "mismatched dimensions",
			vectors: [][]float32{{1.0}, {1.0, 2.0}},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := MeanPooling(tt.vectors)
			if (err != nil) != tt.wantErr {
				t.Errorf("MeanPooling() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if !tt.wantErr {
				if len(got) != len(tt.want) {
					t.Errorf("MeanPooling() length = %d, want %d", len(got), len(tt.want))
					return
				}
				for i := range got {
					if math.Abs(float64(got[i]-tt.want[i])) > float64(tt.tolerance) {
						t.Errorf("MeanPooling()[%d] = %v, want %v", i, got[i], tt.want[i])
					}
				}
			}
		})
	}
}

func TestMaxPooling(t *testing.T) {
	tests := []struct {
		name      string
		vectors   [][]float32
		want      []float32
		wantErr   bool
		tolerance float32
	}{
		{
			name:      "selects max",
			vectors:   [][]float32{{1.0, 4.0}, {3.0, 2.0}},
			want:      []float32{3.0, 4.0},
			tolerance: 1e-6,
		},
		{
			name:      "single vector",
			vectors:   [][]float32{{2.0, 4.0}},
			want:      []float32{2.0, 4.0},
			tolerance: 1e-6,
		},
		{
			name:    "empty vectors",
			vectors: [][]float32{},
			wantErr: true,
		},
		{
			name:    "mismatched dimensions",
			vectors: [][]float32{{1.0}, {1.0, 2.0}},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := MaxPooling(tt.vectors)
			if (err != nil) != tt.wantErr {
				t.Errorf("MaxPooling() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if !tt.wantErr {
				if len(got) != len(tt.want) {
					t.Errorf("MaxPooling() length = %d, want %d", len(got), len(tt.want))
					return
				}
				for i := range got {
					if math.Abs(float64(got[i]-tt.want[i])) > float64(tt.tolerance) {
						t.Errorf("MaxPooling()[%d] = %v, want %v", i, got[i], tt.want[i])
					}
				}
			}
		})
	}
}

func TestNormalize(t *testing.T) {
	tests := []struct {
		name      string
		v         []float32
		wantLen   float32
		wantErr   bool
		tolerance float32
	}{
		{
			name:      "non-zero vector",
			v:         []float32{3.0, 4.0},
			wantLen:   1.0,
			tolerance: 1e-6,
		},
		{
			name:    "zero vector",
			v:       []float32{0.0, 0.0},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := Normalize(tt.v)
			if (err != nil) != tt.wantErr {
				t.Errorf("Normalize() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if !tt.wantErr {
				// Check magnitude is 1
				var mag float32
				for _, v := range got {
					mag += v * v
				}
				mag = float32(math.Sqrt(float64(mag)))
				if math.Abs(float64(mag-tt.wantLen)) > float64(tt.tolerance) {
					t.Errorf("Normalize() magnitude = %v, want %v", mag, tt.wantLen)
				}
			}
		})
	}
}

func TestPairwiseRejectsNonFiniteComponents(t *testing.T) {
	nan := float32(math.NaN())
	inf := float32(math.Inf(1))
	for _, tc := range []struct {
		name string
		a, b []float32
	}{
		{"nan in a", []float32{nan, 0}, []float32{1, 0}},
		{"inf in b", []float32{1, 0}, []float32{inf, 0}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			for name, fn := range map[string]func(a, b []float32) (float32, error){
				"CosineSimilarity":  CosineSimilarity,
				"EuclideanDistance": EuclideanDistance,
				"DotProduct":        DotProduct,
			} {
				if _, err := fn(tc.a, tc.b); !errors.Is(err, ErrNonFinite) {
					t.Errorf("%s: error = %v, want ErrNonFinite", name, err)
				}
			}
		})
	}
}

func TestPairwiseDimensionMismatchIsSentinel(t *testing.T) {
	for name, fn := range map[string]func(a, b []float32) (float32, error){
		"CosineSimilarity":  CosineSimilarity,
		"EuclideanDistance": EuclideanDistance,
		"DotProduct":        DotProduct,
	} {
		if _, err := fn([]float32{1}, []float32{1, 2}); !errors.Is(err, ErrDimensionMismatch) {
			t.Errorf("%s: error = %v, want ErrDimensionMismatch", name, err)
		}
	}
}

func TestPairwiseRejectsFloat32OverflowResult(t *testing.T) {
	// Finite components whose products overflow float32 must be rejected, not
	// returned as Inf/NaN: DotProduct of two MaxFloat32 vectors is ~1e76.
	big := []float32{math.MaxFloat32, math.MaxFloat32}
	if _, err := DotProduct(big, big); !errors.Is(err, ErrNonFinite) {
		t.Errorf("DotProduct overflow: error = %v, want ErrNonFinite", err)
	}
	if _, err := EuclideanDistance(big, []float32{-math.MaxFloat32, -math.MaxFloat32}); !errors.Is(err, ErrNonFinite) {
		t.Errorf("EuclideanDistance overflow: error = %v, want ErrNonFinite", err)
	}
	// Cosine normalizes to [-1,1], so the same large finite inputs stay finite.
	if got, err := CosineSimilarity(big, big); err != nil || got < 0.999 {
		t.Errorf("CosineSimilarity(big, big) = %v, %v; want ~1.0, nil", got, err)
	}
}
