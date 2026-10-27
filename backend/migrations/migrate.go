// Package migrations applies the versioned MySQL schema migrations embedded in this directory.
package migrations

import (
	"context"
	"embed"
	"fmt"
	"io/fs"
	"sort"
	"strings"

	"gorm.io/gorm"
)

//go:embed *.sql
var files embed.FS

// Apply executes unapplied SQL migrations in filename order. A pinned database
// connection is used because migrations may use MySQL session variables and PREPARE.
func Apply(ctx context.Context, db *gorm.DB) error {
	if db.Dialector.Name() != "mysql" {
		return fmt.Errorf("versioned SQL migrations require MySQL, got %s", db.Dialector.Name())
	}
	return db.WithContext(ctx).Connection(func(conn *gorm.DB) error {
		if err := conn.Exec(`CREATE TABLE IF NOT EXISTS schema_migrations (
			version VARCHAR(255) NOT NULL PRIMARY KEY,
			applied_at DATETIME(3) NOT NULL
		) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4`).Error; err != nil {
			return fmt.Errorf("create schema_migrations: %w", err)
		}

		var appliedVersions []string
		if err := conn.Raw("SELECT version FROM schema_migrations").Scan(&appliedVersions).Error; err != nil {
			return fmt.Errorf("read schema_migrations: %w", err)
		}
		applied := make(map[string]struct{}, len(appliedVersions))
		for _, version := range appliedVersions {
			applied[version] = struct{}{}
		}

		entries, err := fs.ReadDir(files, ".")
		if err != nil {
			return err
		}
		var names []string
		for _, entry := range entries {
			if !entry.IsDir() && strings.HasSuffix(entry.Name(), ".sql") {
				names = append(names, entry.Name())
			}
		}
		sort.Strings(names)
		for _, name := range names {
			version := strings.TrimSuffix(name, ".sql")
			if _, ok := applied[version]; ok {
				continue
			}
			source, err := files.ReadFile(name)
			if err != nil {
				return err
			}
			for _, statement := range splitStatements(string(source)) {
				if err := conn.Exec(statement).Error; err != nil {
					return fmt.Errorf("migration %s: %w", name, err)
				}
			}
			if err := conn.Exec("INSERT INTO schema_migrations (version, applied_at) VALUES (?, CURRENT_TIMESTAMP(3))", version).Error; err != nil {
				return fmt.Errorf("record migration %s: %w", name, err)
			}
		}
		return nil
	})
}

// splitStatements is intentionally limited to the checked-in migration format:
// statements do not contain semicolons inside quoted literals.
func splitStatements(source string) []string {
	parts := strings.Split(source, ";")
	statements := make([]string, 0, len(parts))
	for _, part := range parts {
		if strings.TrimSpace(part) != "" {
			statements = append(statements, strings.TrimSpace(part))
		}
	}
	return statements
}
