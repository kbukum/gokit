// Package tiktoken provides an OpenAI BPE [llm.TokenCounter] backed by
// tiktoken-go, for exact token counts against OpenAI encodings.
//
// The encoding is chosen explicitly at construction and the BPE ranks are loaded
// from the offline, embedded vocab (tiktoken-go-loader), so counting is fully
// deterministic with no network access and no import-time side effects. Build a
// counter with [New] or [NewCounter] and inject it wherever a [llm.TokenCounter]
// is required (for example bench's token metric):
//
//	counter, err := tiktoken.New(tiktoken.Cl100kBase)
//	if err != nil {
//	    return err
//	}
//	n, err := counter.Count("hello world")
package tiktoken
