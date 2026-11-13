package spherex_test

import (
	"bytes"
	"fmt"
	"os"
	"slices"
	"strings"
	"testing"

	"github.com/janearc/celestial-data/spherex"
)

// What the crafted FITS files are made of. Each is defined by the FITS
// standard or by the test that uses it, as its comment says.
const (
	// cardWidth is the width of a header card, in bytes. It's defined by
	// the FITS standard.
	cardWidth = 80
	// cardsInBlock is the cards a block holds. It's defined by the FITS
	// standard: a block is 2880 bytes and a card 80.
	cardsInBlock = block / cardWidth
	// primaryCards is the cards of the primary header primaryOf writes
	// before any filler. It's defined by primaryOf.
	primaryCards = 3
	// pixels is the pixels of the small images. It's defined.
	pixels = 3
	// down is the second side of the small two-dimensional images. It's
	// defined.
	down = 2
	// oneByte is the bytes of a pixel of BITPIX 8. It's defined by the
	// FITS standard.
	oneByte = 1
	// twoBytes is the bytes of a pixel of BITPIX 16. It's defined by the
	// FITS standard.
	twoBytes = 2
	// fourBytes is the bytes of a pixel of BITPIX 32 or -32. It's
	// defined by the FITS standard.
	fourBytes = 4
	// eightBytes is the bytes of a pixel of BITPIX 64 or -64. It's
	// defined by the FITS standard.
	eightBytes = 8
	// heapBytes is the bytes of the heap of the file that has one. It's
	// defined: the PCOUNT its header says.
	heapBytes = 10
	// padByte is what pads a header. It's defined by the FITS standard.
	padByte = ' '
	// blockLength is a block's length, as a header writes it. It's
	// defined.
	blockLength = "2880"
	// blockAndAByte is a block's length and a byte, as a header writes
	// it. It's defined.
	blockAndAByte = "2881"
	// twoBlocks is two blocks' length, as a header writes it. It's
	// defined.
	twoBlocks = "5760"
)

// filler is a header card that says nothing.
const filler = "COMMENT filler"

// field is a card's first thirty bytes, a keyword and a value, as a
// header writes them.
func field(key, value string) string {
	return fmt.Sprintf("%-8s= %20s", key, value)
}

// cardsOf is cards of the keywords and values given, in turn.
func cardsOf(pairs ...string) []string {
	var cards []string
	for at := none; at < len(pairs); at += down {
		cards = append(cards, field(pairs[at], pairs[at+oneByte]))
	}
	return cards
}

// withCards is the cards given and more of the keywords and values given.
func withCards(cards []string, pairs ...string) []string {
	more := cardsOf(pairs...)
	return append(slices.Clone(cards), more...)
}

// axes is the cards of an image of the lengths given, in a depth.
func axes(bits string, lengths ...string) []string {
	count := fmt.Sprint(len(lengths))
	cards := cardsOf("BITPIX", bits, "NAXIS", count)
	for at, length := range lengths {
		key := fmt.Sprintf("NAXIS%d", at+oneByte)
		cards = append(cards, field(key, length))
	}
	return cards
}

// headerOf is a header of the cards given, then END, padded with spaces
// to a whole number of blocks.
func headerOf(cards ...string) []byte {
	var header bytes.Buffer
	for _, card := range append(slices.Clone(cards), "END") {
		fmt.Fprintf(&header, "%-*s", cardWidth, card)
	}
	for header.Len()%block != none {
		header.WriteByte(padByte)
	}
	return header.Bytes()
}

// primaryOf is a primary header of no array, with the cards given.
func primaryOf(cards ...string) []byte {
	all := cardsOf("SIMPLE", "T", "BITPIX", "8", "NAXIS", "0")
	return headerOf(append(all, cards...)...)
}

// roundedUp is a length made a whole number of blocks.
func roundedUp(length int) int {
	return (length + block - once) / block * block
}

