package arxiv_test

import (
	"errors"
	"fmt"
	"strings"
	"testing"

	"github.com/janearc/celestial-data"
	"github.com/janearc/celestial-data/arxiv"
	"pgregory.net/rapid"
)

// failing is a test or a property's draw, which can stop and say why.
type failing interface {
	Helper()
	Fatalf(format string, args ...any)
}

// parsed is an id as these tests read it, failing the test when it
// will not read.
func parsed(test failing, written string) arxiv.ID {
	test.Helper()
	id, err := arxiv.ParseID(written)
	if err != nil {
		test.Fatalf("%q: %v", written, err)
	}
	return *id
}

// An id is read as people and papers write it: the name alone, after
// arXiv:, or as an abs or pdf address, each with a version or without,
// and is written back as arXiv writes it.
func TestAnIDIsReadAsWritten(t *testing.T) {
	for written, want := range map[string]string{
		"1809.03507":                          "1809.03507",
		"1809.03507v2":                        "1809.03507v2",
		"0704.0001":                           "0704.0001",
		"astro-ph/0602086v1":                  "astro-ph/0602086v1",
		"math.GT/0309136":                     "math/0309136",
		"solv-int/9901001v12":                 "solv-int/9901001v12",
		"arXiv:1809.03507":                    "1809.03507",
		"ARXIV:1412.3451v3":                   "1412.3451v3",
		"  1809.03507  ":                      "1809.03507",
		"https://arxiv.org/abs/1809.03507v2":  "1809.03507v2",
		"https://www.arxiv.org/pdf/1412.3451": "1412.3451",
		"http://export.arxiv.org/pdf/astro-ph/0602086v1.pdf": "astro-ph/" +
			"0602086v1",
	} {
		if got := parsed(t, written).String(); got != want {
			t.Errorf("%q is read as %q, want %q", written, got, want)
		}
	}
}

// Anti-test: what is not an id is refused, saying what was written,
// with no id beside the refusal, before anything is asked of arXiv.
func TestWhatIsNotAnIDIsRefused(t *testing.T) {
	for _, written := range []string{"", "1809.035", "1809.035071",
		"180.03507", "18090.3507", "astro-ph/060208", "astro-ph/06020861",
		"Astro-ph/0602086", "-ph/0602086", "astro-/0602086",
		"astro-ph//0602086", "1809.03507v0", "1809.03507v01",
		"1809.03507v", "1809.03507 v2", "arXiv:", "https://example.org/" +
			"abs/1809.03507", "https://arxiv.org/list/astro-ph",
		"http://[::1", "https://arxiv.org/abs/not-an-id"} {
		id, err := arxiv.ParseID(written)
		quoted := fmt.Sprintf("%q", written)
		if id != nil || !errors.Is(err, celestialdata.Refused()) ||
			!strings.Contains(err.Error(), quoted) {
			t.Errorf("%q: %v, %v", written, id, err)
		}
	}
}

// Anti-test: an address refused says what is wrong with it: not on
// arxiv.org, or not a paper's page there.
func TestAnAddressRefusedSaysWhy(t *testing.T) {
	for written, says := range map[string]string{
		"https://example.org/abs/1809.03507": "is not an address on " +
			"arxiv.org",
		"http://[::1": "is not an address on arxiv.org",
		"https://arxiv.org/list/astro-ph": "is not a paper's abs or pdf " +
			"page",
	} {
		_, err := arxiv.ParseID(written)
		if err == nil || !strings.Contains(err.Error(), says) {
			t.Errorf("%q: %v", written, err)
		}
	}
}

// heldFile is an id and a file it is to hold, or not.
type heldFile struct {
	id   arxiv.ID
	file string
}

// An id without a version holds the file of any version of its paper;
// an id with one holds only that version's.
func TestAnIDHoldsItsPapersFiles(t *testing.T) {
	every, second := parsed(t, "1809.03507"), parsed(t, "1809.03507v2")
	old := parsed(t, "astro-ph/0602086")
	for _, held := range []heldFile{{every, "1809.03507v1.pdf"},
		{every, "1809.03507v12.pdf"}, {second, "1809.03507v2.pdf"},
		{old, "astro-ph_0602086v3.pdf"}} {
		if !held.id.Holds(held.file) {
			t.Errorf("%s does not hold %s", held.id, held.file)
		}
	}
}

