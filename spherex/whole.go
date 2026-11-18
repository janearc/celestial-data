package spherex

import (
	"bytes"
	"errors"
	"fmt"
	"os"
	"strconv"
	"strings"
)

// What a FITS file is made of.
const (
	// blockSize is the length of a FITS block, in bytes. It's defined
	// by the FITS standard, section 3.1.
	blockSize = 2880
	// cardSize is the length of a header card. It's defined by the FITS
	// standard, section 4.1.
	cardSize = 80
	// keyEnd is where a card's keyword ends. It's defined by section
	// 4.1.2.1.
	keyEnd = 8
	// valueAt is where a card's value begins, after "= ". It's defined
	// by section 4.1.2.2.
	valueAt = 10
	// equals is what follows a keyword that has a value.
	equals = "= "
	// endCard is the keyword of the card that ends a header.
	endCard = "END"
	// comment begins the comment after a value.
	comment = "/"
	// primary begins the first header of a FITS file. It's defined by
	// the standard, section 4.4.1.1.
	primary = "SIMPLE  ="
	// extension begins every header after the first. It's defined by
	// section 7.1.1.
	extension = "XTENSION="
	// byteBits is the bits in a byte. It's defined.
	byteBits = 8
	// oneGroup is the GCOUNT of every extension the standard defines,
	// and its default. It's defined by section 7.1.1.
	oneGroup = 1
	// roundUp is what, added to a length, makes dividing by a block
	// round up. It's defined: a block less one byte.
	roundUp = blockSize - 1
	// nothing is a length of none. It's defined.
	nothing = 0
	// first is the offset, and the number, of the first unit. It's
	// defined.
	first = 0
)

// sizes are the values BITPIX may have, each the bits of a pixel: the
// bytes, the integers and the floats. They are defined by the standard,
// section 4.4.1.1.
var sizes = map[int]bool{8: true, 16: true, 32: true, 64: true,
	-32: true, -64: true}

// The extensions of a SPHEREx cutout, by EXTNAME, as IRSA serves them:
// read from the 340 cutouts of both releases fetched on 2026-11-06.
const (
	// planeImage is the image itself.
	planeImage = "IMAGE"
	// planeFlags, planeVariance and planeZodi are the planes beside it,
	// of its size.
	planeFlags    = "FLAGS"
	planeVariance = "VARIANCE"
	planeZodi     = "ZODI"
	// psfQR2 is QR2's PSF, an image of three axes; psfQR3 is QR3's, a
	// table.
	psfQR2 = "PSF"
	psfQR3 = "EPSF"
	// wavelengths is the wavelength table, last.
	wavelengths = "WCS-WAVE"
	// imageKind and tableKind are the extension type of an image and of a
	// binary table. They're defined by the FITS standard, section 7.
	imageKind = "IMAGE"
	tableKind = "BINTABLE"
	// planeAxes is the axes of the image and each plane beside it. It's
	// defined: an image is two axes.
	planeAxes = "2"
	// either is a PSF of either name, for a release not yet seen. It's
	// defined.
	either = ""
)

// psfOf is the name of a release's PSF extension: PSF in QR2, EPSF in
// QR3, and either in a release not yet seen.
func psfOf(release string) string {
	switch release {
	case "qr2":
		return psfQR2
	case "qr3":
		return psfQR3
	}
	return either
}

// wholeCutout is the check that a fetched file is a whole SPHEREx
// cutout: FITS all the way through, and its units the cutout's, each in
// its place. It holds the release's PSF name and nothing else, so
// products stay comparable.
type wholeCutout struct{ psf string }

// Check is nil for a whole cutout, or says why the file is not one. The
// file is read whole into memory: a cutout is a few MB at most. An
// archive that fetched whole images would read it in blocks instead.
func (want wholeCutout) Check(path string) error {
	content, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	found, err := unitsOf(content)
	if err == nil {
		err = want.shaped(found)
	}
	if err != nil {
		return fmt.Errorf("%s: %w", path, err)
	}
	return nil
}

