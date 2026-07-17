package record

// Format identifies a tabular record serialization understood by the readers
// and writers in this package.
type Format int

const (
	// FormatCSV is comma-separated values with a header row.
	FormatCSV Format = iota
	// FormatJSONArray is a single JSON array of record objects.
	FormatJSONArray
	// FormatJSONLines is newline-delimited JSON record objects.
	FormatJSONLines
)

// String returns the lower-case name of the format.
func (f Format) String() string {
	switch f {
	case FormatCSV:
		return "csv"
	case FormatJSONArray:
		return "json_array"
	case FormatJSONLines:
		return "json_lines"
	default:
		return "unknown"
	}
}
