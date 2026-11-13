package ads

import (
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"syscall"
)

// Session is the variable that names where the token is kept:
// celestial-data-secrets ads-login writes it, and celestial-data ads
// reads it.
const Session = "ADS_SESSION"

// TokenFile is the name of the token's file beside the secrets'
// config, where it is kept when Session names nowhere.
const TokenFile = "ads-token"

// The modes and lengths a kept token is held to.
const (
	// onlyItsOwner is the mode a token is kept with. Defined: read and
	// written by the user who kept it, and by no one else.
	onlyItsOwner = 0o600
	// noOneElse is the group's and others' bits of a mode. Defined: a
	// kept token has none of them; its owner's are its owner's own.
	noOneElse = 0o077
	// mostToken is the most bytes a kept token's file is read to.
	// Defined: far past any ADS token; a longer file holds no token.
	mostToken = 4096
	// onePast is the byte read past mostToken. Defined: it tells a file
	// longer than a token from one exactly as long.
	onePast = 1
	// noBits is a mode with none of the bits a mask asks after.
	// Defined.
	noBits = 0
)

// errNoToken is the refusal of an empty token, kept or read.
var errNoToken = errors.New("ads: no token")

// Save keeps a token in path, readable by its owner alone, over a token
// kept there before, so a search need not ask 1Password for it again.
//
// It refuses an empty token, and a link, a folder or a file anyone else
// can read in the token's place. No error holds the token.
//
// The token is written to a new file of its own beside path, then put
// in path's place, so a link put there later is replaced and not
// followed, and a write that fails leaves the token kept before as it
// was.
func Save(token, path string) error {
	if token == "" {
		return errNoToken
	}
	if err := keptOrNone(path); err != nil {
		return err
	}
	part, err := os.CreateTemp(filepath.Dir(path),
		"."+filepath.Base(path)+".*")
	if err != nil {
		return fmt.Errorf("ads: %w", err)
	}
	_, wrote := part.WriteString(token + "\n")
	err = errors.Join(wrote, part.Sync(), part.Close())
	if err == nil {
		err = os.Rename(part.Name(), path)
	}
	if err != nil {
		os.Remove(part.Name())
		return fmt.Errorf("ads: %w", err)
	}
	return nil
}

// Load reads the token Save kept in path. It refuses a link, a folder,
// anything but a plain file, a file anyone but its owner can read, one
// that holds no token, and one longer than a token.
//
// It checks the file it opened, not the name, before it reads, and
// never waits on a pipe in a token's place. No error holds the token.
func Load(path string) (string, error) {
	file, err := os.OpenFile(path,
		os.O_RDONLY|syscall.O_NOFOLLOW|syscall.O_NONBLOCK, 0)
	if errors.Is(err, syscall.ELOOP) {
		return "", fmt.Errorf("ads: %s is a link; a kept token is a "+
			"file of its own", path)
	}
	if err != nil {
		return "", fmt.Errorf("ads: %w", err)
	}
	defer file.Close()
	about, err := file.Stat()
	if err != nil {
		return "", fmt.Errorf("ads: %w", err)
	}
	if err := ownersAlone(path, about); err != nil {
		return "", err
	}
	text, err := io.ReadAll(io.LimitReader(file, mostToken+onePast))
	if err != nil {
		return "", fmt.Errorf("ads: %w", err)
	}
	if len(text) > mostToken {
		return "", fmt.Errorf("ads: %s is longer than a token", path)
	}
	token := strings.TrimSpace(string(text))
	if token == "" {
		return "", fmt.Errorf("%w in %s", errNoToken, path)
	}
	return token, nil
}

// keptOrNone refuses what is in path's place unless it is nothing yet,
// or a token kept as Save keeps one. It looks at path itself, not where
// a link there points.
func keptOrNone(path string) error {
	about, err := os.Lstat(path)
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	if err != nil {
		return fmt.Errorf("ads: %w", err)
	}
	if about.Mode()&os.ModeSymlink != noBits {
		return fmt.Errorf("ads: %s is a link; a kept token is a file "+
			"of its own", path)
	}
	return ownersAlone(path, about)
}

// ownersAlone refuses a file that is not a plain file, or that anyone
// but its owner can read or write.
func ownersAlone(path string, about os.FileInfo) error {
	if about.IsDir() {
		return fmt.Errorf("ads: %s is a directory, not a kept token",
			path)
	}
	if !about.Mode().IsRegular() || about.Mode().Perm()&noOneElse != noBits {
		return fmt.Errorf("ads: %s is %v; a kept token is a file "+
			"readable by no one else, as %v is", path, about.Mode(),
			os.FileMode(onlyItsOwner))
	}
	return nil
}
