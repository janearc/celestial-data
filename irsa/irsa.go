// Package irsa finds what NASA's infrared archive at IPAC holds near a
// place on the sky. Glimpse finds the Spitzer GLIMPSE survey's mosaics.
// Catalogue names the rows of one of the archive's tables as a CSV file.
package irsa

import (
	"errors"
	"fmt"
	"regexp"
	"strconv"
	"strings"

	"github.com/janearc/celestial-data"
	"github.com/janearc/celestial-data/tap"
	"github.com/janearc/celestial-data/web"
)

// The places the archive's tables and images are kept.
const (
	// sync is the archive's TAP service.
	sync = "https://irsa.ipac.caltech.edu/TAP/sync"
	// GlimpseII is the folder of GLIMPSE II's 1.2 arcsecond mosaics. The
	// survey's other image sets are kept in folders laid out the same way.
	GlimpseII = "https://irsa.ipac.caltech.edu/data/SPITZER/GLIMPSE/" +
		"images/II/1.2_mosaics_v3.5/"
)

// tile finds a mosaic tile's preview in a listing. Its name holds the
// tile's centre in hundredths of a degree and its size in degrees, as in
// GLM_00000+0000_3.1x4.5.jpg.
var tile = regexp.MustCompile(`^(GLM_(\d{5})([+-]\d{4}))_([\d.]+)x` +
	`([\d.]+)\.jpg$`)

// The places of a tile's parts among the groups tile finds.
const (
	prefix    = 1 // defined by tile: each of the tile's files begins so
	longitude = 2 // defined by tile: the centre's longitude
	latitude  = 3 // defined by tile: the centre's latitude, with its sign
	wide      = 4 // defined by tile: the width in longitude
	high      = 5 // defined by tile: the height in latitude
)

// The numbers a tile's name is read with.
const (
	hundredths = 100 // defined: names give degrees in hundredths
	both       = 2   // defined: a tile reaches half its size either side
	bits       = 64  // defined: a float64 is 64 bits
)

// Glimpse is a folder of GLIMPSE mosaics.
type Glimpse struct {
	base   *celestialdata.Adapter
	folder string
}

// NewGlimpse is the mosaics in a folder laid out as GlimpseII is, asked
// through get. It refuses no web, and a folder that does not end in a
// slash.
func NewGlimpse(get web.Web, folder string) (*Glimpse, error) {
	base, err := celestialdata.NewAdapter("glimpse", get)
	if err != nil {
		return nil, err
	}
	if !strings.HasSuffix(folder, "/") {
		return nil, base.Refuse("needs a folder that ends in a slash, "+
			"not %q", folder)
	}
	return &Glimpse{base: base, folder: folder}, nil
}

// Name is the adapter's name as the command line takes it.
func (glimpse *Glimpse) Name() string { return glimpse.base.Name() }

// Find is the mosaics of every tile the circle reaches, in each band the
// folder holds.
func (glimpse *Glimpse) Find(near *celestialdata.Near) (
	[]*celestialdata.Product, error) {
	links, err := glimpse.base.Links(glimpse.folder)
	if err != nil {
		return nil, err
	}
	var products []*celestialdata.Product
	for _, link := range links {
		parts := tile.FindStringSubmatch(link)
		if parts == nil {
			continue
		}
		box, err := patchOf(parts)
		if err != nil {
			return nil, glimpse.base.Fail("%s: %w", link, err)
		}
		if near.Reaches(box) {
			found := glimpse.mosaics(links, parts[prefix])
			products = append(products, found...)
		}
	}
	return products, nil
}

// patchOf is the box a tile covers, read from its preview's name.
func patchOf(parts []string) (*celestialdata.Patch, error) {
	lon, lonErr := strconv.ParseFloat(parts[longitude], bits)
	lat, latErr := strconv.ParseFloat(parts[latitude], bits)
	width, widthErr := strconv.ParseFloat(parts[wide], bits)
	height, heightErr := strconv.ParseFloat(parts[high], bits)
	err := errors.Join(lonErr, latErr, widthErr, heightErr)
	if err != nil {
		return nil, err
	}
	return celestialdata.NewPatch(celestialdata.InGalactic, lon/hundredths,
		lat/hundredths, width/both, height/both)
}

// mosaics is the tile's mosaic in each band the listing holds.
func (glimpse *Glimpse) mosaics(links []string,
	name string) []*celestialdata.Product {
	var products []*celestialdata.Product
	for _, link := range links {
		band, isMosaic := strings.CutPrefix(link, name+"_mosaic_")
		band, isFITS := strings.CutSuffix(band, ".fits")
		if !isMosaic || !isFITS {
			continue
		}
		about := "GLIMPSE mosaic, IRAC " + band
		product, err := glimpse.base.Product(name, link,
			glimpse.folder+link, celestialdata.Unsized, about)
		if err != nil {
			// links that are not one plain file name lead out of
			// the folder. They are not the folder's files.
			continue
		}
		products = append(products, product)
	}
	return products
}

// Catalogue is one of the archive's tables.
type Catalogue struct {
	base    *celestialdata.Adapter
	columns string
}

// NewCatalogue is a table of the archive's with the columns to ask it
// for, as ADQL writes them. It refuses no table and no columns. Tables
// whose name cannot be a file's are refused when their rows are named.
func NewCatalogue(name, columns string) (*Catalogue, error) {
	base, err := celestialdata.Named(name)
	if err == nil && strings.TrimSpace(columns) == "" {
		err = base.Refuse("needs its columns, not %q", columns)
	}
	if err != nil {
		return nil, err
	}
	return &Catalogue{base: base, columns: columns}, nil
}

// Name is the adapter's name as the command line takes it: the table's.
func (catalogue *Catalogue) Name() string { return catalogue.base.Name() }

// Find is one file: the table's rows within the circle, in CSV. Nothing is
// asked of the archive until the file is fetched.
func (catalogue *Catalogue) Find(near *celestialdata.Near) (
	[]*celestialdata.Product, error) {
	table := catalogue.base.Name()
	adql := tap.Search(tap.Every, catalogue.columns, table, near, "", "")
	centre := near.Centre()
	ra, dec := number(centre.RA()), number(centre.Dec())
	radius := number(near.Radius())
	name := fmt.Sprintf("%s_%s_%s_r%s.csv", table, ra, dec, radius)
	address := tap.URL(sync, adql, tap.CSV)
	about := fmt.Sprintf("the rows of %s within %s degrees", table, radius)
	return celestialdata.Found(catalogue.base.Product(table, name, address,
		celestialdata.Unsized, about))
}

// The parts of a number as a file name writes it.
const (
	plain    = 'f' // defined by strconv: no exponent
	shortest = -1  // defined by strconv: the fewest digits that read back
)

// number is a number as a file name writes it.
func number(value float64) string {
	return strconv.FormatFloat(value, plain, shortest, bits)
}
