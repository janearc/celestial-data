//ninja:vow dojo a curve's shape and a source's place, through curveShape
package dasch

import (
	"encoding/json/v2"
	"math"
	"os"
	"strings"
	"testing"

	"github.com/janearc/celestial-data"
)

// These tests hold the adapter to review 018's findings on what it
// takes from an answer: a light curve must be one, and a catalogue's
// places must be on the sky and its sources listed once.

// the test: how far apart two places are is what the spherical law of
// cosines says, worked here on its own, at declinations where a slip
// in turning degrees to radians would not cancel out: on the equator,
// in the north and the south, across a pole, and half a turn.
func TestHowFarApartIsTheLawOfCosines(t *testing.T) {
	cosines := func(ra, dec, otherRA, otherDec float64) float64 {
		rad := math.Pi / 180
		return math.Acos(math.Sin(dec*rad)*math.Sin(otherDec*rad)+
			math.Cos(dec*rad)*math.Cos(otherDec*rad)*
				math.Cos((otherRA-ra)*rad)) / rad
	}
	for _, each := range [][4]float64{
		{0, 0, 0, 1}, {0, 60, 90, 60}, {287.5, -22.2, 290, -25},
		{10, 80, 190, 80}, {0, 0, 180, 0}, {123.4, 45.6, 321.0, -12.3},
	} {
		got := apart(each[0], each[1], each[2], each[3])
		if want := cosines(each[0], each[1], each[2], each[3]); math.Abs(
			got-want) > 1e-9 {
			t.Errorf("%v: %.12f degrees apart, not %.12f", each, got, want)
		}
	}
}

// the test: a source exactly on the circle's edge is in the circle.
func TestASourceOnTheEdgeIsInTheCircle(t *testing.T) {
	names, _ := columns()
	centre, err := celestialdata.NewPosition(287.5, -22.2)
	if err != nil {
		t.Fatal(err)
	}
	edge, err := celestialdata.NewNear(centre, apart(287.5, -22.2,
		287.501, -22.2))
	if err != nil {
		t.Fatal(err)
	}
	found, err := within([]string{strings.Join(names, ","),
		"1,2,287.501,-22.2"}, edge)
	if err != nil || len(found) != 1 {
		t.Errorf("a source on the edge: %v, %v", found, err)
	}
}

// recorded is the light curve fixture's lines.
func recorded(t *testing.T) []string {
	t.Helper()
	raw, err := os.ReadFile("testdata/lightcurve.json")
	if err != nil {
		t.Fatal(err)
	}
	var listed []string
	if err := json.Unmarshal(raw, &listed); err != nil {
		t.Fatal(err)
	}
	return listed
}

// the test: the fixture's light curve has the shape of one.
func TestALightCurveHasItsShape(t *testing.T) {
	if err := curveShape(recorded(t)); err != nil {
		t.Error(err)
	}
}

// the anti-test: a 200 that holds an error, and a light curve with a
// line short of a field, are refused.
func TestWhatIsNotALightCurveIsRefused(t *testing.T) {
	short := recorded(t)
	last := len(short) - 1
	short[last] = short[last][:strings.LastIndex(short[last], ",")]
	for name, listed := range map[string][]string{
		"an error":     {"no such source"},
		"a short line": short,
	} {
		err := curveShape(listed)
		if err == nil || !strings.Contains(err.Error(),
			"not a light curve") {
			t.Errorf("%s: %v", name, err)
		}
	}
}

// columns is a catalogue's first line, and where each column is.
func columns() ([]string, map[string]int) {
	names := []string{columnNumber, columnBin, columnRA, columnDec}
	at := map[string]int{}
	for place, name := range names {
		at[name] = place
	}
	return names, at
}

// the test: a source on the sky is read.
func TestASourceOnTheSkyIsRead(t *testing.T) {
	names, at := columns()
	each, err := read([]string{"1", "2", "287.5", "-22.2"}, at, len(names))
	if err != nil || each.ra != 287.5 || each.dec != -22.2 {
		t.Errorf("%+v, %v", each, err)
	}
}

// the anti-test: a place that is not on the sky is an error, not a
// source left out as too far. Counterexamples from review 018.
func TestASourceOffTheSkyIsAnError(t *testing.T) {
	names, at := columns()
	for _, place := range [][2]string{{"NaN", "0"}, {"0", "NaN"},
		{"+Inf", "0"}, {"450", "0"}, {"0", "91"}, {"-1", "0"}} {
		_, err := read([]string{"1", "2", place[0], place[1]}, at,
			len(names))
		if err == nil {
			t.Errorf("%v was read as a place", place)
		}
	}
}

// the anti-test: a source listed twice is an error, not two products of
// one name.
func TestASourceListedTwiceIsAnError(t *testing.T) {
	centre, err := celestialdata.NewPosition(287.5, -22.2)
	if err != nil {
		t.Fatal(err)
	}
	near, err := celestialdata.NewNear(centre, 0.01)
	if err != nil {
		t.Fatal(err)
	}
	header := strings.Join([]string{columnNumber, columnBin, columnRA,
		columnDec}, ",")
	_, err = within([]string{header, "1,2,287.5,-22.2",
		"1,2,287.5,-22.2"}, near)
	if err == nil || !strings.Contains(err.Error(), "listed twice") {
		t.Errorf("%v", err)
	}
}

// the test: a catalogue's columns and a light curve's first are read in
// camel case, as DASCH's web APIs page says the raw API writes them, as
// well as in the snake case its kept answers were in. Counterexamples
// from review 018, round 3.
func TestColumnsAreReadInEitherSpelling(t *testing.T) {
	centre, err := celestialdata.NewPosition(287.5, -22.2)
	if err != nil {
		t.Fatal(err)
	}
	near, err := celestialdata.NewNear(centre, 0.01)
	if err != nil {
		t.Fatal(err)
	}
	found, err := within([]string{"refNumber,gscBinIndex,raDeg,decDeg",
		"1,2,287.5,-22.2"}, near)
	if err != nil || len(found) != 1 || found[first].number != 1 {
		t.Errorf("camel case: %v, %v", found, err)
	}
	curve := recorded(t)
	curve[first] = strings.Replace(curve[first], "date_jd", "dateJd", 1)
	if err := curveShape(curve); err != nil {
		t.Errorf("a light curve in camel case: %v", err)
	}
}

// the test: a run of capitals is one word, as a small letter or a digit
// before a capital begins one. A note from review 018, round 4.
func TestARunOfCapitalsIsOneWord(t *testing.T) {
	for name, want := range map[string]string{
		"dateJD": "date_jd", "dateJd": "date_jd", "date_jd": "date_jd",
		"refNumber": "ref_number", "gscBinIndex": "gsc_bin_index",
		"mag2Flags": "mag2_flags",
	} {
		if got := snake(name); got != want {
			t.Errorf("%s is %s, not %s", name, got, want)
		}
	}
}

// the anti-test: a first line that names one column in both spellings
// is refused, not read by whichever comes last; a light curve's too.
// A counterexample from review 018, round 4.
func TestAColumnNamedBothWaysIsRefused(t *testing.T) {
	_, err := columnsAt([]string{"ref_number", "refNumber"})
	if err == nil || !strings.Contains(err.Error(), "twice") {
		t.Errorf("%v", err)
	}
	err = curveShape([]string{"date_jd,dateJd", "1,2"})
	if err == nil || !strings.Contains(err.Error(), "twice") {
		t.Errorf("a light curve: %v", err)
	}
}
