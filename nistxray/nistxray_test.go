package nistxray_test

import (
	"bytes"
	"crypto/sha256"
	"errors"
	"fmt"
	"io"
	"os"
	"strconv"
	"strings"
	"testing"
	"testing/fstest"

	"github.com/janearc/celestial-data"
	"github.com/janearc/celestial-data/internal/replay"
	"github.com/janearc/celestial-data/mocks"
	"github.com/janearc/celestial-data/nistxray"
	"go.uber.org/mock/gomock"
)

// What the tests ask of New and its pages.
const (
	adapter = "nistxray" // defined: the archive's name
	// address is where NIST serves the database's pages.
	address = "https://physics.nist.gov/PhysRefData/XrayMassCoef/"
	// pages is every page the manifest records: the 92 element pages,
	// the 48 compound pages and the 4 pages of contents and constants.
	pages = 144 // defined: manifest.yaml, fetched 2026-11-11
	// doi names the Standard Reference Database.
	doi = "10.18434/T4D01F"
)

// Lead's page, the page the tests read in full detail. Every value is
// read from testdata/z82-a.html, a real fetch of 2026-11-11, and held
// to numbers.yaml beside the package; the line is the file's line.
const (
	leadURL = address + "ElemTab/z82.html" // defined: the page's address
	// leadBytes is the page's length as NIST served it.
	leadBytes = 12168 // defined: manifest.yaml, fetched 2026-11-11
	// leadSum is the sha256 of the page's content, its one Cloudflare
	// line removed, as the manifest holds it.
	leadSum = "0881af817c92b801dd3031a336b766d9d8cf98f9a291f7f0bc817c" + // defined: manifest.yaml
		"bbde1bd4fd"
	// leadWireA and leadWireB are the sha256 sums of the bytes of the
	// two recorded fetches of the page, as the wire brought them: they
	// differ, for Cloudflare rewrote a line of each.
	leadWireA = "21430c7a9153ceae39b168d07cbf056f9a296132036729162fd7c948d4c5befb" // defined: testdata/z82-a.html
	leadWireB = "4e6822c36386eaf6ffa167221778da66c59fe091d2ace7c79f886add9d6b30e9" // defined: testdata/z82-b.html
)

// The mark of the line Cloudflare injects into every page NIST serves,
// as the package's rule takes it: the whole line, and only a line that
// carries this.
const injected = "window.__CF$cv$params="

// held is one page the manifest records, as the test reads it: its
// path, its length and the sum of its content.
type held struct {
	path  string
	bytes int64
	sum   string
}

// pinned is every page the manifest beside the package records. The
// manifest's own reading is in the package; this is the test's own,
// to hold that reading to what New gives.
func pinned(t *testing.T) []held {
	t.Helper()
	raw, err := os.ReadFile("manifest.yaml")
	if err != nil {
		t.Fatal(err)
	}
	var pages []held
	for _, line := range strings.Split(string(raw), "\n") {
		marked := strings.TrimSpace(line)
		if named, ok := strings.CutPrefix(marked, "- path: "); ok {
			pages = append(pages, held{path: named})
			continue
		}
		if len(pages) == 0 {
			continue
		}
		last := &pages[len(pages)-1]
		if named, ok := strings.CutPrefix(marked, "bytes: "); ok {
			length, err := strconv.ParseInt(named, 10, 64)
			if err != nil {
				t.Fatalf("%s: %v", last.path, err)
			}
			last.bytes = length
		}
		if named, ok := strings.CutPrefix(marked, "sum: "); ok {
			last.sum = named
		}
	}
	if len(pages) != count {
		t.Fatalf("%d pages in the manifest, want %d", len(pages), count)
	}
	return pages
}

// count is the pages the manifest records. It's defined.
const count = 144

