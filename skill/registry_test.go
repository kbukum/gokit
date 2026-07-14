package skill_test

import (
	"errors"
	"testing"

	"github.com/kbukum/gokit/skill"
)

func TestRegistryRejectsInvalidManifest(t *testing.T) {
	reg := skill.NewRegistry()
	if err := reg.Register(provider{manifest: skill.Manifest{}}); !errors.Is(err, skill.ErrManifestInvalid) {
		t.Fatalf("want ErrManifestInvalid, got %v", err)
	}
}
