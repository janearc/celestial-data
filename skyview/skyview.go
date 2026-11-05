// Package skyview asks NASA's SkyView for whole-sky maps: any of its
// surveys resampled onto one image of the sky in galactic longitude and
// latitude, on a plain grid, as FITS.
//
// Each survey carries its own terms, which SkyView's survey page gives:
// 408 MHz, for one, is the Max-Planck-Institut fur Radioastronomie's
// "permission for educational and private non-commercial use granted
// without further request".
//
// This package only fetches to the machine it runs on; nothing it
// fetches is shipped.
//
// The archive holds one map for each survey it is given, wherever it is
// asked, and is asked only by name.
package skyview

import (
	"fmt"
	"net/url"
	"strings"

	"github.com/janearc/celestial-data"
)

// archive is the archive's name as the command line takes it.
const archive = "skyview"

// The query SkyView answers.
const (
	// query is SkyView's batch query.
	query = "https://skyview.gsfc.nasa.gov/current/cgi/runquery.pl"
	// halfTurn and quarterTurn are the sky's width and height in degrees.
	// They're defined.
	halfTurn, quarterTurn = 360, 180
	// tall is how many times wider than tall a whole-sky map is. It's
	// defined by a turn of longitude against half a turn of latitude.
	tall = 2
	// noSurveys is an archive of no surveys. It's defined.
	noSurveys = 0
)

// New is the archive of whole-sky maps, one for each survey, by SkyView's
// name for it. Each is so many pixels wide and half as many tall, centred
// on the galactic centre. It refuses no survey, a survey with no name, and
// a width below two pixels.
func New(width int, surveys ...string) (*celestialdata.Fixed, error) {
	if len(surveys) == noSurveys {
		return nil, celestialdata.Refuse("skyview needs a survey")
	}
	maps := make([]celestialdata.File, len(surveys))
	for at, survey := range surveys {
		if survey == "" || width < tall {
			return nil, needs(survey, width)
		}
		maps[at] = allSky(survey, width)
	}
	return celestialdata.NewFixed(archive, maps...)
}

// needs is the refusal of a survey with no name or a width below two.
func needs(survey string, width int) error {
	return celestialdata.Refuse("skyview needs a survey and a width, "+
		"not %q and %d", survey, width)
}

// allSky is a whole-sky map of a survey, as a file.
func allSky(survey string, width int) celestialdata.File {
	asked := url.Values{}
	asked.Set("Survey", survey)
	asked.Set("Projection", "Car")
	asked.Set("Coordinates", "Galactic")
	asked.Set("Position", "0,0")
	asked.Set("Size", fmt.Sprintf("%d,%d", halfTurn, quarterTurn))
	asked.Set("Pixels", fmt.Sprintf("%d,%d", width, width/tall))
	asked.Set("Return", "FITS")
	name := fmt.Sprintf("skyview-%s-%d.fits", plain(survey), width)
	about := "SkyView whole-sky " + survey +
		"; its terms are on SkyView's survey page"
	return celestialdata.NewFile(survey, name, query+"?"+asked.Encode(),
		celestialdata.Unsized, about)
}

// plain is a survey's name as a file name: letters and digits kept, the
// rest made hyphens.
func plain(survey string) string { return strings.Map(hyphened, survey) }

// hyphened is a letter or digit as it is, and anything else a hyphen.
func hyphened(letter rune) rune {
	if letter >= 'a' && letter <= 'z' || letter >= 'A' && letter <= 'Z' ||
		letter >= '0' && letter <= '9' {
		return letter
	}
	return '-'
}