// numbers is the numbers file beside the package, read.
func numbers(t *testing.T) *celestialdata.Measured {
	t.Helper()
	raw, err := os.ReadFile("numbers.yaml")
	if err != nil {
		t.Fatal(err)
	}
	return celestialdata.NewMeasured("numbers.yaml", raw)
}

// value is a number from the numbers file beside the package.
func value(t *testing.T, name string) float64 {
	t.Helper()
	number, err := numbers(t).Get(name)
	if err != nil {
		t.Fatal(err)
	}
	return number
}

// recorded is a page of the testdata, as the wire brought it.
func recorded(t *testing.T, file string) []byte {
	t.Helper()
	raw, err := os.ReadFile("testdata/" + file)
	if err != nil {
		t.Fatal(err)
	}
	return raw
}

// folder is the pages of the testdata as a folder, each under the
// name the archive's fetch would keep it by: from is the testdata
// file, as the pages the tests read are z82-a.html and its kind, and
// under is the name it takes in the folder.
func folder(pages ...[2]string) fstest.MapFS {
	made := fstest.MapFS{}
	for _, each := range pages {
		made[each[1]] = &fstest.MapFile{Data: []byte(each[0])}
	}
	return made
}

// The archive is the pages the manifest records: 92 element pages, 48
// compound pages and the 4 pages of contents and constants, each with
// the address the manifest's path gives it and the length the manifest
// measured. A fetch of a few bytes is refused, naming that length, and
// a fetch of the right length and another content is refused, naming
// the sum the page is held to, and neither leaves anything behind.
func TestNewHoldsEveryPinnedPageToItsSizeAndSum(t *testing.T) {
	manifest := pinned(t)
	archive, err := nistxray.New()
	if err != nil {
		t.Fatal(err)
	}
	found, err := archive.Find(replay.Asked(t))
	if err != nil || len(found) != pages {
		t.Fatalf("%d products, %v", len(found), err)
	}
	byURL := map[string]held{}
	for at, product := range found {
		if product.Archive() != adapter {
			t.Errorf("%s: archive %s", product.URL(), product.Archive())
		}
		if !strings.Contains(product.About(), "Secretary of Commerce") ||
			!strings.Contains(product.About(), "All rights reserved") ||
			!strings.Contains(product.About(), doi) {
			t.Errorf("%s: about %q", product.URL(), product.About())
		}
		if product.URL() != address+manifest[at].path {
			t.Errorf("product %d: %s, want %s", at, product.URL(),
				address+manifest[at].path)
		}
		byURL[product.URL()] = manifest[at]
	}
	// the length: a fetch of a few bytes is refused, naming the length
	// the manifest measured
	for _, each := range manifest {
		want := fmt.Sprintf("the archive said %d", each.bytes)
		err := replay.Fetched(t, byAddress(t, found, address+each.path))
		if err == nil || !strings.Contains(err.Error(), want) {
			t.Errorf("%s: %v, not %q", each.path, err, want)
		}
	}
	// the sum: a fetch of the right length and another content is
	// refused, naming the sum of the page's content
	get := mocks.NewMockWeb(gomock.NewController(t))
	get.EXPECT().Get(gomock.Any()).Times(pages).DoAndReturn(
		func(url string) (io.ReadCloser, int64, error) {
			length := byURL[url].bytes
			junk := bytes.Repeat([]byte("not the page NIST serves"),
				int(length)/20+1)
			junk = junk[:length]
			return io.NopCloser(bytes.NewReader(junk)), length, nil
		})
	into := t.TempDir()
	for _, product := range found {
		_, err := celestialdata.Fetch(get, product, into)
		pin := byURL[product.URL()]
		if err == nil {
			t.Fatalf("%s: a body of other content came", product.URL())
		}
		if !strings.Contains(err.Error(), "content sha256") ||
			!strings.Contains(err.Error(), pin.sum) {
			t.Errorf("%s: %v", product.URL(), err)
		}
	}
	if held, readErr := os.ReadDir(into); readErr != nil || len(held) != 0 {
		t.Errorf("left behind %v, %v", held, readErr)
	}
}

