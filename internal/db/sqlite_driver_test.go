package db

import (
	"errors"
	"fmt"
	"path/filepath"
	"testing"

	"gorm.io/gorm"
)

// Regression suite for the official GORM SQLite driver (CGO/mattn).
// Run with CGO_ENABLED=1; release binaries are linked statically against musl.
func TestSQLiteNativeDriverPragmasTransactionsAndPersistence(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "native-driver.sqlite")
	database, err := InitDB(dbPath)
	if err != nil {
		t.Fatal(err)
	}
	sqlDB, err := database.DB()
	if err != nil {
		t.Fatal(err)
	}
	closed := false
	t.Cleanup(func() {
		if !closed {
			if err := sqlDB.Close(); err != nil {
				t.Errorf("close SQLite test database: %v", err)
			}
		}
	})

	if sqlDB.Stats().MaxOpenConnections != 1 {
		t.Fatalf("expected serialized SQLite pool, got %d connections", sqlDB.Stats().MaxOpenConnections)
	}

	var mode string
	if err := database.Raw("PRAGMA journal_mode").Scan(&mode).Error; err != nil {
		t.Fatal(err)
	}
	if mode != "wal" {
		t.Fatalf("journal_mode=%q, expected wal", mode)
	}

	var foreignKeys int
	if err := database.Raw("PRAGMA foreign_keys").Scan(&foreignKeys).Error; err != nil {
		t.Fatal(err)
	}
	if foreignKeys != 1 {
		t.Fatalf("foreign key checks disabled: %d", foreignKeys)
	}

	var busyTimeout int
	if err := database.Raw("PRAGMA busy_timeout").Scan(&busyTimeout).Error; err != nil {
		t.Fatal(err)
	}
	if busyTimeout != 5000 {
		t.Fatalf("busy_timeout=%d, expected 5000", busyTimeout)
	}

	if err := database.Exec("CREATE TABLE fk_parent (id INTEGER PRIMARY KEY)").Error; err != nil {
		t.Fatal(err)
	}
	if err := database.Exec("CREATE TABLE fk_child (id INTEGER PRIMARY KEY, parent_id INTEGER REFERENCES fk_parent(id))").Error; err != nil {
		t.Fatal(err)
	}
	if err := database.Exec("INSERT INTO fk_child (id,parent_id) VALUES (1,9999)").Error; err == nil {
		t.Fatal("SQLite driver did not enforce foreign key constraint")
	}

	record := User{ID: "user-driver-probe", Name: "Native DB Driver", Email: "driver-test@example.org", Status: "ACTIVE", AuthType: "local"}
	sentinel := errors.New("rollback requested")
	err = database.Transaction(func(tx *gorm.DB) error {
		if err := tx.Create(&record).Error; err != nil {
			return err
		}
		return sentinel
	})
	if !errors.Is(err, sentinel) {
		t.Fatalf("transaction error=%v, expected rollback sentinel", err)
	}
	var count int64
	if err := database.Model(&User{}).Where("id = ?", record.ID).Count(&count).Error; err != nil {
		t.Fatal(err)
	}
	if count != 0 {
		t.Fatal("rolled-back user persisted")
	}

	if err := database.Create(&record).Error; err != nil {
		t.Fatal(err)
	}
	duplicate := User{ID: "user-driver-duplicate", Name: "Duplicate", Email: record.Email, Status: "ACTIVE", AuthType: "local"}
	if err := database.Create(&duplicate).Error; err == nil {
		t.Fatal("unique email constraint was not enforced")
	}

	if err := sqlDB.Close(); err != nil {
		t.Fatal(err)
	}
	closed = true
	reopened, err := InitDB(dbPath)
	if err != nil {
		t.Fatalf("reopening WAL-mode database failed: %v", err)
	}
	reopenedSQL, err := reopened.DB()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := reopenedSQL.Close(); err != nil {
			t.Errorf("close reopened SQLite database: %v", err)
		}
	})
	var loaded User
	if err := reopened.First(&loaded, "id = ?", record.ID).Error; err != nil {
		t.Fatal(err)
	}
	if loaded.Email != record.Email {
		t.Fatalf("reopen lost user data: got %+v", loaded)
	}
	var countAfter int64
	if err := reopened.Model(&User{}).Count(&countAfter).Error; err != nil {
		t.Fatal(err)
	}
	if countAfter != 1 {
		t.Fatalf("unexpected user count after reopen: %d", countAfter)
	}
}

// An in-memory SQLite database cannot use WAL; startup must fail closed
// rather than claim production WAL/foreign key guarantees were enforced.
func TestSQLiteRejectsInMemoryDatabaseWithoutWAL(t *testing.T) {
	if opened, err := InitDB(":memory:"); err == nil {
		raw, dbErr := opened.DB()
		if dbErr != nil {
			t.Fatalf("unexpectedly opened SQLite database, but cannot get handle: %v", dbErr)
		}
		if closeErr := raw.Close(); closeErr != nil {
			t.Errorf("close unexpected in-memory database: %v", closeErr)
		}
		t.Fatal("in-memory SQLite unexpectedly accepted without WAL mode")
	}
}

func TestSQLiteNativeDriverConcurrentWritesAreSerialized(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "concurrent.sqlite")
	database, err := InitDB(dbPath)
	if err != nil {
		t.Fatal(err)
	}
	raw, err := database.DB()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := raw.Close(); err != nil {
			t.Errorf("close SQLite concurrency test database: %v", err)
		}
	})

	const workers = 12
	results := make(chan error, workers)
	for i := 0; i < workers; i++ {
		go func(i int) {
			user := User{
				ID:    fmt.Sprintf("concurrent-%02d", i),
				Email: fmt.Sprintf("concurrent-%02d@example.org", i),
				Name:  "Concurrent test", AuthType: "local", Status: "ACTIVE",
			}
			results <- database.Transaction(func(tx *gorm.DB) error { return tx.Create(&user).Error })
		}(i)
	}
	for i := 0; i < workers; i++ {
		if err := <-results; err != nil {
			t.Fatalf("concurrent SQLite transaction failed: %v", err)
		}
	}
	var count int64
	if err := database.Model(&User{}).Where("id LIKE ?", "concurrent-%").Count(&count).Error; err != nil {
		t.Fatal(err)
	}
	if count != workers {
		t.Fatalf("expected %d persisted users, got %d", workers, count)
	}

	// Verify SystemState / SessionEpoch lifecycle including nil-DB guards and rotation.
	if err := EnsureSystemState(nil); err == nil {
		t.Fatal("expected nil-DB EnsureSystemState error")
	}
	if got, _ := CurrentSessionEpoch(nil); got != "" {
		t.Fatalf("expected empty epoch for nil DB, got %q", got)
	}
	epoch1, err := CurrentSessionEpoch(database)
	if err != nil || epoch1 == "" {
		t.Fatalf("expected initialized session epoch, err=%v", err)
	}
	var epoch2 string
	if err := database.Transaction(func(tx *gorm.DB) error {
		var rErr error
		epoch2, rErr = RotateSessionEpochTx(tx)
		return rErr
	}); err != nil {
		t.Fatalf("RotateSessionEpochTx failed: %v", err)
	}
	if epoch2 == "" || epoch2 == epoch1 {
		t.Fatalf("expected rotated session epoch, before=%q after=%q", epoch1, epoch2)
	}
	if _, err := RotateSessionEpochTx(nil); err == nil {
		t.Fatal("expected nil DB RotateSessionEpochTx error")
	}
}
