package sse

import (
	"encoding/json"
	"os"
	"testing"
)

// convergenceState is a fixture oracle, not a shipped client. It specifies the acknowledgement and generation rules consuming kits must implement against their own cache lifecycle.
type convergenceState struct {
	Acknowledged uint64 `json:"acknowledged"`
	Applied      int    `json:"applied"`
	Generation   int    `json:"generation"`
	Refetches    int    `json:"refetches"`
	Accepted     int    `json:"accepted"`
	Dirty        bool   `json:"dirty"`
}

func TestPublishedConvergenceScenarios(t *testing.T) {
	t.Parallel()
	data, err := os.ReadFile("testdata/convergence.json")
	if err != nil {
		t.Fatal(err)
	}
	var fixtures struct {
		MaxRefetches int `json:"maxRefetches"`
		Cases        []struct {
			Name       string `json:"name"`
			Operations []struct {
				Kind         string `json:"kind"`
				Sequence     uint64 `json:"sequence"`
				WantSequence uint64 `json:"wantSequence"`
			} `json:"operations"`
			Want convergenceState `json:"want"`
		} `json:"cases"`
	}
	if err := json.Unmarshal(data, &fixtures); err != nil {
		t.Fatal(err)
	}
	for _, c := range fixtures.Cases {
		t.Run(c.Name, func(t *testing.T) {
			t.Parallel()
			state := convergenceState{Dirty: true}
			pending, generation, revision := false, 0, 0
			for _, op := range c.Operations {
				switch op.Kind {
				case "event":
					// Receiving/parsing is not acknowledgement.
				case "apply":
					if op.Sequence > state.Acknowledged {
						state.Acknowledged = op.Sequence
						state.Applied++
						state.Dirty = true
					}
				case "reconnect":
					if state.Acknowledged != op.WantSequence {
						t.Fatal("unapplied cursor used for reconnect")
					}
				case "reset":
					state.Generation++
					state.Acknowledged = 0
					state.Dirty = true
				case "snapshotStart":
					if !pending && state.Refetches < fixtures.MaxRefetches {
						pending, generation, revision = true, state.Generation, state.Applied
						state.Refetches++
					}
				case "snapshotComplete":
					if !pending {
						t.Fatal("snapshot completion without request")
					}
					if generation == state.Generation && revision == state.Applied {
						state.Accepted++
						state.Dirty = false
					}
					pending = false
				default:
					t.Fatalf("unknown operation %q", op.Kind)
				}
			}
			if state != c.Want {
				t.Fatalf("state %+v != %+v", state, c.Want)
			}
		})
	}
}