// byAddress is the product a page of the manifest is held by.
func byAddress(t *testing.T, found []*celestialdata.Product,
	url string) *celestialdata.Product {
	t.Helper()
	for _, product := range found {
		if product.URL() == url {
			return product
		}
	}
	t.Fatalf("no product for %s", url)
	return nil
}

// The rule the pages' sums are of: the two recorded fetches of lead's
// page differ on the wire — Cloudflare rewrote a line of each — and
// agree in the sum of their content, which is the manifest's sum. The
// differing line is the only line that carries Cloudflare's mark, in
// both. This is the evidence the rule earns its keep.
func TestTheTwoRecordedFetchesDifferOnTheWireAndAgreeInContent(t *testing.T) {
	a, b := recorded(t, "z82-a.html"), recorded(t, "z82-b.html")
	wireA, wireB := sha256.Sum256(a), sha256.Sum256(b)
	if bytes.Equal(a, b) || wireA == wireB {
		t.Fatalf("the two fetches came out the same, %x and %x", wireA,
			wireB)
	}
	if sum := nistxray.ContentOf(a); sum.String() != leadSum {
		t.Errorf("a: content sha256 %s, want %s", sum, leadSum)
	}
	if sum := nistxray.ContentOf(b); sum.String() != leadSum {
		t.Errorf("b: content sha256 %s, want %s", sum, leadSum)
	}
	linesA, linesB := bytes.Split(a, []byte("\n")), bytes.Split(b, []byte("\n"))
	if len(linesA) != len(linesB) {
		t.Fatalf("%d and %d lines", len(linesA), len(linesB))
	}
	var differing []int
	for at := range linesA {
		if !bytes.Equal(linesA[at], linesB[at]) {
			differing = append(differing, at)
		}
	}
	if len(differing) != 1 {
		t.Fatalf("the fetches differ in %d lines, %v", len(differing),
			differing)
	}
	for _, lines := range [][]byte{a, b} {
		marked := 0
		for _, line := range bytes.Split(lines, []byte("\n")) {
			if strings.Contains(string(line), injected) {
				marked++
			}
		}
		if marked != 1 {
			t.Fatalf("a page carries Cloudflare's mark in %d lines",
				marked)
		}
	}
	at := differing[0]
	if !strings.Contains(string(linesA[at]), injected) ||
		!strings.Contains(string(linesB[at]), injected) {
		t.Fatalf("line %d differs but carries no mark: %q", at,
			linesA[at])
	}
	// the wire sums, as recorded, for the report to name
	if sum := hexOf(wireA); sum != leadWireA {
		t.Errorf("a: sha256 of its bytes %s, want %s", sum, leadWireA)
	}
	if sum := hexOf(wireB); sum != leadWireB {
		t.Errorf("b: sha256 of its bytes %s, want %s", sum, leadWireB)
	}
}

// hexOf is the hex of a sha256 sum.
func hexOf(sum [sha256.Size]byte) string {
	const digits = "0123456789abcdef"
	text := make([]byte, sha256.Size*2)
	for at, each := range sum {
		text[at*2] = digits[each>>4]
		text[at*2+1] = digits[each&0xf]
	}
	return string(text)
}

