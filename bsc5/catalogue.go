package bsc5

import (
	"bufio"
	"compress/gzip"
	"errors"
	"fmt"
	"io"
	"regexp"
	"strconv"
	"strings"

	"github.com/janearc/celestial-data/internal/rows"
	"github.com/janearc/celestial/sky"
)

// record is a row's length in bytes, the readme's Lrecl, as numbers.yaml
// keeps it. The file trims each line's trailing spaces, so a shorter
// line is padded back to it.
var record = int(measured().Value("record"))

// gzipMagic is the two bytes every gzip stream begins with (RFC 1952,
// 2.3.1), by which a gzipped file is told from a plain one.
const gzipMagic = "\x1f\x8b"

// span is a field's first and last byte, as the readme numbers them,
// from one.
type span struct{ first, last int }

// A row's fields, each by the bytes numbers.yaml keeps for it from the
// readme. A right ascension and a declination are each their parts'
// bytes together, from the first part's first to the last part's last.
var (
	hrBytes        = spanOf("hr")
	nameBytes      = spanOf("name")
	hdBytes        = spanOf("hd")
	raHourBytes    = spanOf("ra-hours")
	raMinuteBytes  = spanOf("ra-minutes")
	raSecondBytes  = spanOf("ra-seconds")
	raBytes        = span{raHourBytes.first, raSecondBytes.last}
	decSignBytes   = spanOf("dec-sign")
	decDegreeBytes = spanOf("dec-degrees")
	decMinuteBytes = spanOf("dec-minutes")
	decSecondBytes = spanOf("dec-seconds")
	decBytes       = span{decSignBytes.first, decSecondBytes.last}
	visualBytes    = spanOf("visual")
	bvBytes        = spanOf("b-v")
	spectralBytes  = spanOf("spectral")
)

// spanOf is a field's span: its first and last byte, as numbers.yaml
// keeps them under the field's name.
func spanOf(field string) span {
	kept := measured()
	return span{int(kept.Value(field + "-first")),
		int(kept.Value(field + "-last"))}
}

// What the reader counts by. All are defined.
const (
	// none is no number, and the least there can be of anything.
	// Defined.
	none = 0
	// fromOne is how far the readme's byte numbers, from one, are ahead
	// of a string's places, from nought. Defined.
	fromOne = 1
	// firstLine is a file's first line, counted from one as the errors
	// say it. Defined.
	firstLine = 1
	// bits64 is the width a decimal is read at: a float64. Defined by
	// strconv.
	bits64 = 64
)

// A declination's sign: south of the equator, or north of it.
const southOfEquator, northOfEquator = "-", "+"

// leastNumber is the least HR or HD number: both count from one. It's
// defined.
const leastNumber = 1

// errNoRows refuses a file with no rows: a download cut short, or
// something else than the catalogue.
var errNoRows = errors.New("bsc5: the file holds no rows: not the catalogue")

// A count is digits alone; a decimal is a sign perhaps, and digits with
// a point among them perhaps.
var (
	wholeDigits      = regexp.MustCompile(`^[0-9]+$`)
	decimalSeparated = regexp.MustCompile(
		`^[+-]?([0-9]+\.?[0-9]*|\.[0-9]+)$`)
)

// The arithmetic of a place: an hour of right ascension is fifteen
// degrees, and a degree or an hour is sixty minutes of three thousand
// six hundred seconds.
const (
	degreesAnHour = 15.0   // defined: a turn is 24 hours and 360 degrees
	minutesIn     = 60.0   // defined: the minutes in an hour or a degree
	secondsIn     = 3600.0 // defined: the seconds in an hour or a degree
)

// Read is every row of the catalogue in a file, gzipped as Harvard
// serves it or not: a row a line, read by the byte columns the readme
// gives. A row it cannot read is refused, naming its line, and so is a
// file with no rows at all, which is a download cut short.
func Read(file io.Reader) ([]Star, error) {
	source, err := opened(file)
	if err != nil {
		return nil, err
	}
	defer source.Close()
	lines := bufio.NewScanner(source)
	var stars []Star
	for line := firstLine; lines.Scan(); line++ {
		star, err := parse(lines.Text())
		if err != nil {
			return nil, fmt.Errorf("bsc5: line %d: %w", line, err)
		}
		stars = append(stars, *star)
	}
	if err := lines.Err(); err != nil {
		return nil, fmt.Errorf("bsc5: %w", err)
	}
	if len(stars) == none {
		return nil, errNoRows
	}
	return stars, nil
}

// opened is the file to read rows from: unzipped when it begins as
// gzip does, and as it is when it does not.
func opened(file io.Reader) (io.ReadCloser, error) {
	buffered := bufio.NewReader(file)
	head, err := buffered.Peek(len(gzipMagic))
	if err != nil || string(head) != gzipMagic {
		return io.NopCloser(buffered), nil
	}
	unzipped, err := gzip.NewReader(buffered)
	if err != nil {
		return nil, fmt.Errorf("bsc5: the gzip: %w", err)
	}
	return unzipped, nil
}

