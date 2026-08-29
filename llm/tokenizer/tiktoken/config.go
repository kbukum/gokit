package tiktoken

// Encoding identifies an OpenAI BPE encoding available offline via the embedded
// vocab. These are the encodings tiktoken-go-loader bundles.
type Encoding string

const (
	// O200kBase is the encoding used by GPT-4o and o-series models.
	O200kBase Encoding = "o200k_base"
	// Cl100kBase is the encoding used by GPT-3.5-turbo and GPT-4.
	Cl100kBase Encoding = "cl100k_base"
	// P50kBase is the encoding used by older Codex and text-davinci models.
	P50kBase Encoding = "p50k_base"
	// R50kBase is the encoding used by GPT-3 (davinci) models.
	R50kBase Encoding = "r50k_base"
)

// valid reports whether e is one of the supported offline encodings.
func (e Encoding) valid() bool {
	switch e {
	case O200kBase, Cl100kBase, P50kBase, R50kBase:
		return true
	default:
		return false
	}
}

// Config selects the encoding by name; it is the config-driven registration seam
// for constructing a counter via [NewCounter].
type Config struct {
	// Encoding is the tiktoken encoding name (for example "cl100k_base").
	Encoding string
}
