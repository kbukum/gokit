package database

import (
	"github.com/kbukum/gokit/provider/namedregistry"

	"gorm.io/gorm"
)

// DriverFunc creates a GORM dialector from a DSN.
type DriverFunc func(dsn string) gorm.Dialector

// DriverRegistry stores database driver factories by backend name.
type DriverRegistry struct {
	inner *namedregistry.Registry[DriverFunc]
}

// NewDriverRegistry creates an isolated driver registry.
func NewDriverRegistry() *DriverRegistry {
	return &DriverRegistry{inner: namedregistry.New[DriverFunc]("database-driver")}
}

// Register stores a driver factory.
func (r *DriverRegistry) Register(name string, fn DriverFunc) error {
	return r.inner.Register(name, fn)
}

// Get returns a driver factory by name.
func (r *DriverRegistry) Get(name string) (DriverFunc, bool) {
	return r.inner.Get(name)
}
