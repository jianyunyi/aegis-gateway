package main

import (
	"bufio"
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"hash"
	"io"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	mysqlDriver "github.com/go-sql-driver/mysql"
	gormMysql "gorm.io/driver/mysql"
	"gorm.io/gorm"

	"aegis-gateway/internal/model"
	"aegis-gateway/internal/repository"
	"aegis-gateway/internal/service"
	"aegis-gateway/migrations"
)

const requiredMigration = "005_eval_sample_retention"

var (
	databaseNamePattern = regexp.MustCompile(`^aegis_rehearsal(?:_[0-9]{8}_[a-f0-9]{6,12})?$`)
	unsafeDumpHeader    = regexp.MustCompile(`(?im)^\s*(?:(?:/\*!\d+\s*)?)(?:CREATE|DROP)\s+DATABASE\b|^\s*USE\s+`)
)

type tableState struct {
	Rows        int64  `json:"rows"`
	Fingerprint string `json:"fingerprint"`
}

type manifest struct {
	Database       string                `json:"database"`
	CapturedAt     string                `json:"captured_at_utc"`
	SchemaVersions []string              `json:"schema_versions"`
	Tables         map[string]tableState `json:"tables"`
}

type sampleRow struct {
	ID                uint64 `json:"id"`
	DatasetID         uint64 `json:"dataset_id"`
	Source            string `json:"source"`
	CreatedAt         string `json:"created_at"`
	ExpiresAt         string `json:"expires_at,omitempty"`
	PolicyConfirmedAt string `json:"manual_policy_confirmed_at,omitempty"`
}

var fingerprintQueries = map[string]string{
	"users":         `SELECT id, username, role, status, created_at FROM users ORDER BY id`,
	"api_keys":      `SELECT id, name, user_id, status, quota_tokens, created_at FROM api_keys ORDER BY id`,
	"providers":     `SELECT id, name, base_url, enabled, created_at FROM providers ORDER BY id`,
	"models":        `SELECT id, provider_id, name, enabled, created_at FROM models ORDER BY id`,
	"usage_logs":    `SELECT id, request_id, api_key_id, model_name, total_tokens, status, created_at FROM usage_logs ORDER BY id`,
	"billing_daily": `SELECT id, date, api_key_id, request_count, total_tokens, cost, created_at FROM billing_daily ORDER BY id`,
	"eval_datasets": `SELECT id, name, status, created_at FROM eval_datasets ORDER BY id`,
	"eval_samples":  `SELECT id, dataset_id, source, label, created_at, SHA2(COALESCE(prompt, ''), 256), SHA2(COALESCE(reference, ''), 256) FROM eval_samples ORDER BY id`,
	"eval_runs":     `SELECT id, dataset_id, model_a, model_b, status, created_at, SHA2(COALESCE(CAST(report AS CHAR), ''), 256) FROM eval_runs ORDER BY id`,
}

func main() {
	if len(os.Args) < 2 {
		usage()
		os.Exit(2)
	}
	dsnFile := os.Getenv("AEGIS_REHEARSAL_DSN_FILE")
	if dsnFile == "" {
		fmt.Fprintln(os.Stderr, "AEGIS_REHEARSAL_DSN_FILE must point to a mode-0600 DSN for a local aegis_rehearsal* database")
		os.Exit(2)
	}
	db, cfg, err := openTarget(dsnFile)
	if err != nil {
		fatal(err)
	}
	defer closeDB(db)

	var runErr error
	switch os.Args[1] {
	case "status":
		runErr = runStatus(db, cfg, os.Args[2:])
	case "backup":
		runErr = runBackup(cfg, os.Args[2:])
	case "restore":
		runErr = runRestore(db, cfg, os.Args[2:])
	case "migrate":
		runErr = runMigrate(db)
	case "verify":
		runErr = runVerify(db, cfg, os.Args[2:])
	case "samples":
		runErr = runSamples(db, cfg, os.Args[2:])
	default:
		usage()
		runErr = fmt.Errorf("unknown command %q", os.Args[1])
	}
	if runErr != nil {
		fatal(runErr)
	}
}

