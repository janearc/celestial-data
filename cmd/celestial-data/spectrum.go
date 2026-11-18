package main

import (
	"cmp"
	"errors"
	"fmt"
	"io"
	"os"
	"strconv"

	"github.com/janearc/celestial-data"
	"github.com/janearc/celestial-data/spherex/spectrum"
	"github.com/janearc/puzzlemouth/pzm"
	"github.com/janearc/puzzlemouth/verb"
)

// measuring is the spectrum verb's facts: it reads a folder of cutouts,
// writes the table into the file --into names or else to standard
// output, replaces no file, and asks no one.
var measuring = verb.NewFacts(verb.WritesTo("the file --into names, or "+
	"standard output"), verb.LeavesAlone, verb.ThisMachine, verb.NoOne,
	verb.Light)

const spectrumPage = "Measures a star's light in each SPHEREx cutout in " +
	"the folder --from names, as fetch --archive spherex brought them: " +
	"forced photometry at --ra and --dec, the star moved to each image's " +
	"time by --pmra and --pmdec, in milliarcseconds a year from --epoch, " +
	"a Julian year, as Gaia gives them. Each line is one image's " +
	"wavelength, its flux in micro-Jy with the 3% systematic in its " +
	"error, and how the fit went. A cutout it cannot measure, a QR2 one " +
	"among them, is a line of its own at the table's end, after a #. It " +
	"reads files only and asks no one; it never replaces a file."

// tableMode is the mode of the file the table is written into. It's
// defined: the owner writes, everyone reads.
const tableMode = 0o644

// spectrumFlags are the spectrum verb's flags: where the cutouts are,
// where the star is and how it moves, and where the table goes.
func spectrumFlags() []pzm.Flag {
	return []pzm.Flag{
		pzm.Value("from", verb.NoShort, "the folder of SPHEREx "+
			"cutouts"),
		pzm.Value("ra", verb.NoShort, "the star's right ascension, in "+
			"degrees"),
		pzm.Value("dec", verb.NoShort, "the star's declination, in "+
			"degrees"),
		pzm.Value("pmra", verb.NoShort, "its proper motion in right "+
			"ascension, along the sky, in mas a year"),
		pzm.Value("pmdec", verb.NoShort, "its proper motion in "+
			"declination, in mas a year"),
		pzm.Value("epoch", verb.NoShort, "the Julian year its place "+
			"is at, as Gaia's ref_epoch"),
		pzm.Value("into", verb.NoShort, "a new file for the table; "+
			"standard output when none"),
	}
}

// spectrumVerb measures the star in each cutout of the folder and writes
// the table where asked.
func (tool command) spectrumVerb(call pzm.Call) error {
	star, err := starOf(call)
	if err != nil {
		return err
	}
	from := call.Value("from")
	if from == "" {
		return celestialdata.Refuse("spectrum needs --from, the " +
			"folder of cutouts")
	}
	into := call.Value("into")
	if call.DryRun() {
		fmt.Fprintf(call.Out(), "would measure the star at ra %g, "+
			"dec %g in each cutout in %s, into %s\n", star.RA,
			star.Dec, from, cmp.Or(into, "standard output"))
		return nil
	}
	measure, err := spectrum.New()
	if err != nil {
		return err
	}
	points, refused, err := measure.Folder(from, star)
	if err != nil {
		return err
	}
	return writeTable(call.Out(), into, points, refused)
}

// starOf is the star a call names: --ra and --dec in degrees, and its
// proper motion and epoch when it gives one. A proper motion needs the
// epoch it counts from.
func starOf(call pzm.Call) (spectrum.Star, error) {
	var star spectrum.Star
	ra, raErr := strconv.ParseFloat(call.Value("ra"), bits)
	dec, decErr := strconv.ParseFloat(call.Value("dec"), bits)
	if err := errors.Join(raErr, decErr); err != nil {
		return star, fmt.Errorf("--ra and --dec are numbers of "+
			"degrees: %w", err)
	}
	star.RA, star.Dec = ra, dec
	if !call.Given("pmra") && !call.Given("pmdec") {
		return star, nil
	}
	if !call.Given("epoch") {
		return star, celestialdata.Refuse("a proper motion needs " +
			"--epoch, the Julian year Gaia's place is at, its " +
			"ref_epoch")
	}
	for name, into := range map[string]*float64{"pmra": &star.PMRA,
		"pmdec": &star.PMDec, "epoch": &star.Epoch} {
		if !call.Given(name) {
			continue
		}
		value, err := strconv.ParseFloat(call.Value(name), bits)
		if err != nil {
			return star, fmt.Errorf("--%s is a number: %w", name,
				err)
		}
		*into = value
	}
	return star, nil
}

// writeTable writes the points, then a line for each cutout refused,
// into the file named, or to out when none is. It refuses a file
// already there.
func writeTable(out io.Writer, into string, points []spectrum.Point,
	refused []error) error {
	if into == "" {
		return tabled(out, points, refused)
	}
	file, err := os.OpenFile(into, os.O_WRONLY|os.O_CREATE|os.O_EXCL,
		tableMode)
	if err != nil {
		return fmt.Errorf("the table is written into a new file: "+
			"%w", err)
	}
	written := tabled(file, points, refused)
	if err := errors.Join(written, file.Close()); err != nil {
		return err
	}
	fmt.Fprintf(out, "measured %d cutouts into %s; refused %d\n",
		len(points), into, len(refused))
	return nil
}

// tabled is the table and its refusals, written to out.
func tabled(out io.Writer, points []spectrum.Point,
	refused []error) error {
	if err := spectrum.Write(out, points); err != nil {
		return err
	}
	for _, why := range refused {
		_, err := fmt.Fprintf(out, "# refused: %v\n", why)
		if err != nil {
			return err
		}
	}
	return nil
}
