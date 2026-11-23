package main

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"strings"

	"github.com/janearc/celestial-data"
	"github.com/janearc/celestial-data/apogee"
	"github.com/janearc/celestial-data/astronn"
	"github.com/janearc/celestial-data/bluemarble"
	"github.com/janearc/celestial-data/bsc5"
	"github.com/janearc/celestial-data/cardiff"
	"github.com/janearc/celestial-data/cds"
	"github.com/janearc/celestial-data/chandra"
	"github.com/janearc/celestial-data/chroma"
	"github.com/janearc/celestial-data/dasch"
	"github.com/janearc/celestial-data/gaia"
	"github.com/janearc/celestial-data/galah"
	"github.com/janearc/celestial-data/horizons"
	"github.com/janearc/celestial-data/irsa"
	"github.com/janearc/celestial-data/k2"
	"github.com/janearc/celestial-data/keck"
	"github.com/janearc/celestial-data/lamost"
	"github.com/janearc/celestial-data/mast"
	"github.com/janearc/celestial-data/naturalearth"
	"github.com/janearc/celestial-data/netcdf"
	"github.com/janearc/celestial-data/nistxray"
	"github.com/janearc/celestial-data/ntrs"
	"github.com/janearc/celestial-data/nustar"
	"github.com/janearc/celestial-data/plates"
	"github.com/janearc/celestial-data/refractiveindex"
	"github.com/janearc/celestial-data/sarao"
	"github.com/janearc/celestial-data/sdssv"
	"github.com/janearc/celestial-data/soniakeys"
	"github.com/janearc/celestial-data/spherex"
	"github.com/janearc/celestial-data/starhorse"
	"github.com/janearc/celestial-data/tap"
	"github.com/janearc/celestial-data/tess"
	"github.com/janearc/celestial-data/usgs"
	"github.com/janearc/celestial-data/usno"
	"github.com/janearc/celestial-data/web"
	"github.com/janearc/puzzlemouth/pzm"
)

// The pages pzm shows for each verb's --help.
const (
	archivesPage = "Lists each archive celestial-data asks, by the " +
		"name the " +
		"other verbs take, with a line on what it holds."
	findPage = "Names the files each archive holds for a circle on " +
		"the sky. For each it says the archive, the archive's id for " +
		"it and the file's name, then a line on what it is and where " +
		"it is kept. Nothing is fetched."
	fetchPage = "Brings the files find names into a folder for each " +
		"archive under --into. A file already there is kept and " +
		"said so. A file that does not come whole is not kept. " +
		"--match keeps to the files whose name holds it."
	fetchSetPage = "Brings a named set of files home, as in " +
		"fetch-set grissom. A set says what to fetch into which " +
		"subfolder, and the circle on the sky it is asked about. " +
		"--into names the folder. Without it the folder is the " +
		"variable named for the set, upper case and _DATA: " +
		"GRISSOM_DATA for grissom. A file already there is kept and " +
		"said so, never fetched again. One failure does not stop the " +
		"others. --dry-run names the files it would bring and " +
		"fetches none. It asks an archive only for the files a set " +
		"does not name."
)

// The settings a call may give, and what they are when it gives none.
const (
	// survey is the HiPS survey drawn when a call names none: the red
	// plates of the Digitized Sky Survey.
	survey = "CDS/P/DSS2/red"
	// catalogue is the IRSA table celestial-data names the rows of: the
	// Galactic Center IRAC catalog, Ramirez et al. 2008.
	catalogue = "galcen_psc"
	// everyColumn asks a catalogue for all of its columns. It's defined
	// by ADQL.
	everyColumn = "*"
	// bits is a float64's size. It's defined.
	bits = 64
)

// What the gaia archive is asked of each star. Picked on 2026-11-05
// for nilgai's search for stars thrown from Sgr A*. Where it is and how
// it moves, with each error and correlation; how bright and what colour;
// how good the fit; and what the parallax's zero point is read from.
const (
	gaiaColumns = "source_id, ref_epoch, ra, ra_error, dec, dec_error, " +
		"parallax, parallax_error, pmra, pmra_error, pmdec, " +
		"pmdec_error, radial_velocity, radial_velocity_error, " +
		"ra_dec_corr, ra_parallax_corr, ra_pmra_corr, ra_pmdec_corr, " +
		"dec_parallax_corr, dec_pmra_corr, dec_pmdec_corr, " +
		"parallax_pmra_corr, parallax_pmdec_corr, pmra_pmdec_corr, " +
		"ruwe, phot_g_mean_mag, bp_rp, nu_eff_used_in_astrometry, " +
		"pseudocolour, ecl_lat, astrometric_params_solved"
	// gaiaWhere keeps to the stars with a radial velocity, about 33.8
	// million by the archive's data model.
	gaiaWhere = "radial_velocity IS NOT NULL"
)

