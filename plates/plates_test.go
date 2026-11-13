package plates_test

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"testing/iotest"

	"go.uber.org/mock/gomock"

	"github.com/janearc/celestial-data"
	"github.com/janearc/celestial-data/internal/replay"
	"github.com/janearc/celestial-data/mocks"
	"github.com/janearc/celestial-data/plates"
)

// The place the recorded answers were asked at on 2026-11-06, and
// what they were asked.
const (
	adapter = "plates" // defined: the archive's name
	// recRA is the right ascension asked, in degrees. It's defined by
	// the recorded queries.
	recRA = 287.50086
	// recDec is the declination asked, in degrees. It's defined by
	// the recorded queries.
	recDec = -22.23481
	// recSmall is a radius whose square is under a side of one
	// arcminute. It's defined.
	recSmall = 0.005
	// overRA is recRA to more decimals than the address writes. It's
	// defined.
	overRA = 287.50086 + 4.9e-7
	// overDec is recDec to more decimals than the address writes.
	// It's defined.
	overDec = -22.23481 - 4e-6
	// recSize is the length of each recorded FITS answer, defined by
	// the Content-Length STScI gave.
	recSize = 66240
	// block is a FITS block, in bytes. It's defined by the FITS
	// standard.
	block = 2880
	// cardLen is a FITS card, in bytes. It's defined by the FITS
	// standard.
	cardLen = 80
	// blocks is the blocks of header the archive reads at most. It's
	// defined by header-blocks in numbers.yaml.
	blocks = 40
	none   = 0 // defined: no products, no bytes
	first  = 1 // defined: the first block, the first failing survey
	// second is the second block. It's defined.
	second = 2
	// unknown is the length web gives when none is said. It's defined
	// by web.Web.
	unknown = -1
	bits    = 64 // defined: the size of a float64
	// oneArcmin is the side of the recorded cutouts, as written.
	oneArcmin = "1.00"
	reading   = "https://archive.stsci.edu/cgi-bin/dss_search?"
	askedAt   = reading + "d=-22.23481&e=J2000&f=fits&h="
)

// addressOf is where a survey's cutout of a side is asked for.
func addressOf(survey, side string) string {
	return askedAt + side + "&r=287.50086&v=" + survey + "&w=" + side
}

// the surveys, in the order they are asked.
var surveys = []string{"poss1_blue", "poss1_red", "poss2ukstu_blue",
	"poss2ukstu_red", "poss2ukstu_ir", "quickv"}

// near is the circle of a radius round a place, or the test stops.
func near(test *testing.T, ra, dec, radius float64) *celestialdata.Near {
	test.Helper()
	centre, err := celestialdata.NewPosition(ra, dec)
	if err != nil {
		test.Fatal(err)
	}
	circle, err := celestialdata.NewNear(centre, radius)
	if err != nil {
		test.Fatal(err)
	}
	return circle
}

// recorded is the circle the answers were recorded for.
func recorded(test *testing.T) *celestialdata.Near {
	test.Helper()
	return near(test, recRA, recDec, recSmall)
}

// made is the archive asked through get, or the test stops.
func made(test *testing.T, get *mocks.MockWeb) *plates.Archive {
	test.Helper()
	archive, err := plates.New(get)
	if err != nil {
		test.Fatal(err)
	}
	return archive
}

// recordedAll is the answers STScI gave at the recorded place.
var recordedAll = map[string]string{
	"poss1_blue":      "poss1_blue.fits",
	"poss1_red":       "poss1_red.fits",
	"poss2ukstu_blue": "noplate-poss2ukstu_blue.html",
	"poss2ukstu_red":  "poss2ukstu_red.fits",
	"poss2ukstu_ir":   "poss2ukstu_ir.fits",
	"quickv":          "noplate-quickv.html",
}

// recordedWeb is a web that answers each survey's recorded address
// with the recorded file given for it, or the test stops.
func recordedWeb(test *testing.T, files map[string]string) *mocks.MockWeb {
	test.Helper()
	replies := map[string]string{}
	for survey, file := range files {
		replies[addressOf(survey, oneArcmin)] = file
	}
	return replay.Web(test, replies)
}

