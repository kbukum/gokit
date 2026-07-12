// Package framing provides bounded length-delimited framing for streaming
// codec values over a byte transport.
//
// [WriteFrame] and [ReadFrame] move a single length-prefixed payload; the
// generic [WriteValue] / [ReadValue] helpers combine framing with a codec. A
// maximum frame size bounds memory, a clean EOF between frames is reported as
// [io.EOF], and truncated prefixes or payloads surface as transport errors
// distinct from a clean end of stream.
package framing