// What gaia-near asks of every source in a circle, picked on 2026-11-07
// for the neighbours SPHEREx's crowded cutouts need fitted beside a
// star: where each is at its epoch, how it moves, and how bright.
const gaiaNearColumns = "source_id, ref_epoch, ra, dec, parallax, " +
	"pmra, pmdec, phot_g_mean_mag, phot_bp_mean_mag, phot_rp_mean_mag"

// What gaia-rv asks of the stars in a circle that have a radial
// velocity, picked on 2026-11-07: the spectrograph's own account of each
// velocity, so one can be judged before it is believed. The names are
// the data model's for gaia_source, read that day.
const gaiaRVColumns = "source_id, ra, dec, radial_velocity, " +
	"radial_velocity_error, rv_method_used, rv_nb_transits, " +
	"rv_nb_deblended_transits, rv_visibility_periods_used, " +
	"rv_expected_sig_to_noise, rv_renormalised_gof, rv_chisq_pvalue, " +
	"rv_time_duration, rv_amplitude_robust, rv_template_teff, " +
	"rv_template_logg, rv_template_fe_h, rv_atm_param_origin, " +
	"grvs_mag, grvs_mag_error"

// What gaia-distance asks, picked on 2026-11-07 for firang's candidates:
// each source's distance as Bailer-Jones et al. (2021) give it from its
// EDR3 parallax, geometric and photogeometric, with their 16th and 84th
// percentiles and the flag that says how each was made.
//
// The table has no places of its own, so it is joined to gaia_source on
// the source_id DR3 shares with EDR3. The names are the archive's for
// external.gaiaedr3_distance, read that day.
const (
	gaiaDistanceTable = "gaiadr3.gaia_source AS g JOIN " +
		"external.gaiaedr3_distance AS d ON g.source_id = d.source_id"
	gaiaDistanceColumns = "g.source_id, g.ra, g.dec, d.r_med_geo, " +
		"d.r_lo_geo, d.r_hi_geo, d.r_med_photogeo, d.r_lo_photogeo, " +
		"d.r_hi_photogeo, d.flag"
)

// holds is what each archive holds in a line, by its name.
var holds = map[string]string{
	"chandra": "Chandra's primary products: events, field of view, aspect",
	"nustar":  "NuSTAR's cleaned events, for both focal plane modules",
	"glimpse": "Spitzer GLIMPSE II mosaics, in IRAC's four bands",
	catalogue: "IRSA's Galactic Center IRAC catalog, as CSV",
	"hpsgc":   "the HST/NICMOS Paschen-alpha survey of the Galactic Centre",
	"meerkat": "MeerKAT's Galactic Centre mosaic at 1.28 GHz",
	"ppmap":   "Hi-GAL PPMAP maps of dust column density and temperature",
	"hips":    "a hips2fits cutout of any HiPS survey",
}

