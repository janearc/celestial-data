// Package netcdf fetches and reads classic netCDF files, and writes them.
// It wraps the whole of github.com/scigolib/netcdf, and on it reads
// gridded fields that follow the CF conventions: a named quantity over
// (time, latitude, longitude).
//
// Which file and which variable are asked for is the caller's; nothing
// here is about one reanalysis.
//
// Its refusals wrap celestialdata.Refused. An error the reader under it
// gave stays reachable with errors.Is, so the two are told apart.
//
// The fetch is one example file of Unidata's: surface fields for July
// 2002 from the ECMWF 40-year reanalysis.
package netcdf

import (
	_ "embed"
	"fmt"

	"github.com/janearc/celestial-data"
)

// adapterName is the archive's name, as the command line takes it.
const adapterName = "netcdf"

// The example file, as Unidata keeps it.
const (
	// era40Name is the file of surface fields for July 2002.
	era40Name = "ECMWF_ERA-40_subset.nc"
	// era40Address is where Unidata serves it.
	era40Address = "https://archive.unidata.ucar.edu/software/netcdf/" +
		"examples/" + era40Name
	// era40ID is the id of the file's product.
	era40ID = "ERA-40 July 2002"
	// era40Size is the name of the file's length in numbers.yaml.
	era40Size = "era40-size"
)

// numbersFile is the numbers file that holds the example file's length.
const numbersFile = "numbers.yaml"

//go:embed numbers.yaml
var numbersRaw []byte

// New is the archive of the example file, asked by name only. It holds
// the one file, whatever the circle. Its sha256, in sources.yaml, is not
// checked by a fetch, which has nowhere to hold it.
//
//ninja:vow practice a library: the grissom set fetches the file by it
func New() (*celestialdata.Fixed, error) {
	size := int64(measured().Value(era40Size))
	file := celestialdata.NewFile(era40ID, era40Name, era40Address, size,
		"ECMWF ERA-40 surface fields, July 2002, as classic netCDF")
	return celestialdata.NewFixed(adapterName, file)
}

// measured is the numbers file, read.
func measured() *celestialdata.Measured {
	return celestialdata.NewMeasured(numbersFile, numbersRaw)
}

// refuse is a refusal that begins with the archive's name.
func refuse(format string, args ...any) error {
	return celestialdata.Refuse(adapterName+" "+format, args...)
}

// failure is an error that names the file and what in it, and keeps the
// cause reachable with errors.Is.
func failure(file, what string, err error) error {
	return fmt.Errorf("%s: %s: %w", file, what, err)
}

// variableWhat is the words that name a variable in an error.
func variableWhat(name string) string {
	return fmt.Sprintf("variable %q", name)
}
