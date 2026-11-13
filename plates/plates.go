// Package plates finds the photographic sky-survey plates that cover a
// place on the sky: for a circle, a cutout from each survey STScI's
// Digitized Sky Survey serves there, as a FITS file with the plate's
// header cards as the scan left them.
//
// The surveys are the two Palomar epochs, POSS-I in the 1950s and
// POSS-II in the north, the UK Schmidt's SERC and AAO surveys in the
// south, and Quick-V. The form, its help and its answers were read on
// 2026-11-06 (sources.yaml).
//
// STScI asks that work using the scans acknowledge the surveys, in the
// words of its acknowledging page.
//
// A survey with no plate at a place is answered with a page, not a
// FITS file. That survey is left out of what Find names.
//
// The archive is asked only by name.
package plates

import (
	"bytes"
	_ "embed"
	"errors"
	"fmt"
	"io"
	"net/url"
	"os"
	"strconv"
	"strings"

	"github.com/janearc/celestial-data"
	"github.com/janearc/celestial-data/web"
)

// adapterName is the archive's name, as the command line takes it.
const adapterName = "plates"

// endpoint is where STScI cuts an image from a survey.
const endpoint = "https://archive.stsci.edu/cgi-bin/dss_search"

// What the cutout address says besides the survey and the place.
const (
	// equinox says the coordinates are J2000. celestial-data's are
	// ICRS, which differs from J2000 by about 0.02 arcseconds.
	equinox = "J2000"
	// format asks for a FITS file, not a GIF.
	format = "fits"
)

// The sizes the cutout is worked out in.
const (
	// arcminutes is how many arcminutes make a degree. It's defined.
	arcminutes = 60
	// twice is how many radii a square's side is: it's defined, the
	// square holds the circle.
	twice = 2
)

// surveys are the surveys the form offers that are scans of
// photographic plates, by the name the address takes. The form also
// offers two for HST proposals, which are not asked.
var surveys = []string{"poss1_blue", "poss1_red", "poss2ukstu_blue",
	"poss2ukstu_red", "poss2ukstu_ir", "quickv"}

// What a FITS file and the answers here are made of.
const (
	// blockSize is the length of a FITS block, in bytes. It's defined
	// by the FITS standard.
	blockSize = 2880
	// cardSize is the length of a header card. It's defined by the
	// FITS standard.
	cardSize = 80
	// keyEnd is where a card's keyword ends. It's defined by the FITS
	// standard.
	keyEnd = 8
	// valueAt is where a card's value begins, after "= ". It's
	// defined by the FITS standard.
	valueAt = 10
	// equals is what follows a keyword that has a value.
	equals = "= "
	// endCard is the keyword of the card that ends the header.
	endCard = "END"
	// quote opens and closes a string value.
	quote = "'"
	// comment begins the comment after a value.
	comment = "/"
	// firstBlock is the number of the block read before the rest. It's
	// defined: blocks are counted from one.
	firstBlock = 1
	// firstCard is where the first card of a block begins. It's
	// defined: a block begins with a card.
	firstCard = 0
	// nothing is the count of cards a header lacks when it lacks
	// none. It's defined.
	nothing = 0
	// simple begins every FITS file.
	simple = "SIMPLE  ="
	// noPlate is what STScI's page says when it made no image, as
	// read on 2026-11-06.
	noPlate = "the image file was not created"
	// wholeBlocks is what is left over of a FITS file's length past its
	// whole blocks. It's defined by the FITS standard: none.
	wholeBlocks = 0
)

// The cards of the header that say which plate it is.
const (
	cardSurvey   = "SURVEY"
	cardPlate    = "PLATEID"
	cardLabel    = "PLTLABEL"
	cardDate     = "DATE-OBS"
	cardExposure = "EXPOSURE"
	cardEmulsion = "EMULSION"
	cardTelescop = "TELESCOP"
	// cardFilter may be blank: a plate with no filter has none.
	cardFilter = "FILTER"
)

// needed are the cards that must say something.
var needed = []string{cardSurvey, cardPlate, cardLabel, cardDate,
	cardExposure, cardEmulsion, cardTelescop}

// numbersFile is the numbers file the sizes are read from.
const numbersFile = "numbers.yaml"

//go:embed numbers.yaml
var numbersRaw []byte

