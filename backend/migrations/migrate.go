// Package migrations applies the versioned MySQL schema migrations embedded in this directory.
package migrations

import (
	"context"
	"embed"
	"fmt"
	"io/fs"
	"sort"
	"strings"
	"time"

	"gorm.io/gorm"
)

//go:embed *.sql
var files embed.FS

const (
	// MySQL advisory lock names are limited to 64 characters. Keep this name
	// stable: every gateway instance must contend for the same lock.
	migrationLockName = "aegis_gateway:versioned_migrations"
	// Keep lock acquisition and cleanup bounded even if the caller is canceled.
	migrationLockWaitSeconds = 5
	migrationLockTimeout     = 5 * time.Second
)

// Apply executes unapplied SQL migrations in filename order. The whole
// discovery/apply/record sequence runs on one pinned MySQL connection while
// holding a session advisory lock. Migrations deliberately do not use a
// transaction: MySQL DDL implicitly commits, so each migration is written to
// be idempotent and is retried when its version row was not recorded.
func Apply(ctx context.Context, db *gorm.DB) error {
	if db.Dialector.Name() != "mysql" {
		return fmt.Errorf("versioned SQL migrations require MySQL, got %s", db.Dialector.Name())
	}
	return applyWithFS(ctx, db, files)
}

// applyWithFS is kept package-private so integration tests can exercise the
// lock and retry behavior with a small isolated migration without modifying
// the production embedded migration set.
func applyWithFS(ctx context.Context, db *gorm.DB, migrationFS fs.FS) error {
	return db.WithContext(ctx).Connection(func(conn *gorm.DB) (resultErr error) {
		locked, err := acquireMigrationLock(conn)
		if err != nil {
			return err
		}
		if !locked {
			return fmt.Errorf("timed out acquiring migration advisory lock %q after %d seconds", migrationLockName, migrationLockWaitSeconds)
		}
		defer func() {
			// A canceled caller context must not prevent RELEASE_LOCK from being
			// sent on the same pinned session.
			releaseCtx := context.WithoutCancel(ctx)
			if releaseErr := releaseMigrationLock(conn.WithContext(releaseCtx)); releaseErr != nil && resultErr == nil {
				resultErr = releaseErr
			}
		}()

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

		entries, err := fs.ReadDir(migrationFS, ".")
		if err != nil {
			return fmt.Errorf("read migration directory: %w", err)
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
			source, err := fs.ReadFile(migrationFS, name)
			if err != nil {
				return fmt.Errorf("read migration %s: %w", name, err)
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

func acquireMigrationLock(conn *gorm.DB) (bool, error) {
	var acquired int
	lockCtx, cancel := context.WithTimeout(context.WithoutCancel(conn.Statement.Context), migrationLockTimeout)
	defer cancel()
	if err := conn.WithContext(lockCtx).Raw("SELECT GET_LOCK(?, ?)", migrationLockName, migrationLockWaitSeconds).Scan(&acquired).Error; err != nil {
		// MySQL may have acquired the session lock before a client-side
		// cancellation/error was observed. Do not leave that lock behind.
		_ = releaseMigrationLock(conn)
		return false, fmt.Errorf("acquire migration advisory lock %q: %w", migrationLockName, err)
	}
	return acquired == 1, nil
}

func releaseMigrationLock(conn *gorm.DB) error {
	var released int
	releaseCtx, cancel := context.WithTimeout(context.WithoutCancel(conn.Statement.Context), migrationLockTimeout)
	defer cancel()
	if err := conn.WithContext(releaseCtx).Raw("SELECT RELEASE_LOCK(?)", migrationLockName).Scan(&released).Error; err != nil {
		return fmt.Errorf("release migration advisory lock %q: %w", migrationLockName, err)
	}
	if released != 1 {
		return fmt.Errorf("release migration advisory lock %q returned %d", migrationLockName, released)
	}
	return nil
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
