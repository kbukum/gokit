package mcp

import (
	"fmt"

	sdkmcp "github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/kbukum/gokit/skill"
	"github.com/kbukum/gokit/tool"
)

type SkillToServerAdapter struct {
	Manifest skill.Manifest
	Registry *tool.Registry
}

func (a SkillToServerAdapter) NewServer(name, version string, opts ...ServerOption) (*sdkmcp.Server, error) {
	if err := skill.Validate(&a.Manifest); err != nil {
		return nil, err
	}
	if a.Registry == nil {
		return nil, fmt.Errorf("mcp: tool registry is required")
	}
	serverOpts := append([]ServerOption{WithAllowedTools(a.Manifest.References.Tools...)}, opts...)
	return NewServer(name, version, a.Registry, serverOpts...)
}
