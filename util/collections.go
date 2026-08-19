package util

import (
	"fmt"
	"slices"
)

// Contains checks if a slice contains a value.
func Contains[T comparable](slice []T, val T) bool {
	return slices.Contains(slice, val)
}

// Filter returns a new slice containing only elements that satisfy the predicate.
func Filter[T any](slice []T, predicate func(T) bool) []T {
	result := make([]T, 0)
	for _, item := range slice {
		if predicate(item) {
			result = append(result, item)
		}
	}
	return result
}

// Map transforms a slice using the given function.
func Map[T, U any](slice []T, transform func(T) U) []U {
	result := make([]U, len(slice))
	for i, item := range slice {
		result[i] = transform(item)
	}
	return result
}

// Unique returns a slice with duplicate values removed, preserving order.
func Unique[T comparable](slice []T) []T {
	seen := make(map[T]struct{}, len(slice))
	result := make([]T, 0, len(slice))
	for _, item := range slice {
		if _, ok := seen[item]; !ok {
			seen[item] = struct{}{}
			result = append(result, item)
		}
	}
	return result
}

// Keys returns the keys of a map.
func Keys[K comparable, V any](m map[K]V) []K {
	keys := make([]K, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	return keys
}

// Values returns the values of a map.
func Values[K comparable, V any](m map[K]V) []V {
	vals := make([]V, 0, len(m))
	for _, v := range m {
		vals = append(vals, v)
	}
	return vals
}

// GroupBy groups items into slices keyed by the key function, preserving order.
func GroupBy[T any, K comparable](items []T, key func(T) K) map[K][]T {
	out := make(map[K][]T)
	for _, item := range items {
		k := key(item)
		out[k] = append(out[k], item)
	}
	return out
}

// IndexBy indexes items by the key function. On duplicate keys the last item wins.
func IndexBy[T any, K comparable](items []T, key func(T) K) map[K]T {
	out := make(map[K]T, len(items))
	for _, item := range items {
		out[key(item)] = item
	}
	return out
}

// Partition splits items into those satisfying pred and the rest, preserving order.
func Partition[T any](items []T, pred func(T) bool) (matched, rest []T) {
	for _, item := range items {
		if pred(item) {
			matched = append(matched, item)
		} else {
			rest = append(rest, item)
		}
	}
	return matched, rest
}

// Chunk splits items into consecutive chunks of at most size elements.
// A size of zero or less returns nil.
func Chunk[T any](items []T, size int) [][]T {
	if size <= 0 {
		return nil
	}
	out := make([][]T, 0, (len(items)+size-1)/size)
	for i := 0; i < len(items); i += size {
		end := min(i+size, len(items))
		out = append(out, items[i:end:end])
	}
	return out
}

// FindDuplicatesBy returns the keys that occur more than once, in first-seen order.
func FindDuplicatesBy[T any, K comparable](items []T, key func(T) K) []K {
	counts := make(map[K]int, len(items))
	var dups []K
	for _, item := range items {
		k := key(item)
		counts[k]++
		if counts[k] == 2 {
			dups = append(dups, k)
		}
	}
	return dups
}

// DuplicateKeyError reports the first duplicated key found by EnsureUniqueBy.
type DuplicateKeyError[K comparable] struct {
	Key K
}

func (e *DuplicateKeyError[K]) Error() string {
	return fmt.Sprintf("duplicate key: %v", e.Key)
}

// EnsureUniqueBy returns a *DuplicateKeyError for the first duplicated key, or nil
// when every item's key is unique.
func EnsureUniqueBy[T any, K comparable](items []T, key func(T) K) error {
	seen := make(map[K]struct{}, len(items))
	for _, item := range items {
		k := key(item)
		if _, ok := seen[k]; ok {
			return &DuplicateKeyError[K]{Key: k}
		}
		seen[k] = struct{}{}
	}
	return nil
}
