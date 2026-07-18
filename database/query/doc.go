// Package query translates request-level filtering, faceting, and includes into GORM query clauses.
//
// [ParseFromRequest] builds a structured query from request parameters
// and the Apply* helpers ([ApplyToGorm], [ApplyConditions], [ApplyIncludes], [ComputeFacetsWithFilters]) project it onto a GORM statement using parameterized conditions rather than string concatenation.
package query
