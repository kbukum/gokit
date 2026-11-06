// Package huggingface reads model metadata from the Hugging Face Hub API without downloading model files.
//
// [Client.Search] lists models with cursor pagination, [Client.Model] reads one repository's details, including
// file sizes, LFS digests, card licenses and the GGUF facts the Hub extracts, and [Client.File] opens a bounded
// byte prefix of a repository file for the GGUF header reader. Every result reports whether the repository is
// gated, so a caller can tell a user to accept its terms before a download.
//
// Hub data is untrusted: bodies are bounded, repository IDs, revisions and file paths are validated, and failures
// are typed [github.com/kbukum/gokit/errors.AppError] values that never carry a response body. Gated or private
// access uses the injected adapter's auth, such as an access token; the client never stores credentials.
//
// See the [Hub API reference].
//
// [Hub API reference]: https://huggingface.co/docs/hub/api
package huggingface