// answering is a web that answers one address with content and the
// length given, or the test stops.
func answering(test *testing.T, address string, content io.Reader,
	length int64) *mocks.MockWeb {
	test.Helper()
	get := mocks.NewMockWeb(gomock.NewController(test))
	get.EXPECT().Get(address).Return(io.NopCloser(content), length, nil)
	return get
}

// saying is an answer that is the same for every address.
type saying struct {
	content []byte
	length  int64
}

// get is the answer, whatever the address.
func (said saying) get(string) (io.ReadCloser, int64, error) {
	return io.NopCloser(bytes.NewReader(said.content)), said.length, nil
}

// servingAll is a web that answers every survey's address with the
// content and the length given, or the test stops.
func servingAll(test *testing.T, content []byte,
	length int64) *mocks.MockWeb {
	test.Helper()
	get := mocks.NewMockWeb(gomock.NewController(test))
	said := saying{content: content, length: length}
	get.EXPECT().Get(gomock.Any()).DoAndReturn(said.get).Times(
		len(surveys))
	return get
}

// noting is a page that says there is no plate, given to every
// address, and the addresses it was asked for.
type noting struct {
	page      []byte
	addresses []string
}

// get notes the address and answers with the page.
func (noted *noting) get(address string) (io.ReadCloser, int64, error) {
	noted.addresses = append(noted.addresses, address)
	return io.NopCloser(bytes.NewReader(noted.page)), int64(none), nil
}

// the test: the archive is named, asked by name only, and refuses no
// web.
func TestTheArchiveIsAskedByName(test *testing.T) {
	get := mocks.NewMockWeb(gomock.NewController(test))
	archive := made(test, get)
	if archive.Name() != adapter || !archive.ByNameOnly() {
		test.Errorf("%q, by name only %v", archive.Name(),
			archive.ByNameOnly())
	}
	if nowhere, err := plates.New(nil); nowhere != nil || err == nil {
		test.Errorf("no web: %v, %v", nowhere, err)
	}
}

// plate is a product as a search should give it.
type plate struct{ survey, id, name, about string }

// the test: a search where STScI has four plates and no more finds the
// four, each named for its survey, plate, date and place, and says the
// plate's id, label, telescope, emulsion, filter, exposure and date;
// the two surveys that have none are left out.
func TestASearchNamesEachPlate(test *testing.T) {
	archive := made(test, recordedWeb(test, recordedAll))
	products, err := archive.Find(recorded(test))
	if err != nil {
		test.Fatal(err)
	}
	place := "_ra287.50086_dec-22.23481_1.00arcmin.fits"
	wanted := []plate{
		{"poss1_blue", "A3VA", "poss1_blue_A3VA_1954-07-30" + place,
			"POSSI-O plate A3VA (O1155), Palomar Schmidt, 103aO, " +
				"12.0 minutes, 1954-07-30T07:05:00"},
		{"poss1_red", "A2M9", "poss1_red_A2M9_1954-07-30" + place,
			"POSSI-E plate A2M9 (E1155), Palomar Schmidt, " +
				"103aE through plexi, 50.0 minutes, " +
				"1954-07-30T06:15:00"},
		{"poss2ukstu_red", "A2QU",
			"poss2ukstu_red_A2QU_1998-05-27" + place,
			"AAO-GR plate A2QU (OR18024), UK Schmidt - Doubl, " +
				"IIIaF through OG590, 5.0 minutes, " +
				"1998-05-27T19:07:00"},
		{"poss2ukstu_ir", "A38B",
			"poss2ukstu_ir_A38B_1997-09-09" + place,
			"SERC-I plate A38B (I17718), UK Schmidt - Doubl, " +
				"IVN through RG715, 90.0 minutes, " +
				"1997-09-09T10:08:00"}}
	if len(products) != len(wanted) {
		test.Fatalf("%d products", len(products))
	}
	for at, want := range wanted {
		product := products[at]
		about := want.about + "; the " + want.survey +
			" cutout, 1.00 arcminutes square"
		address := addressOf(want.survey, oneArcmin)
		if product.ID() != want.id || product.Name() != want.name ||
			product.URL() != address || product.About() != about ||
			product.Archive() != adapter {
			test.Errorf("%s %s %s %q", product.ID(), product.Name(),
				product.URL(), product.About())
		}
	}
}

