// Package bsc5 is the adapter for the Yale Bright Star Catalogue, its
// fifth revised edition (Hoffleit and Warren, 1991): 9,110 rows, the
// stars a naked eye sees with their J2000 places, V, B-V and spectral
// types, and fourteen with no place, novae and things found not stars.
//
// The archive is the one file, ybsc5.gz, as Harvard's Telescope Data
// Center serves it, laid out as Warren's ybsc5.readme beside it says,
// read 2026-11-10 (sources.yaml). Read reads it; a Star is one row.
//
// CDS Strasbourg keeps the same catalogue as V/50. It did not answer
// from here that day, so the adapter asks Harvard.
package bsc5

import (
	_ "embed"

	"github.com/janearc/celestial-data"
)

// adapterName is the archive's name, as the command line takes it.
const adapterName = "bsc5"

// The file, as Harvard's Telescope Data Center keeps it.
const (
	// fileName is the gzipped ASCII catalogue.
	fileName = "ybsc5.gz"
	// address is where the Telescope Data Center serves it.
	address = "http://tdc-www.harvard.edu/catalogs/" + fileName
	// fileID is the id of the file's product.
	fileID = "bsc5"
	// fileSize is the name of the file's length in numbers.yaml.
	fileSize = "catalogue-size"
)

// numbersFile is the numbers file that holds the file's length.
const numbersFile = "numbers.yaml"

//go:embed numbers.yaml
var numbersRaw []byte

// New is the archive of the catalogue, asked by name only. It holds the
// one file, whatever the circle.
func New() (*celestialdata.Fixed, error) {
	size := int64(measured().Value(fileSize))
	catalogue := celestialdata.NewFile(fileID, fileName, address, size,
		"the Yale Bright Star Catalogue, fifth edition: 9,110 stars, "+
			"every one a naked eye sees")
	return celestialdata.NewFixed(adapterName, catalogue)
}

// measured is the numbers file, read.
func measured() *celestialdata.Measured {
	return celestialdata.NewMeasured(numbersFile, numbersRaw)
}
