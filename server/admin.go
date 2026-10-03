package server

import (
	"context"
	"errors"
	"fmt"
	"net"
	"net/http"
	"net/http/pprof"
	"strconv"
	"time"

	"github.com/kbukum/gokit/observability"
)

// AdminConfig enables a separate diagnostics listener. Restrict it to a trusted network; the default bind host is loopback. Port zero asks the OS for an ephemeral port.
type AdminConfig struct {
	Enabled bool         `yaml:"enabled" mapstructure:"enabled"`
	Host    string       `yaml:"host" mapstructure:"host"`
	Port    int          `yaml:"port" mapstructure:"port"`
	Pprof   bool         `yaml:"pprof" mapstructure:"pprof"`
	Metrics http.Handler `yaml:"-" mapstructure:"-"`
}

func (c AdminConfig) Validate() error {
	if c.Port < 0 || c.Port > 65535 {
		return fmt.Errorf("server.admin.port must be between 0 and 65535")
	}
	return nil
}

// AdminAddr returns the bound diagnostics address, or nil when disabled/not started.
func (s *Server) AdminAddr() net.Addr {
	if s.adminListener == nil {
		return nil
	}
	return s.adminListener.Addr()
}

func (s *Server) startAdmin(ctx context.Context) error {
	cfg := s.config.Admin
	if cfg == nil || !cfg.Enabled {
		return nil
	}
	host := cfg.Host
	if host == "" {
		host = "127.0.0.1"
	}
	mux := http.NewServeMux()
	metrics := cfg.Metrics
	if metrics == nil {
		var err error
		metrics, err = observability.RuntimeMetricsHandler()
		if err != nil {
			return err
		}
	}
	mux.Handle("GET /metrics", metrics)
	if cfg.Pprof {
		mux.HandleFunc("GET /debug/pprof/", pprof.Index)
		mux.HandleFunc("GET /debug/pprof/cmdline", pprof.Cmdline)
		mux.HandleFunc("GET /debug/pprof/profile", pprof.Profile)
		mux.HandleFunc("GET /debug/pprof/symbol", pprof.Symbol)
		mux.HandleFunc("POST /debug/pprof/symbol", pprof.Symbol)
		mux.HandleFunc("GET /debug/pprof/trace", pprof.Trace)
	}
	var lc net.ListenConfig
	listener, err := lc.Listen(ctx, "tcp", net.JoinHostPort(host, strconv.Itoa(cfg.Port)))
	if err != nil {
		return fmt.Errorf("admin listener: %w", err)
	}
	s.adminListener = listener
	s.adminServer = &http.Server{Handler: mux, ReadHeaderTimeout: 5 * time.Second, IdleTimeout: 30 * time.Second, WriteTimeout: 30 * time.Second}
	s.serveWG.Add(1)
	go func() {
		defer s.serveWG.Done()
		if err := s.adminServer.Serve(listener); err != nil && !errors.Is(err, http.ErrServerClosed) {
			s.log.Error("Admin listener failed", map[string]any{"error": err.Error()})
		}
	}()
	return nil
}

func (s *Server) stopAdmin(ctx context.Context) error {
	if s.adminServer == nil {
		return nil
	}
	if err := s.adminServer.Shutdown(ctx); err != nil {
		return errors.Join(err, s.adminServer.Close())
	}
	return nil
}
