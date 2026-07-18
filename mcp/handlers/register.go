package handlers

// Register wires the allowed tools, prompts, resources, and resource templates into the SDK server.
// Tools are filtered by the allow-list at registration time; prompts
// and resources are additionally gated at call time by their wrappers.
func (h *Handler) Register(prompts []PromptEntry, resources []ResourceEntry, templates []ResourceTemplateEntry) {
	defs := h.registry.List()
	for i := range defs {
		callable, ok := h.registry.Get(defs[i].Name)
		if !ok || !h.policy.AllowsTool(defs[i].Name) {
			continue
		}
		h.registerTool(callable)
	}
	for _, entry := range prompts {
		h.sdk.AddPrompt(entry.Prompt, h.wrapPromptHandler(entry))
	}
	for _, entry := range resources {
		h.sdk.AddResource(entry.Resource, h.wrapResourceHandler(entry.Resource.URI, entry.Handler))
	}
	for _, entry := range templates {
		h.sdk.AddResourceTemplate(entry.Template, h.wrapResourceHandler(entry.Template.URITemplate, entry.Handler))
	}
}
