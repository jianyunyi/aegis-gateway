package migrations

import (
	"strings"
	"testing"
)

func TestSplitStatementsKeepsDynamicMigrationStatements(t *testing.T) {
	source, err := files.ReadFile("003_api_key_accounting.sql")
	if err != nil {
		t.Fatal(err)
	}
	statements := splitStatements(string(source))
	if len(statements) != 13 {
		t.Fatalf("split %d statements, want 13", len(statements))
	}
	for _, token := range []string{"PREPARE aegis_stmt", "EXECUTE aegis_stmt", "DEALLOCATE PREPARE aegis_stmt"} {
		found := false
		for _, statement := range statements {
			found = found || strings.Contains(statement, token)
		}
		if !found {
			t.Fatalf("migration statement %q was lost", token)
		}
	}
}

func TestFreshBaselineContainsApiKeyAccountingColumns(t *testing.T) {
	baseline, err := files.ReadFile("001_init.sql")
	if err != nil {
		t.Fatal(err)
	}
	for _, field := range []string{"used_tokens", "default_model", "budget_monthly", "usage_estimated"} {
		if !strings.Contains(string(baseline), field) {
			t.Fatalf("fresh baseline missing %s", field)
		}
	}
	if strings.Contains(strings.ToUpper(string(baseline)), "USE AEGIS") {
		t.Fatal("migration must not change the database selected by MYSQL_DSN")
	}
}

func TestUsageEstimateMigrationIsIdempotent(t *testing.T) {
	migration, err := files.ReadFile("004_usage_estimated.sql")
	if err != nil {
		t.Fatal(err)
	}
	for _, token := range []string{"information_schema.columns", "usage_logs", "usage_estimated", "ALTER TABLE"} {
		if !strings.Contains(string(migration), token) {
			t.Fatalf("004 migration missing %q", token)
		}
	}
	if statements := splitStatements(string(migration)); len(statements) != 5 {
		t.Fatalf("004 split %d statements, want 5", len(statements))
	}
}
