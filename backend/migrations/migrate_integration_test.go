//go:build integration

package migrations

import (
	"context"
	"crypto/rand"
	"database/sql"
	"fmt"
	"os"
	"sync"
	"testing"

	mysqlDriver "github.com/go-sql-driver/mysql"
	gormMysql "gorm.io/driver/mysql"
	"gorm.io/gorm"
	"testing/fstest"
)

func TestApplyIntegrationSerializesExecutorsAndRecordsOnce(t *testing.T) {
	db := newIsolatedMySQL(t)
	fixture := fstest.MapFS{
		"001_probe.sql": &fstest.MapFile{Data: []byte("SELECT SLEEP(1); CREATE TABLE migration_probe (id INT NOT NULL PRIMARY KEY); INSERT INTO migration_probe (id) VALUES (1);")},
	}
	first, second := openSecondConnection(t, db), openSecondConnection(t, db)
	var wg sync.WaitGroup
	errs := make(chan error, 2)
	wg.Add(2)
	go func() {
		defer wg.Done()
		errs <- applyWithFS(context.Background(), first, fixture)
	}()
	go func() {
		defer wg.Done()
		errs <- applyWithFS(context.Background(), second, fixture)
	}()
	wg.Wait()
	close(errs)
	for err := range errs {
		if err != nil {
			t.Fatal(err)
		}
	}

	var migrations, probes int
	if err := db.Raw("SELECT COUNT(*) FROM schema_migrations WHERE version = ?", "001_probe").Scan(&migrations).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Raw("SELECT COUNT(*) FROM migration_probe").Scan(&probes).Error; err != nil {
		t.Fatal(err)
	}
	if migrations != 1 || probes != 1 {
		t.Fatalf("migration recorded %d times and probe has %d rows, want 1 and 1", migrations, probes)
	}
}

func TestApplyIntegrationRetriesAfterNonTransactionalFailure(t *testing.T) {
	db := newIsolatedMySQL(t)
	fixture := fstest.MapFS{
		"001_retry.sql": &fstest.MapFile{Data: []byte("CREATE TABLE retry_probe (id INT NOT NULL PRIMARY KEY); THIS IS NOT VALID SQL;")},
	}
	if err := applyWithFS(context.Background(), db, fixture); err == nil {
		t.Fatal("first apply unexpectedly succeeded")
	}
	var recorded int
	if err := db.Raw("SELECT COUNT(*) FROM schema_migrations WHERE version = ?", "001_retry").Scan(&recorded).Error; err != nil {
		t.Fatal(err)
	}
	if recorded != 0 {
		t.Fatalf("failed migration was recorded %d times", recorded)
	}

	fixture["001_retry.sql"].Data = []byte("CREATE TABLE IF NOT EXISTS retry_probe (id INT NOT NULL PRIMARY KEY); INSERT INTO retry_probe (id) VALUES (1);")
	if err := applyWithFS(context.Background(), db, fixture); err != nil {
		t.Fatalf("retry failed: %v", err)
	}
	if err := db.Raw("SELECT COUNT(*) FROM retry_probe").Scan(&recorded).Error; err != nil {
		t.Fatal(err)
	}
	if recorded != 1 {
		t.Fatalf("retry probe has %d rows, want 1", recorded)
	}
}

func newIsolatedMySQL(t *testing.T) *gorm.DB {
	t.Helper()
	dsn := os.Getenv("TEST_MYSQL_DSN")
	if dsn == "" {
		t.Skip("integration requires TEST_MYSQL_DSN")
	}
	cfg, err := mysqlDriver.ParseDSN(dsn)
	if err != nil {
		t.Fatalf("parse TEST_MYSQL_DSN: %v", err)
	}
	adminCfg := *cfg
	adminCfg.DBName = ""
	admin, err := sql.Open("mysql", adminCfg.FormatDSN())
	if err != nil {
		t.Fatalf("open MySQL admin connection: %v", err)
	}
	t.Cleanup(func() { _ = admin.Close() })
	if err := admin.Ping(); err != nil {
		t.Fatalf("ping TEST_MYSQL_DSN: %v", err)
	}
	schema := uniqueSchemaName(t)
	if _, err := admin.Exec("CREATE DATABASE `" + schema + "`"); err != nil {
		t.Fatalf("create isolated schema: %v", err)
	}
	t.Cleanup(func() { _, _ = admin.Exec("DROP DATABASE `" + schema + "`") })

	isolatedCfg := *cfg
	isolatedCfg.DBName = schema
	gormDB, err := gorm.Open(gormMysql.Open(isolatedCfg.FormatDSN()), &gorm.Config{})
	if err != nil {
		t.Fatalf("open isolated GORM database: %v", err)
	}
	t.Cleanup(func() {
		if sqlDB, err := gormDB.DB(); err == nil {
			_ = sqlDB.Close()
		}
	})
	return gormDB
}

func openSecondConnection(t *testing.T, db *gorm.DB) *gorm.DB {
	t.Helper()
	base, err := db.DB()
	if err != nil {
		t.Fatal(err)
	}
	// Reuse the DSN with a separate pool so each Apply can hold its own session.
	cfg, err := mysqlDriver.ParseDSN(os.Getenv("TEST_MYSQL_DSN"))
	if err != nil {
		t.Fatal(err)
	}
	var schema string
	if err := db.Raw("SELECT DATABASE()").Scan(&schema).Error; err != nil {
		t.Fatal(err)
	}
	cfg.DBName = schema
	other, err := gorm.Open(gormMysql.Open(cfg.FormatDSN()), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	otherSQL, err := other.DB()
	if err != nil {
		t.Fatal(err)
	}
	otherSQL.SetMaxOpenConns(1)
	t.Cleanup(func() { _ = otherSQL.Close() })
	base.SetMaxOpenConns(1)
	return other
}

func uniqueSchemaName(t *testing.T) string {
	t.Helper()
	buf := make([]byte, 8)
	if _, err := rand.Read(buf); err != nil {
		t.Fatal(err)
	}
	return fmt.Sprintf("aegis_migration_test_%x", buf)
}