// the anti-test: where every survey answers with the page that says
// no image was made, a search finds nothing and is not an error; no
// page is kept as a FITS file.
func TestASearchWhereThereIsNoPlateFindsNone(test *testing.T) {
	pages := map[string]string{}
	for _, survey := range surveys {
		pages[survey] = "noplate-quickv.html"
	}
	archive := made(test, recordedWeb(test, pages))
	found, err := archive.Find(recorded(test))
	if found != nil || err != nil {
		test.Errorf("%v, %v", found, err)
	}
}

// side is a radius and the side the cutout is asked at.
type side struct {
	radius float64
	asked  string
}

// sides are the radii and sides recorded in testdata/sides.txt, or the
// test stops.
func sides(test *testing.T) []side {
	test.Helper()
	text, err := os.ReadFile("testdata/sides.txt")
	if err != nil {
		test.Fatal(err)
	}
	var rows []side
	for _, line := range strings.Split(string(text), "\n") {
		fields := strings.Fields(line)
		if len(fields) == none || strings.HasPrefix(line, "#") {
			continue
		}
		radius, err := strconv.ParseFloat(fields[none], bits)
		if err != nil {
			test.Fatal(err)
		}
		rows = append(rows, side{radius, fields[first]})
	}
	return rows
}

// the test: a cutout is a square twice the radius a side, no less than
// one arcminute and no more than sixty, asked of each survey in turn;
// the centre is written to five decimals of a degree.
func TestACutoutIsSizedByTheRadius(test *testing.T) {
	page, err := os.ReadFile("testdata/noplate-quickv.html")
	if err != nil {
		test.Fatal(err)
	}
	cases := sides(test)
	cases = append(cases, side{recSmall, oneArcmin})
	for _, each := range cases {
		noted := &noting{page: page}
		get := mocks.NewMockWeb(gomock.NewController(test))
		get.EXPECT().Get(gomock.Any()).DoAndReturn(noted.get).Times(
			len(surveys))
		centre := near(test, overRA, overDec, each.radius)
		if found, err := made(test, get).Find(centre); err != nil ||
			found != nil || len(noted.addresses) != len(surveys) {
			test.Fatalf("%v, %v, %v", found, err, noted.addresses)
		}
		for at, survey := range surveys {
			want := addressOf(survey, each.asked)
			if noted.addresses[at] != want {
				test.Errorf("radius %v: %s", each.radius,
					noted.addresses[at])
			}
		}
	}
}

// the test: a plate found is fetched whole, as the bytes STScI sent,
// with its header cards as they were.
func TestAPlateIsFetchedWhole(test *testing.T) {
	archive := made(test, recordedWeb(test, recordedAll))
	products, err := archive.Find(recorded(test))
	if err != nil {
		test.Fatal(err)
	}
	found := products[none]
	get := recordedWeb(test, map[string]string{
		"poss1_blue": "poss1_blue.fits"})
	home, err := celestialdata.Fetch(get, found, test.TempDir())
	if err != nil {
		test.Fatal(err)
	}
	got, err := os.ReadFile(home)
	want, wantErr := os.ReadFile("testdata/poss1_blue.fits")
	if err != nil || wantErr != nil || !bytes.Equal(got, want) ||
		len(got) != recSize || filepath.Base(home) != found.Name() {
		test.Errorf("%s: %d bytes, %v, %v", home, len(got), err,
			wantErr)
	}
}

