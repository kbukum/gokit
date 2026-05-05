package messaging

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestCoreModuleHasNoBrokerSDKDependencies(t *testing.T) {
	t.Parallel()

	banned := []string{
		"github.com/" + "segmentio/kafka-go",
		"github.com/" + "nats-io/nats.go",
		"github.com/" + "rabbitmq/amqp091-go",
	}

	for _, file := range []string{"go.mod", "go.sum"} {
		data, err := os.ReadFile(file)
		if err != nil && file == "go.sum" && os.IsNotExist(err) {
			continue
		}
		if err != nil {
			t.Fatalf("read %s: %v", file, err)
		}
		for _, dep := range banned {
			if strings.Contains(string(data), dep) {
				t.Fatalf("%s contains broker SDK dependency %q", file, dep)
			}
		}
	}
}

func TestCorePackagesDoNotImportBrokerSDKs(t *testing.T) {
	t.Parallel()

	banned := []string{
		"github.com/" + "segmentio/kafka-go",
		"github.com/" + "nats-io/nats.go",
		"github.com/" + "rabbitmq/amqp091-go",
	}

	err := filepath.WalkDir(".", func(path string, entry os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if entry.IsDir() {
			switch path {
			case "kafka", "nats", "rabbitmq":
				return filepath.SkipDir
			}
			return nil
		}
		if filepath.Ext(path) != ".go" {
			return nil
		}
		data, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		for _, dep := range banned {
			if strings.Contains(string(data), dep) {
				t.Fatalf("core file %s imports broker SDK %q", path, dep)
			}
		}
		return nil
	})
	if err != nil {
		t.Fatalf("scan core imports: %v", err)
	}
}