// holdsByName is the same for each archive that is asked only when it is
// named, so it is not in holds, which is those asked with no name.
var holdsByName = map[string]string{
	"bluemarble":   "NASA's Blue Marble, July 2004, and Black Marble 2016",
	"usgs":         "the USGS Spectral Library Version 7, as one archive",
	"naturalearth": "Natural Earth's land outlines at 1:50m, as GeoJSON",
	"netcdf":       "ECMWF ERA-40 surface fields, July 2002, as netCDF",
	"chroma":       "the colourways in janearc/chroma, listed by GitHub",
	"gaia":         "Gaia DR3's stars with a radial velocity, as a job",
	"gaia-near":    "Gaia DR3's every source in a circle, as a job",
	"gaia-rv":      "Gaia DR3's radial velocities in a circle, judged",
	"gaia-distance": "Bailer-Jones' distances for Gaia's " +
		"sources in a circle",
	"apogee": "APOGEE DR17's allStar: each star's parameters and " +
		"abundances",
	"galah": "GALAH DR4's allstar: each star's parameters and " +
		"abundances",
	"bsc5": "the Yale Bright Star Catalogue, naked-eye stars",
	"sdssv": "SDSS-V DR19's BOSS spectra, as " +
		"SkyServer lists them",
	"lamost": "LAMOST DR10's low and medium resolution " +
		"spectra, by TAP",
	"keck": "the Keck Observatory Archive's HIRES " +
		"frames and spectra",
	"astronn":   "astroNN's APOGEE DR17 ages, distances and orbits",
	"starhorse": "StarHorse's APOGEE DR17 masses, distances and ages",
	"tess":      "TESS's light curves at MAST, QLP's among them",
	"spherex": "SPHEREx's spectral images at IRSA, cut " +
		"round a place",
	"plates": "the Digitized Sky Survey's Palomar and " +
		"UK Schmidt plates",
	"k2":    "K2's light curves and full-frame images at MAST",
	"dasch": "DASCH DR7's plate light curves, from Starglass",
	"dasch-pages": "DASCH DR7's own pages the dasch adapter " +
		"was read from",
	"usno": "the US Naval Observatory's pages celestial reads",
	"soniakeys": "Sonia Keys's Meeus tests, to check " +
		"against, not derive",
	"horizons": "JPL Horizons: the sun, earth and moon " +
		"as Webb sees them",
	"ntrs": "NASA's Technical Reports Server, by words",
	"refractiveindex": "the refractiveindex.info database, " +
		"optical constants by wavelength",
	"nistxray": "NIST's X-Ray Mass Attenuation " +
		"Coefficients (SRD 126), by page",
}

// holdsOf is the line on what an archive holds, by its name.
func holdsOf(name string) string {
	if line, ok := holds[name]; ok {
		return line
	}
	return holdsByName[name]
}

// settings are what to ask the archives with.
type settings struct {
	most, pixels int
	host, hips   string
	curves       dasch.Doer
}

// defaults are the settings when a call gives none: the picked numbers,
// Strasbourg and the survey above.
func (tool command) defaults() (*settings, error) {
	most, mostErr := tool.picked.Get("most")
	pixels, pixelsErr := tool.picked.Get("pixels")
	curves, curvesErr := tool.daschAsker()
	if err := errors.Join(mostErr, pixelsErr, curvesErr); err != nil {
		return nil, err
	}
	return &settings{most: int(most), pixels: int(pixels),
		host: cds.Strasbourg, hips: survey, curves: curves}, nil
}

// daschAsker is what dasch is asked through: the one a test gives, or
// the network's, with the whole request held to dasch-whole. Either is
// asked through the tool's hosts: see gentleDoer.
//
// No redirect is followed: a POST redirected would be sent again as a
// GET, or its body re-sent, perhaps off Harvard's host, so a redirect is
// its status, which the asker refuses.
func (tool command) daschAsker() (dasch.Doer, error) {
	if tool.curves != nil {
		return tool.gentleDoer(tool.curves), nil
	}
	whole, err := tool.picked.Get("dasch-whole")
	if err != nil {
		return nil, err
	}
	return tool.gentleDoer(&http.Client{Timeout: web.Seconds(whole),
		CheckRedirect: unfollowed}), nil
}

// unfollowed has a client keep a redirect's answer instead of following
// it.
func unfollowed(*http.Request, []*http.Request) error {
	return http.ErrUseLastResponse
}

// settingsOf are the settings a call gives, and the defaults for the
// rest.
func (tool command) settingsOf(call pzm.Call) (*settings, error) {
	given, err := tool.defaults()
	if err != nil {
		return nil, err
	}
	most, mostErr := count(call, "most", given.most)
	pixels, pixelsErr := count(call, "pixels", given.pixels)
	if err := errors.Join(mostErr, pixelsErr); err != nil {
		return nil, err
	}
	given.most, given.pixels = most, pixels
	given.host = valueOr(call, "hips-host", given.host)
	given.hips = valueOr(call, "hips", given.hips)
	return given, nil
}

// count is the whole number a flag gives, or otherwise when it gives
// none.
func count(call pzm.Call, name string, otherwise int) (int, error) {
	if !call.Given(name) {
		return otherwise, nil
	}
	number, err := strconv.Atoi(call.Value(name))
	if err != nil {
		return otherwise, fmt.Errorf("--%s is a whole number, not %q",
			name, call.Value(name))
	}
	return number, nil
}