// Anti-test: an id holds no other paper's file, no version that is not
// one, nothing that is not a PDF, and, with a version, no other
// version's.
func TestAnIDHoldsNoOtherFile(t *testing.T) {
	every, second := parsed(t, "1809.03507"), parsed(t, "1809.03507v2")
	old := parsed(t, "astro-ph/0602086")
	for _, unheld := range []heldFile{{every, "1809.035071v1.pdf"},
		{every, "x1809.03507v1.pdf"}, {every, "1809.03507v0.pdf"},
		{every, "1809.03507v.pdf"}, {every, "1809.03507v2.txt"},
		{every, "1809.03507v2"}, {every, "1809.03507v1.pdf.part"},
		{every, "2.pdf"}, {second, "1809.03507v1.pdf"},
		{second, "1809.03507v20.pdf"}, {old, "astro-ph/0602086v1.pdf"}} {
		if unheld.id.Holds(unheld.file) {
			t.Errorf("%s holds %s", unheld.id, unheld.file)
		}
	}
}

// The parts of an id as arXiv writes one.
const (
	yearAndMonth = 4 // defined by arXiv's ids: two of year, two of month
	fewestDigits = 4 // defined by arXiv's ids: before 2015
	mostDigits   = 5 // defined by arXiv's ids: from 2015 on
	oldDigits    = 7 // defined by arXiv's old ids: year, month, number
)

// The versions an id is drawn with.
const (
	noVersion   = 0  // defined: an id that names no version
	mostVersion = 99 // defined: past what arXiv's papers reach
)

// archives are old-style archive names, as arXiv used them.
var archives = []string{"astro-ph", "hep-th", "cond-mat", "math",
	"solv-int", "gr-qc", "physics"}

// digits is a run of so many digits, drawn.
func digits(draw *rapid.T, count int, label string) string {
	pattern := fmt.Sprintf("[0-9]{%d}", count)
	return rapid.StringMatching(pattern).Draw(draw, label)
}

// drawnName is a paper's name, new-style or old-style, drawn: as
// written, and as arXiv names it, without an old-style subject.
func drawnName(draw *rapid.T) (written, named string) {
	if rapid.Bool().Draw(draw, "old") {
		archive := rapid.SampledFrom(archives).Draw(draw, "archive")
		subject := rapid.SampledFrom([]string{"", ".GT", ".AI"}).
			Draw(draw, "subject")
		number := digits(draw, oldDigits, "yymmnnn")
		return archive + subject + "/" + number, archive + "/" + number
	}
	width := rapid.IntRange(fewestDigits, mostDigits).Draw(draw, "width")
	name := digits(draw, yearAndMonth, "yymm") + "." + digits(draw, width,
		"number")
	return name, name
}

// drawn is an id as written, and as arXiv names it, without an
// old-style subject, and its version.
type drawn struct {
	written, named string
	version        int
}

// drawnID is an id drawn as people write one, with a version or
// without.
func drawnID(draw *rapid.T) drawn {
	written, named := drawnName(draw)
	version := rapid.IntRange(noVersion, mostVersion).Draw(draw, "v")
	if version == noVersion {
		return drawn{written, named, version}
	}
	suffix := fmt.Sprintf("v%d", version)
	return drawn{written + suffix, named + suffix, version}
}

// readsBack is the property that an id reads back as arXiv names it,
// and that one with a version holds its own file: that name with the
// slash made an underscore, and .pdf.
func readsBack(draw *rapid.T) {
	id := drawnID(draw)
	read := parsed(draw, id.written)
	if read.String() != id.named {
		draw.Fatalf("%q reads back as %q, not %q", id.written, read,
			id.named)
	}
	file := strings.ReplaceAll(id.named, "/", "_") + ".pdf"
	if id.version != noVersion && !read.Holds(file) {
		draw.Fatalf("%q does not hold %q", id.written, file)
	}
}

// The property: an id reads back as arXiv names it, and a version holds
// its own file.
func TestAnIDReadsBackAsItIsWritten(t *testing.T) {
	rapid.Check(t, readsBack)
}

// marks are what lists of ids are written with between them.
var marks = []string{" ", ",", ";"}

// oneIn is how far in from either end of an id a mark is put. It's
// defined: inside the id, where trimming does not take it away.
const oneIn = 1

// refusesASeparator is the property that an id with a space, a comma or
// a semicolon put into it is refused.
func refusesASeparator(draw *rapid.T) {
	written := drawnID(draw).written
	at := rapid.IntRange(oneIn, len(written)-oneIn).Draw(draw, "at")
	mark := rapid.SampledFrom(marks).Draw(draw, "mark")
	joined := written[:at] + mark + written[at:]
	if _, err := arxiv.ParseID(joined); err == nil {
		draw.Fatalf("%q is read as an id", joined)
	}
}

// Anti-test, the property: an id with a space, a comma or a semicolon in
// it is refused, so no list of ids is ever taken for one.
func TestAnIDWithASeparatorInItIsRefused(t *testing.T) {
	rapid.Check(t, refusesASeparator)
}
