package backup

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/local/trade-discipline-desktop/backend/internal/store"
)

func seededDatabase(t *testing.T) (string, *store.Store) {
	t.Helper()
	path := filepath.Join(t.TempDir(), "discipline.db")
	db, err := store.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	if err := db.Migrate(context.Background()); err != nil {
		t.Fatal(err)
	}
	return path, db
}

func TestCreateAndValidateBackup(t *testing.T) {
	source, db := seededDatabase(t)
	defer db.Close()
	destination := filepath.Join(t.TempDir(), "backup.db")
	if err := Create(context.Background(), db.DB(), destination); err != nil {
		t.Fatal(err)
	}
	metadata, err := Validate(destination)
	if err != nil || metadata.SchemaVersion != store.CurrentSchemaVersion {
		t.Fatalf("metadata=%#v err=%v", metadata, err)
	}
	if source == destination {
		t.Fatal("backup path must differ")
	}
}

func TestInvalidBackupNeverReplacesCurrentDatabase(t *testing.T) {
	current, db := seededDatabase(t)
	_ = db.Close()
	invalid := filepath.Join(t.TempDir(), "invalid.db")
	if err := os.WriteFile(invalid, []byte("not sqlite"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := Restore(context.Background(), invalid, current); err == nil {
		t.Fatal("expected validation failure")
	}
	reopened, err := store.Open(current)
	if err != nil {
		t.Fatal(err)
	}
	defer reopened.Close()
	if _, err := reopened.CurrentRule(context.Background()); err != nil {
		t.Fatalf("current database was damaged: %v", err)
	}
}
