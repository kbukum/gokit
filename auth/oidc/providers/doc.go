// Package providers implements concrete OIDC/OAuth2 provider integrations (Apple, Google, and other generic providers) on top of the oidc package.
//
// Each provider supplies its authorization-URL construction, code exchange,
// and user-info mapping through a shared [ProviderConfig] so the oidc flow stays provider-agnostic.
// Provider-specific quirks (client-id parameter name, scope separator, claim mapping) are isolated here.
package providers