// the anti-test: where the address answers with the page that says no
// image was made, the plate found is not kept as a FITS file: its
// length is not the one the plate had, and nothing is left behind.
func TestAPageIsNotKeptAsAPlate(test *testing.T) {
	archive := made(test, recordedWeb(test, recordedAll))
	products, err := archive.Find(recorded(test))
	if err != nil {
		test.Fatal(err)
	}
	found := products[none]
	get := recordedWeb(test, map[string]string{
		"poss1_blue": "noplate-quickv.html"})
	into := test.TempDir()
	home, err := celestialdata.Fetch(get, found, into)
	left, _ := os.ReadDir(into)
	if err == nil || home != "" || len(left) != none ||
		!strings.Contains(err.Error(), "bytes came") {
		test.Errorf("%q, %v, %d left", home, err, len(left))
	}
}

// fetchedAs is what a fetch of a plate found is answered with: the
// length Find was told, the length the fetch is told, the bytes, and
// what the refusal says.
type fetchedAs struct {
	found, came int64
	content     []byte
	said        string
}

// the anti-test: what is fetched is held to the plate Find read there,
// not to a length alone. A page where the length was unsaid, a page of
// the plate's own length, and another plate of the same length are
// each refused, and nothing is left; when the length was the only
// check, each was kept under the plate's name.
func TestOnlyThePlateFoundIsKept(test *testing.T) {
	plate := fitsOf(plateCards(test, nil)...)
	page := []byte(fmt.Sprintf("%-*s", len(plate), "<html>busy</html>"))
	other := fitsOf(plateCards(test, map[string]string{
		"PLATEID": "'ZZZZ'"})...)
	length := int64(len(plate))
	headerOnly := fitsOf(imagedCards(test)...)
	for name, each := range map[string]fetchedAs{
		"the plate's header without its image": {unknown, unknown,
			headerOnly, "short of the 5760 of the header and the image"},
		"a page, its length unsaid": {unknown, unknown, page,
			"neither a FITS file"},
		"a page of the plate's length": {length, length, page,
			"neither a FITS file"},
		"another plate of its length": {length, length, other,
			`plate "ZZZZ" came, not "A3VA"`},
	} {
		products, err := made(test, servingAll(test, plate, each.found)).
			Find(recorded(test))
		if err != nil || len(products) == none {
			test.Fatalf("%s: %d found, %v", name, len(products), err)
		}
		found := products[none]
		into := test.TempDir()
		get := answering(test, found.URL(), bytes.NewReader(each.content),
			each.came)
		home, err := celestialdata.Fetch(get, found, into)
		left, _ := os.ReadDir(into)
		if err == nil || home != "" || len(left) != none ||
			!strings.Contains(err.Error(), each.said) {
			test.Errorf("%s: %q, %v, %d left", name, home, err, len(left))
		}
	}
}

// The image the crafted plates declare, each defined: ten by ten
// pixels of two bytes, 200 bytes, a block once padded.
const (
	imageSide  = "10"
	imageBlock = block
)

// imagedCards are the plate's cards and an image's: BITPIX 16, two axes
// of imageSide.
func imagedCards(test *testing.T) []string {
	test.Helper()
	return append(plateCards(test, nil), card("BITPIX", "16"),
		card("NAXIS", "2"), card("NAXIS1", imageSide),
		card("NAXIS2", imageSide))
}

// the test: a plate fetched with the image its header declares is kept,
// though the server says no length; the counterpart of the plate's
// header alone, which is refused.
func TestAPlateWithItsImageIsKept(test *testing.T) {
	plate := fitsOf(plateCards(test, nil)...)
	products, err := made(test, servingAll(test, plate, unknown)).
		Find(recorded(test))
	if err != nil || len(products) == none {
		test.Fatalf("%d found, %v", len(products), err)
	}
	found := products[none]
	whole := append(fitsOf(imagedCards(test)...),
		make([]byte, imageBlock)...)
	get := answering(test, found.URL(), bytes.NewReader(whole), unknown)
	home, err := celestialdata.Fetch(get, found, test.TempDir())
	if err != nil || home == "" {
		test.Errorf("%q, %v", home, err)
	}
}

// the test: a server that says no length leaves the plate's size
// unsaid, and the plates are still found.
func TestAnUnsaidLengthIsNotASize(test *testing.T) {
	cards := plateCards(test, nil)
	get := servingAll(test, fitsOf(cards...), unknown)
	found, err := made(test, get).Find(recorded(test))
	if len(found) != len(surveys) || err != nil {
		test.Errorf("%d found, %v", len(found), err)
	}
}

