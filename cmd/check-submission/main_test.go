package main

import (
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

func TestRunWritesSummaryAndFailsOnErrors(t *testing.T) {
	repo, err := filepath.Abs("../..")
	if err != nil {
		t.Fatal(err)
	}
	summary := filepath.Join(t.TempDir(), "summary.md")
	if err := run(repo, repo, summary); err != nil {
		t.Fatalf("current repository: %v", err)
	}
	data, err := os.ReadFile(summary)
	if err != nil || !strings.Contains(string(data), "0 errors") {
		t.Fatalf("summary %q: %v", data, err)
	}

	head := t.TempDir()
	copyTree(t, repo, head)
	stray := filepath.Join(head, "testnet", "zz_probe_test.go")
	if err := os.WriteFile(stray, []byte("package probe"), 0o644); err != nil {
		t.Fatal(err)
	}
	err = run(head, repo, summary)
	if err == nil || !regexp.MustCompile(`^1 error in \d+ entries$`).MatchString(err.Error()) {
		t.Fatalf("want one error, got %v", err)
	}
	data, _ = os.ReadFile(summary)
	if !strings.Contains(string(data), "testnet/zz_probe_test.go") {
		t.Fatalf("summary misses the stray file:\n%s", data)
	}
}

func TestRunWithoutBaseOrSummary(t *testing.T) {
	if err := run("../..", "", ""); err != nil {
		t.Fatal(err)
	}
}

func copyTree(t *testing.T, from, to string) {
	t.Helper()
	err := filepath.WalkDir(from, func(name string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		rel, _ := filepath.Rel(from, name)
		if d.Name() == ".git" {
			if d.IsDir() {
				return filepath.SkipDir
			}
			return nil
		}
		if d.IsDir() {
			return os.MkdirAll(filepath.Join(to, rel), 0o755)
		}
		data, err := os.ReadFile(name)
		if err != nil {
			return err
		}
		return os.WriteFile(filepath.Join(to, rel), data, 0o644)
	})
	if err != nil {
		t.Fatal(err)
	}
}