// shaped is nil when a whole FITS file's units are a cutout's: the primary,
// holding no array; the image, of sides above none, with its flags,
// variance and zodiacal light as images of its two axes and size; the
// release's PSF, of its name's kind; the wavelength table; nothing more.
//
// A cutout cut short where a unit ends is whole FITS, and is refused
// here.
func (want wholeCutout) shaped(found []map[string]string) error {
	psf := want.psf
	names := []string{planeImage, planeFlags, planeVariance, planeZodi,
		psf, wavelengths}
	if len(found) != len(names)+once {
		return fmt.Errorf("%d units, not a cutout's %d", len(found),
			len(names)+once)
	}
	for at, name := range names {
		got := unquoted(found[at+once]["EXTNAME"])
		if name == either {
			if got != psfQR2 && got != psfQR3 {
				return fmt.Errorf("unit %d is %q, not %q or %q",
					at+once, got, psfQR2, psfQR3)
			}
			continue
		}
		if got != name {
			return fmt.Errorf("unit %d is %q, not %q", at+once, got,
				name)
		}
	}
	if found[first]["NAXIS"] != noAxes {
		return errors.New("the primary holds an array; a cutout's " +
			"holds none")
	}
	image := found[once]
	if !positive(image["NAXIS1"]) || !positive(image["NAXIS2"]) {
		return fmt.Errorf("%s is empty", planeImage)
	}
	for at, name := range names[:len(names)-twoMore] {
		unit := found[at+once]
		if unquoted(unit["XTENSION"]) != imageKind ||
			unit["NAXIS"] != planeAxes ||
			unit["NAXIS1"] != image["NAXIS1"] ||
			unit["NAXIS2"] != image["NAXIS2"] {
			return fmt.Errorf("%s is not an image the size of %s",
				name, planeImage)
		}
	}
	if err := psfShaped(found[len(found)-twoMore]); err != nil {
		return err
	}
	if unquoted(found[len(found)-once]["XTENSION"]) != tableKind {
		return fmt.Errorf("%s is not a table", wavelengths)
	}
	return nil
}

// psfShaped is nil when a PSF unit is of its name's kind: PSF, QR2's,
// an image of three axes; EPSF, QR3's, a table.
func psfShaped(unit map[string]string) error {
	kind := unquoted(unit["XTENSION"])
	switch name := unquoted(unit["EXTNAME"]); name {
	case psfQR2:
		if kind != imageKind || unit["NAXIS"] != psfAxes {
			return fmt.Errorf("%s is not an image of %s axes", name,
				psfAxes)
		}
	case psfQR3:
		if kind != tableKind {
			return fmt.Errorf("%s is not a table", name)
		}
	}
	return nil
}

// positive is whether a card's value is a whole number above none.
func positive(value string) bool {
	number, err := strconv.Atoi(value)
	return err == nil && number > nothing
}

// More counts the cutout check uses, each defined.
const (
	// once is one unit: the primary, before the extensions. Defined.
	once = 1
	// twoMore is the units after the four planes: the PSF and the
	// wavelengths. Defined.
	twoMore = 2
	// noAxes is a cutout's primary's axis count: it holds no array.
	noAxes = "0"
	// psfAxes is QR2's PSF's axis count: a stack of images, three axes.
	psfAxes = "3"
)

// unquoted is a card's string value without its quotes and padding.
func unquoted(value string) string {
	return strings.TrimSpace(strings.Trim(strings.TrimSpace(value), "'"))
}

// wholeBytes is nil when a file's bytes are a FITS file all the way
// through. See unitsOf.
func wholeBytes(content []byte) error {
	_, err := unitsOf(content)
	return err
}

// unitsOf is the cards of each unit of a file that is FITS all the way
// through: it begins with the primary header, each header ends in its
// END card, and each unit's data, to the block, ends where the next
// header begins, with the last unit's ending where the file does.
//
// A file cut short, a page or any other answer that is not FITS, and a
// file with bytes past its end are all refused.
//
// A unit is at least a block, so the walk always moves on.
func unitsOf(content []byte) ([]map[string]string, error) {
	var found []map[string]string
	at := first
	for unit := first; unit == first || at < len(content); unit++ {
		cards, next, err := unitEnd(content, at, unit)
		if err != nil {
			return nil, err
		}
		found = append(found, cards)
		at = max(next, at+blockSize)
	}
	return found, nil
}

// blocks is a length in bytes made a whole number of blocks.
func blocks(length int) int {
	return (length + roundUp) / blockSize * blockSize
}

