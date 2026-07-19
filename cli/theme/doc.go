// Package theme is the visual vocabulary shared by every CLI renderer: color and status glyphs.
//
// The theme layer resolves how output looks against the environment and user preference,
// independent of what is being rendered:
//
//   - [Palette] — a semantic color set (success/error/warn/info/dim/bold) that
//     honors the NO_COLOR standard and TTY detection.
//   - [Glyphs] — a semantic symbol set (✓ ✗ ⚠ ℹ • → …) with a pure-ASCII
//     fallback for terminals without UTF-8 support.
//   - [Style] — the two bundled together, so renderers thread one value instead of a
//     separate palette/glyphs pair.
//
// Both resolve from a single boolean
// so callers render identically regardless of terminal capability,
// and both expose an environment-free constructor ([NewPalette], [NewGlyphs]) for deterministic tests alongside the environment-driven resolvers ([ResolveColor], [GlyphsFromEnv]).
package theme
