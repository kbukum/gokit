// Package dockerhub lists repositories and tags from the Docker Hub API, such as the models in the "ai" namespace,
// without contacting the registry or downloading anything.
//
// [Client.Repositories] pages through a namespace with popularity counts, [Client.Repository] reads one
// repository with its overview, and [Client.Tags] pages through a repository's tags with each tag's manifest
// digest and the total size the Hub reports. Resolve a tag's digest with the oci package to read the facts it pins.
//
// Hub data is untrusted: bodies are bounded, names, digests and pagination links are validated, an item that does
// not have the documented shape is skipped and counted instead of failing its page, and failures are typed
// [github.com/kbukum/gokit/errors.AppError] values that never carry a response body. The overview is the
// publisher's own text, often HTML or Markdown; render it only as untrusted text.
//
// The Hub API has its own rate limit, separate from the registry pull limit; reading a listing never counts as a
// pull.
package dockerhub
