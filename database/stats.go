package database

import "database/sql"

// Stats exposes finite pool pressure and the lifetime slow-query count. Reader is nil for a shared pool.
type Stats struct {
	Primary     sql.DBStats
	Reader      *sql.DBStats
	SlowQueries uint64
}

// Stats snapshots the pools without starting a polling goroutine.
func (d *DB) Stats() (Stats, error) {
	pool, err := d.GormDB.DB()
	if err != nil {
		return Stats{}, err
	}
	result := Stats{Primary: pool.Stats()}
	if d.queryLog != nil {
		result.SlowQueries = d.queryLog.slowQueries.Load()
	}
	if d.reader != nil {
		reader, err := d.reader.DB()
		if err != nil {
			return Stats{}, err
		}
		stats := reader.Stats()
		result.Reader = &stats
	}
	return result, nil
}
