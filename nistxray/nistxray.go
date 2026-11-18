// Package nistxray fetches and reads NIST X-Ray Mass Attenuation
// Coefficients, Standard Reference Database 126, and the tables of
// Hubbell and Seltzer.
//
// Its tables give mass attenuation μ/ρ and mass energy-absorption
// μen/ρ coefficients for every element Z = 1 to 92, plus 48 compounds
// and mixtures.
//
// The tables cover materials of dosimetric interest, from 1 keV to
// 20 MeV. NIST last updated the data in July 2004. The archive is the
// pages themselves, asked by page: an element's, a compound's, and the
// four pages of contents and material constants that go with them.
//
// The data is not open. In NIST's own words on the database's page:
// "© 1989, 1990, 1996 copyright by the U.S. Secretary of Commerce on
// behalf of the United States of America. All rights reserved. NIST
// reserves the right to charge for these data in the future."
//
// Work using it cites "Hubbell, J.H. and Seltzer, S.M. (2004), Tables
// of X-Ray Mass Attenuation Coefficients and Mass Energy-Absorption
// Coefficients (version 1.4). [Online] Available:
// http://physics.nist.gov/xaamdi", doi 10.18434/T4D01F.
//
// NIST serves each page through Cloudflare, which appends to every
// response a script of its own. The one line carrying
// "window.__CF$cv$params=" holds a per-request ray id and timestamp,
// so no two fetches of the same page have the same bytes.
//
// A page is pinned by the sum of its content, not its bytes: its content
// is the page as NIST serves it with that one line removed, and nothing
// else removed. The rule takes whole lines, and only those with
// Cloudflare's mark.
//
// Any other change that NIST or Cloudflare makes leaves the sum wrong
// and the fetch refused, never silently passed. The rule was proved
// when the pages were pinned: each was fetched twice on 2026-11-11.
//
// Every pair differed on the wire in exactly that one line while the
// sums of their content agreed. Two fetches are kept in testdata as
// z82-a.html and z82-b.html, and a test reads them: that test is the
// evidence the rule earns its keep.
package nistxray

import (
	"bytes"
	"crypto/sha256"
	_ "embed"
	"encoding/hex"
	"fmt"
	"os"
	"path"
	"strconv"
	"strings"

	"github.com/janearc/celestial-data"
)

// adapterName is the archive's name, as the command line takes it.
const adapterName = "nistxray"

// The database, as NIST serves it.
const (
	// doi names the Standard Reference Database.
	doi = "10.18434/T4D01F"
	// address is where NIST serves the database's pages: the element
	// pages under ElemTab/, the compounds and mixtures under ComTab/,
	// and the pages of contents and material constants beside them.
	address = "https://physics.nist.gov/PhysRefData/XrayMassCoef/"
)

// manifestRaw is the manifest of the pages the archive pins, with
// what was measured of each: its path under the address, its name, its
// atomic number when it is an element's, its length in bytes and the
// sum of its content.
//
//go:embed manifest.yaml
var manifestRaw []byte

// injected is the mark of the line Cloudflare appends to every page
// it serves for NIST, carrying a per-request ray id and timestamp. It
// is the one thing a page's content is taken without.
const injected = "window.__CF$cv$params="

// newline is the line break the pages are written with. Some pages
// use carriage returns as well; a line's break is the newline, and a
// carriage return before one stays with the line it ends.
var newline = []byte("\n")

// hexLen is the characters a sha256 sum is written in. It's defined:
// a sum's bytes take two hex characters each.
const hexLen = sha256.Size * 2

// Content is the sha256 sum of a page's content: the page as NIST
// serves it, with the one line Cloudflare injects removed. It contains
// nothing but the sum, so products stay comparable, as a Checker
// should.
//
// It is the check every page the archive holds is held to, as
// celestialdata's Digest checks a file whose bytes do not change.
type Content struct {
	sum [sha256.Size]byte
}

// ParseContent is the sha256 sum text holds, written in 64 hex
// characters, in either case. It refuses a text of any other length,
// and one that is not hex, saying which.
func ParseContent(text string) (Content, error) {
	if len(text) != hexLen {
		return Content{}, refuse("a sha256 sum is %d hex characters, "+
			"not %q", hexLen, text)
	}
	raw, err := hex.DecodeString(text)
	if err != nil {
		return Content{}, refuse("%q is not written in hex", text)
	}
	var content Content
	copy(content.sum[:], raw)
	return content, nil
}

// ContentOf is the sum of a page's bytes as its content: the bytes
// with every line that carries Cloudflare's mark removed, whole
// lines, and nothing else removed.
func ContentOf(raw []byte) Content {
	var kept [][]byte
	for _, line := range bytes.Split(raw, newline) {
		if bytes.Contains(line, []byte(injected)) {
			continue
		}
		kept = append(kept, line)
	}
	return Content{sum: sha256.Sum256(bytes.Join(kept, newline))}
}

// Check says nothing for a fetched page whose content has this sum,
// or why not: the page's path, the sum its content should have had,
// and the sum it has. A page that will not read is wrapped with its
// path.
func (content Content) Check(page string) error {
	raw, err := os.ReadFile(page)
	if err != nil {
		return fmt.Errorf("%s: %w", page, err)
	}
	if read := ContentOf(raw); read != content {
		return fmt.Errorf("%s: content sha256 %s, not %s", page, read,
			content)
	}
	return nil
}

