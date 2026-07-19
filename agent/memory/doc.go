// Package memory provides conversation persistence and context-window compaction for the agent loop: a Store keeps session history across runs, and a Policy compacts a message slice when it outgrows the provider's context window.
package memory
