// Package lamost finds LAMOST's spectra near a place on the sky: those
// of DR10 v2.0, public to all since September 2024, at low resolution
// and at medium. The medium-resolution spectra's red arm, 6300 to 6800 A
// at a resolution of 7500, holds lithium's line at 6708 A.
//
// Both are asked of the release's TAP service, each spectrum fetched by
// its obsid as a gzipped FITS file. The service, its tables and the
// files' places were read on 2026-11-06 from LAMOST's pages and its
// answers.
//
// The survey asks that work using the data acknowledge it, in
// the words of its data policy, lamost.org/lmusers/cms/article/view?id=1.
//
// LAMOST's sky stops near declination -10: south of that, a search
// finds nothing. The archive is asked only by name.
package lamost

import (
	"errors"
	"fmt"
	"path"

	"github.com/janearc/celestial-data"
	"github.com/janearc/celestial-data/tap"
	"github.com/janearc/celestial-data/web"
)

// adapterName is the archive's name, as the command line takes it.
const adapterName = "lamost"

// The release's places: its TAP service, and where each resolution's
// spectra are fetched by obsid.
const (
	release     = "https://www.lamost.org/dr10/v2.0"
	sync        = release + "/voservice/tap/sync"
	lowFiles    = release + "/spectrum/fits/"
	mediumFiles = release + "/medspectrum/fits/"
)

// The tables asked, each a catalogue joined with its parameters and its
// file, and what is asked of them.
const (
	lowTable      = `"public".combined`
	mediumTable   = `"public".med_combined`
	lowColumns    = "obsid, class, subclass, snrr, file"
	mediumColumns = "obsid, snr, file"
	// redArm keeps a medium-resolution spectrum's combined red row: each
	// has a row an arm and an exposure, coadd 1 marking an arm's combined
	// row, and one file holds them all. DR10 v2.0 has one such red row a
	// spectrum: 4,166,525 of each, counted 2026-11-06.
	redArm      = "band = 'R' AND coadd = 1"
	lowOrder    = "snrr DESC"
	mediumOrder = "snr DESC"
)

// least is the fewest spectra a search may ask for of each resolution.
// It's defined: a search that asks for none finds nothing.
const least = 1

// Archive is LAMOST's DR10 v2.0 spectra.
type Archive struct {
	base *celestialdata.Adapter
	most int
}

// New is LAMOST's spectra, asked through get for at most most spectra
// of each resolution a search. It refuses no web and a most below one.
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

// Find is the spectra whose place falls in the circle: the low-resolution
// ones, clearest in the r band first, then the medium-resolution ones,
// clearest in the red arm first, at most as many of each as the archive
// was made to ask for.
func (archive *Archive) Find(near *celestialdata.Near) (
	[]*celestialdata.Product, error) {
	low, err := archive.low(near)
	if err != nil {
		return nil, err
	}
	medium, err := archive.medium(near)
	if err != nil {
		return nil, err
	}
	return append(low, medium...), nil
}

// low is the low-resolution spectra in the circle.
func (archive *Archive) low(near *celestialdata.Near) (
	[]*celestialdata.Product, error) {
	adql := tap.Search(archive.most, lowColumns, lowTable, near, "",
		lowOrder)
	table, err := tap.Query(archive.base.Web(), sync, adql)
	if err != nil {
		return nil, archive.base.Wrap(err)
	}
	var products []*celestialdata.Product
	for at := range table.Len() {
		class, classErr := table.Text(at, "class")
		subclass, subclassErr := table.Text(at, "subclass")
		snr, snrErr := table.Number(at, "snrr")
		if err := errors.Join(classErr, subclassErr, snrErr); err != nil {
			return nil, archive.base.Wrap(err)
		}
		about := fmt.Sprintf("a LAMOST low-resolution spectrum, %s %s, "+
			"S/N %.1f in r", class, subclass, snr)
		product, err := archive.spectrum(table, at, lowFiles, about)
		if err != nil {
			return nil, err
		}
		products = append(products, product)
	}
	return products, nil
}

// medium is the medium-resolution spectra in the circle.
func (archive *Archive) medium(near *celestialdata.Near) (
	[]*celestialdata.Product, error) {
	adql := tap.Search(archive.most, mediumColumns, mediumTable, near,
		redArm, mediumOrder)
	table, err := tap.Query(archive.base.Web(), sync, adql)
	if err != nil {
		return nil, archive.base.Wrap(err)
	}
	var products []*celestialdata.Product
	for at := range table.Len() {
		snr, err := table.Number(at, "snr")
		if err != nil {
			return nil, archive.base.Wrap(err)
		}
		about := fmt.Sprintf("a LAMOST medium-resolution spectrum, both "+
			"arms, S/N %.1f in the red", snr)
		product, err := archive.spectrum(table, at, mediumFiles, about)
		if err != nil {
			return nil, err
		}
		products = append(products, product)
	}
	return products, nil
}

// spectrum is a row's file: named as the release names it, fetched by
// its obsid.
func (archive *Archive) spectrum(table rows, at int, files,
	about string) (*celestialdata.Product, error) {
	obsid, obsidErr := table.Text(at, "obsid")
	file, fileErr := table.Text(at, "file")
	if err := errors.Join(obsidErr, fileErr); err != nil {
		return nil, archive.base.Wrap(err)
	}
	product, err := archive.base.Product(obsid, path.Base(file),
		files+obsid, celestialdata.Unsized, about)
	if err != nil {
		return nil, archive.base.Wrap(err)
	}
	return product, nil
}

// rows is what spectrum reads of an answered table.
type rows interface {
	Text(at int, name string) (string, error)
}
