// Package httpx provides request binding and parsing for Gin adapters and the shared RFC 9457 response writer for HTTP handlers.
//
// All binding functions return gokit/errors.AppError on failure,
// so callers can pass errors directly to server.RespondWithError.
package httpx
