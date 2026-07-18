package database

import "gorm.io/gorm"

// SetSessionVariable sets a PostgreSQL session variable using set_config(). When isLocal is true, the variable is scoped to the current transaction only. This is used for PostgreSQL Row Level Security (RLS) policies that read session variables via current_setting().
//
// Example: SetSessionVariable(db, "app.workspace_id", workspaceID, true)
func SetSessionVariable(db *gorm.DB, name, value string, isLocal bool) error {
	return db.Exec("SELECT set_config(?, ?, ?)", name, value, isLocal).Error
}

// ScopeToTenant returns a new GORM session with a WHERE clause filtering by the given tenant column and value. Use this to create a workspace-scoped database session for multi-tenant queries.
//
// Example: scopedDB := ScopeToTenant(db, "workspace_id", workspaceID)
func ScopeToTenant(db *gorm.DB, column string, value any) *gorm.DB {
	return db.Where(column+" = ?", value)
}
