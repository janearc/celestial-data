package main

import (
	"bytes"
	"fmt"
	"os"
	"strings"
	"testing"

	"github.com/janearc/celestial-data"
	"github.com/janearc/celestial-data/mocks"
	"go.uber.org/mock/gomock"
)

// said runs celestial-data and returns what it said. Its web is a mock that
// expects no question, so an example asks nothing of the network. When the
// run fails it panics with what was said on errs.
func said(args ...string) string {
	get := mocks.NewMockWeb(gomock.NewController(stopping{}))
	return saidThrough(get, args...)
}

// saidThrough runs celestial-data asking through the web given, and
// returns what it said.
func saidThrough(get *mocks.MockWeb, args ...string) string {
	var out, errs bytes.Buffer
	tool := command{get: get, picked: celestialdata.NewMeasured(numbersFile,
		numbersRaw), sets: standardSets()}
	if code := tool.run(args, strings.NewReader(""), &out,
		&errs); code != exitOK {
		panic(errs.String())
	}
	return out.String()
}

// stopping is what a mock in an example reports to. Examples have no
// test to fail, so it stops the example with what went wrong.
type stopping struct{}

// Errorf stops the example with what went wrong.
func (stopping) Errorf(format string, args ...any) {
	panic(fmt.Sprintf(format, args...))
}

// Fatalf stops the example with what went wrong.
func (stopping) Fatalf(format string, args ...any) {
	panic(fmt.Sprintf(format, args...))
}

func Example_archives() {
	fmt.Print(said("archives"))
	// Output:
	// chandra       Chandra's primary products: events, field of view, aspect
	// nustar        NuSTAR's cleaned events, for both focal plane modules
	// glimpse       Spitzer GLIMPSE II mosaics, in IRAC's four bands
	// galcen_psc    IRSA's Galactic Center IRAC catalog, as CSV
	// hpsgc         the HST/NICMOS Paschen-alpha survey of the Galactic Centre
	// meerkat       MeerKAT's Galactic Centre mosaic at 1.28 GHz
	// ppmap         Hi-GAL PPMAP maps of dust column density and temperature
	// hips          a hips2fits cutout of any HiPS survey
	// bluemarble    NASA's Blue Marble, July 2004, and Black Marble 2016
	// usgs          the USGS Spectral Library Version 7, as one archive
	// naturalearth  Natural Earth's land outlines at 1:50m, as GeoJSON
	// chroma        the colourways in janearc/chroma, listed by GitHub
	// gaia          Gaia DR3's stars with a radial velocity, as a job
}

// The catalogue asks nothing until its rows are fetched, so the example
// asks nothing of the network.
func Example_find() {
	fmt.Print(said("find", "--archive", "galcen_psc", "--ra",
		"266.4168371", "--dec", "-29.0078106", "--radius", "0.1"))
	// Output:
	// round ra 266.4168371, dec -29.0078106; l 359.9443, b -0.0462; within 0.1 degrees
	// galcen_psc galcen_psc galcen_psc_266.4168371_-29.0078106_r0.1.csv
	//     the rows of galcen_psc within 0.1 degrees
	//     https://irsa.ipac.caltech.edu/TAP/sync?FORMAT=csv&LANG=ADQL&QUERY=SELECT+%2A+FROM+galcen_psc+WHERE+CONTAINS%28POINT%28%27ICRS%27%2C+ra%2C+dec%29%2C+CIRCLE%28%27ICRS%27%2C+266.4168371%2C+-29.0078106%2C+0.1%29%29+%3D+1&REQUEST=doQuery
}

// Dry fetches say what they would bring and where. They bring nothing.
func Example_fetch() {
	fmt.Print(said("fetch", "--archive", "hips", "--ra", "266.4168371",
		"--dec", "-29.0078106", "--radius", "0.1", "--pixels", "256",
		"--into", "sky", "--dry-run"))
	// Output:
	// would fetch https://alasky.cds.unistra.fr/hips-image-services/hips2fits?coordsys=icrs&dec=-29.0078106&format=fits&fov=0.2&height=256&hips=CDS%2FP%2FDSS2%2Fred&projection=TAN&ra=266.4168371&width=256
	//     into sky/hips/CDS_P_DSS2_red_266.4168371_-29.0078106_r0.1.fits
}

// A dry fetch of a set names each file it would bring. It asks only for
// the colourways listing, which a fake chroma answers, so the example
// asks nothing of the network.
func Example_fetchSet() {
	get := mocks.NewMockWeb(gomock.NewController(stopping{}))
	get.EXPECT().Get(chromaSources).DoAndReturn(colourwayPages().get)
	fmt.Print(saidThrough(get, "fetch-set", "grissom", "--into",
		"grissom-data", "--dry-run"))
	// Output:
	// would fetch sgra/glimpse/GLM_00000+0000_mosaic_I1.fits
	// would fetch sgra/glimpse/GLM_00000+0000_mosaic_I4.fits
	// would fetch sgra/meerkat/MeerKAT_Galactic_Centre_1284MHz-StokesI.fits
	// would fetch sgra/ppmap/l000_cdens.fits
	// would fetch earth/world.200407.3x5400x2700.jpg
	// would fetch earth/BlackMarble_2016_01deg.jpg
	// would fetch earth/ASCIIdata_splib07a.zip
	// would fetch earth/ne_50m_land.geojson
	// would fetch sky/skyview-Mellinger-Green-3600.fits
	// would fetch sky/skyview-H-Alpha-Comp-3600.fits
	// would fetch sky/skyview-408MHz-3600.fits
	// would fetch sky/skyview-IRIS-100-1800.fits
	// would fetch colourways/aqua.css
	// would fetch colourways/twilight.css
	// fetch-set grissom: 14 to fetch, 0 kept, in grissom-data
}

// Every verb has its example, and pzm holds each to what it prints.
func TestEveryVerbHasItsExample(t *testing.T) {
	tool := command{picked: picked(t, numbersRaw)}
	if err := tool.declared().Examples(os.DirFS(".")); err != nil {
		t.Error(err)
	}
}
