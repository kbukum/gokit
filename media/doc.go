// Package media detects media types from content.
//
// [Detect], [DetectReader], and [DetectFile] inspect leading bytes to classify
// content into a [Type] and return [Info], providing a light, dependency-free
// alternative to trusting client-supplied content types. It is a deliberately
// light mirror of rskit's media capability; heavy audio/video/matrix operations
// stay rskit-only.
package media