// the test: a card's value is read as FITS writes it. A string is what
// lies between its quotes, whatever it holds, with a quote inside it
// written as two, where the string was cut at the first; a number is
// what lies before its comment; a line that is not a keyword with a
// value, a comment line or one that only looks like one, says nothing.
func TestACardIsReadAsFITSWritesIt(test *testing.T) {
	cards := plateCards(test, map[string]string{
		"PLTLABEL": "'O/1155 ' / the label, with a slash in it",
		"TELESCOP": "'O''Neil Schmidt'",
		"EMULSION": "'103aO   '", "FILTER": "'OG590   '"})
	cards = append(cards,
		fmt.Sprintf("%-*s", cardLen, "PLATEID   'ZZZZ'"),
		fmt.Sprintf("%-*s", cardLen, "COMMENT  EXPOSURE = 99"))
	get := servingAll(test, fitsOf(cards...), int64(block))
	found, err := made(test, get).Find(recorded(test))
	if err != nil || len(found) != len(surveys) {
		test.Fatalf("%d found, %v", len(found), err)
	}
	want := "POSSI-O plate A3VA (O/1155), O'Neil Schmidt, 103aO " +
		"through OG590, 12.0 minutes, 1954-07-30T07:05:00; the " +
		"poss1_blue cutout, 1.00 arcminutes square"
	if found[none].ID() != "A3VA" || found[none].About() != want {
		test.Errorf("%q, %q", found[none].ID(), found[none].About())
	}
}

// the test: a header may run to the last block read, and no further.
func TestAHeaderMayRunToTheBlocksRead(test *testing.T) {
	cards := plateCards(test, nil)
	for _, ends := range []int{first, second, blocks} {
		content := fitsEndingIn(ends, cards...)
		length := int64(len(content))
		get := servingAll(test, content, length)
		found, err := made(test, get).Find(recorded(test))
		if err != nil || len(found) != len(surveys) {
			test.Errorf("ends in block %d: %d found, %v", ends,
				len(found), err)
		}
	}
}

// the test: the last card of a block is read, so an END card that
// fills the block's last place ends the header there.
func TestTheLastCardOfABlockIsRead(test *testing.T) {
	cards := plateCards(test, nil)
	head := card("SIMPLE", "T") + strings.Join(cards, "")
	room := block - cardLen
	end := fmt.Sprintf("%-*s", cardLen, "END")
	content := []byte(fmt.Sprintf("%-*s", room, head) + end)
	get := servingAll(test, content, int64(block))
	found, err := made(test, get).Find(recorded(test))
	if err != nil || len(found) != len(surveys) {
		test.Errorf("%d found, %v", len(found), err)
	}
}

// errBroken is why a read fails.
var errBroken = errors.New("link dropped")

// failure is an answer the archive cannot read, and what it should say.
type failure struct {
	name    string
	content io.Reader
	said    string
}

// failures are the answers that are not read: nothing at all, another
// page, a header that ends early or never, a header that lacks a card,
// a plate id that is no file name, and a read that fails in the first
// block or a later one.
func failures(test *testing.T) []failure {
	test.Helper()
	cards := plateCards(test, nil)
	twoBlocks := fitsEndingIn(second, cards...)
	cut := twoBlocks[:block+cardLen]
	never := fitsEndingIn(blocks+first, cards...)
	stub := strings.NewReader(card("SIMPLE", "T"))
	later := io.MultiReader(bytes.NewReader(twoBlocks[:block]),
		iotest.ErrReader(errBroken))
	return []failure{
		{"nothing", strings.NewReader(""), "neither a FITS file"},
		{"a page", strings.NewReader("<html>busy</html>"),
			"neither a FITS file"},
		{"cut in the first block", stub, "ends before its END card"},
		{"cut in a later block", bytes.NewReader(cut),
			"ends before its END card"},
		{"never ends", bytes.NewReader(never),
			"no END card in 40 blocks"},
		{"no plate id", plateOf(test, map[string]string{
			"PLATEID": "-"}), "lacks PLATEID"},
		{"no filter card", plateOf(test, map[string]string{
			"FILTER": "-"}), "lacks FILTER"},
		{"a blank exposure", plateOf(test, map[string]string{
			"EXPOSURE": "' '"}), "lacks EXPOSURE"},
		{"two lacking", plateOf(test, map[string]string{"PLATEID": "-",
			"FILTER": "-"}), "lacks PLATEID, FILTER"},
		{"an id with a slash", plateOf(test, map[string]string{
			"PLATEID": "'A/3VA'"}), "plain"},
		{"a string never closed", plateOf(test, map[string]string{
			"EMULSION": "'103aO"}),
			"card EMULSION: a string with no closing quote"},
		{"a read that fails", iotest.ErrReader(errBroken),
			errBroken.Error()},
		{"a later read that fails", later, errBroken.Error()}}
}

