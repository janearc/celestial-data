package netcdf_test

import (
	"crypto/sha256"
	"encoding/hex"
	"os"
	"testing"

	"github.com/janearc/celestial-data"
	"github.com/janearc/celestial-data/netcdf"
)

// The example file, as sources.yaml and numbers.yaml hold it.
const (
	era40Name = "ECMWF_ERA-40_subset.nc" // defined: Unidata's file
	era40Size = 22165040                 // defined: numbers.yaml's era40-size
	era40Sha  = "f5d5bb82811e74179894a87e439acbb9" +
		"52eb692181570b97bcfbc9f61aa18890" // defined: sources.yaml's sha256
	era40URL = "https://archive.unidata.ucar.edu/software/netcdf/examples/" +
		era40Name
	// era40Variable is the variable the checks below read.
	era40Variable = "tcc"
	// era40Environment names the file on disk, for the one test that
	// reads it.
	era40Environment = "CELESTIAL_DATA_ERA40"
)

// The archive holds the one example file, by name only, whatever the
// circle, and its length is the one numbers.yaml holds.
func TestTheArchiveHoldsTheExampleFile(t *testing.T) {
	archive, err := netcdf.New()
	check(t, err)
	products, err := archive.Find(nil)
	check(t, err)
	if len(products) != 1 || products[0].Name() != era40Name ||
		products[0].URL() != era40URL || archive.Name() != "netcdf" ||
		!archive.ByNameOnly() {
		t.Errorf("%v", products)
	}
	raw, err := os.ReadFile("numbers.yaml")
	check(t, err)
	size := celestialdata.NewMeasured("numbers.yaml", raw)
	if got, err := size.Get("era40-size"); err != nil || got != era40Size {
		t.Errorf("era40-size is %v, %v", got, err)
	}
}

// With the real file named in the environment, it is the file the
// fetch declares and the reader reads it as Unidata describes it: 144
// longitudes, 73 latitudes, 62 steps, and tcc with data on the odd steps
// only, the first of which runs from none to one and has a mean near
// two thirds. Without the variable, the test is skipped.
func TestTheRealFileIsTheFileDeclared(t *testing.T) {
	path := os.Getenv(era40Environment)
	if path == "" {
		t.Skip(era40Environment + " does not name the example file")
	}
	raw, err := os.ReadFile(path)
	check(t, err)
	sum := sha256.Sum256(raw)
	if len(raw) != era40Size || hex.EncodeToString(sum[:]) != era40Sha {
		t.Fatalf("%s is %d bytes, sha256 %x", path, len(raw), sum)
	}
	file, err := netcdf.Open(path)
	check(t, err)
	defer file.Close()
	found, err := file.Field(era40Variable)
	check(t, err)
	checkRealGrid(t, found)
}

// checkRealGrid checks the example file's grid against what Unidata's
// description gives.
func checkRealGrid(t *testing.T, found *netcdf.Field) {
	t.Helper()
	const (
		longitudes = 144 // defined: by the file's header
		latitudes  = 73  // defined: by the file's header
		steps      = 62  // defined: by the file's header
		withData   = 31  // defined: the odd steps
		meanTcc    = 0.666
		meanSlack  = 0.01 // defined: how near "near" is
	)
	every, err := found.All()
	check(t, err)
	if len(found.Longitudes()) != longitudes ||
		len(found.Latitudes()) != latitudes || len(every) != steps {
		t.Fatalf("%d %d %d", len(found.Longitudes()), len(found.Latitudes()),
			len(every))
	}
	holding := 0
	for at, step := range every {
		if step.Holds() {
			holding++
		}
		if step.Holds() != (at%2 == 1) {
			t.Errorf("step %d holds %t", at, step.Holds())
		}
	}
	if holding != withData {
		t.Errorf("%d steps hold data", holding)
	}
	checkRealStep(t, every[1], meanTcc, meanSlack)
}

// checkRealStep checks one step runs from none to one with a mean near
// a value.
func checkRealStep(t *testing.T, step *netcdf.Slice, mean, slack float64) {
	t.Helper()
	least, most, total, count := 2.0, -1.0, 0.0, 0
	for row := range step.Rows() {
		for column := range step.Columns() {
			if each, have := step.At(row, column); have {
				least, most = min(least, each), max(most, each)
				total += each
				count++
			}
		}
	}
	if !near(least, 0) || !near(most, 1) || count == 0 ||
		total/float64(count) < mean-slack ||
		total/float64(count) > mean+slack {
		t.Errorf("runs %v to %v, mean %v over %d", least, most,
			total/float64(count), count)
	}
}