// imageOf is an image unit with the cards given and so many bytes of
// data, padded to the block.
func imageOf(data int, cards []string) []byte {
	all := append([]string{"XTENSION= 'IMAGE   '"}, cards...)
	unit := headerOf(all...)
	padding := make([]byte, roundedUp(data))
	return append(unit, padding...)
}

// imageFile is a file of an empty primary and an image unit.
func imageFile(data int, cards []string) []byte {
	image := imageOf(data, cards)
	return slices.Concat(primaryOf(), image)
}

// bare is a file of a primary and an image with the cards given and no
// data.
func bare(cards []string) []byte { return imageFile(none, cards) }

// depth is a BITPIX and the bytes of a pixel it means.
type depth struct {
	bits  string
	bytes int
}

// depths are the six BITPIX values the standard allows.
var depths = []depth{{"8", oneByte}, {"16", twoBytes},
	{"32", fourBytes}, {"64", eightBytes}, {"-32", fourBytes},
	{"-64", eightBytes}}

// wholeFiles are files that are FITS all the way through, by name.
func wholeFiles(test *testing.T) map[string][]byte {
	test.Helper()
	image := imageOf(pixels, axes("8", "3"))
	heap := withCards(axes("8", "3", "2"), "PCOUNT", "10", "GCOUNT", "1")
	fillers := slices.Repeat([]string{filler},
		cardsInBlock-primaryCards-once)
	files := map[string][]byte{
		"two units of a real cutout": cutout(test),
		"a primary only":             primaryOf(),
		"three units":                slices.Concat(primaryOf(), image, image),
		"an image of no axes": imageFile(none,
			cardsOf("BITPIX", "8", "NAXIS", "0")),
		"an axis of none": imageFile(none, axes("8", "3", "0")),
		"a block of data": imageFile(block, axes("8", blockLength)),
		"a block and a byte of data": imageFile(block+once,
			axes("8", blockAndAByte)),
		"two axes": imageFile(pixels*down*twoBytes,
			axes("16", "3", "2")),
		"a heap": imageFile(pixels*down+heapBytes, heap),
		"an END that is the last card of its block": primaryOf(fillers...),
	}
	for _, each := range depths {
		files["pixels of BITPIX "+each.bits] = imageFile(
			pixels*each.bytes, axes(each.bits, "3"))
	}
	return files
}

// the test: a file that is FITS all the way through walks whole,
// whatever the depth of its pixels, whether its data fills its last
// block or not, whether it has parameters or not, and wherever its END
// card lies. None of these is a cutout, and a fetch keeps none of them:
// see TestOnlyAWholeCutoutIsKept.
func TestAWholeFileWalksWhole(test *testing.T) {
	for name, content := range wholeFiles(test) {
		if err := spherex.WholeBytes(content); err != nil {
			test.Errorf("%s: %v", name, err)
		}
	}
}

// The crafted cutouts' sizes, each defined.
const (
	// cutSide is the side of a crafted cutout's image, in pixels.
	// It's defined.
	cutSide = "3"
	// otherSide is a plane of another size. It's defined.
	otherSide = "4"
	// psfDepth is QR2's PSF's third axis. It's defined: a few.
	psfDepth = "2"
	// tableRow is the bytes of a crafted table's one row. It's defined.
	tableRow = "4"
	// tableBytes is those bytes as a length. It's defined.
	tableBytes = 4
	// cutUnits is the units of a whole cutout. It's defined by the
	// cutouts IRSA serves: a primary and six extensions.
	cutUnits = 7
)

// unitNamed is an extension of a kind and a name, with the cards given
// and so many bytes of data, padded to the block.
func unitNamed(kind, name string, data int, cards []string) []byte {
	head := fmt.Sprintf("XTENSION= '%-8s'", kind)
	all := append([]string{head}, cards...)
	all = append(all, field("EXTNAME", "'"+name+"'"))
	return append(headerOf(all...), make([]byte, roundedUp(data))...)
}

