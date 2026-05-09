// Package mcp bridges gokit's tool system with the Model Context Protocol.
//
// It provides two integration points:
//
//   - Server: expose a tool.Registry as an MCP server so external clients
//     (LLMs, agents, IDEs) can discover and call kit tools.
//
//   - Client: connect to a remote MCP server and import its tools as
//     tool.Callable instances that can be registered in a local tool.Registry.
//
// The implementation uses the official MCP Go SDK directly while exposing
// lightweight kit-level helpers for agent-facing skill packs and hardened
// Streamable HTTP configuration.
//
// # Server Example
//
//	registry := tool.NewRegistry()
//	if err := registry.Register(myTool.AsCallable()); err != nil { return err }
//
//	server := mcpkit.NewServer("my-service", "1.0.0", registry)
//	server.Run(ctx, &mcp.StdioTransport{})
//
// # Skill Pack Example
//
//	manifest, err := skill.LoadManifest("./skills/database-inspector/" + skill.ManifestFileName)
//	if err != nil { return err }
//	server, err := (mcpkit.SkillToServerAdapter{
//	    Manifest: *manifest,
//	    Registry: registry,
//	}).NewServer("my-service", "1.0.0")
//	if err != nil { return err }
//
// # Skill Discovery Example
//
//	pack, err := skill.NewLoader().Load("./skills/database-inspector")
//	if err != nil { return err }
//	fmt.Println(pack.Manifest.Name, pack.Manifest.Description)
//
//	manifest, err := skill.LoadManifest("./skills/database-inspector/" + skill.ManifestFileName)
//	if err != nil { return err }
//	fmt.Println(manifest.Name, manifest.Description)
//
// # Streamable HTTP Example
//
//	opts, err := mcpkit.NewStreamableHTTPOptions(mcpkit.StreamableHTTPConfig{
//	    AllowedOrigins: []string{"https://app.example.com"},
//	})
//	if err != nil { return err }
//	handler := mcp.NewStreamableHTTPHandler(func(*http.Request) *mcp.Server { return server }, opts)
//
// # Client Example
//
//	session, callables, err := mcpkit.Connect(ctx, transport, nil)
//	for _, c := range callables {
//	    registry.Register(c)
//	}
package mcp
