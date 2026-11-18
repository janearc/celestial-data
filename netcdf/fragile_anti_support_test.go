//go:build fragile

package netcdf_test

// The tools of the anti-tests. An anti-test is the twin of a test of the
// reader's package: where that says a right thing is accepted and comes
// out so, its anti says the thing made wrong in exactly one way is
// refused or comes out differently; where that says a thing is refused,
// its anti says the nearest right thing is accepted. A test with no anti
// can pass on code that accepts, or refuses, everything.

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	classic "github.com/scigolib/netcdf"
)

// antiFixture is one of netcdf's own test fixtures, as bytes; the test is
// skipped when the variable names no folder or the fixture is not in it.
func antiFixture(t *testing.T, name string) []byte {
	t.Helper()
	directory := os.Getenv(fixtureDirectoryVariable)
	if directory == "" {
		t.Skipf("%s names no folder of netcdf's fixtures",
			fixtureDirectoryVariable)
	}
	raw, err := os.ReadFile(filepath.Join(directory, name))
	if err != nil {
		t.Skipf("netcdf's fixture %s is not there: %v", name, err)
	}
	return raw
}

// antiOpen opens bytes with his own package.
func antiOpen(raw []byte) (*classic.File, error) {
	return classic.NewReader(bytes.NewReader(raw), int64(len(raw)))
}

// antiObserve is everything his package says of bytes: the refusal, or
// the format, the records, the dimensions, the attributes, and each
// variable with what it reads as.
func antiObserve(raw []byte) string {
	file, err := antiOpen(raw)
	if err != nil {
		return "refused: " + err.Error()
	}
	defer file.Close()
	var out strings.Builder
	fmt.Fprintf(&out, "%s records %d\n", file.Format(), file.NumRecs())
	for _, each := range file.Dimensions() {
		fmt.Fprintf(&out, "dimension %q %d %t\n", each.Name(), each.Len(),
			each.Unlimited())
	}
	antiAttributes(&out, "global", file.Attributes())
	for _, each := range file.Variables() {
		fmt.Fprintf(&out, "variable %q %s %v %t\n", each.Name(), each.Type(),
			each.Shape(), each.Record())
		antiAttributes(&out, each.Name(), each.Attributes())
		got, err := readHis(each, int64(len(raw)))
		fmt.Fprintf(&out, "values %v, %v\n", got, err)
	}
	return out.String()
}

// antiAttributes writes attributes of an owner into a builder.
func antiAttributes(out *strings.Builder, owner string,
	list []*classic.Attribute) {
	for _, each := range list {
		fmt.Fprintf(out, "attribute %s %q %s %x\n", owner, each.Name(),
			each.Type(), each.Bytes())
	}
}

// antiDiffers fails the test unless a mutation of bytes that open is
// observed differently, a refusal being a difference.
func antiDiffers(t *testing.T, what string, original, mutated []byte) {
	t.Helper()
	was := antiObserve(original)
	if strings.HasPrefix(was, "refused") {
		t.Fatalf("%s: the file before the change is refused: %s", what, was)
	}
	if now := antiObserve(mutated); now == was {
		t.Errorf("%s: the change is not seen, the file reads as it did:\n%s",
			what, was)
	}
}

// antiSame fails the test if a file is observed differently from
// another, for a change that must change nothing.
func antiSame(t *testing.T, what string, original, changed []byte) {
	t.Helper()
	if was, now := antiObserve(original), antiObserve(changed); was != now {
		t.Errorf("%s: the file reads differently:\nbefore:\n%s\nafter:\n%s",
			what, was, now)
	}
}

// antiPatch is bytes with the first run of old replaced by new, which
// must be as long; the test stops if old is not in them.
func antiPatch(t *testing.T, raw, old, replacement []byte) []byte {
	t.Helper()
	at := bytes.Index(raw, old)
	if at < 0 || len(old) != len(replacement) {
		t.Fatalf("antiPatch: % x is not in the file, or is not as long as "+
			"% x", old, replacement)
	}
	found := slices.Clone(raw)
	copy(found[at:], replacement)
	return found
}

// antiSetAt is bytes with the bytes at a place replaced.
func antiSetAt(raw []byte, at int, replacement ...byte) []byte {
	found := slices.Clone(raw)
	copy(found[at:], replacement)
	return found
}

// antiMutation is a change to a file, named for what it changes.
type antiMutation struct {
	what   string
	change func(t *testing.T, raw []byte) []byte
}

// antiRun reads the file of each name in turn, and fails for each
// mutation that is not seen.
func antiRun(t *testing.T, names []string, mutations []antiMutation) {
	for _, name := range names {
		t.Run(name, func(t *testing.T) {
			raw := antiFixture(t, name)
			for _, each := range mutations {
				antiDiffers(t, each.what, raw, each.change(t, raw))
			}
		})
	}
}

// antiBE is a number in width bytes, big-endian.
func antiBE(width int, number uint64) []byte {
	found := make([]byte, 8)
	binary.BigEndian.PutUint64(found, number)
	return found[8-width:]
}

// antiSize is a size_t of a version: 32 bits, or 64 in CDF-5.
func antiSize(version int, number uint64) []byte {
	if version == cdfFive {
		return antiBE(8, number)
	}
	return antiBE(4, number)
}

// antiReads is the first error of reading every variable of bytes, or an
// error of opening them, or nothing.
func antiReads(raw []byte) error {
	file, err := antiOpen(raw)
	if err != nil {
		return err
	}
	defer file.Close()
	for _, each := range file.Variables() {
		if _, err := readHis(each, int64(len(raw))); err != nil {
			return err
		}
	}
	return nil
}

// antiAccepted fails the test unless bytes open and read whole.
func antiAccepted(t *testing.T, what string, raw []byte) {
	t.Helper()
	if err := antiReads(raw); err != nil {
		t.Errorf("%s: the nearest right input is refused: %v", what, err)
	}
}

// antiVersions are the three versions a fixture may be of.
var antiVersions = []int{cdfOne, cdfTwo, cdfFive}

// antiFile is a header and the data after it, as one file.
func antiFile(head header, data ...[]byte) []byte {
	return append(head.Bytes(), bytes.Join(data, nil)...)
}

// antiSaved writes a schema with his own package, lets the steps write,
// closes it, and gives the bytes of the file.
func antiSaved(t *testing.T, schema *classic.Schema,
	write func(*classic.Writer)) []byte {
	t.Helper()
	path := filepath.Join(t.TempDir(), "anti.nc")
	writer, err := classic.Create(path, schema)
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	if write != nil {
		write(writer)
	}
	if err := writer.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return raw
}

// antiWriter is the writer of one variable, which must be there.
func antiWriter(t *testing.T, writer *classic.Writer,
	name string) *classic.VarWriter {
	t.Helper()
	found, ok := writer.Variable(name)
	if !ok {
		t.Fatalf("no variable %q to write", name)
	}
	return found
}

// antiVariable is a variable of a file of his, which must be there.
func antiVariable(t *testing.T, file *classic.File,
	name string) *classic.Variable {
	t.Helper()
	found, ok := file.Variable(name)
	if !ok {
		t.Fatalf("no variable %q", name)
	}
	return found
}
