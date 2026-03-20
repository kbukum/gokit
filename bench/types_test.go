package bench

import "testing"

func TestSampleCreation(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name  string
		id    string
		label string
		input []byte
	}{
		{"basic string label", "s1", "positive", []byte("hello")},
		{"empty input", "s2", "negative", nil},
		{"with metadata", "s3", "neutral", []byte("data")},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			s := Sample[string]{
				ID:    tt.id,
				Input: tt.input,
				Label: tt.label,
			}
			if s.ID != tt.id {
				t.Errorf("ID = %q, want %q", s.ID, tt.id)
			}
			if s.Label != tt.label {
				t.Errorf("Label = %q, want %q", s.Label, tt.label)
			}
			if string(s.Input) != string(tt.input) {
				t.Errorf("Input = %q, want %q", s.Input, tt.input)
			}
		})
	}
}

func TestSampleWithMetadata(t *testing.T) {
	t.Parallel()

	s := Sample[string]{
		ID:       "m1",
		Label:    "positive",
		Source:   "test-set",
		Metadata: map[string]any{"lang": "en", "score": 0.9},
	}
	if s.Source != "test-set" {
		t.Errorf("Source = %q, want %q", s.Source, "test-set")
	}
	if s.Metadata["lang"] != "en" {
		t.Errorf("Metadata[lang] = %v, want %q", s.Metadata["lang"], "en")
	}
}

func TestPredictionCreation(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		sampleID string
		label    string
		score    float64
	}{
		{"high confidence positive", "s1", "positive", 0.95},
		{"low confidence negative", "s2", "negative", 0.1},
		{"borderline", "s3", "positive", 0.5},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			p := Prediction[string]{
				SampleID: tt.sampleID,
				Label:    tt.label,
				Score:    tt.score,
			}
			if p.SampleID != tt.sampleID {
				t.Errorf("SampleID = %q, want %q", p.SampleID, tt.sampleID)
			}
			if p.Label != tt.label {
				t.Errorf("Label = %q, want %q", p.Label, tt.label)
			}
			if p.Score != tt.score {
				t.Errorf("Score = %f, want %f", p.Score, tt.score)
			}
		})
	}
}

func TestPredictionWithScores(t *testing.T) {
	t.Parallel()

	p := Prediction[string]{
		SampleID: "s1",
		Label:    "cat",
		Score:    0.8,
		Scores:   map[string]float64{"cat": 0.8, "dog": 0.15, "bird": 0.05},
	}
	if p.Scores["cat"] != 0.8 {
		t.Errorf("Scores[cat] = %f, want 0.8", p.Scores["cat"])
	}
	if p.Scores["dog"] != 0.15 {
		t.Errorf("Scores[dog] = %f, want 0.15", p.Scores["dog"])
	}
}

func TestScoredSamplePairing(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name      string
		sLabel    string
		pLabel    string
		wantMatch bool
	}{
		{"matching labels", "positive", "positive", true},
		{"mismatched labels", "positive", "negative", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			s := Sample[string]{ID: "s1", Label: tt.sLabel, Input: []byte("data")}
			p := Prediction[string]{SampleID: "s1", Label: tt.pLabel, Score: 0.9}
			ss := ScoredSample[string]{Sample: s, Prediction: p}

			if ss.Sample.ID != ss.Prediction.SampleID {
				t.Errorf("sample ID %q != prediction SampleID %q", ss.Sample.ID, ss.Prediction.SampleID)
			}
			match := ss.Sample.Label == ss.Prediction.Label
			if match != tt.wantMatch {
				t.Errorf("label match = %v, want %v", match, tt.wantMatch)
			}
		})
	}
}

func TestLabelMapper(t *testing.T) {
	t.Parallel()

	mapper := LabelMapper[string](func(s string) (string, error) {
		return s, nil
	})

	label, err := mapper("positive")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if label != "positive" {
		t.Errorf("label = %q, want %q", label, "positive")
	}
}

func TestLabelMapperIntLabels(t *testing.T) {
	t.Parallel()

	mapper := LabelMapper[int](func(s string) (int, error) {
		switch s {
		case "cat":
			return 0, nil
		case "dog":
			return 1, nil
		default:
			return -1, nil
		}
	})

	got, err := mapper("cat")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got != 0 {
		t.Errorf("label = %d, want 0", got)
	}
}
