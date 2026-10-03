package testutil

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"

	"github.com/kbukum/gokit/codec"
)

// databaseSnapshot is opaque, component-owned SQL column data. Production/load fixtures should use owned recreation rather than whole-database snapshots.
type databaseSnapshot struct {
	owner *Component
	data  map[string][]map[string]any
}

func (c *Component) snapshot(ctx context.Context) (*databaseSnapshot, error) {
	ctx, cancel := operationContext(ctx)
	defer cancel()
	if c.db.Name() != "sqlite" {
		return nil, fmt.Errorf("snapshots are limited to small SQLite fixtures: %w", errors.ErrUnsupported)
	}
	snapshot := &databaseSnapshot{owner: c, data: make(map[string][]map[string]any)}
	err := c.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		tables, tableErr := GetTableNames(ctx, tx)
		if tableErr != nil {
			return tableErr
		}
		totalRows, totalBytes := 0, 0
		for _, table := range tables {
			columns, columnErr := tx.Migrator().ColumnTypes(table)
			if columnErr != nil {
				return columnErr
			}
			if len(columns) > 128 {
				return fmt.Errorf("snapshot table exceeds 128 columns")
			}
			lengths := make([]string, len(columns))
			for i, column := range columns {
				quoted := tx.Statement.Quote(clause.Column{Name: column.Name()})
				lengths[i] = "coalesce(length(CAST(" + quoted + " AS BLOB)), 0)"
			}
			var stats struct{ Rows, Bytes int }
			// Check raw data size before materializing rows, including large BLOBs. The transaction keeps this preflight and the subsequent read consistent.
			query := "SELECT count(*) AS rows, coalesce(sum(" + strings.Join(lengths, "+") + "), 0) AS bytes FROM (SELECT * FROM " + quoteTable(tx, table) + " LIMIT ?)"
			if err := tx.Raw(query, MaxFixtureRows-totalRows+1).Scan(&stats).Error; err != nil {
				return err
			}
			totalRows += stats.Rows
			totalBytes += stats.Bytes
			if totalRows > MaxFixtureRows || totalBytes > MaxFixtureBytes {
				return fmt.Errorf("snapshot exceeds row or byte budget")
			}
			var rows []map[string]any
			if err := namedTable(tx, table).Limit(MaxFixtureRows + 1).Find(&rows).Error; err != nil {
				return err
			}
			snapshot.data[table] = rows
		}
		encoded, err := codec.Encode(codec.CompactJSON(), snapshot.data)
		if err != nil {
			return err
		}
		if len(encoded) > MaxFixtureBytes {
			return fmt.Errorf("snapshot exceeds %d encoded bytes", MaxFixtureBytes)
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	return snapshot, nil
}

func (c *Component) restore(ctx context.Context, snapshot *databaseSnapshot) error {
	ctx, cancel := operationContext(ctx)
	defer cancel()
	if snapshot == nil || snapshot.owner != c {
		return fmt.Errorf("snapshot belongs to another component")
	}
	return c.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		tables, err := GetTableNames(ctx, tx)
		if err != nil {
			return err
		}
		if len(tables) != len(snapshot.data) {
			return fmt.Errorf("snapshot schema changed; recreate the owned database")
		}
		for _, table := range tables {
			if _, ok := snapshot.data[table]; !ok {
				return fmt.Errorf("snapshot schema changed; recreate the owned database")
			}
		}
		if err := clearTables(tx, tables); err != nil {
			return err
		}
		for _, table := range tables {
			for _, row := range snapshot.data[table] {
				if err := namedTable(tx, table).Create(row).Error; err != nil {
					return err
				}
			}
		}
		return nil
	})
}
