package testutil_test

import (
	"fmt"

	dbtestutil "github.com/kbukum/gokit/database/testutil"
	"github.com/kbukum/gokit/testutil"
)

// Example of basic database test component usage
func Example_basicUsage() {
	// This would be in a test function
	// t := &testing.T{} // mocked for example
	
	db := dbtestutil.NewComponent()
	// In real tests: testutil.T(t).Setup(db)
	db.Start(nil)
	defer db.Stop(nil)
	
	// Create table and insert data
	db.DB().Exec("CREATE TABLE users (id INTEGER PRIMARY KEY, name TEXT)")
	db.DB().Exec("INSERT INTO users (name) VALUES (?)", "Alice")
	
	// Query data
	var name string
	db.DB().Raw("SELECT name FROM users").Scan(&name)
	
	fmt.Println(name)
	// Output: Alice
}

// Example of using models with auto-migration
func Example_withModels() {
	type User struct {
		ID   uint   `gorm:"primarykey"`
		Name string
	}
	
	db := dbtestutil.NewComponent().WithModels(&User{})
	db.Start(nil)
	defer db.Stop(nil)
	
	// Table is automatically created
	db.DB().Create(&User{Name: "Bob"})
	
	var user User
	db.DB().First(&user)
	
	fmt.Println(user.Name)
	// Output: Bob
}

// Example of using Reset for test isolation
func Example_reset() {
	db := dbtestutil.NewComponent()
	db.Start(nil)
	defer db.Stop(nil)
	
	db.DB().Exec("CREATE TABLE users (id INTEGER PRIMARY KEY, name TEXT)")
	db.DB().Exec("INSERT INTO users (name) VALUES (?)", "Alice")
	
	var count int64
	db.DB().Raw("SELECT COUNT(*) FROM users").Scan(&count)
	fmt.Println("Before reset:", count)
	
	// Reset clears all data
	db.Reset(nil)
	
	db.DB().Raw("SELECT COUNT(*) FROM users").Scan(&count)
	fmt.Println("After reset:", count)
	
	// Output:
	// Before reset: 1
	// After reset: 0
}

// Example of using Snapshot and Restore
func Example_snapshotRestore() {
	db := dbtestutil.NewComponent()
	db.Start(nil)
	defer db.Stop(nil)
	
	db.DB().Exec("CREATE TABLE users (id INTEGER PRIMARY KEY, name TEXT)")
	db.DB().Exec("INSERT INTO users (name) VALUES (?)", "Alice")
	
	// Take snapshot
	snapshot, _ := db.Snapshot(nil)
	
	// Modify data
	db.DB().Exec("INSERT INTO users (name) VALUES (?)", "Bob")
	db.DB().Exec("INSERT INTO users (name) VALUES (?)", "Charlie")
	
	var count int64
	db.DB().Raw("SELECT COUNT(*) FROM users").Scan(&count)
	fmt.Println("After modifications:", count)
	
	// Restore to snapshot
	db.Restore(nil, snapshot)
	
	db.DB().Raw("SELECT COUNT(*) FROM users").Scan(&count)
	fmt.Println("After restore:", count)
	
	// Output:
	// After modifications: 3
	// After restore: 1
}

// Example of using fixture helpers
func Example_fixtures() {
	// In real tests, you would pass *testing.T
	db := dbtestutil.NewComponent()
	db.Start(nil)
	defer db.Stop(nil)
	
	db.DB().Exec("CREATE TABLE users (id INTEGER PRIMARY KEY, name TEXT, email TEXT)")
	
	// Load fixture data
	dbtestutil.LoadFixture(db.DB(), "users", []map[string]interface{}{
		{"name": "Alice", "email": "alice@example.com"},
		{"name": "Bob", "email": "bob@example.com"},
	})
	
	count, _ := dbtestutil.CountRows(db.DB(), "users")
	fmt.Println("User count:", count)
	
	// Truncate table
	dbtestutil.TruncateTable(db.DB(), "users")
	
	count, _ = dbtestutil.CountRows(db.DB(), "users")
	fmt.Println("After truncate:", count)
	
	// Output:
	// User count: 2
	// After truncate: 0
}

// Example of using TestManager with database component
func Example_testManager() {
	manager := testutil.NewManager(nil)
	
	db := dbtestutil.NewComponent()
	manager.Add(db)
	
	// Start all components
	manager.StartAll()
	defer manager.Cleanup()
	
	// Use the database
	dbComp := manager.Get("database-test").(*dbtestutil.Component)
	dbComp.DB().Exec("CREATE TABLE users (id INTEGER PRIMARY KEY)")
	
	exists := dbtestutil.TableExists(dbComp.DB(), "users")
	fmt.Println("Table exists:", exists)
	
	// Output:
	// Table exists: true
}
