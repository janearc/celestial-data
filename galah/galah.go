// Package galah is the adapter for GALAH DR4's allstar, from Data
// Central: one row a star the GALAH survey observed with HERMES on the
// Anglo-Australian Telescope, with the parameters and abundances its
// pipeline fit, lithium among them.
//
// The archive is the one file. A Catalogue reads it on disk, and a Star
// is one of its rows. The file writes no units on the columns read:
// each value's unit here is the survey's, and the flags are its flags
// page's, both read 2026-11-06 (sources.yaml).
package galah

import (
	_ "embed"

	"github.com/janearc/celestial-data"
)

// adapterName is the archive's name, as the command line takes it.
const adapterName = "galah"

// The file, as Data Central keeps it.
const (
	// allStarName is DR4's allstar file.
	allStarName = "galah_dr4_allstar_240705.fits"
	// address is where Data Central serves it.
	address = "https://cloud.datacentral.org.au/teamdata/GALAH/public/" +
		"GALAH_DR4/catalogs/" + allStarName
	// allStarID is the id of the file's product.
	allStarID = "dr4 allstar"
	// allStarSize is the name of the file's length in numbers.yaml.
	allStarSize = "allstar-size"
)

// numbersFile is the numbers file that holds the file's length.
const numbersFile = "numbers.yaml"

//go:embed numbers.yaml
var numbersRaw []byte

// New is the archive of DR4's allstar, asked by name only. It holds the
// one file, whatever the circle.
func New() (*celestialdata.Fixed, error) {
	size := int64(measured().Value(allStarSize))
	allStar := celestialdata.NewFile(allStarID, allStarName, address, size,
		"GALAH DR4's allstar: each star's parameters and abundances")
	return celestialdata.NewFixed(adapterName, allStar)
}

// measured is the numbers file, read.
func measured() *celestialdata.Measured {
	return celestialdata.NewMeasured(numbersFile, numbersRaw)
}