// A fetch of the real page is kept, with the bytes the wire brought,
// Cloudflare's line among them, and the page reads back from the
// folder it was kept in.
func TestFetchKeepsTheRealPageAndItReadsBack(t *testing.T) {
	archive, err := nistxray.New()
	if err != nil {
		t.Fatal(err)
	}
	found, err := archive.Find(replay.Asked(t))
	if err != nil {
		t.Fatal(err)
	}
	var lead *celestialdata.Product
	for _, product := range found {
		if product.Name() == "z82.html" {
			lead = product
		}
	}
	if lead == nil {
		t.Fatal("no z82.html among the products")
	}
	get := replay.Web(t, map[string]string{leadURL: "z82-a.html"})
	into := t.TempDir()
	kept, err := celestialdata.Fetch(get, lead, into)
	if err != nil {
		t.Fatal(err)
	}
	raw, err := os.ReadFile(kept)
	if err != nil {
		t.Fatal(err)
	}
	// the file keeps the bytes as the wire brought them, Cloudflare's
	// line included: it is the content those bytes hold that is summed
	if !bytes.Equal(raw, recorded(t, "z82-a.html")) {
		t.Error("the kept page is not the page as fetched")
	}
	table, err := nistxray.Element(os.DirFS(into), 82)
	if err != nil {
		t.Fatal(err)
	}
	if table.Header() != "Lead\nZ = 82" || len(table.Rows()) == 0 {
		t.Errorf("header %q, %d rows", table.Header(), len(table.Rows()))
	}
}

// A fetch of lead's page with one figure of a row changed, and the
// length kept the same, is refused, names the sum the page is held to,
// and leaves nothing behind: neither the page nor its .part.
func TestAFetchOfOtherContentIsRefusedAndLeavesNothing(t *testing.T) {
	archive, err := nistxray.New()
	if err != nil {
		t.Fatal(err)
	}
	found, err := archive.Find(replay.Asked(t))
	if err != nil {
		t.Fatal(err)
	}
	var lead *celestialdata.Product
	for _, product := range found {
		if product.Name() == "z82.html" {
			lead = product
		}
	}
	if lead == nil {
		t.Fatal("no z82.html among the products")
	}
	// one figure of the first row, changed but of the same length
	other := bytes.ReplaceAll(recorded(t, "z82-a.html"),
		[]byte("5.210E+03"), []byte("5.211E+03"))
	if len(other) != leadBytes {
		t.Fatalf("%d bytes, want %d", len(other), leadBytes)
	}
	get := mocks.NewMockWeb(gomock.NewController(t))
	get.EXPECT().Get(leadURL).Return(io.NopCloser(bytes.NewReader(other)),
		int64(len(other)), nil)
	into := t.TempDir()
	_, err = celestialdata.Fetch(get, lead, into)
	if err == nil || !strings.Contains(err.Error(), "content sha256") ||
		!strings.Contains(err.Error(), leadSum) {
		t.Fatalf("fetch of other content: %v", err)
	}
	if held, readErr := os.ReadDir(into); readErr != nil || len(held) != 0 {
		t.Errorf("left behind %v, %v", held, readErr)
	}
}

// Lead, as the page the tests read records it: 58 rows, from the
// first at 1 keV to the last at 20 MeV, with the K edge among them.
// Every value is from numbers.yaml, which says the line of the page
// each was read from; the rows are the page's own, in its order.
func TestElementReadsLeadsRows(t *testing.T) {
	table, err := nistxray.Element(folder([2]string{string(
		recorded(t, "z82-a.html")), "z82.html"}), 82)
	if err != nil {
		t.Fatal(err)
	}
	if table.Header() != "Lead\nZ = 82" {
		t.Errorf("header %q", table.Header())
	}
	rows := table.Rows()
	if len(rows) != int(value(t, "lead-rows")) {
		t.Fatalf("%d rows, want %v", len(rows), value(t, "lead-rows"))
	}
	// line 445 of z82-a.html, the first row, at 1 keV
	first := nistxray.Row{Energy: value(t, "lead-first-energy"),
		MuOverRho:   value(t, "lead-first-mu"),
		MuEnOverRho: value(t, "lead-first-muen")}
	// line 446, the second row
	second := nistxray.Row{Energy: value(t, "lead-second-energy"),
		MuOverRho:   value(t, "lead-second-mu"),
		MuEnOverRho: value(t, "lead-second-muen")}
	// line 502, the last row, at 20 MeV
	last := nistxray.Row{Energy: value(t, "lead-last-energy"),
		MuOverRho:   value(t, "lead-last-mu"),
		MuEnOverRho: value(t, "lead-last-muen")}
	for _, each := range []struct {
		why  string
		got  nistxray.Row
		want nistxray.Row
	}{
		{"line 445, the first", rows[0], first},
		{"line 446, the second", rows[1], second},
		{"line 502, the last", rows[len(rows)-1], last},
	} {
		if each.got != each.want {
			t.Errorf("%s: %+v, want %+v", each.why, each.got, each.want)
		}
	}
	// line 482, the K edge row, the second measurement at the same
	// energy as the row before it, labelled K in the page's own column
	var edge nistxray.Row
	for _, row := range rows {
		if row.Edge == "K" {
			edge = row
		}
	}
	want := nistxray.Row{Edge: "K", Energy: value(t, "lead-k-edge-energy"),
		MuOverRho:   value(t, "lead-k-edge-mu"),
		MuEnOverRho: value(t, "lead-k-edge-muen")}
	if edge != want {
		t.Errorf("K edge row: %+v, want %+v", edge, want)
	}
	if rows[0].Edge != "" {
		t.Errorf("the first row carries the edge %q", rows[0].Edge)
	}
}

