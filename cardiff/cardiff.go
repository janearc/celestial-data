// Package cardiff finds the Hi-GAL PPMAP results kept at Cardiff. They are
// maps of dust column density and temperature, made by point process
// mapping of Herschel's Hi-GAL survey of the galactic plane. Each tile
// along the plane has its own folder, named for its longitude.
package cardiff

import (
	_ "embed"
	"regexp"
	"strconv"
	"strings"

	"github.com/janearc/celestial-data"
	"github.com/janearc/celestial-data/web"
)

// results is the folder that holds each tile's folder.
const results = "http://www.astro.cardiff.ac.uk/research/ViaLactea/" +
	"PPMAP_Results/"

// tileFolder finds a tile's folder in the listing. Its name holds the
// tile's longitude in whole degrees, as in l000_results/.
var tileFolder = regexp.MustCompile(`^l(\d+)_results/$`)

// The numbers a tile's folder is read with.
const (
	// degreesAt is the place of the longitude among the groups
	// tileFolder finds. It's defined by tileFolder.
	degreesAt = 1
	// slack is how far a tile's centre may lie from its name. It's
	// defined: the names are whole degrees that step by 2 or by 3. So a
	// name is its centre rounded one way or the other. One degree holds it.
	slack = 1
)

// numbersFile is the numbers file that holds the l000 tile's cards.
const numbersFile = "numbers.yaml"

//go:embed numbers.yaml
var numbersRaw []byte

// Footprint is the l000 tile's box, read from its column density map's
// header cards kept beside this package. Every tile is taken to be this
// size.
func Footprint() (*celestialdata.Patch, error) {
	measured := celestialdata.NewMeasured(numbersFile, numbersRaw)
	return measured.Footprint(celestialdata.InGalactic, "l000")
}

// Results is the PPMAP results' tiles at Cardiff.
type Results struct {
	base *celestialdata.Adapter
}

// New is the results asked through get, each tile the size of one.
// Footprint gives that tile. It refuses no web and no tile.
func New(get web.Web, tile *celestialdata.Patch) (*Results, error) {
	base, err := celestialdata.NewField("ppmap", get, tile)
	if err != nil {
		return nil, err
	}
	return &Results{base: base}, nil
}

// Name is the adapter's name as the command line takes it.
func (found *Results) Name() string { return found.base.Name() }

// Find is the FITS files of every tile the circle may reach. Tiles are
// taken to be centred on their name's longitude, give or take the slack.
// So a circle near a tile's edge may find its neighbour too.
func (found *Results) Find(near *celestialdata.Near) ([]*celestialdata.Product,
	error) {
	links, err := found.base.Links(results)
	if err != nil {
		return nil, err
	}
	var products []*celestialdata.Product
	for _, link := range links {
		parts := tileFolder.FindStringSubmatch(link)
		if parts == nil {
			continue
		}
		lon, err := strconv.Atoi(parts[degreesAt])
		if err != nil {
			return nil, found.base.Fail("%s: %w", link, err)
		}
		box := found.base.Footprint().Moved(float64(lon), slack)
		if near.Reaches(box) {
			files, err := found.tileFiles(link)
			if err != nil {
				return nil, err
			}
			products = append(products, files...)
		}
	}
	return products, nil
}

// tileFiles is the FITS files in one tile's folder.
func (found *Results) tileFiles(folder string) ([]*celestialdata.Product,
	error) {
	tile := strings.TrimSuffix(folder, "_results/")
	return found.base.Files(results+folder, celestialdata.EndsIn(".fits"),
		tile, "Hi-GAL PPMAP, tile "+tile)
}
