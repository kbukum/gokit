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
// The implementation uses the official MCP Go SDK directly.
//
// # Server Example
//
//	registry := tool.NewRegistry()
//	if err := registry.Register(myTool.AsCallable()); err != nil { return err }
//
//	server := mcpkit.NewServer("my-service", "1.0.0", registry)
//	server.Run(ctx, &mcp.StdioTransport{})
//
// # Client Example
//
//	session, callables, err := mcpkit.Connect(ctx, transport, nil)
//	for _, c := range callables {
//	    registry.Register(c)
//	}
package mcp