// sided is the pixels of a square of a side.
func sided(side string) int {
	length := sideOf(side)
	return length * length
}

// sideOf is a side as a number of pixels.
func sideOf(side string) int {
	var pixels int
	fmt.Sscan(side, &pixels)
	return pixels
}

// planeOf is an image extension of a name, a BITPIX of four bytes and a
// square side.
func planeOf(name, bits, side string) []byte {
	return unitNamed("IMAGE", name, sided(side)*fourBytes,
		axes(bits, side, side))
}

// tableNamed is a binary table extension of a name and one short row.
func tableNamed(name string) []byte {
	cards := withCards(axes("8", tableRow, "1"), "PCOUNT", "0", "GCOUNT",
		"1")
	return unitNamed("BINTABLE", name, tableBytes, cards)
}

// The PSF of each release, crafted: QR2's an image of three axes, QR3's
// a table.
func qr2PSF() []byte {
	return unitNamed("IMAGE", "PSF",
		sided(cutSide)*sideOf(psfDepth)*fourBytes,
		axes("-32", cutSide, cutSide, psfDepth))
}

// qr3PSF is QR3's PSF, a table.
func qr3PSF() []byte { return tableNamed("EPSF") }

// cutoutUnits are a crafted cutout's units in order: the empty primary,
// the image and its flags, variance and zodiacal light, the PSF given,
// and the wavelength table. flags is the side of the flags plane.
func cutoutUnits(psf []byte, flags string) [][]byte {
	return [][]byte{primaryOf(), planeOf("IMAGE", "-32", cutSide),
		planeOf("FLAGS", "32", flags), planeOf("VARIANCE", "-32", cutSide),
		planeOf("ZODI", "-32", cutSide), psf, tableNamed("WCS-WAVE")}
}

// the test: a whole cutout is kept: QR3's with its EPSF table, QR2's
// with its PSF image, and, for a release not yet seen, either.
func TestOnlyAWholeCutoutIsKept(test *testing.T) {
	qr3 := slices.Concat(cutoutUnits(qr3PSF(), cutSide)...)
	qr2 := slices.Concat(cutoutUnits(qr2PSF(), cutSide)...)
	for name, each := range map[string]struct {
		row     []string
		content []byte
	}{
		"QR3":              {qr3Row, qr3},
		"QR2":              {releasedAs("qr2"), qr2},
		"QR4 with an EPSF": {releasedAs("qr4"), qr3},
		"QR4 with a PSF":   {releasedAs("qr4"), qr2},
	} {
		into, err := fetchRowOf(test, each.row, each.content)
		kept, readErr := os.ReadDir(into)
		if err != nil || readErr != nil || len(kept) != once {
			test.Errorf("%s: %v, %v, %d files", name, err, readErr,
				len(kept))
		}
	}
}

