package handlers

import (
	"encoding/json"
	"fmt"
)

// contentTooLarge reports whether the untrusted content v exceeds the configured result size limit.
// It measures v's serialized JSON size;
// a marshal failure is treated as oversized whenever a limit is configured,
// so content that cannot be measured fails closed rather than bypassing the size gate.
// reason is a human-readable rejection cause when tooLarge is true.
func (h *Handler) contentTooLarge(v any) (reason string, tooLarge bool) {
	if h.policy.MaxResultBytes <= 0 {
		return "", false
	}
	data, err := json.Marshal(v)
	if err != nil {
		return fmt.Sprintf("content is not serializable and cannot be checked against limit %d", h.policy.MaxResultBytes), true
	}
	if size := len(data); h.policy.ResultTooLarge(size) {
		return fmt.Sprintf("size %d exceeds limit %d", size, h.policy.MaxResultBytes), true
	}
	return "", false
}
