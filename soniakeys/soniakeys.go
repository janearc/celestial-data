// Package soniakeys fetches test files of Sonia Keys's Go implementation
// of Meeus's Astronomical Algorithms (github.com/soniakeys/meeus, MIT
// licence), at the one commit celestial cites, bfbd9ac. Her tests
// transcribe the book's worked examples with their pages.
//
// They are fetched to check celestial's work against, never to derive
// it from: none of her code, formulas or constants is used, and only
// her test files are kept (briefs/sources.md on the bookshelf). Today
// that is one: her solar coordinates test, for Meeus's example 25.a.
package soniakeys

import "github.com/janearc/celestial-data"

// adapterName is the archive's name, as the command line takes it.
const adapterName = "soniakeys"

// commit is the one revision of her repository fetched from, the one
// celestial's other transcriptions cite. It's defined: a hash, so the
// file cannot change under the address.
const commit = "bfbd9ac2c7f709c94f1dee190c633d24a723f628"

// raw is where GitHub serves a file of her repository at that commit.
const raw = "https://raw.githubusercontent.com/soniakeys/meeus/" + commit +
	"/v3/"

// The solar coordinates test, as her repository keeps it.
const (
	// solarID is the id of the file's product.
	solarID = "solar test"
	// solarName is the name it is kept by, as celestial's other
	// transcriptions are named: a .txt, so no tool reads it as Go.
	solarName = "soniakeys-meeus-solar-test.go.txt"
	// solarAddress is where it is served.
	solarAddress = raw + "solar/solar_test.go"
	// solarAbout is a few words on what it is.
	solarAbout = "Sonia Keys's meeus solar test at bfbd9ac, MIT: " +
		"examples to check against, not to derive from"
)

// New is the archive of her test files, asked by name only. It holds the
// same files whatever the circle. A file's length is not known until it
// is served.
func New() (*celestialdata.Fixed, error) {
	solar := celestialdata.NewFile(solarID, solarName, solarAddress,
		celestialdata.Unsized, solarAbout)
	return celestialdata.NewFixed(adapterName, solar)
}
