package main

import (
	"bytes"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func Example_caltech() {
	var out, errs bytes.Buffer
	code := run([]string{"caltech", "--dry-run"}, strings.NewReader(""), &out, &errs)
	fmt.Printf("missing provenance refused: %t\n", code != 0)
	// Output: missing provenance refused: true
}

func TestCaltechRetainsASelectedDocumentAndRecordsItsHash(t *testing.T) {
	tool := askingHosts(t)
	args, into := retentionArgs(t, "caltech")
	body := "<!doctype html><html><p>392 solar cells</p></html>"
	tool.get.EXPECT().Get("https://www.spitzer.caltech.edu/guide.html").
		Return(io.NopCloser(strings.NewReader(body)), int64(len(body)), nil)
	out, errs, code := ran(tool.command, args...)
	if code != exitOK {
		t.Fatalf("exit %d: %s %s", code, out, errs)
	}
	got, err := os.ReadFile(filepath.Join(into, "guide.html"))
	if err != nil || string(got) != body {
		t.Fatalf("retained %q, %v", got, err)
	}
	log, err := os.ReadFile(filepath.Join(into, "FETCHED.md"))
	if err != nil || !strings.Contains(string(log), "celestial-data/caltech") ||
		!strings.Contains(string(log), "SHA-256:") ||
		!strings.Contains(string(log), "Status: SUCCESS") {
		t.Fatalf("provenance %q, %v", log, err)
	}
}

func TestCaltechDryRunValidatesScopeWithoutWriting(t *testing.T) {
	folder := t.TempDir()
	catalog := filepath.Join(folder, "catalog.json")
	into := filepath.Join(folder, "absent")
	good := `[{"ID":"solar","Name":"solar.html","URL":"https://www.spitzer.caltech.edu/mission/the-solar-panel-assembly","Title":"Solar panel","Kind":"html"}]`
	if err := os.WriteFile(catalog, []byte(good), 0600); err != nil {
		t.Fatal(err)
	}
	args := []string{"caltech", "--dry-run", "--catalog", catalog, "--into", into, "--adapter-commit", strings.Repeat("a", 40), "--command", "test", "--purpose", "test", "--consumer", "test", "solar"}
	var out, errs bytes.Buffer
	if code := run(args, strings.NewReader(""), &out, &errs); code != 0 || !strings.Contains(out.String(), "would fetch solar") {
		t.Fatalf("%d: %s %s", code, out.String(), errs.String())
	}
	if _, err := os.Stat(into); !os.IsNotExist(err) {
		t.Fatalf("dry run wrote output: %v", err)
	}
	if err := os.WriteFile(catalog, []byte(strings.Replace(good, "www.spitzer.caltech.edu", "example.org", 1)), 0600); err != nil {
		t.Fatal(err)
	}
	out.Reset()
	errs.Reset()
	if code := run(args, strings.NewReader(""), &out, &errs); code == 0 {
		t.Fatal("accepted foreign host")
	}
}
