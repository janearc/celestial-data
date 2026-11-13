package bsc5_test

import (
	"strings"
	"testing"

	"github.com/janearc/celestial-data/bsc5"
	"github.com/janearc/celestial-data/internal/replay"
)

// The archive's numbers, each defined.
const (
	adapter      = "bsc5" // defined: the archive's name
	lone         = 1      // defined: the one file
	first        = 0      // defined: its place
	catalogueLen = 573921 // defined: numbers.yaml's catalogue-size
)

// the test: the archive is asked by name only, and holds the one file,
// the catalogue at Harvard's Telescope Data Center, whatever the circle.
func TestTheArchiveIsTheCatalogueFile(test *testing.T) {
	archive, err := bsc5.New()
	if err != nil {
		test.Fatal(err)
	}
	if archive.Name() != adapter || !archive.ByNameOnly() {
		test.Errorf("%q, by name only %v", archive.Name(), archive.ByNameOnly())
	}
	found, err := archive.Find(replay.Asked(test))
	if err != nil || len(found) != lone {
		test.Fatalf("%d products, %v", len(found), err)
	}
	file := found[first]
	if file.Archive() != adapter || file.ID() != "bsc5" ||
		file.Name() != "ybsc5.gz" ||
		file.URL() != "http://tdc-www.harvard.edu/catalogs/ybsc5.gz" ||
		!strings.Contains(file.About(), "Bright Star Catalogue") {
		test.Errorf("%+v", file)
	}
}
