package mcp

import (
	"fmt"

	"github.com/kbukum/gokit/skill"
	"github.com/kbukum/gokit/tool"
)

// SkillToServerAdapter builds a hardened MCP Server that exposes only the tools referenced by a skill manifest.
type SkillToServerAdapter struct {
	Manifest skill.Manifest
	Registry *tool.Registry
}

// NewServer validates the manifest and constructs a Server whose tool allow-list is pinned to the manifest's referenced tools.
func (a SkillToServerAdapter) NewServer(name, version string, opts ...ServerOption) (*Server, error) {
	if err := skill.Validate(&a.Manifest); err != nil {
		return nil, err
	}
	if a.Registry == nil {
		return nil, fmt.Errorf("mcp: tool registry is required")
	}
	serverOpts := append([]ServerOption{WithAllowedTools(a.Manifest.References.Tools...)}, opts...)
	return NewServer(name, version, a.Registry, serverOpts...)
}
