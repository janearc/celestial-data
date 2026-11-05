// Package cds names cutouts from the hips2fits service at the Centre de
// Données astronomiques de Strasbourg. The service draws any HiPS survey
// onto a FITS image of the square round a circle. Its host can be set,
// since a mirror may be needed when Strasbourg cannot be reached.
package cds

import (
	"fmt"
	"net/url"
	"strconv"
	"strings"

	"github.com/janearc/celestial-data"
)

// Strasbourg is the service's own host.
const Strasbourg = "alasky.cds.unistra.fr"

// service is the service's path on its host.
const service = "/hips-image-services/hips2fits"

// The numbers a cutout is asked for with.
const (
	least    = 1   // defined: an image holds at least one pixel a side
	across   = 2   // defined: a circle's square is twice its radius across
	plain    = 'f' // defined by strconv: no exponent
	shortest = -1  // defined by strconv: the fewest digits that read back
	bits     = 64  // defined: a float64 is 64 bits
)

// Cutouts is one HiPS survey, drawn by one host's hips2fits.
type Cutouts struct {
	base       *celestialdata.Adapter
	host, hips string
	pixels     int
}

// New is the survey with a HiPS id such as CDS/P/DSS2/red. It is drawn by
// the service on a host at pixels a side. It refuses no host and a host
// that is more than a name. It refuses no survey and no pixels.
func New(host, hips string, pixels int) (*Cutouts, error) {
	base, err := celestialdata.Named("hips")
	if err == nil && (host == "" || strings.ContainsAny(host, "/:?#@ ") ||
		hips == "" || pixels < least) {
		err = base.Refuse("needs a host's name, a survey and a size "+
			"in pixels, not %q, %q and %d", host, hips, pixels)
	}
	if err != nil {
		return nil, err
	}
	return &Cutouts{base: base, host: host, hips: hips,
		pixels: pixels}, nil
}

// Name is the adapter's name as the command line takes it.
func (cutouts *Cutouts) Name() string { return cutouts.base.Name() }

// Find is one file: the survey drawn on the square round the circle. It
// is in the tangent projection with north up. Nothing is asked of the
// service until it is fetched.
func (cutouts *Cutouts) Find(near *celestialdata.Near) (
	[]*celestialdata.Product, error) {
	centre := near.Centre()
	ra, dec := number(centre.RA()), number(centre.Dec())
	radius := number(near.Radius())
	side := strconv.Itoa(cutouts.pixels)
	asked := url.Values{}
	asked.Set("hips", cutouts.hips)
	asked.Set("ra", ra)
	asked.Set("dec", dec)
	fov := number(near.Radius() * across)
	asked.Set("fov", fov)
	asked.Set("width", side)
	asked.Set("height", side)
	asked.Set("projection", "TAN")
	asked.Set("coordsys", "icrs")
	asked.Set("format", "fits")
	address := "https://" + cutouts.host + service + "?" + asked.Encode()
	survey := strings.ReplaceAll(cutouts.hips, "/", "_")
	name := fmt.Sprintf("%s_%s_%s_r%s.fits", survey, ra, dec, radius)
	about := fmt.Sprintf("%s within %s degrees, %s pixels a side",
		cutouts.hips, radius, side)
	return celestialdata.Found(cutouts.base.Product(cutouts.hips, name,
		address, celestialdata.Unsized, about))
}

// number is a number as the service and a file name write it.
func number(value float64) string {
	return strconv.FormatFloat(value, plain, shortest, bits)
}