// notACutout are whole FITS files that are not a whole cutout, by what
// the refusal says: a cutout cut where any unit ends, the two units of
// the real cutout kept in testdata among them; one with a unit more;
// its units out of order; the other release's PSF; a plane of another
// size; and wavelengths that are not a table.
func notACutout(test *testing.T) map[string]refusal {
	test.Helper()
	units := cutoutUnits(qr3PSF(), cutSide)
	refused := map[string]refusal{}
	for ends := once; ends < cutUnits; ends++ {
		says := fmt.Sprintf("%d units, not a cutout's %d", ends, cutUnits)
		refused[fmt.Sprintf("cut after unit %d", ends-once)] = refusal{
			slices.Concat(units[:ends]...), says}
	}
	refused["the two units of a real cutout"] = refusal{cutout(test),
		"2 units, not a cutout's 7"}
	refused["a unit more"] = refusal{slices.Concat(append(
		slices.Clone(units), tableNamed("WCS-WAVE"))...), "8 units"}
	swapped := slices.Clone(units)
	swapped[2], swapped[3] = swapped[3], swapped[2]
	refused["flags and variance swapped"] = refusal{
		slices.Concat(swapped...), `unit 2 is "VARIANCE", not "FLAGS"`}
	refused["QR2's PSF in a QR3 cutout"] = refusal{slices.Concat(
		cutoutUnits(qr2PSF(), cutSide)...), `unit 5 is "PSF", not "EPSF"`}
	refused["flags of another size"] = refusal{slices.Concat(
		cutoutUnits(qr3PSF(), otherSide)...),
		"FLAGS is not an image the size of IMAGE"}
	imageWave := slices.Clone(units)
	imageWave[len(imageWave)-once] = unitNamed("IMAGE", "WCS-WAVE",
		tableBytes, axes("8", tableRow))
	refused["wavelengths that are an image"] = refusal{
		slices.Concat(imageWave...), "WCS-WAVE is not a table"}
	imagePSF := slices.Clone(units)
	imagePSF[psfUnit] = unitNamed("IMAGE", "EPSF", tableBytes,
		axes("8", tableRow))
	refused["an EPSF that is an image"] = refusal{
		slices.Concat(imagePSF...), "EPSF is not a table"}
	arrayed := slices.Clone(units)
	arrayed[first] = append(headerOf(cardsOf("SIMPLE", "T", "BITPIX", "8",
		"NAXIS", "1", "NAXIS1", tableRow)...), make([]byte, block)...)
	refused["a primary that holds an array"] = refusal{
		slices.Concat(arrayed...), "the primary holds an array"}
	empty := slices.Clone(units)
	for at, name := range []string{"IMAGE", "FLAGS", "VARIANCE", "ZODI"} {
		empty[at+once] = planeOf(name, "-32", "0")
	}
	refused["planes of no pixels"] = refusal{slices.Concat(empty...),
		"IMAGE is empty"}
	// a plane wrong in one way alone is refused for it: planes empty in
	// width only; FLAGS of another width alone; FLAGS of a third axis;
	// and FLAGS that is a table of the image's size
	narrow := slices.Clone(units)
	for at, name := range []string{"IMAGE", "FLAGS", "VARIANCE", "ZODI"} {
		narrow[at+once] = unitNamed("IMAGE", name, none,
			axes("-32", "0", cutSide))
	}
	refused["planes of no width"] = refusal{slices.Concat(narrow...),
		"IMAGE is empty"}
	wider := slices.Clone(units)
	wider[flagsUnit] = unitNamed("IMAGE", "FLAGS",
		sideOf(otherSide)*sideOf(cutSide)*fourBytes,
		axes("32", otherSide, cutSide))
	refused["flags of another width alone"] = refusal{
		slices.Concat(wider...), "FLAGS is not an image the size of IMAGE"}
	deeper := slices.Clone(units)
	deeper[flagsUnit] = unitNamed("IMAGE", "FLAGS",
		sided(cutSide)*sideOf(psfDepth)*fourBytes,
		axes("32", cutSide, cutSide, psfDepth))
	refused["flags of a third axis"] = refusal{slices.Concat(deeper...),
		"FLAGS is not an image the size of IMAGE"}
	tabled := slices.Clone(units)
	tabled[flagsUnit] = unitNamed("BINTABLE", "FLAGS", sided(cutSide),
		withCards(axes("8", cutSide, cutSide), "PCOUNT", "0", "GCOUNT",
			"1"))
	refused["flags that are a table"] = refusal{slices.Concat(tabled...),
		"FLAGS is not an image the size of IMAGE"}
	return refused
}

// flagsUnit is where a cutout's FLAGS plane lies among its units. It's
// defined: after the primary and the image.
const flagsUnit = 2

// psfUnit is where a cutout's PSF lies among its units. It's defined:
// after the primary and the four planes.
const psfUnit = 5

