package live_test

import (
	"bytes"
	"strings"
	"sync"
	"testing"

	"github.com/kbukum/gokit/cli/live"
	"github.com/kbukum/gokit/cli/theme"
)

func newConsole(buf *bytes.Buffer, cfg live.Config) *live.Console {
	return live.NewConsole(buf, cfg, theme.NewPalette(false))
}

func TestConsoleRendersRegionTilesAndLatestLines(t *testing.T) {
	t.Parallel()
	var buf bytes.Buffer
	console := newConsole(&buf, live.Config{MaxRegions: 4, RegionHeight: 3})
	region, err := console.AddRegion("build")
	if err != nil {
		t.Fatal(err)
	}
	region.Println("compiling")
	region.Println("linking")
	if err := console.Render(); err != nil {
		t.Fatal(err)
	}
	out := buf.String()
	if !strings.Contains(out, "build") || !strings.Contains(out, "compiling") || !strings.Contains(out, "linking") {
		t.Errorf("render missing content:\n%s", out)
	}
}

func TestRegionRetainsOnlyRecentLines(t *testing.T) {
	t.Parallel()
	var buf bytes.Buffer
	console := newConsole(&buf, live.Config{MaxRegions: 2, RegionHeight: 2})
	region, _ := console.AddRegion("task")
	region.Println("one")
	region.Println("two")
	region.Println("three")
	if err := console.Render(); err != nil {
		t.Fatal(err)
	}
	out := buf.String()
	if strings.Contains(out, "one") {
		t.Errorf("scrolled-out line must drop from live view:\n%s", out)
	}
	if !strings.Contains(out, "two") || !strings.Contains(out, "three") {
		t.Errorf("recent lines must be retained:\n%s", out)
	}
}

func TestSecondRenderOverwritesPreviousFrame(t *testing.T) {
	t.Parallel()
	var buf bytes.Buffer
	console := newConsole(&buf, live.Config{MaxRegions: 2, RegionHeight: 3})
	region, _ := console.AddRegion("task")
	region.Println("first")
	if err := console.Render(); err != nil {
		t.Fatal(err)
	}
	buf.Reset()
	region.Println("second")
	if err := console.Render(); err != nil {
		t.Fatal(err)
	}
	out := buf.String()
	// Second frame must move the cursor up to overwrite the prior frame.
	if !strings.Contains(out, "\x1b[") || !strings.Contains(out, "A") {
		t.Errorf("second render must reposition cursor:\n%q", out)
	}
}

func TestAddRegionEnforcesMaxRegionsBound(t *testing.T) {
	t.Parallel()
	var buf bytes.Buffer
	console := newConsole(&buf, live.Config{MaxRegions: 1, RegionHeight: 2})
	if _, err := console.AddRegion("a"); err != nil {
		t.Fatal(err)
	}
	if _, err := console.AddRegion("b"); err == nil {
		t.Error("exceeding MaxRegions must error (backpressure)")
	}
}

func TestFinishWritesDurableVerdicts(t *testing.T) {
	t.Parallel()
	var buf bytes.Buffer
	console := live.NewConsole(&buf, live.Config{MaxRegions: 3, RegionHeight: 2}, theme.NewPalette(false))
	ok, _ := console.AddRegion("build")
	ok.Println("...")
	ok.Done("passed")
	bad, _ := console.AddRegion("test")
	bad.Fail("2 failures")
	if err := console.Render(); err != nil {
		t.Fatal(err)
	}
	buf.Reset()
	if err := console.Finish(); err != nil {
		t.Fatal(err)
	}
	out := buf.String()
	if !strings.Contains(out, "✓ build: passed") {
		t.Errorf("done verdict missing:\n%s", out)
	}
	if !strings.Contains(out, "✗ test: 2 failures") {
		t.Errorf("failed verdict missing:\n%s", out)
	}
}

func TestConfigDefaultsApplied(t *testing.T) {
	t.Parallel()
	var buf bytes.Buffer
	console := newConsole(&buf, live.Config{})
	// Defaults allow several regions; add a handful without error.
	for range 8 {
		if _, err := console.AddRegion("r"); err != nil {
			t.Fatalf("default MaxRegions too small: %v", err)
		}
	}
}

func TestRegionConcurrentAppendsAreRaceFree(t *testing.T) {
	t.Parallel()
	var buf bytes.Buffer
	console := newConsole(&buf, live.Config{MaxRegions: 4, RegionHeight: 10})
	region, _ := console.AddRegion("worker")

	var wg sync.WaitGroup
	for i := range 4 {
		wg.Add(1)
		go func(n int) {
			defer wg.Done()
			for j := range 20 {
				region.Println("line")
				_ = n
				_ = j
			}
		}(i)
	}
	// Render concurrently with appends to exercise the lock.
	wg.Add(1)
	go func() {
		defer wg.Done()
		for range 10 {
			_ = console.Render()
		}
	}()
	wg.Wait()
	if err := console.Render(); err != nil {
		t.Fatal(err)
	}
}
