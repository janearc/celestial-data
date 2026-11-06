// Package sdssv finds SDSS-V's optical spectra near a place on the sky:
// the BOSS spectra of DR19 that SkyServer's spAll table lists, each a
// FITS file on the Science Archive Server. The archive is asked only by
// name.
//
// The table, its columns and where the files are kept were read on
// 2026-11-06 from SkyServer's DR19 SQL service and the server's own
// answers: a file is spectra/full/FIELD/MJD/ under its pipeline run,
// FIELD and MJD as its name writes them.
package sdssv

import (
	"encoding/csv"
	"fmt"
	"net/url"
	"slices"
	"strconv"
	"strings"

	"github.com/janearc/celestial-data"
	"github.com/janearc/celestial-data/web"
)

// adapterName is the archive's name, as the command line takes it.
const adapterName = "sdssv"

// The places the spectra are listed and kept.
const (
	search = "https://skyserver.sdss.org/dr19/SkyServerWS/SearchTools/SqlSearch"
	tree   = "https://dr19.sdss.org/sas/dr19/spectro/boss/redux"
)

// What SkyServer is asked and answers, as its DR19 service does.
const (
	// answerIn is the format the service is asked to answer in.
	answerIn = "csv"
	// tableLine is the line the service heads a CSV answer with.
	tableLine = "#Table1"
	// query asks spAll for the spectra in a circle, the clearest first:
	// at most so many, the circle's centre in degrees and its radius in
	// arcminutes, as fDistanceArcMinEq takes them.
	query = "SELECT TOP %d spec_file, run2d, class, sn_median_all " +
		"FROM spAll WHERE dbo.fDistanceArcMinEq(%s, %s, racat, deccat) " +
		"<= %s ORDER BY sn_median_all DESC"
)

// The numbers a search works with, each defined.
const (
	least         = 1       // defined: a search that asks for none finds nothing
	arcminutes    = 60      // defined: arcminutes in a degree
	plain         = 'f'     // defined by strconv: no exponent
	shortest      = -1      // defined by strconv: the fewest digits that read back
	bits          = 64      // defined: a float64 is 64 bits
	nameParts     = 3       // defined: a file names its field, MJD and target
	fieldAt       = 0       // defined: the field is the name's first part
	mjdAt         = 1       // defined: the MJD its second
	anyFields     = -1      // defined by encoding/csv: rows of any length
	headingAt     = 0       // defined: the table line's place in the answer
	columnsAt     = 1       // defined: the header's place
	firstRowAt    = 2       // defined: the first row's place
	tableColumns  = 1       // defined: the table line is one field
	spectrumStart = "spec-" // defined: how a spectrum file's name begins
	spectrumEnd   = ".fits" // defined: and how it ends
)

// The columns of the answer the products are made from.
var wanted = []string{"spec_file", "run2d", "class", "sn_median_all"}

// Archive is SDSS-V's BOSS spectra in DR19.
type Archive struct {
	base *celestialdata.Adapter
	most int
}

// New is SDSS-V's spectra, asked through get for at most most spectra
// a search. It refuses no web and a most below one.
func New(get web.Web, most int) (*Archive, error) {
	base, err := celestialdata.NewAdapter(adapterName, get)
	if err != nil {
		return nil, err
	}
	if most < least {
		return nil, base.Refuse("needs at least one spectrum a search, "+
			"not %d", most)
	}
	return &Archive{base: base, most: most}, nil
}

// Name is the adapter's name as the command line takes it.
func (archive *Archive) Name() string { return archive.base.Name() }

// ByNameOnly says the archive is asked only when it is named.
func (*Archive) ByNameOnly() bool { return true }

// Find is the spectra whose catalogue place falls in the circle, the
// clearest first, at most as many as the archive was made to ask for.
func (archive *Archive) Find(near *celestialdata.Near) (
	[]*celestialdata.Product, error) {
	asked := Asking(near, archive.most)
	body, _, err := archive.base.Web().Get(asked)
	if err != nil {
		return nil, archive.base.Wrap(err)
	}
	defer body.Close()
	reader := csv.NewReader(body)
	reader.FieldsPerRecord = anyFields
	records, err := reader.ReadAll()
	if err != nil {
		return nil, archive.base.Wrap(err)
	}
	rows, err := archive.rowsOf(records)
	if err != nil {
		return nil, err
	}
	var products []*celestialdata.Product
	for _, row := range rows {
		product, err := archive.spectrum(row)
		if err != nil {
			return nil, err
		}
		products = append(products, product)
	}
	return products, nil
}

// Asking is the address Find asks SkyServer at for a circle, and at
// most so many spectra.
func Asking(near *celestialdata.Near, most int) string {
	centre := near.Centre()
	radius := number(near.Radius() * arcminutes)
	ra, dec := number(centre.RA()), number(centre.Dec())
	sql := fmt.Sprintf(query, most, ra, dec, radius)
	values := url.Values{"cmd": {sql}, "format": {answerIn}}
	return search + "?" + values.Encode()
}

// number is a number as SQL and a file name write it.
func number(value float64) string {
	return strconv.FormatFloat(value, plain, shortest, bits)
}

// row is one answered spectrum, by its columns' names.
type row map[string]string

// rowsOf is the answer's rows. It refuses an answer that is not headed
// with the table line, one that lacks a column the products need, and
// a row of more or fewer fields than the header.
func (archive *Archive) rowsOf(records [][]string) ([]row, error) {
	if len(records) < firstRowAt || len(records[headingAt]) != tableColumns ||
		records[headingAt][headingAt] != tableLine {
		return nil, archive.base.Fail("the answer is not one table")
	}
	header := records[columnsAt]
	for _, name := range wanted {
		if !slices.Contains(header, name) {
			return nil, archive.base.Fail("the answer has no %s", name)
		}
	}
	var rows []row
	for _, record := range records[firstRowAt:] {
		if len(record) != len(header) {
			return nil, archive.base.Fail("a row of %d fields under %d "+
				"columns", len(record), len(header))
		}
		each := row{}
		for at, name := range header {
			each[name] = record[at]
		}
		rows = append(rows, each)
	}
	return rows, nil
}

// spectrum is one row's file. It refuses a file name not of the form
// spec-FIELD-MJD-TARGET.fits.
func (archive *Archive) spectrum(answered row) (*celestialdata.Product,
	error) {
	name := answered["spec_file"]
	stem := strings.TrimSuffix(strings.TrimPrefix(name, spectrumStart),
		spectrumEnd)
	parts := strings.Split(stem, "-")
	if !strings.HasPrefix(name, spectrumStart) ||
		!strings.HasSuffix(name, spectrumEnd) || slices.Contains(parts, "") ||
		len(parts) != nameParts {
		return nil, archive.base.Fail("a spectrum file %q is not "+
			"spec-FIELD-MJD-TARGET.fits", name)
	}
	address := strings.Join([]string{tree, answered["run2d"], "spectra",
		"full", parts[fieldAt], parts[mjdAt], name}, "/")
	about := fmt.Sprintf("a BOSS spectrum, %s, median S/N %s",
		strings.ToLower(answered["class"]), answered["sn_median_all"])
	return archive.base.Product(stem, name, address, celestialdata.Unsized,
		about)
}
