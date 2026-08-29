// Package huggingface provides a Hugging Face [llm.TokenCounter] backed by the
// pure-Go sugarme/tokenizer, for exact token counts against any tokenizer
// serialized in the Hugging Face tokenizer.json format.
//
// The tokenizer definition is loaded explicitly at construction from a local
// file or reader — never over the network — and is validated with a probe encode
// so a malformed definition fails fast. Counting is then deterministic with no
// import-time side effects. The strategy name embeds a short fingerprint of the
// definition bytes so distinct tokenizers are distinguishable. Build a counter
// with [FromFile], [FromReader], or [NewCounter] and inject it wherever a
// [llm.TokenCounter] is required (for example bench's token metric):
//
//	counter, err := huggingface.FromFile("tokenizer.json")
//	if err != nil {
//	    return err
//	}
//	n, err := counter.Count("hello world")
package huggingface
