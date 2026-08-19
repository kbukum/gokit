// Package util provides small, generic helpers shared across gokit — the scoped
// foundation owner for capabilities too small to deserve their own package, never a
// dumping ground. Prefer the modern standard library (slices, maps, cmp) first and a
// dedicated owner (fs for filesystem, codec for formats) where one exists.
//
// The helpers group by concern:
//
//   - Collections: Contains, Filter, Map, Unique, Chunk, Partition, GroupBy, IndexBy,
//     Keys, Values, Coalesce, DeepMerge, and uniqueness checks.
//   - Pointers: Ptr and Deref.
//   - Strings: casing (ToSnakeCase, ToKebabCase, ToCamelCase), Truncate/TruncateEllipsis,
//     glob matching (GlobMatch, NewGlob), and fuzzy lookup (Nearest, ResolveUnique).
//   - Sanitization: SanitizeString, SanitizeEnvValue, and IsSafeString boundary checks.
//   - Secrets: SecretString and SecretKeyMatcher for in-memory redaction, plus MaskSecret.
//     Cryptography stays in the encryption and security packages.
//   - Environment: GetEnv, GetEnvOr, GetEnvBool, and typed GetEnvParsed.
//   - Hashing: ContentHasher and the Sha256Hex/HashHex/Sha256Reader helpers.
//   - Sizes and time: FormatBytes/ParseBytes, FormatDuration/ParseDuration, TimeIt, and
//     the injectable Clock (with FakeClock for deterministic tests).
//   - Templates: ParseTemplate and ParseDynamicTemplate for placeholder substitution.
package util
