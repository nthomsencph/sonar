package cmd

import (
	"reflect"
	"testing"
)

func TestEnvExportsAreSortedAndQuoted(t *testing.T) {
	got := envExports(map[string]string{
		"PORT":    "21408",
		"DB_URL":  "postgres://localhost:5432/app",
		"MESSAGE": "it's $HOME, not \"mine\"",
	})
	want := []string{
		`export DB_URL=postgres://localhost:5432/app`,
		`export MESSAGE='it'\''s $HOME, not "mine"'`,
		`export PORT=21408`,
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("envExports =\n%s\nwant\n%s", got, want)
	}
}

func TestEnvSummaryLeavesPortToTheColumn(t *testing.T) {
	got := envSummary(map[string]string{
		"PORT":         "21409",
		"VITE_API_URL": "http://localhost:21408",
		"API_PORT":     "21408",
	})
	if want := "API_PORT=21408 VITE_API_URL=http://localhost:21408"; got != want {
		t.Errorf("envSummary = %q, want %q", got, want)
	}
	if got := envSummary(map[string]string{"PORT": "1"}); got != "" {
		t.Errorf("envSummary with only PORT = %q, want empty", got)
	}
}