// Anti-test: in a release not yet seen, a PSF unit with no EXTNAME is
// not kept. Either name was asked for there, and a missing name equalled
// the mark for either, so it passed.
func TestAnUnnamedPSFIsNotKept(test *testing.T) {
	head := fmt.Sprintf("XTENSION= '%-8s'", "BINTABLE")
	cards := withCards(axes("8", tableRow, "1"), "PCOUNT", "0", "GCOUNT",
		"1")
	unnamed := append(headerOf(append([]string{head}, cards...)...),
		make([]byte, roundedUp(tableBytes))...)
	content := slices.Concat(cutoutUnits(unnamed, cutSide)...)
	into, err := fetchRowOf(test, releasedAs("qr4"), content)
	kept, readErr := os.ReadDir(into)
	if err == nil || readErr != nil || len(kept) != none ||
		!strings.Contains(err.Error(), `unit 5 is "", not "PSF" or "EPSF"`) {
		test.Errorf("%v, %v, %d files", err, readErr, len(kept))
	}
}

// Anti-test: a QR2 cutout whose PSF is an image of two axes is not kept.
// The check read the PSF's name alone, so any unit named PSF passed.
func TestAQR2PSFOfTwoAxesIsNotKept(test *testing.T) {
	flat := unitNamed("IMAGE", "PSF", sided(cutSide)*fourBytes,
		axes("-32", cutSide, cutSide))
	content := slices.Concat(cutoutUnits(flat, cutSide)...)
	into, err := fetchRowOf(test, releasedAs("qr2"), content)
	kept, readErr := os.ReadDir(into)
	if err == nil || readErr != nil || len(kept) != none ||
		!strings.Contains(err.Error(), "PSF is not an image of 3 axes") {
		test.Errorf("%v, %v, %d files", err, readErr, len(kept))
	}
}

// Anti-test: a whole FITS file that is not a whole cutout is not kept.
// Where the check was FITS alone, a cutout cut where a unit ends was a
// whole FITS file and was kept under the cutout's name, the primary
// alone and the real two units among them.
func TestAFileThatIsNotACutoutIsNotKept(test *testing.T) {
	for name, each := range notACutout(test) {
		into, err := fetchOf(test, each.content)
		kept, readErr := os.ReadDir(into)
		if err == nil || readErr != nil || len(kept) != none ||
			!strings.Contains(err.Error(), each.says) {
			test.Errorf("%s: %v, %v, %d files", name, err, readErr,
				len(kept))
		}
	}
}

// refusal is a file that is not whole, and what the error should say of
// it.
type refusal struct {
	content []byte
	says    string
}

// page is what a server says when it made no image.
var page = []byte("<html><body>An error occurred</body></html>")

// notFITS are files that are not FITS at all, or not from their first
// byte.
func notFITS(test *testing.T) map[string]refusal {
	test.Helper()
	genuine := cutout(test)
	blocks := append(slices.Clone(page), make([]byte, block)...)
	return map[string]refusal{
		"nothing":     {[]byte{}, "ends before unit 0's END"},
		"a few bytes": {genuine[:once], "ends before unit 0's END"},
		"a page":      {page, "ends before unit 0's END"},
		"a page of a block": {blocks,
			`unit 0 does not begin with "SIMPLE`},
		"no primary": {bytes.Replace(genuine, []byte("SIMPLE  ="),
			[]byte("XTENSION="), once),
			`unit 0 does not begin with "SIMPLE`},
		"no extension": {bytes.Replace(genuine,
			[]byte("XTENSION= 'IMAGE"), []byte("XTENSIOX= 'IMAGE"),
			once), `unit 1 does not begin with "XTENSION`},
	}
}