// valueOr is the value a flag gives, or otherwise when it gives none.
func valueOr(call pzm.Call, name, otherwise string) string {
	if !call.Given(name) {
		return otherwise
	}
	return call.Value(name)
}

// known is every archive celestial-data asks, in the order it asks them. Any
// archive that refuses its settings is an error, with the rest.
func known(get web.Web, with *settings) ([]celestialdata.Archive, error) {
	var asked shelf
	chandraArchive, err := chandra.New(get, with.most)
	asked.keep(chandraArchive, err)
	nustarArchive, err := nustar.New(get, with.most)
	asked.keep(nustarArchive, err)
	glimpse, err := irsa.NewGlimpse(get, irsa.GlimpseII)
	asked.keep(glimpse, err)
	rows, err := irsa.NewCatalogue(catalogue, everyColumn)
	asked.keep(rows, err)
	field, fieldErr := mast.Footprint()
	paschen, err := mast.New(get, field)
	asked.keep(paschen, errors.Join(fieldErr, err))
	mosaicField, mosaicErr := sarao.Footprint()
	mosaic, err := sarao.New(get, mosaicField)
	asked.keep(mosaic, errors.Join(mosaicErr, err))
	tile, tileErr := cardiff.Footprint()
	results, err := cardiff.New(get, tile)
	asked.keep(results, errors.Join(tileErr, err))
	cutouts, err := cds.New(with.host, with.hips, with.pixels)
	asked.keep(cutouts, err)
	marble, err := bluemarble.New()
	asked.keep(marble, err)
	spectra, err := usgs.New()
	asked.keep(spectra, err)
	land, err := naturalearth.New()
	asked.keep(land, err)
	weather, err := netcdf.New()
	asked.keep(weather, err)
	colours, err := chroma.New(get)
	asked.keep(colours, err)
	stars, err := gaia.New(gaiaColumns, gaiaWhere)
	asked.keep(stars, err)
	neighbours, err := gaia.Called("gaia-near", gaiaNearColumns, "")
	asked.keep(neighbours, err)
	velocities, err := gaia.Called("gaia-rv", gaiaRVColumns, gaiaWhere)
	asked.keep(velocities, err)
	distances, err := gaia.From("gaia-distance", gaiaDistanceTable,
		gaiaDistanceColumns, "")
	asked.keep(distances, err)
	allStar, err := apogee.New()
	asked.keep(allStar, err)
	survey, err := galah.New()
	asked.keep(survey, err)
	bright, err := bsc5.New()
	asked.keep(bright, err)
	boss, err := sdssv.New(get, with.most)
	asked.keep(boss, err)
	fibres, err := lamost.New(get, with.most)
	asked.keep(fibres, err)
	echelle, err := keck.New(get, with.most)
	asked.keep(echelle, err)
	ages, err := astronn.New()
	asked.keep(ages, err)
	fitted, err := starhorse.New()
	asked.keep(fitted, err)
	curves, err := tess.New(get)
	asked.keep(curves, err)
	infrared, err := spherex.New(get, with.most)
	asked.keep(infrared, err)
	photographs, err := plates.New(get)
	asked.keep(photographs, err)
	kepler, err := k2.New(get)
	asked.keep(kepler, err)
	harvard, err := dasch.New(with.curves, agent, with.most)
	asked.keep(harvard, err)
	read, err := dasch.Pages()
	asked.keep(read, err)
	observatory, err := usno.New()
	asked.keep(observatory, err)
	transcriptions, err := soniakeys.New()
	asked.keep(transcriptions, err)
	ephemeris, err := horizons.New()
	asked.keep(ephemeris, err)
	asked.keep(ntrs.New(), nil)
	optical, err := refractiveindex.New()
	asked.keep(optical, err)
	attenuation, err := nistxray.New()
	asked.keep(attenuation, err)
	if err := errors.Join(asked.refused...); err != nil {
		return nil, err
	}
	return asked.archives, nil
}

// shelf is the archives made so far, and the refusals of those that
// were not.
type shelf struct {
	archives []celestialdata.Archive
	refused  []error
}

// keep puts an archive on the shelf, or its refusal beside it.
func (kept *shelf) keep(archive celestialdata.Archive, err error) {
	if err != nil {
		kept.refused = append(kept.refused, err)
		return
	}
	kept.archives = append(kept.archives, archive)
}