// the anti-test: an answer the archive cannot read as a plate or as the
// page that says there is none is refused, by the archive's name and
// the survey's, saying why.
func TestAnAnswerNotReadIsRefused(test *testing.T) {
	for _, each := range failures(test) {
		address := addressOf(surveys[none], oneArcmin)
		get := answering(test, address, each.content, int64(block))
		found, err := made(test, get).Find(recorded(test))
		prefix := adapter + ": " + surveys[none] + ": "
		if found != nil || err == nil ||
			!strings.HasPrefix(err.Error(), prefix) ||
			!strings.Contains(err.Error(), each.said) {
			test.Errorf("%s: %v, %v", each.name, found, err)
		}
	}
}

// the anti-test: a survey the web cannot answer stops the search, by
// the archive's name and the survey's, whether it is the first asked
// or a later one, and nothing found before it is given.
func TestASurveyTheWebCannotAnswerIsRefused(test *testing.T) {
	unreachable := errors.New("no route to STScI")
	content := fitsOf(plateCards(test, nil)...)
	for _, failing := range []int{none, first} {
		get := mocks.NewMockWeb(gomock.NewController(test))
		for at := range failing {
			address := addressOf(surveys[at], oneArcmin)
			plate := bytes.NewReader(content)
			get.EXPECT().Get(address).Return(io.NopCloser(plate),
				int64(block), nil)
		}
		address := addressOf(surveys[failing], oneArcmin)
		get.EXPECT().Get(address).Return(nil, int64(none), unreachable)
		found, err := made(test, get).Find(recorded(test))
		wanted := adapter + ": " + surveys[failing] + ": "
		if found != nil || !errors.Is(err, unreachable) ||
			!strings.HasPrefix(err.Error(), wanted) {
			test.Errorf("%d: %v, %v", failing, found, err)
		}
	}
}

// plateCards are the cards of the plate in testdata/plate.cards, those
// in change replacing the value of a card of the same keyword, and a
// value of "-" leaving the card out; or the test stops.
func plateCards(test *testing.T, change map[string]string) []string {
	test.Helper()
	text, err := os.ReadFile("testdata/plate.cards")
	if err != nil {
		test.Fatal(err)
	}
	var cards []string
	trimmed := strings.TrimSpace(string(text))
	for _, line := range strings.Split(trimmed, "\n") {
		key, value, _ := strings.Cut(line, " ")
		if changed, ok := change[key]; ok {
			value = changed
		}
		if value != "-" {
			cards = append(cards, card(key, value))
		}
	}
	return cards
}

// plateOf is the header of the plate, changed as plateCards changes
// it, to be read.
func plateOf(test *testing.T, change map[string]string) io.Reader {
	test.Helper()
	cards := plateCards(test, change)
	content := fitsOf(cards...)
	return bytes.NewReader(content)
}

// card is one header card as FITS writes it: a keyword, "= " and the
// value text, padded to 80 characters.
func card(key, value string) string {
	return fmt.Sprintf("%-8s= %-70s", key, value)
}

// fitsOf is a FITS header of the cards, ended in its first block.
func fitsOf(cards ...string) []byte {
	return fitsEndingIn(first, cards...)
}