func usage() {
	fmt.Fprintln(os.Stderr, `Usage: db-rehearsal <command>
Commands:
  restore --from BACKUP.sql       restore a plain mysqldump into an empty isolated schema
  status [--out baseline.json]    write a metadata-only manifest (no prompt text)
  backup --out SNAPSHOT.sql       make a private mysqldump of the isolated schema
  migrate                         apply this checkout's versioned SQL migrations
  verify --baseline baseline.json compare row counts/fingerprints and required schema
         --pre-migration checks an exact pre-migration restore instead
  samples inventory               list sample IDs/source/dates, never prompt contents
  samples delete --id ID --confirm-id ID --backup-before-delete SNAPSHOT.sql
                                  back up then require typing DELETE ID before removing one sample`)
}

func openTarget(dsnFile string) (*gorm.DB, *mysqlDriver.Config, error) {
	info, err := os.Stat(dsnFile)
	if err != nil {
		return nil, nil, fmt.Errorf("stat rehearsal DSN file: %w", err)
	}
	if info.Mode().Perm()&0o077 != 0 {
		return nil, nil, fmt.Errorf("rehearsal DSN file must not be readable by group/other (chmod 600 %s)", dsnFile)
	}
	raw, err := os.ReadFile(dsnFile)
	if err != nil {
		return nil, nil, fmt.Errorf("read rehearsal DSN file: %w", err)
	}
	cfg, err := mysqlDriver.ParseDSN(strings.TrimSpace(string(raw)))
	if err != nil {
		return nil, nil, fmt.Errorf("parse rehearsal DSN: %w", err)
	}
	if err := validateTarget(cfg); err != nil {
		return nil, nil, err
	}
	db, err := gorm.Open(gormMysql.Open(cfg.FormatDSN()), &gorm.Config{})
	if err != nil {
		return nil, nil, fmt.Errorf("open local rehearsal database: %w", err)
	}
	sqlDB, err := db.DB()
	if err != nil {
		return nil, nil, err
	}
	sqlDB.SetMaxOpenConns(2)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := sqlDB.PingContext(ctx); err != nil {
		_ = sqlDB.Close()
		return nil, nil, fmt.Errorf("ping local rehearsal database: %w", err)
	}
	return db, cfg, nil
}

func validateTarget(cfg *mysqlDriver.Config) error {
	if cfg == nil || cfg.Net != "tcp" || !databaseNamePattern.MatchString(cfg.DBName) {
		return errors.New("refusing database target: only local TCP schema aegis_rehearsal or aegis_rehearsal_YYYYMMDD_<random hex> is allowed")
	}
	host, _, err := net.SplitHostPort(cfg.Addr)
	if err != nil {
		return fmt.Errorf("invalid rehearsal MySQL address: %w", err)
	}
	if !strings.EqualFold(host, "localhost") {
		ip := net.ParseIP(strings.Trim(host, "[]"))
		if ip == nil || !ip.IsLoopback() {
			return fmt.Errorf("refusing non-loopback MySQL host %q; restore production backups only to a local isolated MySQL server", host)
		}
	}
	if cfg.User == "" || cfg.Passwd == "" {
		return errors.New("rehearsal DSN must use a non-empty dedicated database credential")
	}
	return nil
}

func runStatus(db *gorm.DB, cfg *mysqlDriver.Config, args []string) error {
	flags := flag.NewFlagSet("status", flag.ContinueOnError)
	out := flags.String("out", "", "write manifest to this new file")
	if err := flags.Parse(args); err != nil {
		return err
	}
	state, err := buildManifest(db, cfg.DBName)
	if err != nil {
		return err
	}
	encoded, err := json.MarshalIndent(state, "", "  ")
	if err != nil {
		return err
	}
	if *out != "" {
		if err := writeNewPrivateFile(*out, append(encoded, '\n')); err != nil {
			return err
		}
		fmt.Printf("manifest written to %s\n", *out)
	}
	fmt.Println(string(encoded))
	return nil
}

