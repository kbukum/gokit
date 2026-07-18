// Package httpx provides request binding and parsing helpers for gin-based HTTP port adapters.
// It pairs with the server.Respond* functions to give port implementations a complete,
// consistent HTTP toolkit.
//
// All binding functions return gokit/errors.AppError on failure,
// so callers can pass errors directly to server.RespondWithError.
package httpx