// fitsEndingIn is a FITS header of ends blocks: the cards in the
// first, blank blocks after it, and the END card at the start of the
// last, or after the cards when there is one block.
func fitsEndingIn(ends int, cards ...string) []byte {
	each := make([]string, ends)
	each[none] = card("SIMPLE", fmt.Sprintf("%20s", "T")) +
		strings.Join(cards, "")
	each[ends-first] += fmt.Sprintf("%-*s", cardLen, "END")
	for at := range each {
		each[at] = fmt.Sprintf("%-*s", block, each[at])
	}
	return []byte(strings.Join(each, ""))
}

// the anti-test: a header that names the plate is still refused when
// it declares an image the standard does not allow, or one the file
// cannot hold, or when the file is a byte short of whole blocks; so is
// the page that says there is no plate, padded to whole blocks.
//
// No test before this one sent any of them, so a refusal taken away
// would have gone unseen. The counterpart, a header of no axes, is
// kept: its image is none.
func TestAPlateIsHeldToTheImageItDeclares(test *testing.T) {
	const oneByte = 1 // defined: the least a file can fall short by
	plate := fitsOf(plateCards(test, nil)...)
	page, err := os.ReadFile("testdata/noplate-quickv.html")
	if err != nil {
		test.Fatal(err)
	}
	with := func(cards ...string) []byte {
		return fitsOf(append(plateCards(test, nil), cards...)...)
	}
	products, err := made(test, servingAll(test, plate, unknown)).
		Find(recorded(test))
	if err != nil || len(products) == none {
		test.Fatalf("%d found, %v", len(products), err)
	}
	found := products[none]
	for name, each := range map[string]fetchedAs{
		"the page that says there is no plate": {unknown, unknown,
			[]byte(fmt.Sprintf("%-*s", block, page)),
			"the page that says there is no plate came"},
		"the plate a byte short of whole blocks": {unknown, unknown,
			plate[:len(plate)-oneByte],
			"bytes is not whole FITS blocks"},
		"a BITPIX the standard does not allow": {unknown, unknown,
			with(card("BITPIX", "12")),
			`BITPIX "12" is not a pixel's`},
		"axes that are not a count": {unknown, unknown,
			with(card("BITPIX", "16"), card("NAXIS", "x")),
			`NAXIS "x" is not a count of axes`},
		"more axes than the standard allows": {unknown, unknown,
			with(card("BITPIX", "16"), card("NAXIS", "1000")),
			`NAXIS "1000" is not a count of axes`},
		"an axis below none": {unknown, unknown,
			with(card("BITPIX", "16"), card("NAXIS", "1"),
				card("NAXIS1", "-1")),
			`NAXIS1 "-1" is not a length`},
		"an image longer than the file": {unknown, unknown,
			with(card("BITPIX", "16"), card("NAXIS", "1"),
				card("NAXIS1", "100000")),
			"the image NAXIS1 declares is longer than the file"},
	} {
		into := test.TempDir()
		get := answering(test, found.URL(),
			bytes.NewReader(each.content), each.came)
		home, err := celestialdata.Fetch(get, found, into)
		left, _ := os.ReadDir(into)
		if err == nil || home != "" || len(left) != none ||
			!strings.Contains(err.Error(), each.said) {
			test.Errorf("%s: %q, %v, %d left", name, home, err,
				len(left))
		}
	}
	noAxes := with(card("BITPIX", "16"), card("NAXIS", "0"))
	get := answering(test, found.URL(), bytes.NewReader(noAxes),
		unknown)
	home, err := celestialdata.Fetch(get, found, test.TempDir())
	if err != nil || home == "" {
		test.Errorf("a header of no axes: %q, %v", home, err)
	}
}

// the anti-test: a check of a file that is not there says so, and says
// it as the file system does, where it was taken for a plate.
func TestAPlateThatIsNotThereIsNotAPlate(test *testing.T) {
	missing := filepath.Join(test.TempDir(), "not-there.fits")
	err := plates.CheckPlate("A3VA", blocks, missing)
	if !errors.Is(err, os.ErrNotExist) {
		test.Errorf("a missing file said %v", err)
	}
}
