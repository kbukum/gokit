package model

import (
	"strings"
	"testing"
)

func TestOidStringAndZero(t *testing.T) {
	t.Parallel()

	var zero Oid
	if !zero.IsZero() {
		t.Fatal("zero OID reported non-zero")
	}
	if got := zero.String(); got != strings.Repeat("0", 40) {
		t.Fatalf("zero OID string = %q", got)
	}

	var oid Oid
	for i := range oid {
		oid[i] = byte(i)
	}
	if oid.IsZero() {
		t.Fatal("non-zero OID reported zero")
	}
	if got, want := oid.String(), "000102030405060708090a0b0c0d0e0f10111213"; got != want {
		t.Fatalf("OID string = %q, want %q", got, want)
	}
}

func TestEnumStrings(t *testing.T) {
	t.Parallel()

	statuses := map[FileStatus]string{
		FileAdded:       "added",
		FileModified:    "modified",
		FileDeleted:     "deleted",
		FileRenamed:     "renamed",
		FileCopied:      "copied",
		FileUntracked:   "untracked",
		FileIgnored:     "ignored",
		FileTypeChanged: "typechanged",
		FileConflicted:  "conflicted",
		FileStatus(99):  "unknown",
	}
	for status, want := range statuses {
		if got := status.String(); got != want {
			t.Fatalf("FileStatus(%d).String() = %q, want %q", status, got, want)
		}
	}

	states := map[EntryState]string{
		Staged:         "staged",
		Unstaged:       "unstaged",
		Untracked:      "untracked",
		Conflicted:     "conflicted",
		EntryState(99): "unknown",
	}
	for state, want := range states {
		if got := state.String(); got != want {
			t.Fatalf("EntryState(%d).String() = %q, want %q", state, got, want)
		}
	}

	kinds := map[EntryKind]string{
		EntryKindBlob:      "blob",
		EntryKindTree:      "tree",
		EntryKindSubmodule: "submodule",
		EntryKind(99):      "unknown",
	}
	for kind, want := range kinds {
		if got := kind.String(); got != want {
			t.Fatalf("EntryKind(%d).String() = %q, want %q", kind, got, want)
		}
	}

	modes := map[ResetMode]string{
		ResetMixed:    "mixed",
		ResetSoft:     "soft",
		ResetHard:     "hard",
		ResetMode(99): "unknown",
	}
	for mode, want := range modes {
		if got := mode.String(); got != want {
			t.Fatalf("ResetMode(%d).String() = %q, want %q", mode, got, want)
		}
	}
}
