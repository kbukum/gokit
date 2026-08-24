package embedded

import (
	"bytes"
	"strings"

	ggconfig "github.com/go-git/go-git/v5/config"
	rawconfig "github.com/go-git/go-git/v5/plumbing/format/config"

	giterr "github.com/kbukum/gokit/git/internal/giterr"
)

// ConfigGet gets the last configured value for a key.
func (b *Backend) ConfigGet(key string) (string, error) {
	values, err := b.ConfigGetAll(key)
	if err != nil {
		return "", err
	}
	return values[len(values)-1], nil
}

// ConfigGetAll gets all configured values for a key.
func (b *Backend) ConfigGetAll(key string) ([]string, error) {
	parts, err := parseConfigKey(key)
	if err != nil {
		return nil, err
	}
	cfg, err := b.repo.Config()
	if err != nil {
		return nil, giterr.Internal(err)
	}
	values := configValues(cfg.Raw, parts.section, parts.subsection, parts.key)
	if len(values) == 0 {
		return nil, giterr.ConfigNotFound(key)
	}
	return values, nil
}

// ConfigSet sets a config key to a single value.
func (b *Backend) ConfigSet(key, value string) error {
	parts, err := parseConfigKey(key)
	if err != nil {
		return err
	}
	cfg, err := b.repo.Config()
	if err != nil {
		return giterr.Internal(err)
	}
	if cfg.Raw == nil {
		cfg.Raw = rawconfig.New()
	}
	cfg.Raw.SetOption(parts.section, parts.subsection, parts.key, value)
	var buf bytes.Buffer
	if err := rawconfig.NewEncoder(&buf).Encode(cfg.Raw); err != nil {
		return giterr.Internal(err)
	}
	updated := ggconfig.NewConfig()
	if err := updated.Unmarshal(buf.Bytes()); err != nil {
		return giterr.Internal(err)
	}
	if err := b.repo.SetConfig(updated); err != nil {
		return giterr.Internal(err)
	}
	return nil
}

type parsedConfigKey struct {
	section    string
	subsection string
	key        string
}

func parseConfigKey(key string) (parsedConfigKey, error) {
	parts := strings.Split(key, ".")
	if len(parts) < 2 {
		return parsedConfigKey{}, giterr.InvalidConfigKey(key)
	}
	for _, part := range parts {
		if part == "" {
			return parsedConfigKey{}, giterr.InvalidConfigKey(key)
		}
	}
	parsed := parsedConfigKey{section: parts[0], key: parts[len(parts)-1]}
	if len(parts) > 2 {
		parsed.subsection = strings.Join(parts[1:len(parts)-1], ".")
	}
	return parsed, nil
}

func configValues(raw *rawconfig.Config, section, subsection, key string) []string {
	if raw == nil || !raw.HasSection(section) {
		return nil
	}
	sec := raw.Section(section)
	if subsection == "" {
		return append([]string(nil), sec.OptionAll(key)...)
	}
	if !sec.HasSubsection(subsection) {
		return nil
	}
	return append([]string(nil), sec.Subsection(subsection).OptionAll(key)...)
}