// unitEnd is the cards of the unit that begins at a byte, and the byte
// where it ends: its header and its data, each padded to the block. The
// data may not reach past the end of the file.
//
// When there is an error the unit ends at the end of the file, so that
// a loop that went on past it would stop.
func unitEnd(content []byte, at, unit int) (map[string]string, int,
	error) {
	cards, header, err := read(content, at, unit)
	if err != nil {
		return nil, len(content), err
	}
	data, err := dataOf(cards, len(content))
	if err != nil {
		return nil, len(content), fmt.Errorf("unit %d: %w", unit, err)
	}
	next := at + header + blocks(data)
	if next > len(content) {
		return nil, len(content), fmt.Errorf(
			"the file ends in the data of unit %d", unit)
	}
	return cards, next, nil
}

// read is the cards with values of the header that begins at a byte, and
// the header's length in bytes. The unit counts from none.
func read(content []byte, at, unit int) (map[string]string, int, error) {
	cards := map[string]string{}
	begins := extension
	if unit == first {
		begins = primary
	}
	last := len(content) - blockSize
	for length := nothing; at+length <= last; length += blockSize {
		block := content[at+length : at+length+blockSize]
		starts := bytes.HasPrefix(block, []byte(begins))
		if length == nothing && !starts {
			return nil, nothing, fmt.Errorf(
				"unit %d does not begin with %q", unit, begins)
		}
		if ended(block, cards) {
			return cards, length + blockSize, nil
		}
	}
	return nil, nothing, fmt.Errorf(
		"the file ends before unit %d's END card", unit)
}

// ended adds the cards of a block that have values to cards, and says
// whether the END card was among them.
func ended(block []byte, cards map[string]string) bool {
	for at := first; at+cardSize <= len(block); at += cardSize {
		raw := string(block[at : at+cardSize])
		key := strings.TrimSpace(raw[:keyEnd])
		if key == endCard {
			return true
		}
		if raw[keyEnd:valueAt] == equals {
			text, _, _ := strings.Cut(raw[valueAt:], comment)
			cards[key] = strings.TrimSpace(text)
		}
	}
	return false
}

// dataOf is the length of the data a header's cards describe, in bytes
// before padding: the pixel's bytes times the parameters and the product
// of the axes. It refuses what the standard does not allow, and a length
// past the whole file's size, which also keeps the sum from overflowing.
func dataOf(cards map[string]string, size int) (int, error) {
	bits, bitErr := count(cards, "BITPIX", false, nothing)
	axes, axesErr := count(cards, "NAXIS", false, nothing)
	heap, heapErr := count(cards, "PCOUNT", true, nothing)
	groups, groupErr := count(cards, "GCOUNT", true, oneGroup)
	if err := errors.Join(bitErr, axesErr, heapErr, groupErr); err != nil {
		return nothing, err
	}
	if !sizes[bits] || axes < nothing || heap < nothing ||
		heap > size || groups != oneGroup {
		return nothing, fmt.Errorf("BITPIX %d, NAXIS %d, PCOUNT %d, "+
			"GCOUNT %d", bits, axes, heap, groups)
	}
	cells, err := cellsOf(cards, axes, size)
	if err != nil {
		return nothing, err
	}
	return max(bits, -bits) / byteBits * (heap + cells), nil
}

// cellsOf is the product of a header's axes, no more than the size of the
// file, and none for an array of no axes.
func cellsOf(cards map[string]string, axes, size int) (int, error) {
	if axes == nothing {
		return nothing, nil
	}
	cells := oneGroup
	for axis := oneGroup; axis <= axes; axis++ {
		key := "NAXIS" + strconv.Itoa(axis)
		length, err := count(cards, key, false, nothing)
		if err != nil {
			return nothing, err
		}
		past := length != nothing && cells > size/length
		if length < nothing || past {
			return nothing, fmt.Errorf("%s is %d", key, length)
		}
		cells *= length
	}
	return cells, nil
}

// count is a header's whole number under a keyword. A keyword the header
// lacks is otherwise when it may be left out, else an error.
func count(cards map[string]string, key string, optional bool,
	otherwise int) (int, error) {
	text, there := cards[key]
	if !there && optional {
		return otherwise, nil
	}
	if !there {
		return nothing, fmt.Errorf("no %s card", key)
	}
	number, err := strconv.Atoi(text)
	if err != nil {
		return nothing, fmt.Errorf("%s: %w", key, err)
	}
	return number, nil
}