// The names of the numbers in numbers.yaml.
const (
	mostSide     = "most-side"
	leastSide    = "least-side"
	headerBlocks = "header-blocks"
	skyPlaces    = "ra-dec-places"
	sidePlaces   = "side-places"
)

// errNoPlate is a survey that has no plate at the place.
var errNoPlate = errors.New("no plate")

// Archive is the plates STScI's Digitized Sky Survey has scanned.
type Archive struct {
	base *celestialdata.Adapter
	// most and least are the longest and shortest side asked, in
	// arcminutes; blocks the most header blocks read; skyDecimals and
	// sideDecimals the decimals the centre and the side are written with.
	most, least                       float64
	blocks, skyDecimals, sideDecimals int
}

// New is the plates, asked through get. It refuses no web.
func New(get web.Web) (*Archive, error) {
	base, err := celestialdata.NewAdapter(adapterName, get)
	if err != nil {
		return nil, err
	}
	kept := celestialdata.NewMeasured(numbersFile, numbersRaw)
	return &Archive{base: base, most: kept.Value(mostSide),
		least:        kept.Value(leastSide),
		blocks:       int(kept.Value(headerBlocks)),
		skyDecimals:  int(kept.Value(skyPlaces)),
		sideDecimals: int(kept.Value(sidePlaces))}, nil
}

// Name is the adapter's name as the command line takes it.
func (archive *Archive) Name() string { return archive.base.Name() }

// ByNameOnly says the archive is asked only when it is named.
func (*Archive) ByNameOnly() bool { return true }

// cut is the square of sky a cutout holds, as the address and the
// file's name write it: the centre in degrees and the side in
// arcminutes.
type cut struct{ ra, dec, side string }

// cutOf is the square round the circle's centre whose side is twice
// the radius, no less than the least a side is asked at and no more
// than the form allows.
func (archive *Archive) cutOf(near *celestialdata.Near) cut {
	centre := near.Centre()
	wide := twice * near.Radius() * arcminutes
	clamped := min(max(wide, archive.least), archive.most)
	return cut{ra: archive.degrees(centre.RA()),
		dec:  archive.degrees(centre.Dec()),
		side: fmt.Sprintf("%.*f", archive.sideDecimals, clamped)}
}

// degrees is a coordinate as the address writes it.
func (archive *Archive) degrees(angle float64) string {
	return fmt.Sprintf("%.*f", archive.skyDecimals, angle)
}

// address is where STScI cuts the square from a survey.
func (square cut) address(survey string) string {
	return endpoint + "?" + url.Values{"v": {survey}, "e": {equinox},
		"f": {format}, "r": {square.ra}, "d": {square.dec},
		"h": {square.side}, "w": {square.side}}.Encode()
}

// Find is a cutout from each survey that has a plate at the circle's
// centre, in the order the surveys are listed. Each is asked for in
// turn, and its header read for the plate's id and date; the rest of
// the file is left to Fetch. A survey with no plate is left out.
func (archive *Archive) Find(near *celestialdata.Near) (
	[]*celestialdata.Product, error) {
	var products []*celestialdata.Product
	square := archive.cutOf(near)
	for _, survey := range surveys {
		product, err := archive.cutout(survey, square)
		if errors.Is(err, errNoPlate) {
			continue
		}
		if err != nil {
			return nil, archive.base.Fail("%s: %w", survey, err)
		}
		products = append(products, product)
	}
	return products, nil
}

// cutout is the product of one survey's answer, or errNoPlate.
func (archive *Archive) cutout(survey string,
	square cut) (*celestialdata.Product, error) {
	address := square.address(survey)
	body, length, err := archive.base.Web().Get(address)
	if err != nil {
		return nil, err
	}
	defer body.Close()
	cards, _, err := header(body, archive.blocks)
	if err != nil {
		return nil, err
	}
	if err := missing(cards); err != nil {
		return nil, err
	}
	day, _, _ := strings.Cut(cards[cardDate], "T")
	name := fmt.Sprintf("%s_%s_%s_ra%s_dec%s_%sarcmin.fits", survey,
		cards[cardPlate], day, square.ra, square.dec, square.side)
	about := plateAbout(cards) + "; the " + survey + " cutout, " +
		square.side + " arcminutes square"
	product, err := archive.base.Product(cards[cardPlate], name, address,
		max(length, celestialdata.Unsized), about)
	check := plateFile{plate: cards[cardPlate], blocks: archive.blocks}
	return product.Checked(check), err
}

