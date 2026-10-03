package query

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"database/sql/driver"
	"encoding/base64"
	"encoding/hex"
	"reflect"
	"strconv"
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
	"gorm.io/gorm/schema"

	"github.com/kbukum/gokit/codec"
	apperrors "github.com/kbukum/gokit/errors"
)

// CursorParams requests a forward page. An empty cursor starts a traversal.
type CursorParams struct {
	PageSize int
	Cursor   string
	Query    FilterQuery
}

// CursorConfig defines server-owned ordering. Scope must identify the resource, tenant and any base-query restrictions not present in Query. Cursor columns must be immutable, non-null strings, integers, time.Time or uuid.UUID; custom SQL conversions and serializers are unsupported. UniqueBy must be the model's sole primary key.
type CursorConfig struct {
	Config
	Scope      string
	OrderBy    string
	UniqueBy   string
	Descending bool
}

// CursorPagination omits NextCursor at the end. Traversal is forward-only.
type CursorPagination struct {
	PageSize   int     `json:"pageSize"`
	NextCursor *string `json:"nextCursor,omitempty"`
}

// CursorResult is a bounded page; Data is an empty array rather than null when no rows match.
type CursorResult[T any] struct {
	Data       []T              `json:"data"`
	Pagination CursorPagination `json:"pagination"`
}

type cursorToken struct {
	Version int    `json:"v"`
	Binding string `json:"binding"`
	Order   string `json:"order"`
	Unique  string `json:"unique"`
}

const maxCursorBytes = 4096

// ApplyCursorToGorm executes a keyset page without a count query. A cursor is an untrusted position, never authorization; the caller must apply the same access restrictions on every request.
func ApplyCursorToGorm[T any](ctx context.Context, db *gorm.DB, params CursorParams, config CursorConfig) (*CursorResult[T], error) {
	if config.Scope == "" || len(config.Scope) > 512 {
		return nil, apperrors.InvalidInput("scope", "a bounded cursor scope is required")
	}
	statement := &gorm.Statement{DB: db}
	if err := statement.Parse(new(T)); err != nil {
		return nil, err
	}
	order := statement.Schema.LookUpField(config.ResolveField(config.OrderBy))
	unique := statement.Schema.LookUpField(config.ResolveField(config.UniqueBy))
	if !cursorField(order) || !cursorField(unique) || len(statement.Schema.PrimaryFields) != 1 || unique != statement.Schema.PrimaryFields[0] {
		return nil, apperrors.InvalidInput("order", "cursor ordering requires scalar fields and a single primary key")
	}
	binding, err := cursorBinding(params.Query, config, statement.Schema.Table)
	if err != nil {
		return nil, err
	}
	q := db.WithContext(ctx).Session(&gorm.Session{})
	if params.Query.FreeText != "" {
		q = applySearch(q, params.Query.FreeText, config.SearchFields)
	}
	q = ApplyConditions(q, params.Query.Conditions, config.Config)
	orderColumn := clause.Column{Table: clause.CurrentTable, Name: order.DBName}
	uniqueColumn := clause.Column{Table: clause.CurrentTable, Name: unique.DBName}
	if params.Cursor != "" {
		token, decodeErr := decodeCursor(params.Cursor, binding)
		if decodeErr != nil {
			return nil, decodeErr
		}
		orderValue, orderErr := parseCursorValue(token.Order, order)
		if orderErr != nil {
			return nil, orderErr
		}
		uniqueValue, uniqueErr := parseCursorValue(token.Unique, unique)
		if uniqueErr != nil {
			return nil, uniqueErr
		}
		var afterOrder, afterUnique clause.Expression = clause.Gt{Column: orderColumn, Value: orderValue}, clause.Gt{Column: uniqueColumn, Value: uniqueValue}
		if config.Descending {
			afterOrder = clause.Lt{Column: orderColumn, Value: orderValue}
			afterUnique = clause.Lt{Column: uniqueColumn, Value: uniqueValue}
		}
		q = q.Where(clause.Or(afterOrder, clause.And(clause.Eq{Column: orderColumn, Value: orderValue}, afterUnique)))
	}
	q = q.Clauses(clause.OrderBy{Columns: []clause.OrderByColumn{
		{Column: orderColumn, Desc: config.Descending, Reorder: true},
		{Column: uniqueColumn, Desc: config.Descending},
	}})
	size := pageSize(params.PageSize, config.Config)
	data := make([]T, 0)
	if findErr := q.Offset(-1).Limit(size + 1).Find(&data).Error; findErr != nil {
		return nil, findErr
	}
	result := &CursorResult[T]{Data: data, Pagination: CursorPagination{PageSize: size}}
	if len(data) <= size {
		return result, nil
	}
	result.Data = data[:size:size]
	last := reflect.ValueOf(data[size-1])
	orderText, err := cursorValue(ctx, last, order)
	if err != nil {
		return nil, err
	}
	uniqueText, err := cursorValue(ctx, last, unique)
	if err != nil {
		return nil, err
	}
	text, err := codec.Encode(codec.CompactJSON(), cursorToken{Version: 1, Binding: binding, Order: orderText, Unique: uniqueText})
	if err != nil {
		return nil, err
	}
	next := base64.RawURLEncoding.EncodeToString([]byte(text))
	if len(next) > maxCursorBytes {
		return nil, apperrors.InvalidInput("cursor", "ordering values exceed the cursor size limit")
	}
	result.Pagination.NextCursor = &next
	return result, nil
}

