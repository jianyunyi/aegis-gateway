//go:build integration

package migrations

import (
	"context"
	"crypto/rand"
	"database/sql"
	"fmt"
	"os"
	"strings"
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

func TestApplyIntegrationUpgradesExistingSchemaAndPreservesRows(t *testing.T) {
	db := newIsolatedMySQL(t)
	baseline, err := files.ReadFile("001_init.sql")
	if err != nil {
		t.Fatal(err)
	}
	missingColumns := []string{"used_tokens", "default_model", "budget_monthly", "prompt_preview", "prompt_for_eval", "chat_messages", "usage_estimated"}
	found := make(map[string]bool, len(missingColumns))
	var legacyLines []string
	for _, line := range strings.Split(string(baseline), "\n") {
		trimmed := strings.TrimSpace(line)
		remove := false
		for _, column := range missingColumns {
			if strings.HasPrefix(trimmed, column+" ") {
				found[column] = true
				remove = true
				break
			}
		}
		if !remove {
			legacyLines = append(legacyLines, line)
		}
	}
	if len(found) != len(missingColumns) {
		t.Fatalf("legacy schema fixture did not remove all expected columns: found %v", found)
	}
	legacyBaseline := strings.Join(legacyLines, "\n")
	legacyFS := fstest.MapFS{
		"001_init.sql": &fstest.MapFile{Data: []byte(legacyBaseline)},
	}
	if err := applyWithFS(context.Background(), db, legacyFS); err != nil {
		t.Fatalf("create existing legacy schema: %v", err)
	}
	if err := db.Exec(`INSERT INTO api_keys (id, name, key_hash, key_prefix, user_id, status, quota_tokens, rps_limit, burst, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, CURRENT_TIMESTAMP(3), CURRENT_TIMESTAMP(3))`, 41, "existing key", strings.Repeat("a", 64), "ak_legacy", 7, 1, 12345, 10, 20).Error; err != nil {
		t.Fatalf("seed existing API key: %v", err)
	}
	requestID := strings.Repeat("c", 32)
	if err := db.Exec(`INSERT INTO usage_logs (request_id, api_key_id, model_name, kind, prompt_tokens, completion_tokens, total_tokens, status, created_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, CURRENT_TIMESTAMP(3))`, requestID, 41, "legacy-model", "chat", 11, 5, 16, 0).Error; err != nil {
		t.Fatalf("seed existing usage row: %v", err)
	}
	if err := db.Exec(`INSERT INTO eval_samples (dataset_id, prompt, source, created_at)
		VALUES (?, ?, ?, CURRENT_TIMESTAMP(3))`, 9, "legacy benchmark prompt", "manual").Error; err != nil {
		t.Fatalf("seed existing evaluation sample: %v", err)
	}

	if err := Apply(context.Background(), db); err != nil {
		t.Fatalf("upgrade existing schema: %v", err)
	}
	if err := Apply(context.Background(), db); err != nil {
		t.Fatalf("repeat upgrade after completion: %v", err)
	}

	var versions []string
	if err := db.Raw("SELECT version FROM schema_migrations ORDER BY version").Scan(&versions).Error; err != nil {
		t.Fatal(err)
	}
	if got, want := strings.Join(versions, ","), "001_init,002_eval_prompt_storage,003_api_key_accounting,004_usage_estimated"; got != want {
		t.Fatalf("applied migrations %q, want %q", got, want)
	}

	var keyQuota, keyUsed int64
	var keyDefaultModel string
	var keyBudget float64
	if err := db.Raw("SELECT quota_tokens, used_tokens, default_model, budget_monthly FROM api_keys WHERE id = ?", 41).Row().Scan(&keyQuota, &keyUsed, &keyDefaultModel, &keyBudget); err != nil {
		t.Fatal(err)
	}
	if keyQuota != 12345 || keyUsed != 0 || keyDefaultModel != "" || keyBudget != 0 {
		t.Fatalf("API key data changed during upgrade: quota=%d used=%d model=%q budget=%f", keyQuota, keyUsed, keyDefaultModel, keyBudget)
	}

	var gotRequestID, gotModel string
	var preview, promptForEval sql.NullString
	var estimated int
	if err := db.Raw("SELECT request_id, model_name, prompt_preview, prompt_for_eval, usage_estimated FROM usage_logs WHERE request_id = ?", requestID).Row().Scan(&gotRequestID, &gotModel, &preview, &promptForEval, &estimated); err != nil {
		t.Fatal(err)
	}
	if gotRequestID != requestID || gotModel != "legacy-model" || preview.Valid || promptForEval.Valid || estimated != 0 {
		t.Fatalf("usage data changed during upgrade: id=%q model=%q preview=%v prompt=%v estimated=%d", gotRequestID, gotModel, preview, promptForEval, estimated)
	}

	var samplePrompt string
	var chatMessages sql.NullString
	if err := db.Raw("SELECT prompt, chat_messages FROM eval_samples WHERE dataset_id = ?", 9).Row().Scan(&samplePrompt, &chatMessages); err != nil {
		t.Fatal(err)
	}
	if samplePrompt != "legacy benchmark prompt" || chatMessages.Valid {
		t.Fatalf("evaluation sample changed during upgrade: prompt=%q chat_messages=%v", samplePrompt, chatMessages)
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
