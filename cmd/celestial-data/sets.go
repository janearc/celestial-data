package main

import (
	"errors"
	"maps"
	"slices"
	"strings"

	"github.com/janearc/celestial-data"
	"github.com/janearc/celestial-data/bluemarble"
	"github.com/janearc/celestial-data/cardiff"
	"github.com/janearc/celestial-data/chroma"
	"github.com/janearc/celestial-data/irsa"
	"github.com/janearc/celestial-data/naturalearth"
	"github.com/janearc/celestial-data/netcdf"
	"github.com/janearc/celestial-data/sarao"
	"github.com/janearc/celestial-data/skyview"
	"github.com/janearc/celestial-data/usgs"
	"github.com/janearc/celestial-data/web"
)

// A part is a subfolder of a set and the archive that fills it. Only,
// when it is given, names the files to keep of what the archive lists.
// With none, every file the archive lists is wanted.
type part struct {
	into string
	from celestialdata.Archive
	only []string
}

// A set is a name for a list of what to fetch and which subfolder of
// the folder it goes into, with the circle on the sky its archives are
// asked about.
type set struct {
	near  *celestialdata.Near
	parts []part
}

// A maker makes a set, asking its archives through get.
type maker func(tool command, get web.Web) (*set, error)

// standardSets are the sets celestial-data knows, by name. A name is one
// word of lower case letters, so the variable named for it is its name
// in upper case and _DATA.
func standardSets() map[string]maker {
	return map[string]maker{"grissom": grissomSet}
}

// setNames are the names of the sets the command knows, in order.
func (tool command) setNames() string {
	names := slices.Sorted(maps.Keys(tool.sets))
	return strings.Join(names, ", ")
}

// The place the grissom viewer looks at: Sgr A*, in ICRS degrees.
const (
	sgrRA  = 266.4168371 // defined: the viewer's centre
	sgrDec = -29.0078106 // defined: the viewer's centre
)

// grissomSet is everything the grissom viewer reads: the Glimpse,
// MeerKAT and dust map files round Sgr A*, the earth's images and
// spectra, four whole-sky maps, one weather file and the viewer's
// colourways.
//
// It refuses when an archive will not be made, all its reasons said
// together.
func grissomSet(tool command, get web.Web) (*set, error) {
	radius, radiusErr := tool.picked.Get("grissom-radius")
	wide, wideErr := tool.picked.Get("grissom-sky-width")
	iris, irisErr := tool.picked.Get("grissom-iris-width")
	centre, centreErr := celestialdata.NewPosition(sgrRA, sgrDec)
	near, nearErr := celestialdata.NewNear(centre, radius)
	glimpse, glimpseErr := irsa.NewGlimpse(get, irsa.GlimpseII)
	field, fieldErr := sarao.Footprint()
	meerkat, meerkatErr := sarao.New(get, field)
	tile, tileErr := cardiff.Footprint()
	ppmap, ppmapErr := cardiff.New(get, tile)
	blue, blueErr := bluemarble.New()
	spectra, spectraErr := usgs.New()
	land, landErr := naturalearth.New()
	weather, weatherErr := netcdf.New()
	radio, radioErr := skyview.New(int(wide), "Mellinger Green",
		"H-Alpha Comp", "408MHz")
	dust, dustErr := skyview.New(int(iris), "IRIS 100")
	colours, coloursErr := chroma.New(get)
	err := errors.Join(radiusErr, wideErr, irisErr, centreErr, nearErr,
		glimpseErr, fieldErr, meerkatErr, tileErr, ppmapErr, blueErr,
		spectraErr, landErr, weatherErr, radioErr, dustErr, coloursErr)
	if err != nil {
		return nil, err
	}
	return &set{near: near, parts: []part{
		{"sgra/glimpse", glimpse, []string{
			"GLM_00000+0000_mosaic_I1.fits",
			"GLM_00000+0000_mosaic_I4.fits"}},
		{"sgra/meerkat", meerkat, []string{
			"MeerKAT_Galactic_Centre_1284MHz-StokesI.fits"}},
		{"sgra/ppmap", ppmap, []string{"l000_cdens.fits"}},
		{"earth", blue, nil},
		{"earth", spectra, nil},
		{"earth", land, nil},
		{"sky", radio, nil},
		{"sky", dust, nil},
		{"weather", weather, nil},
		{"colourways", colours, nil},
	}}, nil
}
