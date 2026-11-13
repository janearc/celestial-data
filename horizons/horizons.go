// Package horizons asks JPL's Horizons system where the sun, the earth
// and the moon stand as seen from the James Webb Space Telescope at one
// moment: 20:53 on 2027-05-22, when the star Gaia DR3
// 4081136684118965760 reaches the edge of where Webb may point.
//
// That is what a picture of Webb at that moment needs and the
// telescope's pointing does not give: where on its orbit about L2 it is.
//
// Horizons is the ephemeris service of JPL's Solar System Dynamics
// group, and carries Webb as a body, -170. Each question is one address
// of its API, read on 2026-11-12 from
// https://ssd-api.jpl.nasa.gov/doc/horizons.html.
//
// Its answer is kept as the text Horizons sends, not parsed: the reader
// works from it and cites it.
//
// What the answers are, so they are not read for more:
//   - the telescope's place then is a prediction. Webb holds its orbit
//     by a burn every few weeks, each planned nearer its time; NAIF's
//     note on Webb's trajectory files says the predicted one runs about
//     a year forward.
//   - the vectors are geometric, with no light time or aberration, in
//     kilometres and kilometres a second, against the ICRF's equator.
//   - the moment is on Horizons' own clock for a vector table,
//     barycentric dynamical time, which stood 69 seconds from UTC in
//     2026. Nothing as seen from Webb moves by an arcsecond in that.
//
// JPL is run for NASA, and Horizons asks for no key and no login. It
// takes an address to write to if something goes wrong; none is sent.
package horizons

import "github.com/janearc/celestial-data"

// adapterName is the archive's name, as the command line takes it.
const adapterName = "horizons"

// How every question is asked.
const (
	// api is where Horizons answers.
	api = "https://ssd.jpl.nasa.gov/api/horizons.api"
	// asWebbSeesIt is the rest of every question after the body: a
	// table of state vectors, as text, with Webb at the middle, from
	// the moment to a minute after it in one step, against the ICRF's
	// equator, in kilometres and seconds, a line a time.
	//
	// An apostrophe is %27 and a space %20, as the API's page says to
	// write them.
	asWebbSeesIt = "&CENTER=%27500@-170%27" +
		"&START_TIME=%272027-05-22%2020:53%27" +
		"&STOP_TIME=%272027-05-22%2020:54%27&STEP_SIZE=%271%20min%27" +
		"&REF_PLANE=FRAME&REF_SYSTEM=ICRF&VEC_TABLE=2&OUT_UNITS=KM-S" +
		"&CSV_FORMAT=YES"
	// asked begins every question: text back, an ephemeris and not the
	// body's page of facts.
	asked = "?format=text&MAKE_EPHEM=YES&OBJ_DATA=NO&EPHEM_TYPE=VECTORS" +
		"&COMMAND=%27"
	// about is a few words on what each file is, after the body's name.
	about = " as seen from Webb at 2027-05-22 20:53, from JPL Horizons"
)

// The bodies asked after, by Horizons' own numbers for them.
const (
	sunBody   = "10"
	earthBody = "399"
	moonBody  = "301"
)

// question is the address that asks where a body stands as seen from
// Webb at the moment.
func question(body string) string {
	return api + asked + body + "%27" + asWebbSeesIt
}

// seen is the file of where a body stands as seen from Webb, by the
// body's name and Horizons' number for it. Its length is not known
// until it is served.
func seen(name, body string) celestialdata.File {
	return celestialdata.NewFile(name+" from webb",
		name+"-from-webb-2027-05-22.txt", question(body),
		celestialdata.Unsized, "the "+name+about)
}

// New is the archive of Horizons' answers, asked by name only. It holds
// the same three whatever the circle: the sun, the earth and the moon,
// each as seen from Webb at its moment.
func New() (*celestialdata.Fixed, error) {
	return celestialdata.NewFixed(adapterName, seen("sun", sunBody),
		seen("earth", earthBody), seen("moon", moonBody))
}