// Aluminum, as z13.html records it: 38 rows, with the K edge at
// 1.5596 keV, and the same shape as lead's.
func TestElementReadsAluminumsRows(t *testing.T) {
	table, err := nistxray.Element(folder([2]string{string(
		recorded(t, "z13.html")), "z13.html"}), 13)
	if err != nil {
		t.Fatal(err)
	}
	if table.Header() != "Aluminum\nZ = 13" {
		t.Errorf("header %q", table.Header())
	}
	rows := table.Rows()
	if len(rows) != int(value(t, "aluminum-rows")) {
		t.Fatalf("%d rows, want %v", len(rows), value(t, "aluminum-rows"))
	}
	// line 309 of z13.html, the first row
	first := nistxray.Row{Energy: value(t, "aluminum-first-energy"),
		MuOverRho:   value(t, "aluminum-first-mu"),
		MuEnOverRho: value(t, "aluminum-first-muen")}
	// line 346, the last row
	last := nistxray.Row{Energy: value(t, "aluminum-last-energy"),
		MuOverRho:   value(t, "aluminum-last-mu"),
		MuEnOverRho: value(t, "aluminum-last-muen")}
	if rows[0] != first || rows[len(rows)-1] != last {
		t.Errorf("first %+v last %+v; want %+v and %+v", rows[0],
			rows[len(rows)-1], first, last)
	}
	// line 312, the K edge row
	var edge nistxray.Row
	for _, row := range rows {
		if row.Edge == "K" {
			edge = row
		}
	}
	want := nistxray.Row{Edge: "K", Energy: value(t, "aluminum-k-edge-energy"),
		MuOverRho:   value(t, "aluminum-k-edge-mu"),
		MuEnOverRho: value(t, "aluminum-k-edge-muen")}
	if edge != want {
		t.Errorf("K edge row: %+v, want %+v", edge, want)
	}
}

// Water, as ComTab/water.html records it: 36 rows, none at an edge,
// for the water molecule's own edges lie below the table's first
// energy. Its header names it as Table 2 does: "Water, Liquid".
func TestCompoundReadsWatersRows(t *testing.T) {
	table, err := nistxray.Compound(folder([2]string{string(
		recorded(t, "water.html")), "water.html"}), "water")
	if err != nil {
		t.Fatal(err)
	}
	if table.Header() != "Water, Liquid" {
		t.Errorf("header %q", table.Header())
	}
	rows := table.Rows()
	if len(rows) != int(value(t, "water-rows")) {
		t.Fatalf("%d rows, want %v", len(rows), value(t, "water-rows"))
	}
	// line 281 of water.html, the first row
	first := nistxray.Row{Energy: value(t, "water-first-energy"),
		MuOverRho:   value(t, "water-first-mu"),
		MuEnOverRho: value(t, "water-first-muen")}
	// line 316, the last row
	last := nistxray.Row{Energy: value(t, "water-last-energy"),
		MuOverRho:   value(t, "water-last-mu"),
		MuEnOverRho: value(t, "water-last-muen")}
	if rows[0] != first || rows[len(rows)-1] != last {
		t.Errorf("first %+v last %+v; want %+v and %+v", rows[0],
			rows[len(rows)-1], first, last)
	}
	for _, row := range rows {
		if row.Edge != "" {
			t.Errorf("a row of water carries the edge %q", row.Edge)
		}
	}
}