// chosen is the archives a call names, or when it names none, every one
// but those asked only by name, whose files do not depend on the circle.
func chosen(archives []celestialdata.Archive,
	names []string) ([]celestialdata.Archive, error) {
	if len(names) == none {
		every := slices.Clone(archives)
		return slices.DeleteFunc(every, byNameOnly), nil
	}
	var picked []celestialdata.Archive
	var unknown []string
	for _, name := range names {
		found := false
		for _, archive := range archives {
			if archive.Name() == name {
				picked = append(picked, archive)
				found = true
			}
		}
		if !found {
			unknown = append(unknown, name)
		}
	}
	if len(unknown) > none {
		return nil, fmt.Errorf("no archive is called %s; "+
			"celestial-data "+
			"archives lists them", strings.Join(unknown, " or "))
	}
	return picked, nil
}

// byNameOnly is whether an archive is asked only when it is named.
func byNameOnly(archive celestialdata.Archive) bool {
	named, ok := archive.(celestialdata.ByName)
	return ok && named.ByNameOnly()
}

// none is no names, and no archives. It's defined.
const none = 0

// circle is the circle a call names with --ra, --dec and --radius.
func circle(call pzm.Call) (*celestialdata.Near, error) {
	ra, raErr := strconv.ParseFloat(call.Value("ra"), bits)
	dec, decErr := strconv.ParseFloat(call.Value("dec"), bits)
	radius, radiusErr := strconv.ParseFloat(call.Value("radius"), bits)
	if err := errors.Join(raErr, decErr, radiusErr); err != nil {
		return nil, fmt.Errorf("--ra, --dec and --radius are numbers "+
			"of degrees: %w", err)
	}
	centre, err := celestialdata.NewPosition(ra, dec)
	if err != nil {
		return nil, err
	}
	return celestialdata.NewNear(centre, radius)
}

// search is what a call asks to search: the circle, the web and the
// archives.
type search struct {
	near     *celestialdata.Near
	get      web.Web
	curves   dasch.Doer
	archives []celestialdata.Archive
}

// searchOf is the search a call asks for.
func (tool command) searchOf(call pzm.Call) (*search, error) {
	near, err := circle(call)
	if err != nil {
		return nil, err
	}
	with, err := tool.settingsOf(call)
	if err != nil {
		return nil, err
	}
	get, err := tool.insistedWeb(call)
	if err != nil {
		return nil, err
	}
	every, err := known(get, with)
	if err != nil {
		return nil, err
	}
	archives, err := chosen(every, call.List("archive"))
	if err != nil {
		return nil, err
	}
	return &search{near: near, get: get, curves: with.curves,
		archives: archives}, nil
}

// archivesVerb lists the archives celestial-data asks.
func (tool command) archivesVerb(call pzm.Call) error {
	with, err := tool.defaults()
	if err != nil {
		return err
	}
	get, err := tool.web()
	if err != nil {
		return err
	}
	archives, err := known(get, with)
	if err != nil {
		return err
	}
	width := none
	for _, archive := range archives {
		name := archive.Name()
		width = max(width, len(name))
	}
	for _, archive := range archives {
		name := archive.Name()
		line := holdsOf(name)
		fmt.Fprintf(call.Out(), "%-*s  %s\n", width, name, line)
	}
	return nil
}

// findVerb names what the archives hold in the circle, after saying the
// circle in both frames. Each archive that cannot be asked is said at the
// end, and the rest are still asked.
func (tool command) findVerb(call pzm.Call) error {
	asked, err := tool.searchOf(call)
	if err != nil {
		return err
	}
	centre := asked.near.Centre()
	galactic := centre.Galactic()
	fmt.Fprintf(call.Out(), "round ra %v, dec %v; l %.4f, b %.4f; within "+
		"%v degrees\n", centre.RA(), centre.Dec(), galactic.Longitude(),
		galactic.Latitude(), asked.near.Radius())
	var failed []error
	for _, archive := range asked.archives {
		products, err := archive.Find(asked.near)
		failed = append(failed, named(archive, err))
		for _, product := range products {
			say(call.Out(), product)
		}
	}
	return errors.Join(failed...)
}

