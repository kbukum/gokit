// Package convert holds the typed conversions between gokit tool types
// and the MCP Go SDK wire types (tool definitions and call results).
// The conversions keep every surface typed: JSON-Schema documents are schema.JSON
// and safety hints map onto tool.Envelope so the parent mcp package
// and its client never hand-roll SDK marshaling.
package convert