// The material constants: lead's from Table 1, by its symbol, with
// its atomic number, Z/A and density and its row's own text; water's
// from Table 2, by the name the table writes, with its composition in
// its row's own text.
func TestConstantsReadLeadAndWater(t *testing.T) {
	consts := folder(
		[2]string{string(recorded(t, "tab1.html")), "tab1.html"},
		[2]string{string(recorded(t, "tab2.html")), "tab2.html"})
	lead, err := nistxray.Material(consts, "Pb")
	if err != nil {
		t.Fatal(err)
	}
	// lead's row of Table 1: 82 Pb Lead 0.39575 823.0 1.135E+01
	if lead.Z != 82 || lead.Symbol != "Pb" || lead.Name != "Lead" ||
		lead.OverA != value(t, "lead-overa") ||
		lead.Density != value(t, "lead-density") {
		t.Errorf("lead: %+v", lead)
	}
	if lead.Row != "82 Pb Lead 0.39575 823.0 1.135E+01" {
		t.Errorf("lead's row: %q", lead.Row)
	}
	water, err := nistxray.Material(consts, "Water, Liquid")
	if err != nil {
		t.Fatal(err)
	}
	// water's row of Table 2: Water, Liquid, 0.55508, 75.0 eV,
	// 1.000E+00 g/cm^3, and its composition
	if water.Z != 0 || water.Symbol != "" || water.Name != "Water, Liquid" ||
		water.OverA != value(t, "water-overa") ||
		water.Density != value(t, "water-density") {
		t.Errorf("water: %+v", water)
	}
	if !strings.Contains(water.Row, "1.000E+00") ||
		!strings.Contains(water.Row, "1: 0.111898") ||
		!strings.Contains(water.Row, "8: 0.888102") {
		t.Errorf("water's row: %q", water.Row)
	}
	// the first rows of the two tables, which the pages lay out with
	// ROWSPAN spacer cells between their values: hydrogen's row of
	// Table 1, 1 H Hydrogen 0.99212 19.2 8.375E-05, and A-150's row of
	// Table 2, A-150 Tissue-Equivalent Plastic 0.54903 65.1 1.127E+00
	hydrogen, err := nistxray.Material(consts, "H")
	if err != nil {
		t.Fatal(err)
	}
	if hydrogen.Z != 1 || hydrogen.Symbol != "H" ||
		hydrogen.Name != "Hydrogen" ||
		hydrogen.OverA != value(t, "hydrogen-overa") ||
		hydrogen.Density != value(t, "hydrogen-density") {
		t.Errorf("hydrogen: %+v", hydrogen)
	}
	a150, err := nistxray.Material(consts, "A-150 Tissue-Equivalent Plastic")
	if err != nil {
		t.Fatal(err)
	}
	if a150.Name != "A-150 Tissue-Equivalent Plastic" ||
		a150.OverA != value(t, "a150-overa") ||
		a150.Density != value(t, "a150-density") {
		t.Errorf("A-150: %+v", a150)
	}
}

