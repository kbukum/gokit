// Package vllm implements an inference provider for vLLM servers.
//
// [New] builds a [Provider] from [Config] and [Register] installs it through a [Factory].
// The provider uses vLLM's OpenAI-compatible API
// so prediction requests are portable across compatible backends.
package vllm