func buildManifest(db *gorm.DB, database string) (*manifest, error) {
	state := &manifest{Database: database, CapturedAt: time.Now().UTC().Format(time.RFC3339Nano), Tables: make(map[string]tableState)}
	if db.Migrator().HasTable("schema_migrations") {
		if err := db.Table("schema_migrations").Order("version ASC").Pluck("version", &state.SchemaVersions).Error; err != nil {
			return nil, fmt.Errorf("read schema_migrations: %w", err)
		}
	}
	for table, query := range fingerprintQueries {
		if !db.Migrator().HasTable(table) {
			state.Tables[table] = tableState{Fingerprint: "absent"}
			continue
		}
		fingerprint, rows, err := fingerprintRows(db, query)
		if err != nil {
			return nil, fmt.Errorf("fingerprint table %s: %w", table, err)
		}
		state.Tables[table] = tableState{Rows: rows, Fingerprint: fingerprint}
	}
	return state, nil
}

func fingerprintRows(db *gorm.DB, query string) (string, int64, error) {
	rows, err := db.Raw(query).Rows()
	if err != nil {
		return "", 0, err
	}
	defer rows.Close()
	columns, err := rows.Columns()
	if err != nil {
		return "", 0, err
	}
	h := sha256.New()
	var count int64
	for rows.Next() {
		cells := make([]sql.RawBytes, len(columns))
		values := make([]any, len(cells))
		for i := range cells {
			values[i] = &cells[i]
		}
		if err := rows.Scan(values...); err != nil {
			return "", count, err
		}
		writeRowHash(h, cells)
		count++
	}
	if err := rows.Err(); err != nil {
		return "", count, err
	}
	return hex.EncodeToString(h.Sum(nil)), count, nil
}

func writeRowHash(h hash.Hash, cells []sql.RawBytes) {
	for _, cell := range cells {
		_, _ = fmt.Fprintf(h, "%d:", len(cell))
		_, _ = h.Write(cell)
		_, _ = h.Write([]byte{0})
	}
	_, _ = h.Write([]byte{0xff})
}

func runBackup(cfg *mysqlDriver.Config, args []string) error {
	flags := flag.NewFlagSet("backup", flag.ContinueOnError)
	out := flags.String("out", "", "new output file (mode 0600)")
	if err := flags.Parse(args); err != nil {
		return err
	}
	if *out == "" {
		return errors.New("--out is required")
	}
	digest, err := dumpDatabase(cfg, *out)
	if err != nil {
		return err
	}
	fmt.Printf("isolated database backup written: %s\nsha256: %s\n", *out, digest)
	return nil
}

func dumpDatabase(cfg *mysqlDriver.Config, output string) (string, error) {
	optionFile, cleanup, err := makeMySQLOptionFile(cfg)
	if err != nil {
		return "", err
	}
	defer cleanup()
	if err := os.MkdirAll(filepath.Dir(output), 0o700); err != nil {
		return "", err
	}
	file, err := os.OpenFile(output, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o600)
	if err != nil {
		return "", fmt.Errorf("create new backup file %s: %w", output, err)
	}
	cmd := exec.Command("mysqldump", "--defaults-extra-file="+optionFile, "--single-transaction", "--routines", "--triggers", "--events", "--set-gtid-purged=OFF", cfg.DBName)
	cmd.Stdout = file
	cmd.Stderr = os.Stderr
	err = cmd.Run()
	closeErr := file.Close()
	if err != nil || closeErr != nil {
		_ = os.Remove(output)
		if err != nil {
			return "", fmt.Errorf("mysqldump failed: %w", err)
		}
		return "", closeErr
	}
	digest, err := hashFile(output)
	if err != nil {
		_ = os.Remove(output)
		return "", err
	}
	return digest, nil
}

func runRestore(db *gorm.DB, cfg *mysqlDriver.Config, args []string) error {
	flags := flag.NewFlagSet("restore", flag.ContinueOnError)
	from := flags.String("from", "", "plain mysqldump file without CREATE DATABASE or USE")
	if err := flags.Parse(args); err != nil {
		return err
	}
	if *from == "" {
		return errors.New("--from is required")
	}
	file, err := os.Open(*from)
	if err != nil {
		return fmt.Errorf("open backup: %w", err)
	}
	defer file.Close()
	if err := validateDumpHeader(file); err != nil {
		return err
	}
	if !targetIsEmpty(db) {
		return errors.New("restore target is not empty; create a fresh aegis_rehearsal* schema and update the local DSN first")
	}
	if _, err := file.Seek(0, io.SeekStart); err != nil {
		return err
	}
	optionFile, cleanup, err := makeMySQLOptionFile(cfg)
	if err != nil {
		return err
	}
	defer cleanup()
	cmd := exec.Command("mysql", "--defaults-extra-file="+optionFile, "--binary-mode=1", "--database="+cfg.DBName)
	cmd.Stdin = file
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("restore into local isolated schema failed: %w", err)
	}
	fmt.Printf("restored backup into isolated schema %s; source database was not modified\n", cfg.DBName)
	return nil
}

