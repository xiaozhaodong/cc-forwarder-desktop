package migration

import (
	"context"
	"database/sql"
	"fmt"
	"math"
	"os"
	"path/filepath"
)

const migrationDiskSpaceMultiplier uint64 = 3

func ensureMigrationDiskSpace(ctx context.Context, db *sql.DB) error {
	databasePath, err := sqliteMainDatabasePath(ctx, db)
	if err != nil {
		return err
	}
	if databasePath == "" {
		return nil
	}
	info, err := os.Stat(databasePath)
	if err != nil {
		return fmt.Errorf("inspect active SQLite database for disk preflight: %w", err)
	}
	available, err := availableDiskBytes(filepath.Dir(databasePath))
	if err != nil {
		return fmt.Errorf("inspect migration filesystem free space: %w", err)
	}
	return validateMigrationDiskSpace(uint64(info.Size()), available)
}

func sqliteMainDatabasePath(ctx context.Context, db *sql.DB) (string, error) {
	rows, err := db.QueryContext(ctx, `PRAGMA database_list`)
	if err != nil {
		return "", fmt.Errorf("resolve active SQLite database path: %w", err)
	}
	defer rows.Close()
	for rows.Next() {
		var sequence int
		var name, path string
		if err := rows.Scan(&sequence, &name, &path); err != nil {
			return "", fmt.Errorf("scan active SQLite database path: %w", err)
		}
		if name == "main" {
			if path == "" {
				return "", nil
			}
			absolute, err := filepath.Abs(path)
			if err != nil {
				return "", fmt.Errorf("normalize active SQLite database path: %w", err)
			}
			return absolute, nil
		}
	}
	if err := rows.Err(); err != nil {
		return "", err
	}
	return "", fmt.Errorf("active SQLite main database path was not reported")
}

func validateMigrationDiskSpace(databaseSize, available uint64) error {
	if databaseSize > math.MaxUint64/migrationDiskSpaceMultiplier {
		return fmt.Errorf("active SQLite database is too large for migration disk calculation")
	}
	required := databaseSize * migrationDiskSpaceMultiplier
	if available < required {
		return fmt.Errorf("insufficient free space for timezone migration: available=%d required=%d database_size=%d",
			available, required, databaseSize)
	}
	return nil
}