// cutShort are real files cut short, or with bytes past their end.
func cutShort(test *testing.T) map[string]refusal {
	test.Helper()
	genuine := cutout(test)
	extra := append(slices.Clone(genuine), padByte)
	more := append(slices.Clone(genuine), make([]byte, block)...)
	return map[string]refusal{
		"a short header": {genuine[:block/down], "ends before unit 0"},
		"a header with no end": {genuine[:primaryEnd+block],
			"ends before unit 1's END"},
		"the data cut short": {genuine[:imageData+block],
			"the file ends in the data of unit 1"},
		"one byte short": {genuine[:imageEnd-once],
			"the file ends in the data of unit 1"},
		"a byte too many": {extra, "ends before unit 2's END"},
		"a block too many": {more,
			`unit 2 does not begin with "XTENSION`},
	}
}

// badCards are files whose cards are missing or not numbers.
func badCards() map[string]refusal {
	return map[string]refusal{
		"no BITPIX": {bare(cardsOf("NAXIS", "0")), "no BITPIX card"},
		"BITPIX not a number": {bare(cardsOf("BITPIX", "float",
			"NAXIS", "0")), "BITPIX: strconv.Atoi"},
		"no NAXIS": {bare(cardsOf("BITPIX", "8")), "no NAXIS card"},
		"no NAXIS1": {bare(cardsOf("BITPIX", "8", "NAXIS", "1")),
			"no NAXIS1 card"},
		"no NAXIS2": {bare(cardsOf("BITPIX", "8", "NAXIS", "2",
			"NAXIS1", "3")), "no NAXIS2 card"},
		"NAXIS1 not a number": {bare(axes("8", "x")),
			"NAXIS1: strconv.Atoi"},
	}
}

// badSizes are files whose cards are numbers the standard does not allow,
// or that are longer than the file.
func badSizes() map[string]refusal {
	three := axes("8", "3")
	return map[string]refusal{
		"an odd BITPIX": {bare(cardsOf("BITPIX", "-31", "NAXIS", "0")),
			"BITPIX -31"},
		"NAXIS under": {bare(cardsOf("BITPIX", "8", "NAXIS", "-1")),
			"NAXIS -1"},
		"a negative axis": {bare(axes("8", "-5")), "NAXIS1 is -5"},
		"an axis past the file": {bare(axes("8", "99999999")),
			"NAXIS1 is 99999999"},
		"axes past the file": {bare(axes("8", "29", "99999999")),
			"NAXIS2 is 99999999"},
		"a heap past the file": {bare(withCards(three, "PCOUNT",
			"99999999")), "PCOUNT 99999999"},
		"a negative heap": {bare(withCards(three, "PCOUNT", "-1")),
			"PCOUNT -1"},
		"two groups": {bare(withCards(three, "GCOUNT", "2")),
			"GCOUNT 2"},
	}
}

// justPast are files whose lengths are the most that the file could hold,
// or the least that it could not, and an empty array with data after it.
func justPast() map[string]refusal {
	enough := "the file ends in the data of unit 1"
	heap := withCards(axes("8", "3"), "PCOUNT", twoBlocks)
	return map[string]refusal{
		"cells as many as the file can hold": {bare(axes("8",
			blockLength, "2")), enough},
		"a heap as long as the file": {bare(heap), enough},
		"an empty array with data after it": {imageFile(block,
			axes("8", "3", "0")), `unit 2 does not begin with "XTENS`},
	}
}

// Anti-tests: a file that is cut short at any of these places, that has
// a byte past its end, that is no FITS file at all, or whose headers
// break the standard, is not kept: the fetch fails, the folder is left
// empty though the server said no length, and the error says what is
// wrong and which file it is.
func TestAFileThatIsNotWholeIsNotKept(test *testing.T) {
	for _, group := range []map[string]refusal{notFITS(test),
		cutShort(test), badCards(), badSizes(), justPast()} {
		for name, each := range group {
			into, err := fetchOf(test, each.content)
			kept, readErr := os.ReadDir(into)
			if err == nil || readErr != nil || len(kept) != none ||
				!strings.Contains(err.Error(), each.says) ||
				!strings.Contains(err.Error(), ".fits.part") {
				test.Errorf("%s: %v, %v, %d files", name, err, readErr,
					len(kept))
			}
		}
	}
}
