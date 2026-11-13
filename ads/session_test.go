package ads_test

import (
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"syscall"
	"testing"
	"time"

	"github.com/janearc/celestial-data/ads"
)

// The modes a kept token's file is tested at.
const (
	ownerOnly = 0o600 // defined: read and written by its owner alone
	anyReader = 0o644 // defined: read by anyone, written by its owner
)

// kept is the path of a token kept in a test's folder.
func kept(t *testing.T) string {
	t.Helper()
	return filepath.Join(t.TempDir(), "ads-token")
}

// A token kept is read back as it was, from a file its owner alone can
// read, and keeping another replaces it.
//
// Anti-test: the file is not left readable by anyone else.
func TestATokenKeptIsReadBack(t *testing.T) {
	path := kept(t)
	for _, want := range []string{token, "a-second-token"} {
		if err := ads.Save(want, path); err != nil {
			t.Fatal(err)
		}
		got, err := ads.Load(path)
		about, statErr := os.Stat(path)
		if err != nil || got != want || statErr != nil ||
			about.Mode().Perm() != ownerOnly {
			t.Errorf("%q, %v, %v", got, err, statErr)
		}
	}
}

// Anti-test: a file anyone else can read is not read, nor written over,
// and its refusal says the mode and never the token.
func TestATokenOthersCanReadIsRefused(t *testing.T) {
	path := kept(t)
	if err := os.WriteFile(path, []byte(token), anyReader); err != nil {
		t.Fatal(err)
	}
	_, loadErr := ads.Load(path)
	saveErr := ads.Save(token, path)
	for _, err := range []error{loadErr, saveErr} {
		if err == nil || !strings.Contains(err.Error(), "-rw-r--r--") ||
			!strings.Contains(err.Error(), "as -rw------- is") ||
			strings.Contains(err.Error(), token) {
			t.Errorf("%v", err)
		}
	}
}

// Anti-test: an empty token is not kept, and a file that holds none is
// not read as one.
func TestNoTokenIsNoToken(t *testing.T) {
	path := kept(t)
	if err := ads.Save("", path); err == nil {
		t.Error("an empty token was kept")
	}
	if err := os.WriteFile(path, []byte("\n"), ownerOnly); err != nil {
		t.Fatal(err)
	}
	if got, err := ads.Load(path); err == nil || got != "" {
		t.Errorf("%q, %v", got, err)
	}
}

// Anti-test: a token never kept, and a folder in a token's place, are
// refused.
func TestNoFileIsNoToken(t *testing.T) {
	if _, err := ads.Load(kept(t)); !errors.Is(err, fs.ErrNotExist) {
		t.Errorf("a missing file gave %v, not that it is not there", err)
	}
	folder := t.TempDir()
	if err := ads.Save(token, folder); err == nil {
		t.Error("a token was kept over a folder")
	}
	if _, err := ads.Load(folder); err == nil ||
		!strings.Contains(err.Error(), "is a directory") {
		t.Errorf("a folder gave %v", err)
	}
}

// More modes and sizes the tests write, each defined.
const (
	ownerReads  = 0o400 // defined: read by its owner alone, written by no one
	closedDir   = 0o500 // defined: a folder its owner reads but cannot write in
	openDir     = 0o700 // defined: a folder its owner reads and writes in
	pastAToken  = 4097  // defined: one byte more than a token's file is read to
	oneMore     = 1     // defined: the byte that makes a file too long
	pipePatient = 2     // defined: seconds a read of a pipe is given to refuse
)

// Anti-test: a link in a token's place is neither read nor written
// through. Save and Load followed one, so a token was kept where the
// link pointed and read from there.
func TestALinkInATokensPlaceIsRefused(t *testing.T) {
	path := kept(t)
	elsewhere := filepath.Join(t.TempDir(), "elsewhere")
	if err := os.WriteFile(elsewhere, []byte("left as it was"),
		ownerOnly); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(elsewhere, path); err != nil {
		t.Fatal(err)
	}
	saveErr := ads.Save(token, path)
	_, loadErr := ads.Load(path)
	for _, err := range []error{saveErr, loadErr} {
		if err == nil || !strings.Contains(err.Error(), "is a link") {
			t.Errorf("a link was taken: %v", err)
		}
	}
	if left, err := os.ReadFile(elsewhere); err != nil ||
		string(left) != "left as it was" {
		t.Errorf("the link's target became %q, %v", left, err)
	}
}