func cursorBinding(filter FilterQuery, cfg CursorConfig, table string) (string, error) {
	text, err := codec.Encode(codec.CompactJSON(), struct {
		Filter FilterQuery
		Config CursorConfig
		Table  string
	}{filter, cfg, table})
	if err != nil {
		return "", err
	}
	hash := sha256.Sum256([]byte(text))
	return hex.EncodeToString(hash[:]), nil
}

func decodeCursor(text, binding string) (cursorToken, error) {
	if len(text) > maxCursorBytes {
		return cursorToken{}, apperrors.InvalidInput("cursor", "cursor exceeds the size limit")
	}
	bytes, err := base64.RawURLEncoding.Strict().DecodeString(text)
	if err != nil {
		return cursorToken{}, apperrors.InvalidInput("cursor", "malformed cursor").WithCause(err)
	}
	token, err := codec.Decode[cursorToken](codec.CompactJSON(), string(bytes))
	if err != nil || token.Version != 1 || token.Binding != binding {
		return cursorToken{}, apperrors.InvalidInput("cursor", "cursor does not match this query").WithCause(err)
	}
	return token, nil
}

func cursorField(field *schema.Field) bool {
	if field == nil || !isSafeIdentifier(field.DBName) || field.FieldType.Kind() == reflect.Pointer || field.Serializer != nil {
		return false
	}
	if field.FieldType == reflect.TypeFor[uuid.UUID]() {
		return true
	}
	if field.FieldType == reflect.TypeFor[time.Time]() {
		return field.DataType == schema.Time
	}
	for _, typ := range []reflect.Type{field.FieldType, reflect.PointerTo(field.FieldType)} {
		if typ.Implements(reflect.TypeFor[driver.Valuer]()) || typ.Implements(reflect.TypeFor[sql.Scanner]()) || typ.Implements(reflect.TypeFor[gorm.Valuer]()) {
			return false
		}
	}
	kind := field.FieldType.Kind()
	switch field.DataType {
	case schema.String:
		return kind == reflect.String
	case schema.Int:
		return kind >= reflect.Int && kind <= reflect.Int64
	case schema.Uint:
		return kind >= reflect.Uint && kind <= reflect.Uint64
	default:
		return false
	}
}

func cursorValue(ctx context.Context, row reflect.Value, field *schema.Field) (string, error) {
	value, _ := field.ValueOf(ctx, row)
	v := reflect.ValueOf(value)
	if !v.IsValid() {
		return "", apperrors.InvalidInput("cursor", "cursor ordering values cannot be null")
	}
	if id, ok := value.(uuid.UUID); ok {
		return id.String(), nil
	}
	switch field.DataType {
	case schema.String:
		return v.String(), nil
	case schema.Int:
		return strconv.FormatInt(v.Int(), 10), nil
	case schema.Uint:
		return strconv.FormatUint(v.Uint(), 10), nil
	case schema.Time:
		if stamp, ok := value.(time.Time); ok {
			return stamp.Format(time.RFC3339Nano), nil
		}
	default:
		return "", apperrors.InvalidInput("cursor", "unsupported cursor ordering value")
	}
	return "", apperrors.InvalidInput("cursor", "unsupported cursor ordering value")
}

// parseCursorValue returns an opaque SQL binding with the model field's scalar type.
func parseCursorValue(text string, field *schema.Field) (any, error) {
	if field.FieldType == reflect.TypeFor[uuid.UUID]() {
		id, err := uuid.Parse(text)
		if err != nil {
			return nil, apperrors.InvalidInput("cursor", "invalid UUID ordering value").WithCause(err)
		}
		return id, nil
	}
	var value any
	var err error
	switch field.DataType {
	case schema.String:
		value = text
	case schema.Int:
		value, err = strconv.ParseInt(text, 10, field.FieldType.Bits())
	case schema.Uint:
		value, err = strconv.ParseUint(text, 10, field.FieldType.Bits())
	case schema.Time:
		value, err = time.Parse(time.RFC3339Nano, text)
	default:
		return nil, apperrors.InvalidInput("cursor", "unsupported cursor ordering type")
	}
	if err != nil {
		return nil, apperrors.InvalidInput("cursor", "invalid cursor ordering value").WithCause(err)
	}
	return value, nil
}
