package storage

import (
	"encoding/json"
	"testing"
	"time"
)

func TestFileInfo_MarshalsToSnakeCaseJSON(t *testing.T) {
	t.Parallel()

	fi := FileInfo{
		Path:         "dir/a.txt",
		Size:         5,
		LastModified: time.Date(2023, time.January, 2, 3, 4, 5, 0, time.UTC),
		ContentType:  "text/plain",
		Metadata:     map[string]string{"owner": "team"},
		Checksum:     "abc123",
	}

	data, err := json.Marshal(fi)
	if err != nil {
		t.Fatalf("Marshal: %v", err)
	}

	var raw map[string]json.RawMessage
	if err := json.Unmarshal(data, &raw); err != nil {
		t.Fatalf("Unmarshal: %v", err)
	}
	for _, key := range []string{"path", "size", "last_modified", "content_type", "metadata", "checksum"} {
		if _, ok := raw[key]; !ok {
			t.Errorf("missing key %q in %s", key, data)
		}
	}
}

func TestFileInfo_OmitsEmptyMetadataAndChecksum(t *testing.T) {
	t.Parallel()

	data, err := json.Marshal(FileInfo{Path: "a.txt", Size: 1})
	if err != nil {
		t.Fatalf("Marshal: %v", err)
	}
	var raw map[string]json.RawMessage
	if err := json.Unmarshal(data, &raw); err != nil {
		t.Fatalf("Unmarshal: %v", err)
	}
	if _, ok := raw["metadata"]; ok {
		t.Errorf("metadata should be omitted: %s", data)
	}
	if _, ok := raw["checksum"]; ok {
		t.Errorf("checksum should be omitted: %s", data)
	}
}
