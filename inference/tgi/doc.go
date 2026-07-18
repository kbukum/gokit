// Package tgi implements an inference provider for Hugging Face Text Generation Inference servers.
//
// [New] builds a [Provider] from [Config] and [Register] installs it through a [Factory].
// The provider speaks TGI's OpenAI-compatible API
// so prediction requests are portable across compatible backends.
package tgi