// Anti-test: Save writes a new file and puts it in the token's place; it
// never writes through the file that was there. A token kept under a
// second name, a hard link, kept the old token there: the old Save
// wrote the new one through it.
func TestSaveNeverWritesThroughTheFileThere(t *testing.T) {
	path := kept(t)
	if err := ads.Save("the-first-token", path); err != nil {
		t.Fatal(err)
	}
	second := filepath.Join(filepath.Dir(path), "second-name")
	if err := os.Link(path, second); err != nil {
		t.Fatal(err)
	}
	if err := ads.Save(token, path); err != nil {
		t.Fatal(err)
	}
	if left, err := os.ReadFile(second); err != nil ||
		strings.TrimSpace(string(left)) != "the-first-token" {
		t.Errorf("the new token went through the old file: %q, %v",
			left, err)
	}
}

// Anti-test: a write that fails leaves the token kept before as it was.
// The folder is closed to writing after the first token is kept.
func TestAFailedSaveKeepsTheTokenBefore(t *testing.T) {
	path := kept(t)
	if err := ads.Save("the-first-token", path); err != nil {
		t.Fatal(err)
	}
	folder := filepath.Dir(path)
	if err := os.Chmod(folder, closedDir); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { os.Chmod(folder, openDir) })
	if err := ads.Save(token, path); err == nil ||
		strings.Contains(err.Error(), token) {
		t.Errorf("a save into a closed folder said %v", err)
	}
	if got, err := ads.Load(path); err != nil || got != "the-first-token" {
		t.Errorf("the token before became %q, %v", got, err)
	}
}

// A token its owner alone can read, and no one write, is read: no group
// or other bits is what is asked, not exactly read and write.
//
// Anti-test: one longer than a token is refused, and a pipe in a token's
// place is refused at once, where reading it waited for a writer.
func TestATokenIsReadOnlyFromAFileOfItsOwn(t *testing.T) {
	path := kept(t)
	if err := os.WriteFile(path, []byte(token+"\n"), ownerReads); err != nil {
		t.Fatal(err)
	}
	if got, err := ads.Load(path); err != nil || got != token {
		t.Errorf("a token read by its owner alone gave %q, %v", got, err)
	}
	// a file as long as a token's file is read to is read
	whole := kept(t)
	longest := strings.Repeat("t", pastAToken-oneMore)
	if err := os.WriteFile(whole, []byte(longest), ownerOnly); err != nil {
		t.Fatal(err)
	}
	if got, err := ads.Load(whole); err != nil || got != longest {
		t.Errorf("a token of the longest length gave %d bytes, %v",
			len(got), err)
	}
	long := kept(t)
	if err := os.WriteFile(long, make([]byte, pastAToken),
		ownerOnly); err != nil {
		t.Fatal(err)
	}
	if _, err := ads.Load(long); err == nil ||
		!strings.Contains(err.Error(), "longer than a token") {
		t.Errorf("a long file gave %v", err)
	}
	pipe := kept(t)
	if err := syscall.Mkfifo(pipe, ownerOnly); err != nil {
		t.Fatal(err)
	}
	refused := make(chan error, 1)
	go func() { _, err := ads.Load(pipe); refused <- err }()
	select {
	case err := <-refused:
		if err == nil {
			t.Error("a pipe gave a token")
		}
	case <-time.After(pipePatient * time.Second):
		t.Error("a read of a pipe waited")
	}
}

// A path that cannot be looked at, here one under a file rather than a
// folder, is refused before anything is written, in words that hold no
// token.
//
// Anti-test: it is refused at the look at the path itself, not let
// through as nothing there yet and refused later, where the new file
// is made; both say "not a directory", so the path looked at is what
// tells them apart.
func TestAPathThatCannotBeLookedAtIsRefused(t *testing.T) {
	file := kept(t)
	err := os.WriteFile(file, []byte("a file\n"), ownerOnly)
	if err != nil {
		t.Fatal(err)
	}
	under := filepath.Join(file, "ads-token")
	err = ads.Save(token, under)
	if err == nil || strings.Contains(err.Error(), token) ||
		!strings.Contains(err.Error(), "lstat "+under) {
		t.Errorf("a save under a file said %v", err)
	}
	if _, statErr := os.Lstat(under); statErr == nil {
		t.Error("a save under a file wrote something")
	}
}
