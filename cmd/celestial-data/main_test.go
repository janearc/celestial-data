package main

import (
	"os"
	"os/exec"
	"strings"
	"testing"
)

// asMain is the variable that makes this test binary run main.
const asMain = "CELESTIAL_DATA_TEST_AS_MAIN"

// deadProxy is an address nothing listens on: the discard port, on this
// machine.
const deadProxy = "http://127.0.0.1:9"

// TestMain runs main when it is asked to, rather than the tests. That is
// how the tests see main's exit code.
//
// It points every proxy at a port nothing listens on first. The tests
// ask the network nothing. Should a change to the code let one reach the
// real web, that asks no one, and fails fast. A server on this machine is
// still reached, since Go sends no loopback address through a proxy.
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

// mainExits is the code this binary exits with when run as celestial-data with
// the words, and what it said.
func mainExits(t *testing.T, words ...string) (int, string) {
	t.Helper()
	self, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	child := exec.Command(self, words...)
	child.Env = append(os.Environ(), asMain+"=1")
	out, err := child.Output()
	if exited, isExit := err.(*exec.ExitError); isExit {
		return exited.ExitCode(), string(out)
	}
	if err != nil {
		t.Fatal(err)
	}
	return exitOK, string(out)
}

func TestMainRunsTheWordsAfterItsName(t *testing.T) {
	code, out := mainExits(t, "archives")
	if code != exitOK || !strings.HasPrefix(out, "chandra ") {
		t.Errorf("celestial-data archives exited %d, said %q", code, out)
	}
}

// Anti-tests: a set named with no folder given is refused by the real
// command, which reads the real environment. Nothing is asked of the
// network: the refusal comes first.
func TestMainRefusesASetWithNoFolder(t *testing.T) {
	t.Setenv("GRISSOM_DATA", "")
	if code, out := mainExits(t, "fetch-set", "grissom"); code != failed ||
		out != "" {
		t.Errorf("celestial-data fetch-set grissom exited %d, said %q",
			code, out)
	}
}

func TestMainExitsTwoForNothingToRun(t *testing.T) {
	if code, _ := mainExits(t, "frobnicate"); code != exitUsage {
		t.Errorf("celestial-data frobnicate exited %d, want %d", code, exitUsage)
	}
}
