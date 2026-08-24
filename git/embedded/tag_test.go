package embedded_test

import (
	"testing"

	"github.com/kbukum/gokit/git/embedded"
)

func TestTagCRUD(t *testing.T) {
	t.Parallel()
	dir := initTestRepo(t)
	createTag(t, dir, "v1.0.0")
	repo, err := embedded.Open(dir, nil)
	if err != nil {
		t.Fatal(err)
	}

	if err := repo.CreateTag("v2.0.0", "HEAD", "release v2"); err != nil {
		t.Fatalf("CreateTag() error: %v", err)
	}
	tags, err := repo.ListTags()
	if err != nil {
		t.Fatalf("ListTags() error: %v", err)
	}
	foundLightweight := false
	foundAnnotated := false
	for _, tag := range tags {
		switch tag.Name {
		case "v1.0.0":
			foundLightweight = true
			if tag.Message != "" {
				t.Errorf("lightweight tag message = %q, want empty", tag.Message)
			}
		case "v2.0.0":
			foundAnnotated = true
			if tag.Message != "release v2\n" {
				t.Errorf("annotated tag message = %q", tag.Message)
			}
			if tag.Tagger == nil {
				t.Error("annotated tag missing tagger")
			}
		}
	}
	if !foundLightweight || !foundAnnotated {
		t.Fatal("ListTags() missing expected tags")
	}
	if err := repo.DeleteTag("v1.0.0"); err != nil {
		t.Fatalf("DeleteTag() error: %v", err)
	}
	tags, err = repo.ListTags()
	if err != nil {
		t.Fatalf("ListTags() after delete error: %v", err)
	}
	for _, tag := range tags {
		if tag.Name == "v1.0.0" {
			t.Fatal("DeleteTag() did not remove v1.0.0")
		}
	}
}

func TestTagErrors(t *testing.T) {
	t.Parallel()

	dir := initTestRepo(t)
	repo, err := embedded.Open(dir, nil)
	if err != nil {
		t.Fatal(err)
	}
	if err := repo.CreateTag("v1.0.0", "missing", ""); err == nil {
		t.Fatal("CreateTag(missing target) expected error")
	}
	if err := repo.CreateTag("v1.0.0", "HEAD", ""); err != nil {
		t.Fatalf("CreateTag() error: %v", err)
	}
	if err := repo.CreateTag("v1.0.0", "HEAD", ""); err == nil {
		t.Fatal("CreateTag(duplicate) expected error")
	}
	if err := repo.DeleteTag("missing"); err == nil {
		t.Fatal("DeleteTag(missing) expected error")
	}
}