// plateFile is the check of a fetched plate: the file is FITS, of whole
// blocks, its header names the plate Find read there, and it reaches at
// least the end of the image that header declares, padded to the block.
//
// STScI's file has an extension after the image, so it may be longer.
// A page in a plate's place, of any length or none, another plate, and
// a plate's header without its image, are refused, and the file is not
// kept.
//
// It reads the header itself rather than through celestial's fits: the
// celestial released, v0.0.7, has crash bugs on crafted headers that
// its next release fixes, and these headers come from a server.
type plateFile struct {
	plate  string
	blocks int
}

// plateFile is a check a product can hold.
var _ celestialdata.Checker = plateFile{}

// Check says nothing for the plate asked for, or why the file is not it.
func (want plateFile) Check(path string) error {
	file, err := os.Open(path)
	if err != nil {
		return err
	}
	defer file.Close()
	about, err := file.Stat()
	if err != nil {
		return err
	}
	if about.Size()%blockSize != wholeBlocks {
		return fmt.Errorf("%s: %d bytes is not whole FITS blocks",
			adapterName, about.Size())
	}
	cards, blocks, err := header(file, want.blocks)
	if errors.Is(err, errNoPlate) {
		return fmt.Errorf("%s: the page that says there is no plate "+
			"came, not plate %s", adapterName, want.plate)
	}
	if err != nil {
		return fmt.Errorf("%s: %w", adapterName, err)
	}
	if cards[cardPlate] != want.plate {
		return fmt.Errorf("%s: plate %q came, not %q", adapterName,
			cards[cardPlate], want.plate)
	}
	image, err := imageBytes(cards, about.Size())
	if err != nil {
		return fmt.Errorf("%s: %w", adapterName, err)
	}
	primary := int64(blocks)*blockSize + padded(image)
	if about.Size() < primary {
		return fmt.Errorf("%s: %d bytes came, short of the %d of the "+
			"header and the image it declares", adapterName,
			about.Size(), primary)
	}
	return nil
}

// widths are the bytes of a pixel of each BITPIX the standard allows,
// section 4.4.1.1.
//
//ninja:vow source defined by the FITS standard, 4.4.1.1: no measure
var widths = map[int]int64{8: 1, 16: 2, 32: 4, 64: 8, -32: 4, -64: 8}

// The axes a primary array may have, and the cards that say them, each
// defined by the FITS standard, section 4.4.1.1.
const (
	mostAxes = 999 // defined by the FITS standard: NAXIS is 999 at most
	naxis    = "NAXIS"
)

// imageBytes is the bytes of the image a primary header declares, from
// its BITPIX and its NAXISn, before padding. It refuses a BITPIX the
// standard does not allow, axes that are not counts, and an image longer
// than the file, so no product can overflow.
func imageBytes(cards map[string]string, size int64) (int64, error) {
	bitpix, err := strconv.Atoi(cards["BITPIX"])
	width, known := widths[bitpix]
	if err != nil || !known {
		return nothing, fmt.Errorf("BITPIX %q is not a pixel's",
			cards["BITPIX"])
	}
	axes, err := strconv.Atoi(cards[naxis])
	if err != nil || axes < nothing || axes > mostAxes {
		return nothing, fmt.Errorf("NAXIS %q is not a count of axes",
			cards[naxis])
	}
	if axes == nothing {
		return nothing, nil
	}
	image := width
	for axis := firstAxis; axis <= axes; axis++ {
		key := naxis + strconv.Itoa(axis)
		length, err := strconv.ParseInt(cards[key], decimal, wholeBits)
		if err != nil || length < nothing {
			return nothing, fmt.Errorf("%s %q is not a length", key,
				cards[key])
		}
		if length != nothing && image > size/length {
			return nothing, fmt.Errorf("the image %s declares is "+
				"longer than the file", key)
		}
		image *= length
	}
	return image, nil
}

// padded is a length made up to whole blocks.
func padded(length int64) int64 {
	return (length + blockSize - oneByte) / blockSize * blockSize
}

