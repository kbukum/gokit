package logging

import (
	"context"
	"fmt"
	"io"
	"log/slog"
	"strings"
	"sync"
)

// consoleHandler renders records as compact, optionally colored single lines for
// local development: an optional service tag, a level tag, the message, then
// key=value attributes.
type consoleHandler struct {
	mu      *sync.Mutex
	out     io.Writer
	lv      slog.Leveler
	service string
	noColor bool
	time    bool
	attrs   []slog.Attr
	groups  []string
}

// newConsoleHandler builds a console sink honoring color, timestamp, and level
// settings from cfg, writing to w.
func newConsoleHandler(cfg *Config, serviceName string, lv slog.Leveler, w io.Writer) slog.Handler {
	return &consoleHandler{
		mu:      &sync.Mutex{},
		out:     w,
		lv:      lv,
		service: serviceName,
		noColor: cfg.NoColor,
		time:    cfg.Timestamp,
	}
}

func (h *consoleHandler) Enabled(_ context.Context, level slog.Level) bool {
	return level >= h.lv.Level()
}

func (h *consoleHandler) Handle(_ context.Context, rec slog.Record) error {
	var b strings.Builder
	if h.time {
		b.WriteString(rec.Time.Format("15:04:05"))
		b.WriteByte(' ')
	}
	b.WriteString(h.levelTag(rec.Level))
	b.WriteByte(' ')
	b.WriteString(sanitizeConsole(rec.Message))

	// Bound attrs were already group-qualified when WithAttrs captured them, so
	// they are written verbatim; record attrs are qualified here against the
	// currently open groups. Re-prefixing bound attrs would double the group
	// (db.db.host) and pull attrs bound before a group into it.
	for _, a := range h.attrs {
		h.writeAttr(&b, a.Key, a)
	}
	prefix := ""
	if len(h.groups) > 0 {
		prefix = strings.Join(h.groups, ".") + "."
	}
	rec.Attrs(func(a slog.Attr) bool {
		h.writeAttr(&b, prefix+a.Key, a)
		return true
	})
	b.WriteByte('\n')

	h.mu.Lock()
	defer h.mu.Unlock()
	_, err := io.WriteString(h.out, b.String())
	return err
}

func (h *consoleHandler) WithAttrs(attrs []slog.Attr) slog.Handler {
	next := *h
	next.attrs = append(append([]slog.Attr(nil), h.attrs...), h.qualify(attrs)...)
	return &next
}

func (h *consoleHandler) WithGroup(name string) slog.Handler {
	if name == "" {
		return h
	}
	next := *h
	next.groups = append(append([]string(nil), h.groups...), name)
	return &next
}

// qualify prefixes attribute keys with the currently open groups.
func (h *consoleHandler) qualify(attrs []slog.Attr) []slog.Attr {
	if len(h.groups) == 0 {
		return attrs
	}
	prefix := strings.Join(h.groups, ".") + "."
	out := make([]slog.Attr, len(attrs))
	for i, a := range attrs {
		out[i] = slog.Attr{Key: prefix + a.Key, Value: a.Value}
	}
	return out
}

func (h *consoleHandler) writeAttr(b *strings.Builder, key string, a slog.Attr) {
	if a.Equal(slog.Attr{}) {
		return
	}
	fmt.Fprintf(b, " %s=%s", key, sanitizeConsole(fmt.Sprintf("%v", a.Value.Resolve().Any())))
}

// sanitizeConsole neutralizes C0 control characters and DEL in untrusted message or
// attribute content so a value carrying CR/LF or ANSI escape (ESC) sequences cannot
// forge extra log lines or drive the terminal when rendered to the human-readable
// console sink. Tabs are preserved for alignment. The color/level tags this handler
// emits itself are applied to trusted, handler-controlled text and are unaffected.
// Structured sinks (JSON, OTLP) carry the raw value and are escaped by their encoder,
// so sanitization stays in the presentation layer rather than the logging facade.
func sanitizeConsole(s string) string {
	if strings.IndexFunc(s, isForgingRune) < 0 {
		return s
	}
	var b strings.Builder
	b.Grow(len(s))
	for _, r := range s {
		switch r {
		case '\n':
			b.WriteString(`\n`)
		case '\r':
			b.WriteString(`\r`)
		case '\t':
			b.WriteByte('\t')
		default:
			if isForgingRune(r) {
				fmt.Fprintf(&b, `\x%02x`, r)
			} else {
				b.WriteRune(r)
			}
		}
	}
	return b.String()
}

// isForgingRune reports whether r is a C0 control character (except tab) or DEL — the
// runes that can forge log lines or inject terminal escape sequences.
func isForgingRune(r rune) bool {
	return (r < 0x20 && r != '\t') || r == 0x7f
}

// levelTag renders the colored level tag, prefixed with a 3-letter service tag
// when a meaningful service name is set.
func (h *consoleHandler) levelTag(level slog.Level) string {
	var short string
	switch LevelName(level) {
	case "trace":
		short = "TRC"
	case "debug":
		short = "DBG"
	case "info":
		short = "INF"
	case "warn":
		short = "WRN"
	case "error":
		short = "ERR"
	case "fatal":
		short = "FTL"
	default:
		short = "LOG"
	}

	tag := fmt.Sprintf("[%s]", short)
	if !h.noColor {
		tag = colorize(short, level)
	}
	if h.service != "" && h.service != "default" && len(h.service) >= 3 {
		svc := strings.ToUpper(h.service[:3])
		if h.noColor {
			return fmt.Sprintf("[%s]%s", svc, tag)
		}
		return fmt.Sprintf("\033[34m[%s]\033[0m%s", svc, tag)
	}
	return tag
}

// colorize wraps the level tag in an ANSI color by severity.
func colorize(short string, level slog.Level) string {
	var color string
	switch {
	case level <= slog.LevelDebug:
		color = "36" // cyan
	case level < slog.LevelWarn:
		color = "32" // green (info)
	case level < slog.LevelError:
		color = "33" // yellow (warn)
	case level < LevelFatal:
		color = "31" // red (error)
	default:
		color = "35" // magenta (fatal)
	}
	return fmt.Sprintf("\033[%sm[%s]\033[0m", color, short)
}
