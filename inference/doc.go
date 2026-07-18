// Package inference defines the model-serving runtime adapter layer for gokit.
//
// It is intentionally lower-level than chat completions: adapters target Triton, KServe v2,
// vLLM raw generation, TGI, BentoML, ONNX Runtime Server, TFServing,
// and custom REST/gRPC model servers. Conversational LLM APIs live in the llm module;
// inference carries named inputs/outputs, tensors, bytes, text, JSON pass-through values,
// serving-runtime descriptors, and explicit adapter registration.
//
// Backends are opt-in sub-packages. They register with an injected Registry via
// explicit Register calls; there are no package-level global registries or init
// side effects. Adapters expose lean CapabilityHints (streaming, batching,
// tool-calls) on their Descriptor; the richer permission envelope owned by
// package tool is layered on top by consumers and is intentionally not part
// of the inference contract (D13: inference must not import tool).
package inference
