package celestialdata

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"os"
)

// hexLen is the characters a sha256 sum is written in. It's defined: a
// sum's bytes take two hex characters each.
const hexLen = sha256.Size * 2

// Digest is the sha256 sum a file's bytes should have. It is a value
// with nothing in it but the sum, so products stay comparable, as a
// Checker should be. Summed gives a File one, and Fetch keeps the
// fetched file only when its bytes sum to it.
type Digest struct {
	sum [sha256.Size]byte
}

// ParseDigest is the sha256 sum text holds, written in 64 hex
// characters, in either case. It refuses a text of any other length,
// and one that is not hex, saying which.
func ParseDigest(text string) (Digest, error) {
	if len(text) != hexLen {
		return Digest{}, Refuse("a sha256 sum is %d hex characters, "+
			"not %q", hexLen, text)
	}
	raw, err := hex.DecodeString(text)
	if err != nil {
		return Digest{}, Refuse("%q is not written in hex", text)
	}
	var digest Digest
	copy(digest.sum[:], raw)
	return digest, nil
}

// Check says nothing for a file whose bytes have this sum, or why not:
// the file's path, the sum it should have had, and the sum it has. A
// file that will not read is wrapped with its path.
func (digest Digest) Check(path string) error {
	file, err := os.Open(path)
	if err != nil {
		return fmt.Errorf("%s: %w", path, err)
	}
	defer file.Close()
	hashed := sha256.New()
	if _, err := io.Copy(hashed, file); err != nil {
		return fmt.Errorf("%s: %w", path, err)
	}
	var have [sha256.Size]byte
	copy(have[:], hashed.Sum(nil))
	if have != digest.sum {
		return fmt.Errorf("%s: sha256 %s, not %s", path,
			Digest{sum: have}, digest)
	}
	return nil
}

// String is the sum as lowercase hex.
func (digest Digest) String() string {
	return hex.EncodeToString(digest.sum[:])
}

// Equal is whether two digests are of the same bytes.
func (digest Digest) Equal(other Digest) bool {
	return digest == other
}
