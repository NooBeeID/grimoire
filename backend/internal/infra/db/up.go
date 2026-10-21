package db

import (
	"context"
	"log/slog"

	"grimoire/migrations"
)

// Up menjalankan semua migrasi ter-embed. Dipakai `migrate up` dan RUN_MIGRATION.
func Up(ctx context.Context, d *DB, log *slog.Logger) (MigrateResult, error) {
	return Migrate(ctx, d, migrations.FS, log)
}