// Anti-tests: the readers refuse what the archive does not hold, by
// name — an element beyond the tables, a compound the folder does not
// hold, a material neither constants table holds — and a page that is
// not a page of rows, and a row cut short, by the page's name.
func TestTheReadersRefuseWhatTheArchiveDoesNotHold(t *testing.T) {
	empty := folder()
	consts := folder(
		[2]string{string(recorded(t, "tab1.html")), "tab1.html"},
		[2]string{string(recorded(t, "tab2.html")), "tab2.html"})
	notRows := folder([2]string{string(recorded(t, "tab3.html")),
		"z82.html"})
	shortRow := folder([2]string{
		"<PRE>\n1.00000E-03  5.210E+03\n</PRE>", "z82.html"})
	for _, each := range []struct {
		why   string
		table func() error
		says  string
	}{
		{"no element 93", func() error {
			_, err := nistxray.Element(empty, 93)
			return err
		}, "nistxray holds no element Z 93"},
		{"no element 0", func() error {
			_, err := nistxray.Element(empty, 0)
			return err
		}, "nistxray holds no element Z 0"},
		{"no compound", func() error {
			_, err := nistxray.Compound(empty, "unobtainium")
			return err
		}, "nistxray holds no unobtainium.html"},
		{"no page for the element", func() error {
			_, err := nistxray.Element(empty, 82)
			return err
		}, "nistxray holds no z82.html"},
		{"no material", func() error {
			_, err := nistxray.Material(consts, "unobtainium")
			return err
		}, "nistxray holds no material unobtainium"},
		{"a page with no table", func() error {
			_, err := nistxray.Element(notRows, 82)
			return err
		}, "nistxray page z82.html holds no table of rows"},
		{"a row cut short", func() error {
			_, err := nistxray.Element(shortRow, 82)
			return err
		}, "nistxray page z82.html has a row of 2 numbers, not 3"},
	} {
		err := each.table()
		if !errors.Is(err, celestialdata.Refused()) ||
			!strings.Contains(err.Error(), each.says) {
			t.Errorf("%s: %v", each.why, err)
		}
	}
}

// New is a fixed archive of the database, asked only by name, and it
// finds the same for every circle.
func TestNewIsAFixedArchiveAskedOnlyByName(t *testing.T) {
	archive, err := nistxray.New()
	if err != nil {
		t.Fatal(err)
	}
	if archive.Name() != adapter || !archive.ByNameOnly() {
		t.Errorf("%q, by name only %v", archive.Name(),
			archive.ByNameOnly())
	}
	var _ celestialdata.ByName = archive
	held, err := archive.Find(replay.Asked(t))
	if err != nil || len(held) != pages {
		t.Fatalf("%d products, %v", len(held), err)
	}
	north, east := 60.0, 10.0 // defined: a far declination and right ascension
	pole, err := celestialdata.NewPosition(east, north)
	if err != nil {
		t.Fatal(err)
	}
	far, err := celestialdata.NewNear(pole, 0.1) // defined: a radius, in degrees
	if err != nil {
		t.Fatal(err)
	}
	for _, circle := range []*celestialdata.Near{far, replay.Asked(t)} {
		found, err := archive.Find(circle)
		if err != nil || len(found) != pages {
			t.Fatalf("%v, %v", found, err)
		}
		for at := range found {
			if *found[at] != *held[at] {
				t.Errorf("circle %v: page %d is not the page held",
					circle, at)
			}
		}
	}
}

// ParseContent refuses a text that is not a sha256 sum: one of the
// wrong length, and one not written in hex.
func TestParseContentRefusesWhatIsNotASum(t *testing.T) {
	for _, each := range []struct {
		why  string
		text string
		says string
	}{
		{"too short", "5f2a", "64 hex characters"},
		{"not hex", strings.Repeat("z", 64), "not written in hex"},
	} {
		_, err := nistxray.ParseContent(each.text)
		if !errors.Is(err, celestialdata.Refused()) ||
			!strings.Contains(err.Error(), each.says) {
			t.Errorf("%s: %v", each.why, err)
		}
	}
}
