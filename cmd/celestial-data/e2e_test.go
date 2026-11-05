package main

import (
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// stampedCommit is the commit the end-to-end binary is stamped with.
const stampedCommit = "abc1234"

// binary builds celestial-data into a folder of the test's own and returns its
// path. The build is stamped as game stamps it.
func binary(t *testing.T) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "celestial-data")
	stamp := "-X main.build=" + stampedCommit +
		" -X main.built=2026-11-02T00:00:00Z"
	build := exec.Command("go", "build", "-ldflags", stamp, "-o", path, ".")
	if out, err := build.CombinedOutput(); err != nil {
		t.Fatalf("go build: %v: %s", err, out)
	}
	return path
}

// runs runs the binary with the words and gives what it said and its
// exit code.
func runs(t *testing.T, path string, words ...string) (string, int) {
	t.Helper()
	out, err := exec.Command(path, words...).Output()
	if exited, isExit := err.(*exec.ExitError); isExit {
		return string(out), exited.ExitCode()
	}
	if err != nil {
		t.Fatal(err)
	}
	return string(out), exitOK
}

// The built binary tells its age and tells an agent what it does. It lists
// its archives. None of it can ask the network, even when a change to
// the code goes wrong; find and fetch are held by the examples instead.
func TestTheBuiltBinaryAnswers(t *testing.T) {
	path := binary(t)
	for _, each := range []struct {
		words []string
		want  string
	}{
		{[]string{"--age"}, stampedCommit},
		{[]string{"--agent-help"}, "verb fetch: "},
		{[]string{"archives"}, "meerkat       MeerKAT's Galactic Centre"},
	} {
		out, code := runs(t, path, each.words...)
		if code != exitOK || !strings.Contains(out, each.want) {
			t.Errorf("celestial-data %s exited %d, said %q", each.words, code,
				out)
		}
	}
}
