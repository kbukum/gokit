package semconv

const (
	GenAISystem               = "gen_ai.system"
	GenAIOperationName        = "gen_ai.operation.name"
	GenAIRequestID            = "gen_ai.request.id"
	GenAIRequestModel         = "gen_ai.request.model"
	GenAIRequestModelVersion  = "gen_ai.request.model.version"
	GenAIRequestMaxTokens     = "gen_ai.request.max_tokens" // #nosec G101 -- OTel semantic-convention key, not a credential.
	GenAIRequestTemperature   = "gen_ai.request.temperature"
	GenAIResponseModel        = "gen_ai.response.model"
	GenAIResponseFinishReason = "gen_ai.response.finish_reason"
	GenAIToolName             = "gen_ai.tool.name"
	GenAIUsageInputTokens     = "gen_ai.usage.input_tokens"     // #nosec G101 -- OTel semantic-convention key, not a credential.
	GenAIUsageOutputTokens    = "gen_ai.usage.output_tokens"    // #nosec G101 -- OTel semantic-convention key, not a credential.
	GenAIUsageCachedTokens    = "gen_ai.usage.cached_tokens"    // #nosec G101 -- OTel semantic-convention key, not a credential.
	GenAIUsageReasoningTokens = "gen_ai.usage.reasoning_tokens" // #nosec G101 -- OTel semantic-convention key, not a credential.
)

const (
	OpChat             = "chat"
	OpTextCompletion   = "text_completion"
	OpEmbedding        = "embedding"
	OpAgentRun         = "agent.run"
	OpAgentTurn        = "agent.turn"
	OpLLMCall          = "llm.call"
	OpToolCall         = "tool.call"
	OpMCPRequest       = "mcp.request"
	OpStream           = "stream"
	OpInferenceRequest = "inference.request"
)
