// Package authctx carries the authenticated principal through a request's [context.Context].
//
// [Set] attaches a typed principal to a context and [Get] / [GetOrError] retrieve it downstream without leaking auth types across package boundaries. Values are stored under an unexported key so only this package can read or write them.
package authctx