// row is one line of the catalogue, padded to a whole record.
type row string

// rowOf is a line as a whole record.
func rowOf(line string) row {
	short := max(record-len(line), none)
	return row(line + strings.Repeat(" ", short))
}

// field is the row's bytes in a span, without the spaces round them.
func (line row) field(bytes span) string {
	return strings.TrimSpace(string(line[bytes.first-fromOne : bytes.last]))
}

// parse is one row: its number, which every row has, and the rest where
// it is given. A row it cannot read is no star.
func parse(line string) (*Star, error) {
	read := rowOf(line)
	hr, err := whole(read.field(hrBytes))
	if err != nil {
		return nil, fmt.Errorf("HR %q is no number in the catalogue",
			read.field(hrBytes))
	}
	star := Star{hr: hr, name: read.field(nameBytes),
		spectral: read.field(spectralBytes)}
	if err := star.numbered(read); err != nil {
		return nil, fmt.Errorf("HR %d's %w", hr, err)
	}
	return &star, nil
}

// numbered reads the row's numbers into the star: its HD, its place and
// its magnitudes, each where the row gives it.
func (star *Star) numbered(read row) error {
	var err error
	if hd := read.field(hdBytes); hd != "" {
		if star.hd, err = whole(hd); err != nil {
			return fmt.Errorf("HD: %w", err)
		}
	}
	if star.place, err = placed(read); err != nil {
		return fmt.Errorf("place: %w", err)
	}
	star.visual, star.hasVisual, err = magnitude(read.field(visualBytes))
	if err != nil {
		return fmt.Errorf("V: %w", err)
	}
	star.bv, star.hasBV, err = magnitude(read.field(bvBytes))
	if err != nil {
		return fmt.Errorf("B-V: %w", err)
	}
	return nil
}

// whole is a count as the catalogue writes one, digits alone, from one
// up: HR and HD numbers.
func whole(text string) (int, error) {
	value, err := strconv.Atoi(text)
	if !wholeDigits.MatchString(text) || err != nil || value < leastNumber {
		return none, fmt.Errorf("%q is no number", text)
	}
	return value, nil
}

// decimal is a number as the catalogue writes one: a sign perhaps, and
// digits with a point among them perhaps. Not "NaN", "Inf", an exponent
// or hexadecimal, all of which strconv.ParseFloat takes.
func decimal(text string) (float64, error) {
	value, err := strconv.ParseFloat(text, bits64)
	if !decimalSeparated.MatchString(text) || err != nil {
		return none, fmt.Errorf("%q is no number", text)
	}
	return value, nil
}

// magnitude is a magnitude field: its number and true, or none and
// false where it is blank.
func magnitude(text string) (float64, bool, error) {
	if text == "" {
		return none, false, nil
	}
	value, err := decimal(text)
	return value, err == nil, err
}

// placed is the row's J2000 place in degrees, or none when the row
// leaves both its right ascension and its declination blank, as it does
// for novae and objects found not to be stars. One without the other is
// refused, naming which is missing.
func placed(read row) (*sky.Equatorial, error) {
	east, north := read.field(raBytes), read.field(decBytes)
	switch {
	case east == "" && north == "":
		return nil, nil
	case east == "":
		return nil, errors.New("a declination with no right ascension")
	case north == "":
		return nil, errors.New("a right ascension with no declination")
	}
	ascension, err := sexagesimal(read, raHourBytes, raMinuteBytes,
		raSecondBytes)
	if err != nil {
		return nil, err
	}
	declination, err := declinationOf(read)
	if err != nil {
		return nil, err
	}
	return rows.Place(degreesAnHour*ascension, declination)
}

// declinationOf is the row's declination in degrees, signed.
func declinationOf(read row) (float64, error) {
	degrees, err := sexagesimal(read, decDegreeBytes, decMinuteBytes,
		decSecondBytes)
	if err != nil {
		return none, err
	}
	switch sign := read.field(decSignBytes); sign {
	case southOfEquator:
		return -degrees, nil
	case northOfEquator:
		return degrees, nil
	default:
		return none, fmt.Errorf("the declination's sign %q is neither "+
			"+ nor -", sign)
	}
}

// sexagesimal is a whole and its sixtieths and its three thousand six
// hundredths, read from three spans of the row: hours or degrees,
// minutes and seconds.
func sexagesimal(read row, units, minutes, seconds span) (float64, error) {
	unit, err := decimal(read.field(units))
	if err != nil {
		return none, err
	}
	minute, err := decimal(read.field(minutes))
	if err != nil {
		return none, err
	}
	second, err := decimal(read.field(seconds))
	if err != nil {
		return none, err
	}
	return unit + minute/minutesIn + second/secondsIn, nil
}