func validateDumpHeader(file *os.File) error {
	if _, err := file.Seek(0, io.SeekStart); err != nil {
		return err
	}
	reader := bufio.NewReader(file)
	var header strings.Builder
	for header.Len() < 1<<20 {
		line, err := reader.ReadString('\n')
		header.WriteString(line)
		if err != nil {
			if err == io.EOF {
				break
			}
			return err
		}
	}
	if unsafeDumpHeader.MatchString(header.String()) {
		return errors.New("backup header contains CREATE/DROP DATABASE or USE; use a plain mysqldump of one database without --databases")
	}
	if strings.TrimSpace(header.String()) == "" {
		return errors.New("backup file is empty")
	}
	return nil
}

func targetIsEmpty(db *gorm.DB) bool {
	var count int64
	return db.Raw("SELECT COUNT(*) FROM information_schema.tables WHERE table_schema = DATABASE()").Scan(&count).Error == nil && count == 0
}

func runMigrate(db *gorm.DB) error {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()
	if err := migrations.Apply(ctx, db); err != nil {
		return err
	}
	fmt.Println("versioned SQL migrations applied to local isolated schema")
	return nil
}

func runVerify(db *gorm.DB, cfg *mysqlDriver.Config, args []string) error {
	flags := flag.NewFlagSet("verify", flag.ContinueOnError)
	baselinePath := flags.String("baseline", "", "pre-migration status manifest")
	allowDatabaseDiff := flags.Bool("allow-database-diff", false, "allow comparing a separately restored local rollback schema")
	preMigration := flags.Bool("pre-migration", false, "verify a restored pre-migration rollback copy")
	if err := flags.Parse(args); err != nil {
		return err
	}
	if *baselinePath == "" {
		return errors.New("--baseline is required")
	}
	baselineBytes, err := os.ReadFile(*baselinePath)
	if err != nil {
		return fmt.Errorf("read baseline: %w", err)
	}
	var baseline manifest
	if err := json.Unmarshal(baselineBytes, &baseline); err != nil {
		return fmt.Errorf("decode baseline: %w", err)
	}
	if baseline.Database == "" || (!*allowDatabaseDiff && baseline.Database != cfg.DBName) {
		return fmt.Errorf("baseline database %q does not match isolated target %q", baseline.Database, cfg.DBName)
	}
	current, err := buildManifest(db, cfg.DBName)
	if err != nil {
		return err
	}
	for table, before := range baseline.Tables {
		after, ok := current.Tables[table]
		if !ok || before != after {
			return fmt.Errorf("table %s changed: before rows=%d fingerprint=%s, after rows=%d fingerprint=%s", table, before.Rows, before.Fingerprint, after.Rows, after.Fingerprint)
		}
	}
	if *preMigration {
		if strings.Join(baseline.SchemaVersions, ",") != strings.Join(current.SchemaVersions, ",") {
			return fmt.Errorf("pre-migration schema versions changed: baseline=%v current=%v", baseline.SchemaVersions, current.SchemaVersions)
		}
		fmt.Println("pre-migration backup restore verified: schema versions and key-record fingerprints match the baseline")
		return nil
	}
	if !contains(current.SchemaVersions, requiredMigration) {
		return fmt.Errorf("required migration %s is not recorded", requiredMigration)
	}
	for _, column := range []struct{ table, name string }{
		{"api_keys", "used_tokens"}, {"api_keys", "default_model"}, {"api_keys", "budget_monthly"},
		{"usage_logs", "usage_estimated"}, {"eval_samples", "expires_at"}, {"eval_samples", "manual_policy_confirmed_at"},
	} {
		if !db.Migrator().HasColumn(column.table, column.name) {
			return fmt.Errorf("required column %s.%s is missing", column.table, column.name)
		}
	}
	if !db.Migrator().HasIndex(&model.EvalSample{}, "idx_eval_samples_expires_at") {
		return errors.New("required index eval_samples.idx_eval_samples_expires_at is missing")
	}
	fmt.Println("schema and pre-migration record fingerprints verified; migration 005 is recorded")
	return nil
}

