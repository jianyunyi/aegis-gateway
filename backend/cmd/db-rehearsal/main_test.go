package main

import (
	"os"
	"testing"

	mysqlDriver "github.com/go-sql-driver/mysql"
)

func TestValidateTargetRequiresLoopbackAndRehearsalSchema(t *testing.T) {
	tests := []struct {
		name string
		dsn  string
		want bool
	}{
		{"local rehearsal", "aegis:secret@tcp(127.0.0.1:3306)/aegis_rehearsal_20261102_a1b2c3", true},
		{"local base rehearsal", "aegis:secret@tcp(localhost:3306)/aegis_rehearsal", true},
		{"production schema", "aegis:secret@tcp(127.0.0.1:3306)/aegis", false},
		{"lookalike schema", "aegis:secret@tcp(127.0.0.1:3306)/aegis_rehearsal_prod", false},
		{"remote mysql", "aegis:secret@tcp(db.internal:3306)/aegis_rehearsal", false},
		{"remote loopback alias", "aegis:secret@tcp(192.0.2.10:3306)/aegis_rehearsal", false},
		{"unix socket", "aegis:secret@unix(/tmp/mysql.sock)/aegis_rehearsal", false},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			cfg, err := mysqlDriver.ParseDSN(test.dsn)
			if err != nil {
				t.Fatalf("parse test DSN: %v", err)
			}
			err = validateTarget(cfg)
			if (err == nil) != test.want {
				t.Fatalf("validateTarget err=%v, want success=%v", err, test.want)
			}
		})
	}
}

func TestValidateDumpHeaderBlocksDatabaseSwitches(t *testing.T) {
	for _, test := range []struct {
		name string
		body string
		want bool
	}{
		{"plain database dump", "-- MySQL dump\nCREATE TABLE users (id int);\n", true},
		{"use source schema", "-- MySQL dump\nUSE aegis;\n", false},
		{"create database", "CREATE DATABASE aegis;\n", false},
		{"drop database", "DROP DATABASE aegis;\n", false},
	} {
		t.Run(test.name, func(t *testing.T) {
			file, err := os.CreateTemp(t.TempDir(), "dump-*.sql")
			if err != nil {
				t.Fatal(err)
			}
			if _, err := file.WriteString(test.body); err != nil {
				t.Fatal(err)
			}
			if err := file.Close(); err != nil {
				t.Fatal(err)
			}
			file, err = os.Open(file.Name())
			if err != nil {
				t.Fatal(err)
			}
			defer file.Close()
			err = validateDumpHeader(file)
			if (err == nil) != test.want {
				t.Fatalf("validateDumpHeader err=%v, want success=%v", err, test.want)
			}
		})
	}
}