// How the axes are counted and their lengths read, each defined: FITS
// counts axes from one; strconv reads base ten into 64 bits; and a block
// less one byte, added, makes dividing by a block round up.
const (
	firstAxis = 1  // defined by FITS: axes are counted from one
	decimal   = 10 // defined by strconv: base ten
	wholeBits = 64 // defined by strconv: into 64 bits
	oneByte   = 1  // defined: a block less one, added, rounds up
)

// plateAbout is a few words on the plate its header describes.
func plateAbout(cards map[string]string) string {
	emulsion := cards[cardEmulsion]
	if cards[cardFilter] != "" {
		emulsion += " through " + cards[cardFilter]
	}
	return fmt.Sprintf("%s plate %s (%s), %s, %s, %s minutes, %s",
		cards[cardSurvey], cards[cardPlate], cards[cardLabel],
		cards[cardTelescop], emulsion, cards[cardExposure],
		cards[cardDate])
}

// missing is an error that names the cards a header lacks, or none.
func missing(cards map[string]string) error {
	var lacking []string
	for _, card := range needed {
		if cards[card] == "" {
			lacking = append(lacking, card)
		}
	}
	if _, there := cards[cardFilter]; !there {
		lacking = append(lacking, cardFilter)
	}
	if len(lacking) == nothing {
		return nil
	}
	return fmt.Errorf("the header lacks %s", strings.Join(lacking, ", "))
}

// header is the cards of a FITS file's primary header, read from the
// start of an answer, up to most blocks of them. A page that says no
// image was made is errNoPlate; any other answer that is not FITS is an
// error.
func header(body io.Reader, most int) (map[string]string, int, error) {
	block, err := next(body)
	if err != nil {
		return nil, nothing, err
	}
	if !bytes.HasPrefix(block, []byte(simple)) {
		if bytes.Contains(block, []byte(noPlate)) {
			return nil, nothing, errNoPlate
		}
		return nil, nothing, errors.New("the answer is neither a FITS " +
			"file nor the page that says there is no plate")
	}
	cards := map[string]string{}
	for read := firstBlock; ; read++ {
		done, err := ended(block, cards)
		if err != nil {
			return nil, nothing, err
		}
		if done {
			return cards, read, nil
		}
		if read == most {
			return nil, nothing, fmt.Errorf("no END card in %d blocks",
				most)
		}
		if block, err = next(body); err != nil {
			return nil, nothing, err
		}
		if len(block) < blockSize {
			return nil, nothing, errors.New("the header ends before " +
				"its END card")
		}
	}
}

// next is the next block of the answer, shorter if the answer ends. An
// answer that ends is not an error here.
func next(body io.Reader) ([]byte, error) {
	block := make([]byte, blockSize)
	got, err := io.ReadFull(body, block)
	if errors.Is(err, io.EOF) || errors.Is(err, io.ErrUnexpectedEOF) {
		err = nil
	}
	return block[:got], err
}

// ended adds the cards of a block that have values to cards, and says
// whether the END card was among them. A card whose value cannot be
// read is refused, by its key.
func ended(block []byte, cards map[string]string) (bool, error) {
	for at := firstCard; at+cardSize <= len(block); at += cardSize {
		card := string(block[at : at+cardSize])
		key := strings.TrimSpace(card[:keyEnd])
		if key == endCard {
			return true, nil
		}
		if card[keyEnd:valueAt] == equals {
			said, err := value(card[valueAt:])
			if err != nil {
				return false, fmt.Errorf("card %s: %w", key, err)
			}
			cards[key] = said
		}
	}
	return false, nil
}

// value is what a card's value says, without its padding. A string runs
// from its quote to the next quote that is not one of a pair: FITS
// writes a quote inside a string as two, read here as one.
//
// A string with no closing quote is refused. Any other value is the text
// before its comment.
func value(raw string) (string, error) {
	raw = strings.TrimSpace(raw)
	text, quoted := strings.CutPrefix(raw, quote)
	if !quoted {
		number, _, _ := strings.Cut(raw, comment)
		return strings.TrimSpace(number), nil
	}
	var kept strings.Builder
	for {
		before, after, found := strings.Cut(text, quote)
		if !found {
			return "", errors.New("a string with no closing quote")
		}
		kept.WriteString(before)
		if !strings.HasPrefix(after, quote) {
			return strings.TrimSpace(kept.String()), nil
		}
		kept.WriteString(quote)
		text = strings.TrimPrefix(after, quote)
	}
}
