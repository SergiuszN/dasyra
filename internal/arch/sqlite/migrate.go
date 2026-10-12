package sqlite

import (
	"context"
	"embed"
	"fmt"
	"path"
	"strconv"
	"strings"
	"time"
)

// Migrations run in name order, each one once, recorded in the migrations table.
//
//go:embed migrations/*.sql
var migrations embed.FS

// Migrate applies the migrations newer than the last applied version and returns their names.
func (db *DB) Migrate(ctx context.Context) ([]string, error) {
	if _, err := db.ExecContext(ctx, "CREATE TABLE IF NOT EXISTS `migrations` (`name` VARCHAR(255) PRIMARY KEY, `version` VARCHAR(12) NOT NULL, `executed_at` DATETIME NOT NULL)"); err != nil {
		return nil, err
	}
	var current int64
	if err := db.QueryRowContext(ctx, "SELECT COALESCE(MAX(CAST(version AS INTEGER)), 0) FROM migrations").Scan(&current); err != nil {
		return nil, err
	}

	files, err := migrations.ReadDir("migrations")
	if err != nil {
		return nil, err
	}
	var applied []string
	for _, f := range files {
		name := strings.TrimSuffix(f.Name(), ".sql")
		version, err := strconv.ParseInt(strings.SplitN(name, "_", 2)[0], 10, 64)
		if err != nil || version <= current {
			continue
		}
		sql, err := migrations.ReadFile(path.Join("migrations", f.Name()))
		if err != nil {
			return applied, err
		}
		if _, err := db.ExecContext(ctx, string(sql)); err != nil {
			return applied, fmt.Errorf("migration %s: %w", name, err)
		}
		if _, err := db.ExecContext(ctx, "INSERT INTO migrations VALUES (?, ?, ?)", name, version, db.stamp(time.Now())); err != nil {
			return applied, err
		}
		applied = append(applied, name)
	}
	return applied, nil
}

// Backup writes a consistent copy of the database to path, also while the bot is writing to it.
func (db *DB) Backup(ctx context.Context, path string) error {
	_, err := db.ExecContext(ctx, "VACUUM INTO ?", path)
	return err
}