// fetchVerb brings what the archives hold in the circle into a folder
// for each archive. Files that cannot be fetched are said at the end.
// The rest are still fetched.
func (tool command) fetchVerb(call pzm.Call) error {
	into := call.Value("into")
	if into == "" {
		return celestialdata.Refuse("fetch needs --into, the folder " +
			"to " +
			"bring files into")
	}
	asked, err := tool.searchOf(call)
	if err != nil {
		return err
	}
	match := call.Value("match")
	get, err := tool.fetchWeb(asked, call.Out())
	if err != nil {
		return err
	}
	var failed []error
	for _, archive := range asked.archives {
		products, err := archive.Find(asked.near)
		failed = append(failed, named(archive, err))
		folder := filepath.Join(into, archive.Name())
		brought := fetchEach(call, get, products, folder, match)
		failed = append(failed, brought...)
	}
	return errors.Join(failed...)
}

// fetchEach fetches each of the products whose name holds match into the
// folder, and gives what went wrong with each that failed. When an
// archive asks us to stop, it stops, and says how many were not asked.
func fetchEach(call pzm.Call, get web.Web, products []*celestialdata.Product,
	folder, match string) []error {
	var failed []error
	for at, product := range products {
		if strings.Contains(product.Name(), match) {
			err := fetchOne(call, get, product, folder)
			failed = append(failed, err)
			if errors.Is(err, dasch.ErrStopped) {
				left := matching(products[at+thisOne:], match)
				return append(failed, fmt.Errorf(
					"stopped: %d more "+
						"of its files were not "+
						"asked for", left))
			}
		}
	}
	return failed
}

// thisOne is the product just asked for, past which the rest begin. It's
// defined.
const thisOne = 1

// matching is how many of some products have a name that holds match:
// those a fetch would have asked for, not those --match left out.
func matching(products []*celestialdata.Product, match string) int {
	count := none
	for _, product := range products {
		if strings.Contains(product.Name(), match) {
			count++
		}
	}
	return count
}

// fetchOne brings one file into a folder. When the call is dry it says
// what it would do instead. Files already there are kept and said so.
func fetchOne(call pzm.Call, get web.Web, product *celestialdata.Product,
	folder string) error {
	home := filepath.Join(folder, product.Name())
	if call.DryRun() {
		fmt.Fprintf(call.Out(), "would fetch %s\n    into %s\n",
			product.URL(), home)
		return nil
	}
	path, err := celestialdata.Fetch(get, product, folder)
	if err == nil {
		fmt.Fprintf(call.Out(), "fetched %s\n", path)
		return nil
	}
	if _, there := os.Lstat(home); there == nil &&
		errors.Is(err, celestialdata.Refused()) {
		fmt.Fprintf(call.Out(), "kept %s, already there\n", home)
		return nil
	}
	return fmt.Errorf("%s: %w", product.Name(), err)
}

// named is an archive's error with the archive's name, or nothing.
func named(archive celestialdata.Archive, err error) error {
	if err == nil {
		return nil
	}
	return fmt.Errorf("%s: %w", archive.Name(), err)
}

// say writes what find knows of one file. First come the archive, its id
// and the name. Then come what it is and where it is kept.
func say(out io.Writer, product *celestialdata.Product) {
	fmt.Fprintf(out, "%s %s %s\n    %s\n    %s\n", product.Archive(),
		product.ID(), product.Name(), product.About(), product.URL())
}

// fetchWeb is the web fetch brings files through: the search's web, with
// jobs run as jobs and dasch's light curves asked as its API takes them.
func (tool command) fetchWeb(asked *search, told io.Writer) (web.Web,
	error) {
	jobs, err := tool.jobs(asked.get, told)
	if err != nil {
		return nil, err
	}
	curves, err := dasch.NewLightcurves(jobs, asked.curves, agent)
	if err != nil {
		return nil, err
	}
	return curves, nil
}

// jobs is a web over under that runs a query asked of an async endpoint
// as a job, asking after it as often and as many times as the numbers
// say, and saying each job's address to told.
func (tool command) jobs(under web.Web, told io.Writer) (web.Web,
	error) {
	every, err := tool.picked.Get("job-every")
	if err != nil {
		return nil, err
	}
	asks, err := tool.picked.Get("job-asks")
	if err != nil {
		return nil, err
	}
	// nothing above jobs carries a context yet
	return tap.NewJobs(context.Background(), under, web.Wall(),
		web.Seconds(every), int(asks), told)
}
