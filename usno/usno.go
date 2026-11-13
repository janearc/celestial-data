// Package usno fetches pages the US Naval Observatory's Astronomical
// Applications Department publishes, which celestial reads its formulas
// from: first, its approximate solar coordinates, the sun's place to
// about an arcminute within two centuries of 2000.
//
// A page is fetched to be read and kept, not parsed: celestial derives
// from it by hand and cites it, and cited-sources keeps the copy.
//
// The observatory is part of the US Navy, and a work of the US
// government is not under copyright in the US (17 U.S.C. 105; recalled,
// not read here).
package usno

import "github.com/janearc/celestial-data"

// adapterName is the archive's name, as the command line takes it.
const adapterName = "usno"

// The approximate solar coordinates page, as the observatory serves it.
const (
	// sunID is the id of the page's product.
	sunID = "approximate solar coordinates"
	// sunName is the name the page is kept by.
	sunName = "sun-approx.html"
	// sunAddress is where the observatory serves it.
	sunAddress = "https://aa.usno.navy.mil/faq/sun_approx"
	// sunAbout is a few words on what it is.
	sunAbout = "USNO's approximate solar coordinates, a US government work"
)

// New is the archive of the observatory's pages, asked by name only. It
// holds the same pages whatever the circle. A page's length is not known
// until it is served.
func New() (*celestialdata.Fixed, error) {
	sun := celestialdata.NewFile(sunID, sunName, sunAddress,
		celestialdata.Unsized, sunAbout)
	return celestialdata.NewFixed(adapterName, sun)
}