// String is the sum as lowercase hex.
func (content Content) String() string {
	return hex.EncodeToString(content.sum[:])
}

// refuse is a refusal that begins with the archive's name.
func refuse(format string, args ...any) error {
	return celestialdata.Refuse(adapterName+" "+format, args...)
}

// pin is one page the manifest records: where NIST keeps it, what
// NIST calls it, its atomic number when it is an element's, and what
// was measured of it, its length in bytes and the sum of its content.
// z is none when the page is not an element's.
type pin struct {
	path  string
	name  string
	z     int
	bytes int64
	sum   string
}

// pins is the pages the manifest records, read from it. It refuses a
// manifest that will not read: a line in a shape it does not take, a
// length or an atomic number that is not a number, and a line before
// any page.
func pins() ([]pin, error) {
	var pinned []pin
	for _, line := range strings.Split(string(manifestRaw), "\n") {
		marked := strings.TrimSpace(line)
		if marked == "" || strings.HasPrefix(marked, "#") {
			continue
		}
		if entry, ok := strings.CutPrefix(marked, "- "); ok {
			pinned = append(pinned, pin{})
			marked = entry
		}
		if len(pinned) == none {
			return nil, refuse(
				"its manifest holds %q before any page", marked)
		}
		key, given, ok := strings.Cut(marked, ": ")
		if !ok {
			return nil, refuse(
				"its manifest holds the line %q", marked)
		}
		if err := set(&pinned[len(pinned)-1], key,
			unquoted(given)); err != nil {
			return nil, err
		}
	}
	if len(pinned) == none {
		return nil, refuse("its manifest holds no pages")
	}
	return pinned, nil
}

// none is the count of nothing. It's defined.
const none = 0

// set is one field of a page of the manifest, read from the value its
// key gives. A field the manifest does not take is refused, and so is
// a value that will not read, with the page it was meant for.
func set(page *pin, key, value string) error {
	switch key {
	case "path":
		if value == "" || strings.HasPrefix(value, "/") {
			return refuse("its manifest gives a page the path %q",
				value)
		}
		page.path = value
	case "name":
		page.name = value
	case "z":
		z, err := strconv.Atoi(value)
		if err != nil || z < hydrogen || z > uranium {
			return refuse(
				"its manifest gives the page %s "+
					"the atomic number %q",
				page.path, value)
		}
		page.z = z
	case "bytes":
		length, err := strconv.ParseInt(value, 10, 64)
		if err != nil || length < none {
			return refuse(
				"its manifest gives the page %s the length %q",
				page.path, value)
		}
		page.bytes = length
	case "sum":
		page.sum = value
	default:
		return refuse("its manifest gives the page %s the field %q",
			page.path, key)
	}
	return nil
}

// hydrogen and uranium are the first and last elements the database
// holds, Z 1 and Z 92. They're defined by its tables.
const (
	hydrogen = 1
	uranium  = 92
)

// unquoted is a value without the single quotes the manifest may give
// it. Only a pair around the whole value is taken; a quote inside is
// the value's, and a doubled quote is one quote.
func unquoted(given string) string {
	named := strings.TrimSpace(given)
	if len(named) >= 2 && named[0] == '\'' && named[len(named)-1] == '\'' {
		named = named[1 : len(named)-1]
	}
	return strings.ReplaceAll(named, "''", "'")
}

// New is the database's archive, asked by name only. It holds every
// page the manifest records — the 92 element pages, the 48 compound
// pages and the 4 pages of contents and constants — each held to the
// length the manifest measured and to the sum of its content.
func New() (*celestialdata.Fixed, error) {
	pinned, err := pins()
	if err != nil {
		return nil, err
	}
	files := make([]celestialdata.File, len(pinned))
	for at, each := range pinned {
		files[at] = celestialdata.NewFile(pageID(each.path),
			pageName(each.path), address+each.path, each.bytes,
			aboutOf(each))
	}
	archive, err := celestialdata.NewFixed(adapterName, files...)
	if err != nil {
		return nil, err
	}
	// NewFixed can only hold a file to a sum of its whole bytes, and
	// NIST's pages cannot have one, since Cloudflare rewrites a line
	// of every response.
	//
	// Each product is given the check of its content after, through
	// the products Find hands out, which are the archive's own, and
	// the check stays with them.
	products, err := archive.Find(nil)
	if err != nil {
		return nil, refuse("cannot ask itself what it holds: %w", err)
	}
	for at, product := range products {
		sum, err := ParseContent(pinned[at].sum)
		if err != nil {
			return nil, err
		}
		product.Checked(sum)
	}
	return archive, nil
}

// pageName is a page's file name, as Fetch keeps it: the last part of
// the path NIST keeps it under.
func pageName(at string) string {
	return path.Base(at)
}

// pageID is the archive's own name for what a page belongs to: the
// page's name without ".html", NIST's own for it, "z82" for lead's
// page and "water" for water's.
func pageID(at string) string {
	return strings.TrimSuffix(pageName(at), ".html")
}

// aboutOf says what a page is, and on what terms NIST gives it.
func aboutOf(page pin) string {
	what := page.name
	if page.z != none {
		what += fmt.Sprintf(", Z %d", page.z)
	}
	return "NIST Standard Reference Database 126 (Hubbell and " +
		"Seltzer), " + what + "; copyright by the U.S. Secretary of " +
		"Commerce on behalf of the United States of America. All " +
		"rights reserved. Cite Hubbell and Seltzer 2004, doi:" + doi
}
