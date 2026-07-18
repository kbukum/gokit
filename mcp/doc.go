// Package mcp is a hardened, protocol-shaped Model Context Protocol server
// and client backed by gokit's tool system.
//
// It provides two integration points:
//
//   - Server: expose a tool.Registry as an MCP server so external clients
//     (LLMs, agents, IDEs) can discover and call kit tools, with a fail-closed
//     hardening chain (capability allow-list, payload/result size limits,
//     schema validation, authorization, destructive-tool gate, and audit) and
//     typed server-to-client helpers for sampling, elicitation, roots, and
//     logging.
//
//   - Client: connect to a remote MCP server and import its tools as
//     tool.Callable instances that can be registered in a local tool.Registry.
//
// The official MCP Go SDK owns the protocol wire (tools, prompts, resources and templates, subscribe, roots, sampling, elicitation, logging, progress, cancellation) over the stdio
// and Streamable HTTP transports. This package adds the security and observability layers on top
// and keeps every surface typed: untrusted client
// and model payloads are carried as json.RawMessage, and documented JSON-Schema is schema.JSON.
//
// # Server Example
//
//	registry := tool.NewRegistry()
//	if err := registry.Register(myTool.AsCallable()); err != nil { return err }
//
//	server, err := mcpkit.NewServer("my-service", "1.0.0", registry)
//	if err != nil { return err }
//	server.ServeStdio(ctx)
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
//	handler, err := server.StreamableHTTPHandler(mcpkit.StreamableHTTPConfig{
//	    AllowedOrigins: []string{"https://app.example.com"},
//	}, os.Getenv("MCP_TOKEN"))
//	if err != nil { return err }
//	http.Handle("/mcp", handler)
//
// # Client Example
//
//	session, callables, err := mcpkit.Connect(ctx, transport, nil)
//	for _, c := range callables {
//	    registry.Register(c)
//	}
package mcp
