package main

import (
	"os"
	"os/exec"
	"strings"
	"testing"
)

// asMain is the variable that makes this test binary run main.
const asMain = "CELESTIAL_DATA_SECRETS_TEST_AS_MAIN"

// deadProxy is an address nothing listens on: the discard port, on this
// machine.
const deadProxy = "http://127.0.0.1:9"

// TestMain runs the tests, or main when asMain says so. It points every
// proxy at a port nothing listens on first, so a change that let a test
// reach the real archive fails fast and asks no one.
func TestMain(suite *testing.M) {
	for _, name := range []string{"HTTPS_PROXY", "HTTP_PROXY", "ALL_PROXY"} {
		os.Setenv(name, deadProxy)
	}
	if os.Getenv(asMain) != "" {
		main()
		return
	}
	os.Exit(suite.Run())
}

// mainExits is the code this binary exits with when run as
// celestial-data-secrets with the words and the home given, and what it
// said on each stream.
func mainExits(t *testing.T, home string, words ...string) (int, string,
	string,
) {
	t.Helper()
	self, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	child := exec.Command(self, words...)
	child.Env = append(os.Environ(), asMain+"=1", "HOME="+home)
	var out, errs strings.Builder
	child.Stdout, child.Stderr = &out, &errs
	err = child.Run()
	if exited, isExit := err.(*exec.ExitError); isExit {
		return exited.ExitCode(), out.String(), errs.String()
	}
	if err != nil {
		t.Fatal(err)
	}
	return exitOK, out.String(), errs.String()
}

// the test: main runs the words after its name, finding the config
// under the real environment's home.
func TestMainRunsTheWordsAfterItsName(t *testing.T) {
	home := homeWith(t, both...)
	code, out, errs := mainExits(t, home, "list")
	if code != exitOK || !strings.Contains(out, "kept in "+gaiaItem) {
		t.Errorf("exit %d: %s%s", code, out, errs)
	}
}

// the anti-test: a word main does not know runs nothing, and exits as
// pzm says a run of nothing does.
func TestMainRunsNothingItDoesNotKnow(t *testing.T) {
	code, out, _ := mainExits(t, homeWith(t), "unlock")
	if code != exitUsage || out != "" {
		t.Errorf("exit %d: %s", code, out)
	}
}
