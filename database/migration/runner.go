package migration

import (
	"fmt"

	"github.com/skillsenselab/gokit/logger"
	"gorm.io/gorm"
)

// Migration describes a single GORM-based schema migration.
type Migration struct {
	ID          string
	Description string
	Up          func(*gorm.DB) error
	Down        func(*gorm.DB) error
}

// MigrationRunner applies GORM-based migrations tracked in a schema_migrations table.
type MigrationRunner struct {
	db         *gorm.DB
	log        *logger.Logger
	migrations []Migration
}

// NewMigrationRunner creates a runner bound to the given database and logger.
func NewMigrationRunner(db *gorm.DB, log *logger.Logger) *MigrationRunner {
	return &MigrationRunner{
		db:         db,
		log:        log,
		migrations: make([]Migration, 0),
	}
}

// AddMigration registers a migration to be applied.
func (mr *MigrationRunner) AddMigration(migration Migration) {
	mr.migrations = append(mr.migrations, migration)
}

// RunMigrations applies all pending migrations in order.
func (mr *MigrationRunner) RunMigrations() error {
	if err := mr.createMigrationsTable(); err != nil {
		return fmt.Errorf("failed to create migrations table: %w", err)
	}

	for _, migration := range mr.migrations {
		applied, err := mr.isMigrationApplied(migration.ID)
		if err != nil {
			return fmt.Errorf("failed to check migration status: %w", err)
		}
		if applied {
			mr.log.Debug("Migration already applied", map[string]interface{}{
				"id": migration.ID,
			})
			continue
		}

		mr.log.Info("Applying migration", map[string]interface{}{
			"id":          migration.ID,
			"description": migration.Description,
		})

		if err := mr.db.Transaction(func(tx *gorm.DB) error {
			if err := migration.Up(tx); err != nil {
				return err
			}
			return mr.recordMigration(tx, migration.ID)
		}); err != nil {
			return fmt.Errorf("failed to apply migration %s: %w", migration.ID, err)
		}

		mr.log.Info("Migration applied successfully", map[string]interface{}{
			"id": migration.ID,
		})
	}

	return nil
}

func (mr *MigrationRunner) createMigrationsTable() error {
	return mr.db.Exec(`
		CREATE TABLE IF NOT EXISTS schema_migrations (
			id VARCHAR(255) PRIMARY KEY,
			applied_at TIMESTAMP WITH TIME ZONE DEFAULT NOW()
		)
	`).Error
}

func (mr *MigrationRunner) isMigrationApplied(id string) (bool, error) {
	var count int64
	err := mr.db.Table("schema_migrations").Where("id = ?", id).Count(&count).Error
	return count > 0, err
}

func (mr *MigrationRunner) recordMigration(tx *gorm.DB, id string) error {
	return tx.Exec("INSERT INTO schema_migrations (id) VALUES (?)", id).Error
}

// CreateIndexIfNotExists creates a PostgreSQL index only if it does not already exist.
func CreateIndexIfNotExists(tx *gorm.DB, table, index, columns string) error {
	var exists bool
	err := tx.Raw(`
		SELECT EXISTS (
			SELECT 1 FROM pg_indexes
			WHERE tablename = ? AND indexname = ?
		)
	`, table, index).Scan(&exists).Error
	if err != nil {
		return err
	}
	if !exists {
		return tx.Exec(fmt.Sprintf("CREATE INDEX %s ON %s (%s)", index, table, columns)).Error
	}
	return nil
}

// DropIndexIfExists drops a PostgreSQL index if it exists.
func DropIndexIfExists(tx *gorm.DB, index string) error {
	return tx.Exec(fmt.Sprintf("DROP INDEX IF EXISTS %s", index)).Error
}

// AddColumnIfNotExists adds a column to a table only if the column does not already exist.
func AddColumnIfNotExists(tx *gorm.DB, table, column, dataType string) error {
	var exists bool
	err := tx.Raw(`
		SELECT EXISTS (
			SELECT 1 FROM information_schema.columns
			WHERE table_name = ? AND column_name = ?
		)
	`, table, column).Scan(&exists).Error
	if err != nil {
		return err
	}
	if !exists {
		return tx.Exec(fmt.Sprintf("ALTER TABLE %s ADD COLUMN %s %s", table, column, dataType)).Error
	}
	return nil
}
