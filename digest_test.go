package celestialdata_test

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/janearc/celestial-data"
)

// digestOf is the sha256 sum of some bytes, as a digest.
func digestOf(t *testing.T, content []byte) celestialdata.Digest {
	t.Helper()
	sum := sha256.Sum256(content)
	digest, err := celestialdata.ParseDigest(hex.EncodeToString(sum[:]))
	if err != nil {
		t.Fatal(err)
	}
	return digest
}

// A digest is parsed from the hex a sha256 sum is written in, whatever
// its case, and given back as lowercase hex; a text that is not 64 hex
// characters is refused, and says what was wrong.
func TestADigestRoundTripsItsSum(t *testing.T) {
	content := []byte("photons")
	sum := sha256.Sum256(content)
	text := strings.ToUpper(hex.EncodeToString(sum[:]))
	sumTooShort := text[:len(text)-1] // defined: a character short
	sumTooLong := text + "0"          // defined: a character past
	digest, err := celestialdata.ParseDigest(text)
	if err != nil || digest.String() != strings.ToLower(text) ||
		!digest.Equal(digestOf(t, content)) {
		t.Fatalf("%v, %v", digest, err)
	}
	other := digestOf(t, []byte("other photons"))
	if digest.Equal(other) || other.Equal(digest) {
		t.Errorf("%v and %v are equal", digest, other)
	}
	for _, text := range []string{sumTooShort, sumTooLong} {
		if _, err := celestialdata.ParseDigest(text); err == nil ||
			!errors.Is(err, celestialdata.Refused()) ||
			!strings.Contains(err.Error(),
				"a sha256 sum is 64 hex characters") {
			t.Errorf("%q: %v", text, err)
		}
	}
	if _, err := celestialdata.ParseDigest(strings.Repeat("z",
		64)); !errors.Is(err, celestialdata.Refused()) ||
		!strings.Contains(err.Error(), "not written in hex") {
		t.Errorf("not hex: %v", err)
	}
}

// A digest's check says nothing for a file of the bytes it sums, and
// names the file, the sum it wanted and the sum it found for one of
// other bytes. A file that will not read is wrapped.
func TestADigestChecksTheBytesItSums(t *testing.T) {
	content := []byte("photons")
	digest := digestOf(t, content)
	path := filepath.Join(t.TempDir(), file)
	if err := os.WriteFile(path, content, 0o644); err != nil {
		t.Fatal(err)
	}
	if err := digest.Check(path); err != nil {
		t.Errorf("the bytes it sums: %v", err)
	}
	wrong := digestOf(t, []byte("other photons"))
	err := wrong.Check(path)
	if err == nil || !strings.Contains(err.Error(), path) ||
		!strings.Contains(err.Error(), wrong.String()) ||
		!strings.Contains(err.Error(), digest.String()) {
		t.Errorf("other bytes: %v", err)
	}
	absent := filepath.Join(t.TempDir(), "absent.txt")
	if err := digest.Check(absent); !errors.Is(err, os.ErrNotExist) ||
		!strings.Contains(err.Error(), absent) {
		t.Errorf("no file: %v", err)
	}
}

// A file that carries a digest is fetched only when its bytes sum to
// it: the right sum is kept, a wrong one fails the fetch and leaves
// nothing behind, neither the file nor its .part.
func TestAFetchOfASummedFileHoldsItToItsDigest(t *testing.T) {
	content := []byte("photons")
	for why, sum := range map[string]celestialdata.Digest{
		"the bytes' own sum": digestOf(t, content),
		"another bytes' sum": digestOf(t, []byte("other photons")),
	} {
		get := served(t, content, lengthOf(content))
		summed := celestialdata.NewFile("obs", file, where,
			lengthOf(content), about).Summed(sum)
		fixed, err := celestialdata.NewFixed("bluemarble", summed)
		if err != nil {
			t.Fatal(err)
		}
		found, err := fixed.Find(nil)
		if err != nil || len(found) != lone {
			t.Fatalf("%s: %v, %v", why, found, err)
		}
		into := t.TempDir()
		_, err = celestialdata.Fetch(get, found[firstOf], into)
		if why == "the bytes' own sum" {
			if err != nil || !nothingBut(t, into, file) {
				t.Errorf("%s: %v", why, err)
			}
		} else if err == nil || !nothingBut(t, into) {
			t.Errorf("%s: %v", why, err)
		}
	}
}

// A file with no digest fetches as it always did.
func TestAFetchOfAFileWithNoDigestIsAsBefore(t *testing.T) {
	content := []byte("photons")
	get := served(t, content, lengthOf(content))
	plain := celestialdata.NewFile("obs", file, where, lengthOf(content),
		about)
	fixed, err := celestialdata.NewFixed("bluemarble", plain)
	if err != nil {
		t.Fatal(err)
	}
	found, err := fixed.Find(nil)
	if err != nil || len(found) != lone {
		t.Fatalf("%v, %v", found, err)
	}
	into := t.TempDir()
	home, err := celestialdata.Fetch(get, found[firstOf], into)
	if err != nil || home != filepath.Join(into, file) ||
		!nothingBut(t, into, file) {
		t.Errorf("%s, %v", home, err)
	}
}
