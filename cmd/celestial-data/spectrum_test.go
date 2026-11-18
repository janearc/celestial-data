package main

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/janearc/celestial-data"
	"github.com/janearc/celestial-data/mocks"
	"go.uber.org/mock/gomock"
)

// gaiaDR3Star is Gaia DR3 4081136684118965760's place and proper motion,
// read from the archive on 2026-11-06. Its ref_epoch, 2016.0 yr, was read
// from the Gaia product's rows on 2026-11-07.
var gaiaDR3Star = []string{"--ra", "287.50086365607245", "--dec",
	"-22.234814871161884", "--pmra", "6.824", "--pmdec", "-3.703",
	"--epoch", "2016.0"}

// cutouts is the folder of the recorded cutout of Gaia DR3's star above.
const cutouts = "../../spherex/spectrum/testdata"

// A dry spectrum says what it would measure and where the table would
// go. It reads no cutout.
func Example_spectrum() {
	fmt.Print(said(append([]string{"spectrum", "--from", "cutouts",
		"--dry-run"}, gaiaDR3Star...)...))
	// Output:
	// would measure the star at ra 287.50086365607245, dec -22.234814871161884 in each cutout in cutouts, into standard output
}

// spectrumOf runs celestial-data spectrum with the words given, and
// says what it wrote, what it said of errors and the code it ended
// with.
func spectrumOf(t *testing.T, args ...string) (string, string, int) {
	t.Helper()
	var out, errs bytes.Buffer
	get := mocks.NewMockWeb(gomock.NewController(t))
	tool := command{get: get, picked: celestialdata.NewMeasured(
		numbersFile, numbersRaw), sets: standardSets()}
	code := tool.run(append([]string{"spectrum"}, args...),
		strings.NewReader(""), &out, &errs)
	return out.String(), errs.String(), code
}

// the test: the recorded cutout is measured to standard output: the
// columns after a #, then its one line, ending in its file's name.
func TestSpectrumMeasuresTheFolder(t *testing.T) {
	out, errs, code := spectrumOf(t, append([]string{"--from", cutouts},
		gaiaDR3Star...)...)
	lines := strings.Split(strings.TrimSpace(out), "\n")
	if code != exitOK || len(lines) != 2 ||
		!strings.HasPrefix(lines[0], "# wavelength_um\t") ||
		!strings.HasSuffix(lines[1], "noahs-star-qr3-D1-0.966um.fits") {
		t.Fatalf("code %d, said:\n%s\nerrors:\n%s", code, out, errs)
	}
}

// the test: --into is a new file the table is written into, and the
// command says how many it measured; a second run into the same file
// is refused, the file as the first left it.
func TestSpectrumWritesIntoANewFileAndNoOther(t *testing.T) {
	into := filepath.Join(t.TempDir(), "spectrum.tsv")
	args := append([]string{"--from", cutouts, "--into", into}, gaiaDR3Star...)
	out, _, code := spectrumOf(t, args...)
	first, err := os.ReadFile(into)
	if err != nil || code != exitOK ||
		!strings.Contains(out, "measured 1 cutouts into") ||
		!strings.HasPrefix(string(first), "# wavelength_um") {
		t.Fatalf("code %d, said %q, wrote %q, %v", code, out, first, err)
	}
	_, errs, code := spectrumOf(t, args...)
	again, _ := os.ReadFile(into)
	if code == exitOK || !strings.Contains(errs, "a new file") ||
		!bytes.Equal(again, first) {
		t.Fatalf("a second run into %s: code %d, %s", into, code, errs)
	}
}

// the test: a cutout the folder holds that cannot be measured is a line
// of its own at the table's end, after a #, and the rest are measured.
func TestSpectrumSaysWhatItRefused(t *testing.T) {
	folder := t.TempDir()
	recorded, err := os.ReadFile(filepath.Join(cutouts,
		"noahs-star-qr3-D1-0.966um.fits"))
	if err != nil {
		t.Fatal(err)
	}
	for name, body := range map[string][]byte{"a.fits": recorded,
		"b.fits": []byte("no FITS")} {
		if err := os.WriteFile(filepath.Join(folder, name), body,
			tableMode); err != nil {
			t.Fatal(err)
		}
	}
	out, _, code := spectrumOf(t, append([]string{"--from", folder},
		gaiaDR3Star...)...)
	lines := strings.Split(strings.TrimSpace(out), "\n")
	last := lines[len(lines)-1]
	if code != exitOK || len(lines) != 3 ||
		!strings.HasPrefix(last, "# refused: b.fits") {
		t.Fatalf("code %d, said:\n%s", code, out)
	}
}

// the anti-test: spectrum is refused with no folder, a place that is
// not numbers, a proper motion without its epoch or not a number, and a
// folder that is not there; nothing is written.
func TestSpectrumRefusesWhatItCannotMeasure(t *testing.T) {
	place := []string{"--ra", "287.5", "--dec", "-22.2"}
	cases := []struct {
		args []string
		says string
	}{
		{place, "needs --from"},
		{[]string{"--from", cutouts, "--ra", "east", "--dec", "-22"},
			"numbers of degrees"},
		{append([]string{"--from", cutouts, "--pmra", "6.8"}, place...),
			"needs --epoch"},
		{append([]string{"--from", cutouts, "--pmdec", "fast",
			"--epoch", "2016.0"}, place...), "--pmdec is a number"},
		{append([]string{"--from", filepath.Join(t.TempDir(), "none")},
			place...), "no such file"},
	}
	for _, each := range cases {
		out, errs, code := spectrumOf(t, each.args...)
		if code == exitOK || !strings.Contains(errs, each.says) ||
			out != "" {
			t.Errorf("%v: code %d, said %q, errors %q", each.args, code,
				out, errs)
		}
	}
}
