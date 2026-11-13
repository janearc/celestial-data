//ninja:vow dojo each row is held to its fixed columns through row and parse
package bsc5

import (
	"fmt"
	"math"
	"strings"
	"testing"

	"pgregory.net/rapid"
)

// fields is a row's numbers as drawn: whole ones, tenths of a second of
// right ascension, hundredths of a magnitude, and whether south.
type fields struct {
	hr, hd, raHour, raMinute, raTenths int
	decDegree, decMinute, decSecond    int
	vHundredths, bvHundredths          int
	south                              bool
}

// drawFields draws a row's numbers within the catalogue's ranges.
func drawFields(t *rapid.T) fields {
	return fields{
		hr:           rapid.IntRange(1, 9110).Draw(t, "hr"),
		hd:           rapid.IntRange(1, 999999).Draw(t, "hd"),
		raHour:       rapid.IntRange(0, 23).Draw(t, "raHour"),
		raMinute:     rapid.IntRange(0, 59).Draw(t, "raMinute"),
		raTenths:     rapid.IntRange(0, 599).Draw(t, "raTenths"),
		decDegree:    rapid.IntRange(0, 89).Draw(t, "decDegree"),
		decMinute:    rapid.IntRange(0, 59).Draw(t, "decMinute"),
		decSecond:    rapid.IntRange(0, 59).Draw(t, "decSecond"),
		vHundredths:  rapid.IntRange(-146, 999).Draw(t, "v"),
		bvHundredths: rapid.IntRange(-40, 200).Draw(t, "bv"),
		south:        rapid.Bool().Draw(t, "south"),
	}
}

// put writes text into a row's bytes in a span, to their right.
func put(line []byte, at span, text string) {
	width := at.last - at.first + 1
	copy(line[at.first-1:at.last], fmt.Sprintf("%*s", width, text))
}

// row is the fields written where the readme puts them, as the
// catalogue writes them.
func (drawn fields) row() string {
	line := []byte(strings.Repeat(" ", record))
	sign := northOfEquator
	if drawn.south {
		sign = southOfEquator
	}
	put(line, hrBytes, fmt.Sprint(drawn.hr))
	put(line, hdBytes, fmt.Sprint(drawn.hd))
	put(line, raHourBytes, fmt.Sprintf("%02d", drawn.raHour))
	put(line, raMinuteBytes, fmt.Sprintf("%02d", drawn.raMinute))
	put(line, raSecondBytes, fmt.Sprintf("%04.1f", float64(drawn.raTenths)/10))
	put(line, decSignBytes, sign)
	put(line, decDegreeBytes, fmt.Sprintf("%02d", drawn.decDegree))
	put(line, decMinuteBytes, fmt.Sprintf("%02d", drawn.decMinute))
	put(line, decSecondBytes, fmt.Sprintf("%02d", drawn.decSecond))
	put(line, visualBytes, fmt.Sprintf("%5.2f", float64(drawn.vHundredths)/100))
	put(line, bvBytes, fmt.Sprintf("%+5.2f", float64(drawn.bvHundredths)/100))
	return string(line)
}

// readsAsWritten is the property: a row drawn at random reads back as
// its fields, its place by the arithmetic of hours, degrees, minutes
// and seconds.
func readsAsWritten(t *rapid.T) {
	drawn := drawFields(t)
	star, err := parse(drawn.row())
	if err != nil {
		t.Fatalf("%q: %v", drawn.row(), err)
	}
	ra := 15 * (float64(drawn.raHour) + float64(drawn.raMinute)/60 +
		float64(drawn.raTenths)/10/3600)
	dec := float64(drawn.decDegree) + float64(drawn.decMinute)/60 +
		float64(drawn.decSecond)/3600
	if drawn.south {
		dec = -dec
	}
	hd, hasHD := star.HD()
	visual, hasVisual := star.Visual()
	bv, hasBV := star.BV()
	place := star.Place()
	switch {
	case star.HR() != drawn.hr || hd != drawn.hd || !hasHD:
		t.Fatalf("HR %d HD %d from %+v", star.HR(), hd, drawn)
	case place == nil ||
		math.Abs(place.Ascension().InDegrees()-ra) > 1e-9 ||
		math.Abs(place.Declination().InDegrees()-dec) > 1e-9:
		t.Fatalf("at %v, not %v %v, from %+v", place, ra, dec, drawn)
	case !hasVisual ||
		math.Abs(visual-float64(drawn.vHundredths)/100) > 1e-12 ||
		!hasBV || math.Abs(bv-float64(drawn.bvHundredths)/100) > 1e-12:
		t.Fatalf("V %v B-V %v from %+v", visual, bv, drawn)
	}
}

// the test: any row the catalogue could hold reads as the fields it was
// written from.
func TestEveryRowReadsAsItsFields(t *testing.T) {
	rapid.Check(t, readsAsWritten)
}