func contains(values []string, target string) bool {
	for _, value := range values {
		if value == target {
			return true
		}
	}
	return false
}

func runSamples(db *gorm.DB, cfg *mysqlDriver.Config, args []string) error {
	if len(args) == 0 {
		return errors.New("samples requires inventory or delete")
	}
	switch args[0] {
	case "inventory":
		if len(args) != 1 {
			return errors.New("samples inventory accepts no extra arguments")
		}
		return inventorySamples(db)
	case "delete":
		return deleteOneSample(db, cfg, args[1:])
	default:
		return fmt.Errorf("unknown samples command %q", args[0])
	}
}

func inventorySamples(db *gorm.DB) error {
	if !db.Migrator().HasTable(&model.EvalSample{}) {
		return errors.New("eval_samples table is missing from the isolated database")
	}
	expiresExpr, policyExpr := "''", "''"
	if db.Migrator().HasColumn(&model.EvalSample{}, "expires_at") {
		expiresExpr = "COALESCE(DATE_FORMAT(expires_at, '%Y-%m-%dT%H:%i:%s'), '')"
	}
	if db.Migrator().HasColumn(&model.EvalSample{}, "manual_policy_confirmed_at") {
		policyExpr = "COALESCE(DATE_FORMAT(manual_policy_confirmed_at, '%Y-%m-%dT%H:%i:%s'), '')"
	}
	query := fmt.Sprintf(`SELECT id, dataset_id, source, DATE_FORMAT(created_at, '%%Y-%%m-%%dT%%H:%%i:%%s'), %s, %s FROM eval_samples ORDER BY id`, expiresExpr, policyExpr)
	rows, err := db.Raw(query).Rows()
	if err != nil {
		return err
	}
	defer rows.Close()
	var samples []sampleRow
	for rows.Next() {
		var item sampleRow
		if err := rows.Scan(&item.ID, &item.DatasetID, &item.Source, &item.CreatedAt, &item.ExpiresAt, &item.PolicyConfirmedAt); err != nil {
			return err
		}
		samples = append(samples, item)
	}
	if err := rows.Err(); err != nil {
		return err
	}
	counts := make(map[string]int)
	legacyWithoutExpiry := 0
	for _, item := range samples {
		counts[item.Source]++
		if item.ExpiresAt == "" {
			legacyWithoutExpiry++
		}
	}
	result := struct {
		Database                 string         `json:"database"`
		Total                    int            `json:"total_samples"`
		BySource                 map[string]int `json:"by_source"`
		LegacyWithoutExpiryCount int            `json:"legacy_without_expiry"`
		Samples                  []sampleRow    `json:"samples"`
	}{cfgDatabase(db), len(samples), counts, legacyWithoutExpiry, samples}
	encoded, err := json.MarshalIndent(result, "", "  ")
	if err != nil {
		return err
	}
	fmt.Println(string(encoded))
	return nil
}

func cfgDatabase(db *gorm.DB) string {
	var name string
	_ = db.Raw("SELECT DATABASE()").Scan(&name).Error
	return name
}

