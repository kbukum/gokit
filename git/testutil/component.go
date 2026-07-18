package testutil

import (
	"context"
	"fmt"
	"os"
	"sync"

	"github.com/kbukum/gokit/component"
	"github.com/kbukum/gokit/git"
	gotestutil "github.com/kbukum/gokit/testutil"
	"github.com/kbukum/gokit/util"
)

const (
	testUserName  = "Test User"
	testUserEmail = "test@example.com"
)

// Component is a test git component backed by a temporary repository. It implements both component.Component and testutil.TestComponent interfaces.
type Component struct {
	root      string
	repo      *git.Repo
	started   bool
	snapshots map[string]struct{}
	mu        sync.RWMutex
}

var (
	_ component.Component      = (*Component)(nil)
	_ gotestutil.TestComponent = (*Component)(nil)
)

// NewComponent creates a new test git component.
func NewComponent() *Component {
	return &Component{}
}

// Name returns the component name.
func (c *Component) Name() string { return "git-test" }

// Repo returns the underlying test repository, or nil if not started.
func (c *Component) Repo() *git.Repo {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.repo
}

// Root returns the repository root path, or empty string if not started.
func (c *Component) Root() string {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.root
}

// Start initializes a fresh temporary repository.
func (c *Component) Start(_ context.Context) error {
	c.mu.Lock()
	defer c.mu.Unlock()

	if c.started {
		return fmt.Errorf("component already started")
	}

	root, repo, err := newTempRepo()
	if err != nil {
		return err
	}

	c.root = root
	c.repo = repo
	c.started = true
	c.snapshots = make(map[string]struct{})
	return nil
}

// Stop removes the temporary repository and clears component state.
func (c *Component) Stop(_ context.Context) error {
	c.mu.Lock()
	defer c.mu.Unlock()

	if !c.started {
		return nil
	}

	root := c.root
	snapshots := c.snapshotRoots()
	c.root = ""
	c.repo = nil
	c.started = false
	c.snapshots = nil

	if root != "" {
		if err := util.RemoveAll(root); err != nil {
			return err
		}
	}
	for _, snapshot := range snapshots {
		if err := util.RemoveAll(snapshot); err != nil {
			return err
		}
	}
	return nil
}

// Health reports whether the test repository is available.
func (c *Component) Health(_ context.Context) component.Health {
	c.mu.RLock()
	defer c.mu.RUnlock()

	if !c.started || c.repo == nil || c.root == "" {
		return component.Health{Name: c.Name(), Status: component.StatusUnhealthy, Message: "repository not started"}
	}
	return component.Health{Name: c.Name(), Status: component.StatusHealthy}
}

// Reset deletes the repository contents and recreates a fresh repository at the same path.
func (c *Component) Reset(_ context.Context) error {
	c.mu.Lock()
	defer c.mu.Unlock()

	if !c.started || c.root == "" {
		return fmt.Errorf("component not started")
	}
	repo, err := recreateRepo(c.root)
	if err != nil {
		return err
	}
	c.repo = repo
	return nil
}

// Snapshot copies the repository to a separate snapshot directory.
func (c *Component) Snapshot(_ context.Context) (any, error) {
	c.mu.Lock()
	defer c.mu.Unlock()

	if !c.started || c.root == "" {
		return nil, fmt.Errorf("component not started")
	}

	snapshotRoot, err := os.MkdirTemp("", "gokit-git-snapshot-")
	if err != nil {
		return nil, fmt.Errorf("create snapshot directory: %w", err)
	}
	if err := util.CopyDir(c.root, snapshotRoot); err != nil {
		_ = util.RemoveAll(snapshotRoot)
		return nil, fmt.Errorf("copy snapshot: %w", err)
	}
	if c.snapshots == nil {
		c.snapshots = make(map[string]struct{})
	}
	c.snapshots[snapshotRoot] = struct{}{}
	return snapshotRoot, nil
}

// Restore restores the repository from a snapshot created by Snapshot.
func (c *Component) Restore(_ context.Context, snapshot any) error {
	c.mu.Lock()
	defer c.mu.Unlock()

	if !c.started || c.root == "" {
		return fmt.Errorf("component not started")
	}

	snapshotRoot, ok := snapshot.(string)
	if !ok {
		return fmt.Errorf("invalid snapshot type: expected string, got %T", snapshot)
	}
	if _, tracked := c.snapshots[snapshotRoot]; !tracked {
		return fmt.Errorf("unknown snapshot: %s", snapshotRoot)
	}
	defer func() {
		_ = util.RemoveAll(snapshotRoot)
		delete(c.snapshots, snapshotRoot)
	}()

	if err := util.RemoveAll(c.root); err != nil {
		return fmt.Errorf("remove repository root: %w", err)
	}
	if err := util.CopyDir(snapshotRoot, c.root); err != nil {
		return fmt.Errorf("restore snapshot: %w", err)
	}

	repo, err := git.Open(c.root)
	if err != nil {
		return fmt.Errorf("open restored repository: %w", err)
	}
	if err := configureRepo(repo); err != nil {
		return err
	}
	c.repo = repo
	return nil
}

func newTempRepo() (string, *git.Repo, error) {
	root, err := os.MkdirTemp("", "gokit-git-test-")
	if err != nil {
		return "", nil, fmt.Errorf("create temp directory: %w", err)
	}
	repo, err := recreateRepo(root)
	if err != nil {
		_ = util.RemoveAll(root)
		return "", nil, err
	}
	return root, repo, nil
}

func recreateRepo(root string) (*git.Repo, error) {
	if err := util.RemoveAll(root); err != nil {
		return nil, fmt.Errorf("remove repository root: %w", err)
	}
	if err := os.Mkdir(root, 0o700); err != nil {
		return nil, fmt.Errorf("create repository root: %w", err)
	}

	repo, err := git.Init(root)
	if err != nil {
		return nil, fmt.Errorf("init repository: %w", err)
	}
	if err := configureRepo(repo); err != nil {
		return nil, err
	}
	return repo, nil
}

func configureRepo(repo *git.Repo) error {
	if err := repo.ConfigSet("user.name", testUserName); err != nil {
		return fmt.Errorf("set user.name: %w", err)
	}
	if err := repo.ConfigSet("user.email", testUserEmail); err != nil {
		return fmt.Errorf("set user.email: %w", err)
	}
	return nil
}

func (c *Component) snapshotRoots() []string {
	snapshots := make([]string, 0, len(c.snapshots))
	for snapshot := range c.snapshots {
		snapshots = append(snapshots, snapshot)
	}
	return snapshots
}
