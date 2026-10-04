package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"
	"time"

	"github.com/kbukum/gokit/auth/testhost"
	"github.com/kbukum/gokit/codec"
)

func main() {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	err := run(ctx, os.Args[1:], os.Stdout)
	stop()
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func run(ctx context.Context, args []string, output io.Writer) error {
	flags := flag.NewFlagSet("auth-host", flag.ContinueOnError)
	flags.SetOutput(io.Discard)
	config := testhost.Config{}
	var fixturePath, initialize string
	flags.StringVar(&initialize, "init-fixture", "", "create a private synthetic fixture file and exit; never overwrite")
	flags.StringVar(&fixturePath, "fixture", "", "private fixture file from -init-fixture")
	flags.StringVar(&config.Origin, "origin", "", "exact loopback HTTPS origin, with explicit port (0 selects an ephemeral port)")
	flags.StringVar(&config.CertFile, "cert", "", "trusted loopback TLS certificate")
	flags.StringVar(&config.KeyFile, "key", "", "private TLS key")
	flags.StringVar(&config.StateFile, "state", "", "isolated SQLite file; restart retains existing state")
	flags.StringVar(&config.RunID, "run-id", "", "runner-owned readiness identity")
	flags.StringVar(&config.BuildID, "build-id", "", "source/build identity expected by the runner")
	flags.StringVar(&config.AssetsDir, "assets", "", "optional absolute browser build directory served at the same HTTPS origin")
	if err := flags.Parse(args); err != nil {
		return err
	}
	if flags.NArg() != 0 {
		return errors.New("auth-host accepts named flags only")
	}
	if initialize != "" {
		return initializeFixture(initialize, output)
	}
	if err := config.Validate(); err != nil {
		return err
	}
	fixture, err := testhost.LoadFixture(fixturePath)
	if err != nil {
		return err
	}
	defer func() { clear(fixture.DigestKey); clear(fixture.CSRFKey); clear(fixture.KeyPepper) }()
	startCtx, cancelStart := context.WithTimeout(ctx, 30*time.Second)
	host, err := testhost.New(startCtx, config, fixture)
	cancelStart()
	if err != nil {
		return err
	}
	announcement, err := codec.Encode(codec.CompactJSON(), struct {
		Origin   string `json:"origin"`
		Protocol string `json:"protocol"`
		RunID    string `json:"runId"`
		BuildID  string `json:"buildId"`
	}{host.Origin(), testhost.ProtocolVersion, config.RunID, config.BuildID})
	if err == nil {
		_, err = fmt.Fprintln(output, "auth-host-ready "+announcement)
	}
	if err != nil {
		return errors.Join(err, host.Close(ctx))
	}
	<-ctx.Done()
	return host.Close(ctx)
}

func initializeFixture(path string, output io.Writer) error {
	if !filepath.IsAbs(path) {
		return errors.New("fixture path must be absolute")
	}
	fixture, err := testhost.NewFixture()
	if err != nil {
		return err
	}
	defer func() { clear(fixture.DigestKey); clear(fixture.CSRFKey); clear(fixture.KeyPepper) }()
	data, err := codec.Encode(codec.CompactJSON(), fixture)
	if err != nil {
		return err
	}
	file, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o600)
	if err != nil {
		return fmt.Errorf("fixture must be a new file in an existing private directory: %w", err)
	}
	_, writeErr := file.WriteString(data)
	err = errors.Join(writeErr, file.Sync(), file.Close())
	if err != nil {
		return errors.Join(err, os.Remove(path))
	}
	_, err = fmt.Fprintln(output, "private auth fixture created")
	return err
}