func deleteOneSample(db *gorm.DB, cfg *mysqlDriver.Config, args []string) error {
	flags := flag.NewFlagSet("samples delete", flag.ContinueOnError)
	id := flags.Uint64("id", 0, "one sample ID")
	confirmID := flags.Uint64("confirm-id", 0, "must equal --id")
	backupPath := flags.String("backup-before-delete", "", "new private full-schema backup file")
	if err := flags.Parse(args); err != nil {
		return err
	}
	if *id == 0 || *confirmID != *id || *backupPath == "" {
		return errors.New("--id, matching --confirm-id, and --backup-before-delete are required; batch deletion is not supported")
	}
	if !db.Migrator().HasColumn(&model.EvalSample{}, "expires_at") || !db.Migrator().HasColumn(&model.EvalSample{}, "manual_policy_confirmed_at") {
		return errors.New("apply the full retention migration to the isolated copy before deleting any sample")
	}
	var sample sampleRow
	err := db.Raw(`SELECT id, dataset_id, source, DATE_FORMAT(created_at, '%Y-%m-%dT%H:%i:%s'),
		COALESCE(DATE_FORMAT(expires_at, '%Y-%m-%dT%H:%i:%s'), ''),
		COALESCE(DATE_FORMAT(manual_policy_confirmed_at, '%Y-%m-%dT%H:%i:%s'), '')
		FROM eval_samples WHERE id = ?`, *id).Row().Scan(&sample.ID, &sample.DatasetID, &sample.Source, &sample.CreatedAt, &sample.ExpiresAt, &sample.PolicyConfirmedAt)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return gorm.ErrRecordNotFound
		}
		return err
	}
	digest, err := dumpDatabase(cfg, *backupPath)
	if err != nil {
		return fmt.Errorf("required pre-delete backup failed; sample was not deleted: %w", err)
	}
	fmt.Printf("Target: schema=%s sample_id=%d dataset_id=%d source=%s created_at=%s\n", cfg.DBName, sample.ID, sample.DatasetID, sample.Source, sample.CreatedAt)
	fmt.Printf("Private backup: %s (sha256 %s)\n", *backupPath, digest)
	fmt.Printf("Type DELETE %d to remove only this sample and redact its saved report text: ", *id)
	line, err := bufio.NewReader(os.Stdin).ReadString('\n')
	if err != nil && err != io.EOF {
		return err
	}
	if strings.TrimSpace(line) != fmt.Sprintf("DELETE %d", *id) {
		return errors.New("confirmation did not match; no sample was deleted (backup was retained)")
	}
	evals := service.NewEvalService(&repository.Repository{DB: db}, nil, nil, nil)
	if err := evals.DeleteSample(*id); err != nil {
		return err
	}
	fmt.Printf("sample %d deleted from isolated schema %s; backup retained\n", *id, cfg.DBName)
	return nil
}

func makeMySQLOptionFile(cfg *mysqlDriver.Config) (string, func(), error) {
	host, port, err := net.SplitHostPort(cfg.Addr)
	if err != nil {
		return "", func() {}, err
	}
	for _, value := range []string{host, cfg.User, cfg.Passwd} {
		if strings.ContainsAny(value, "\r\n\x00") {
			return "", func() {}, errors.New("MySQL credentials contain unsupported control characters")
		}
	}
	quote := func(value string) string {
		value = strings.ReplaceAll(value, `\`, `\\`)
		value = strings.ReplaceAll(value, `"`, `\"`)
		return `"` + value + `"`
	}
	content := fmt.Sprintf("[client]\nhost=%s\nport=%s\nuser=%s\npassword=%s\nprotocol=TCP\n", quote(host), port, quote(cfg.User), quote(cfg.Passwd))
	file, err := os.CreateTemp("", "aegis-rehearsal-mysql-*.cnf")
	if err != nil {
		return "", func() {}, err
	}
	if err := file.Chmod(0o600); err != nil {
		_ = file.Close()
		_ = os.Remove(file.Name())
		return "", func() {}, err
	}
	if _, err := file.WriteString(content); err != nil {
		_ = file.Close()
		_ = os.Remove(file.Name())
		return "", func() {}, err
	}
	if err := file.Close(); err != nil {
		_ = os.Remove(file.Name())
		return "", func() {}, err
	}
	return file.Name(), func() { _ = os.Remove(file.Name()) }, nil
}

func writeNewPrivateFile(path string, content []byte) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return err
	}
	file, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o600)
	if err != nil {
		return err
	}
	if _, err := file.Write(content); err != nil {
		_ = file.Close()
		_ = os.Remove(path)
		return err
	}
	if err := file.Close(); err != nil {
		_ = os.Remove(path)
		return err
	}
	return nil
}

func hashFile(path string) (string, error) {
	file, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer file.Close()
	h := sha256.New()
	if _, err := io.Copy(h, file); err != nil {
		return "", err
	}
	return hex.EncodeToString(h.Sum(nil)), nil
}

func closeDB(db *gorm.DB) {
	if sqlDB, err := db.DB(); err == nil {
		_ = sqlDB.Close()
	}
}

func fatal(err error) {
	fmt.Fprintln(os.Stderr, "db-rehearsal:", err)
	os.Exit(1)
}
