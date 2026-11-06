// Package apogee is the adapter for APOGEE DR17's allStar, from the
// Sloan Digital Sky Survey: one row a star its spectrographs observed in
// the near infrared, with the parameters and abundances its pipeline,
// ASPCAP, fit to the star's combined spectrum.
//
// The archive is the one file. A Catalogue reads it on disk, and a Star
// is one of its rows. The file writes no units: each value's unit here
// is the one SDSS's data model gives, and the bits of each flag are the
// survey's bitmask page's, both read 2026-11-06 (sources.yaml).
package apogee

import (
	_ "embed"

	"github.com/janearc/celestial-data"
)

// adapterName is the archive's name, as the command line takes it.
const adapterName = "apogee"

// The file, as the Science Archive Server keeps it.
const (
	// allStarName is DR17's allStar file.
	allStarName = "allStar-dr17-synspec_rev1.fits"
	// address is where the server serves it.
	address = "https://data.sdss.org/sas/dr17/apogee/spectro/aspcap/" +
		"dr17/synspec_rev1/" + allStarName
	// allStarID is the id of the file's product.
	allStarID = "dr17 allStar"
	// allStarSize is the name of the file's length in numbers.yaml.
	allStarSize = "allstar-size"
)

// numbersFile is the numbers file that holds the file's length.
const numbersFile = "numbers.yaml"

//go:embed numbers.yaml
var numbersRaw []byte

// New is the archive of DR17's allStar, asked by name only. It holds
// the one file, whatever the circle.
func New() (*celestialdata.Fixed, error) {
	size := int64(measured().Value(allStarSize))
	allStar := celestialdata.NewFile(allStarID, allStarName, address, size,
		"APOGEE DR17's allStar: each star's parameters and abundances")
	return celestialdata.NewFixed(adapterName, allStar)
}

// measured is the numbers file, read.
func measured() *celestialdata.Measured {
	return celestialdata.NewMeasured(numbersFile, numbersRaw)
}
