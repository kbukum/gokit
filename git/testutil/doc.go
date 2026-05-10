// Package testutil provides testing utilities for the git module.
//
// It includes a test git component that creates disposable repositories and
// a fluent builder for constructing repositories with commits, branches, tags,
// and remotes for use in tests.
//
// # Quick Start
//
//	repos := testutil.NewComponent()
//	gotestutil.T(t).Setup(repos)
//
//	// Work with the temporary repository
//	if err := os.WriteFile(filepath.Join(repos.Root(), "README.md"), []byte("hello\n"), 0o644); err != nil {
//	    t.Fatal(err)
//	}
//
//	// Or build a repository fixture on the fly
//	repo := testutil.NewBuilder(t).
//	    WithFile("README.md", "hello\n").
//	    WithCommit("initial commit").
//	    Repo()
//
// See Component and Builder for the available helpers.
package testutil
