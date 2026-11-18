# celestial-data

data files that celestial's tests and examples read: FITS, HDF5, and
the catalogues they come from, and the fetchers that bring them from the
archives that hold them.

every file has an entry below, written when it is added:

- the path, and what the file is
- where it came from, as a url, and the date it was fetched
- the licence its publisher states, in their words
- its size in bytes and its sha256

## fetching

```sh
$ celestial-data archives
$ celestial-data find --ra 266.4168371 --dec -29.0078106 --radius 0.1
$ celestial-data fetch --archive nustar,ppmap --ra 266.4168371 \
    --dec -29.0078106 --radius 0.1 --into ~/sky --dry-run
```

`find` names what each archive holds for the circle and fetches nothing.
`fetch` brings it home, one folder an archive under `--into`. it never
replaces a file: one already there is kept and said so, and one that did
not come whole is not kept. `--match` keeps to the names that hold it.
`--archive` names one or more archives, separated by commas; with none,
every archive is asked.

## sets

a set is a name for a list of what to fetch and which subfolder of one
folder it goes into.

```sh
$ celestial-data fetch-set grissom --into ~/grissom-data --dry-run
$ GRISSOM_DATA=~/grissom-data celestial-data fetch-set grissom
```

`--into` names the folder. without it the folder is the variable named
for the set, in upper case and `_DATA`: `GRISSOM_DATA` for grissom. with
neither, it refuses and says so. a file already there is kept and said
so, never fetched again; one line a file. a file a set names that its
archive does not list is an error that names it. one failure does not
stop the others. `--dry-run` fetches nothing; it asks an archive only for
the files a set does not name.

the sets are in `cmd/celestial-data/sets.go`. the numbers a set picks are
in `cmd/celestial-data/numbers.yaml`.

grissom
: what the grissom viewer reads. round Sgr A* (0.1 degrees): the two
  GLIMPSE mosaics `GLM_00000+0000_mosaic_I1.fits` and `..._I4.fits` into
  `sgra/glimpse/`, the MeerKAT 1284 MHz Stokes I mosaic into
  `sgra/meerkat/`, and `l000_cdens.fits` into `sgra/ppmap/`. all of
  bluemarble, usgs and naturalearth into `earth/`. four whole-sky maps
  from SkyView into `sky/`. every colourway the chroma archive lists
  into `colourways/`.

## dasch

dasch brings the light curves of Harvard's plate archive (DASCH DR7)
home, as CSV, for the APASS sources in a circle.

```sh
$ celestial-data fetch --archive dasch --ra 287.50086 --dec -22.23481 \
    --radius 0.0014 --into ~/sky
```

DR7 gives a light curve only to a POST of JSON, at the public tier of
Harvard's Starglass API, which asks for no key. `find` asks it once, for
the APASS sources in the circle, and names a light curve for each, the
nearest first, at most `--most` of them. `fetch` asks once more for each
light curve it keeps. a light curve's address holds the question; it is
no address a plain GET opens, and it is only `fetch` that answers it.

the file is every measure DASCH has of the source, flagged or not: a
line a plate, the first line the names of the columns. a plate that
covers the place and saw nothing has a line too, with only its date,
plate and limiting magnitude. the measures are the lines that have a
`ref_number`. nothing is dropped here: choosing is the reader's.

a disagreement, said so it is not read as settled: DASCH's web APIs
page, kept on 2026-11-09, says the raw API names its columns in camel
case (`refNumber`, `dateJd`), not the snake case of its column pages.
the API's own answers of 2026-11-08 were in snake case: the light curve
kept as it came begins `date_jd,series,plate_number`, and that day's
catalogue query passed a reader that took only `ref_number` and its
neighbours. the page and the answers disagree; the readers take either
spelling, and refuse a first line that names one column both ways.

what DASCH's own column page says, read in its kept copy on 2026-11-09
(dasch/sources.yaml has its address and hash):

- the magnitude to take is `magcal_magdep` ("Use this value"), with
  `magcal_local_rms` as its uncertainty.
- daschlab's `time` is the HJD midpoint of the exposure, and DASCH has
  not yet said its time scale. the page describes daschlab's column;
  that the API's `date_jd` is the same is not said there.
- `aflags`, `a2flags`, `bflags` and `b2flags` are "not yet documented
  here", pointing to DASCH's Legacy Data Guide, not yet read. firang read
  "unflagged" as `aflags` of zero, which reproduced its count; no reading
  should rest on the bits until the guide is read.

`dasch-pages` brings those pages home, asked by name: DR7's web APIs,
data products and light curve columns, the pages this adapter was read
from. the copies are kept in `cited-sources/`, which `.game` omits and
excludes: in version control, never in a release.

the circle is asked as a box of at most 3600 arcseconds; a wider one is
refused. find says, in each light curve's description, which of the
circle's sources it is and how many there are, so those past `--most`
are not left out unsaid. a source listed twice, or at a place that is
not on the sky, is an error in the answer, not a source to skip.

the questions are paced: one a second at most, from the start of one to
the start of the next, picked, as Starglass gives its public tier no
number. a 429, Starglass asking us to stop, stops the fetch: nothing
more is sent until the time its Retry-After gives, or five minutes when
it gives none, and fetch says how many light curves were not asked
for. nothing is retried, and no redirect is followed.

an answer is held to 32 MiB, and a light curve to its shape: a first
line of columns beginning `date_jd`, and as many fields on every line.
a 200 that holds anything else is refused, not kept.

## usno

usno brings home the pages of the US Naval Observatory's Astronomical
Applications Department that celestial derives formulas from. today
that is one: its approximate solar coordinates.

```sh
$ celestial-data fetch --archive usno --ra 0 --dec 0 --radius 0.1 \
    --into ~/sources
```

the page is kept whole as html, to be read and cited, not parsed. it is
asked only by name, and the circle is not used. a work of the US
government is not under copyright in the US, so the page can be kept
and derived from. its length is not known until it is served.

nothing here checks that what came is the page: a 200 that holds a
maintenance notice would be kept as `sun-approx.html`. the page is
checked where it is cited instead. celestial keeps the copy it derives
from in `cited-sources/usno-sun-approx.html`, with its sha256, its size
and the time it was fetched in `research/sources.yaml`, and its `.game`
extract step greps the copy for the accuracy line and the four formulas,
so a page that is not the page fails there. the page can change under
the same address: a new fetch is a new copy, with its own hash, and the
fetch's own record is a `FETCHED.md` beside it.

## soniakeys

soniakeys brings home test files of Sonia Keys's Go implementation of
Meeus's Astronomical Algorithms (github.com/soniakeys/meeus, MIT), at
the one commit celestial cites, bfbd9ac. today that is one: her solar
coordinates test, which transcribes the book's example 25.a.

```sh
$ celestial-data fetch --archive soniakeys --ra 0 --dec 0 --radius 0.1 \
    --into ~/sources
```

they are kept to check celestial's work against, never to derive it
from: none of her code, formulas or constants is used, so as not to take
on her licence, and a transcription is not the book. each is kept as a
`.txt`, as celestial's cited-sources keeps her other test files. asked
only by name; the circle is not used.

## horizons

horizons asks JPL's Horizons system where the sun, the earth and the
moon stand as seen from the James Webb Space Telescope at one moment:
20:53 on 2027-05-22, when the star gaia dr3 4081136684118965760 reaches
the edge of where webb may point. a picture of webb then needs it: the
telescope's pointing says which way it faces and not where on its orbit
about L2 it is.

```sh
$ celestial-data fetch --archive horizons --ra 0 --dec 0 --radius 0.1 \
    --into ~/sources
```

three files of text, each as Horizons sends it, one line of a body's
place and speed at the moment and one a minute after: kilometres and
kilometres a second, against the ICRF's equator, with webb at the
middle. asked only by name; the circle is not used. no key and no
login; Horizons takes an address to write to if something goes wrong,
and none is sent.

what the answers are not: webb's place then is a prediction, since it
holds its orbit by a burn every few weeks, each planned nearer its
time. the vectors are geometric, with no light time. the moment is on
Horizons' clock for vectors, barycentric dynamical time, 69 seconds
from UTC, in which nothing seen from webb moves an arcsecond.

## papers

papers brings papers home from arXiv by their ids.

```sh
$ celestial-data papers 1809.03507 astro-ph/0602086 --into ~/papers --dry-run
$ celestial-data papers --list papers.txt --into ~/papers
```

an id is written `1809.03507` or `astro-ph/0602086`, with a version
such as `v2` or without, or as `arXiv:1809.03507` or an `arxiv.org`
abs or pdf address, as `sources.yaml` files cite them. an old-style id
with a subject, `math.GT/0309136`, is asked for as arXiv names the
paper, `math/0309136`: arXiv answers nothing for the other (its reply,
recorded 2026-11-06, is `arxiv/testdata/subject-class.xml`). `--list`
names a file of ids, one a line; what follows a `#` is not read. an id
that is not one is refused before arXiv is asked anything.

a paper is kept as its version: `1809.03507v2.pdf`, and
`astro-ph_0602086v1.pdf` for an old-style id. an id without a version
is the latest arXiv has. a paper already in the folder is kept and
costs no request: any version of it, for an id given without one. a
file of a paper's name that is not a PDF is refused rather than kept;
move it aside. what arXiv sends that is not a PDF is not kept. an id
arXiv has no paper for is said, and the run fails naming it.
`--dry-run` fetches no PDF, but it does ask the API, to name the
versions.

it is gentle with arXiv, as arXiv asks of programs:

- every request, to the API for a paper's details and for its PDF,
  goes through one gate: one at a time, each begun at least three
  seconds after the one before began. arXiv's terms ask that of its
  API; the PDFs are held to it too.
- the pace carries from one run to the next, for this user on this
  machine. the gate keeps when its last request began in
  `celestial-data/arxiv-gate` in the user's cache folder
  (`~/Library/Caches` on a mac), written as each request begins, so a
  run stopped with control-c keeps it too. a lock on
  `arxiv-gate.lock` beside it refuses a second run while one is
  asking; the system lets the lock go when a run ends, however it
  ends, so there is never one to clear by hand. another user, another
  cache folder or another machine is not seen; arXiv counts all of a
  caller's machines together, so run papers on one at a time.
- the pace is `arxiv-pace` in `cmd/celestial-data/numbers.yaml`, and
  nothing will go under arXiv's three seconds, which is in
  `arxiv/numbers.yaml` with where it was read.
- ids are asked twenty a query (`arxiv-most`), so a list of forty is
  two queries and its PDFs.
- it asks `export.arxiv.org`, the host arXiv sets aside for programs.
- a 403, a 429 or a 503 is arXiv asking us to stop. the run stops at
  once, says arXiv's Retry-After and which papers were not fetched,
  and asks nothing more. nothing is retried. the wait is written to the
  record as soon as arXiv asks it, so a run killed before it ends still
  leaves it, and a run before it has passed asks nothing and says when
  it will have: the Retry-After's seconds or date, or five minutes
  (`rest-unsaid`) when arXiv said none. a date already past, from a
  server whose clock is behind ours, is no wait: the next run asks at
  the pace. a wait that cannot be written is said beside the stop.
  any other failure of the API
  stops the run too, before any PDF is asked for: an API that fails is
  not asked for more. a version arXiv does not have is said as not on
  arXiv, and the rest of its query are still found.
- the last line says how many requests went to arXiv.

the API is asked through `github.com/Epistemic-Technology/arxiv`
v1.0.1. its own pacing is turned off, since at that version it paces
by three nanoseconds; the gate paces instead. its requests carry Go's
default user agent, since the library lets none be set; the PDFs carry
`celestial-data`. the library's connection and the PDFs' are separate,
so each may keep one idle connection open; only one is ever in use. a
redirect inside one request is followed as part of it, not paced on
its own. a download has no time limit, for a large file on a slow
link: one that stalls holds the run until it is stopped with
control-c.

that library is under the MIT licence, by its LICENSE.md. a binary
built with it carries it, so the library's copyright and licence notice
must travel with any binary that is shipped.

## ads

`ads` searches NASA's Astrophysics Data System, with the words its own
search box takes: `author:`, `year:`, `full:"words"` and the rest.

```sh
$ celestial-data-secrets ads-login
$ celestial-data ads 'full:"4081136684118965760"'
```

it says how many papers match and the newest of them, twenty unless
`--most` says otherwise, each by bibcode, year, first author and
title, and how many searches ADS allows today. a run sends one
request, and follows no redirect: a redirect is said as the status it
is, so the token goes to ADS's host alone. when ADS asks us to stop,
the run says until when.

the token is read from `ads-token`, beside the secrets' config, where
`celestial-data-secrets ads-login` kept it, readable by its owner
alone, so a search asks no one. `ADS_SESSION` names another file for
both, when set. `ads-login` asks op for it once; see
[secrets](#secrets). a link, a pipe or a folder in the token's place
is refused, and so is a file anyone else can read; a file its owner
alone reads, `-r--------`, is read. a token kept again is written to a
new file and put in the old one's place, so a write that fails leaves
the old token as it was.

## earth sources

`earth-sources` lists the terrestrial catalog. `earth-find` lists products
and `earth-fetch` saves them under `--into/<source>/`. Each query names
one source. Bounds are west, south, east, north in WGS84 degrees. Split
a box at the antimeridian before asking. Points are latitude, longitude.

```sh
celestial-data earth-sources
celestial-data earth-find usgs-3dep --bounds=-123,37,-122,38 --most 3
celestial-data earth-find nasa-gibs --bounds=-123,37,-122,38 --text "Black Marble"
celestial-data earth-fetch nws --point 37.8,-122.4 --into earth-data
```

| Source | Input | Product |
| --- | --- | --- |
| `usgs-3dep` | bounds | The National Map lidar point cloud files |
| `nasa-gibs` | bounds, optional text filter | dated Worldview JPEG snapshots |
| `asf` | bounds | public radar interferogram browse images |
| `opensky` | bounds | one current state-vector JSON response |
| `open-meteo` | point | one current-conditions JSON response with units |
| `nws` | point | the nearest listed station's latest observation JSON |

The three catalog adapters default to 10 results. Use `--most` to
set a limit between 1 and 50. A dry fetch lists destinations without
writing files. Catalog and station lookups still make their requests.

OpenSky, Open-Meteo and NWS are sampled once per fetch. To keep several
samples from the same query, use separate destination folders. Other
entries from Kingfisher are visible as catalog references. The fixed
2016 Black Marble image is available through `bluemarble` and `usgs`
serves USGS Spectral Library Version 7.

## the archives

each has an adapter in its own package.

chandra
: Chandra's primary products, from HEASARC's chanmaster table and the
  Chandra Data Archive's folders. the longest observations come first;
  `--most` says how many.

nustar
: NuSTAR's cleaned event files, both focal plane modules, from HEASARC's
  numaster table and its FTP tree.

glimpse
: Spitzer GLIMPSE II mosaics at IRSA, in IRAC's four bands. each tile's
  preview is named for its centre and size, so the tiles a circle reaches
  are read from the folder's listing.

galcen_psc
: IRSA's Galactic Center IRAC catalog, Ramirez et al. 2008: the rows in
  the circle, as CSV, from IRSA's TAP service.

hpsgc
: the HST/NICMOS Paschen-alpha survey of the Galactic Centre at MAST. its
  field is read from the mosaic's own header cards.

meerkat
: SARAO's MeerKAT Galactic Centre mosaic at 1.28 GHz, with its spectral
  index. asked through its DOI. under CC BY-NC 4.0; the page asks that
  work using it cite arXiv:2201.10541.

ppmap
: Cardiff's Hi-GAL PPMAP maps of dust column density and temperature.
  each tile is taken to be the l000 tile's size, centred on its name with
  a degree of slack, so a circle near a tile's edge finds its neighbour
  too.

hips
: a hips2fits cutout of any HiPS survey, at Strasbourg or a mirror:
  `--hips-host`, `--hips` and `--pixels`.

chroma
: the colourways in janearc/chroma on GitHub: the `.css` files of
  `colourways/sources` on `main`, listed through GitHub's contents API
  when asked, so a colourway added there comes with no change here. asked
  only by name, as the earth archives are. GitHub allows sixty unsigned
  questions an hour from one address.

gaia
: ESA's Gaia archive: the stars of Gaia DR3's `gaia_source` with a
  radial velocity, their astrometry with its errors and correlations,
  and their brightness and colour, as FITS. a circle of radius 180 is
  the whole sky, about 33.8 million stars. asked only by name.

  the archive runs it as a job, under the session of whoever signed in,
  so it knows whose work it is. a signed-in user's job runs for up to
  two hours; fetch asks after it every 30 seconds.

  ```sh
  $ export GAIA_SESSION=~/gaia/session
  $ celestial-data-secrets gaia-login
  $ celestial-data fetch --archive gaia --ra 0 --dec 0 --radius 180 \
      --into ~/sky
  ```

  `celestial-data-secrets gaia-login` reads the gaia secret's username
  and password with op, which asks the person at the keyboard to allow
  it, so a person runs it; see [secrets](#secrets). the session it keeps
  is readable by its owner alone, and fetch sends it when
  `GAIA_SESSION` names it.

  the `gaia` package reads what the job answered, too: `OpenCatalogue`
  opens the product through celestial's FITS table reader and holds
  each of the query's 31 columns to the archive's name, kind and unit,
  and a `Source` is one row as celestial's types, for celestial's
  galactocentric frame and orbit solver. the test of the product on
  disk runs with `-tags gaiadata` and `GAIA_DR3_RV` naming the file.

  a `Source`'s `Spread` is its errors ready to draw from: the five
  astrometric parameters' covariance as a triangle, so a `Drawn` source
  moves them together as their correlations have them, and its radial
  velocity by its own error. the caller gives the normal draws.

  `As` puts a source's errors on another star's values, a sighting from
  celestial's galactocentric frame, so that a star made in the frame can
  be drawn as Gaia would have measured it.

  `State` places a source in celestial's galactocentric frame as the orbit
  solver runs it, its place and motion about the centre.

  the reader needs celestial's binary-table reader, which is on
  celestial's galaxy branch until it is released; until then this
  module builds in a go workspace with that branch's checkout, and
  go.mod's celestial version is the last released one.

apogee
: APOGEE DR17's allStar, from SDSS's Science Archive Server: a row a
  star observed in the near infrared, with the parameters and abundances
  ASPCAP fit to its combined spectrum. one file of 4.0 GB, whatever the
  circle. asked only by name.

  the `apogee` package reads it. `OpenCatalogue` holds each column read
  to the kind the file's header gives, to one value a row and to no
  unit: the file writes none, and the units are SDSS's data model's.

  a `Star` is one row: its Gaia DR3 source, its parameters, [Fe/H] and
  22 elements' [X/Fe], each with its uncertainty and flag, and the
  survey's flags. `Find` gives every row of the Gaia sources asked. a
  row the survey gave no place, as its first, VESTA, has none.

  the test of the file on disk runs with `-tags apogeedata` and
  `APOGEE_DR17_ALLSTAR` naming the file. the reader needs celestial's
  `arrays` branch, whose table reader steps over columns of many.

astronn
: astroNN's catalogue of APOGEE DR17, from SDSS's Science Archive
  Server: a row for each of allStar's, in allStar's order, with what
  Leung and Bovy's networks read from each spectrum. one file of 647 MB,
  whatever the circle. asked only by name.

  the `astronn` package reads it as `apogee` reads allStar. a `Star` has
  its age in Gyr (the bias-corrected age the catalogue recommends, the
  uncorrected and linear ones, and their uncertainties), its distance
  from the spectrum and weighed with Gaia's parallax, and its orbit.

  the file names its age, distance and orbit columns in lower case, as
  the data model does not; the reader asks for them as the file does.
  its rows are allStar's, so a star's row in allStar is its row here: a
  reader with that row number needs no pass through `Find`.

  the server answers byte ranges, so one star's row can be read without
  the file: testdata holds the header and firang's star's row, fetched
  so, and the test reads that row against its own bytes.

starhorse
: StarHorse's results for APOGEE DR17 and Gaia EDR3, v2 (Queiroz et al.
  2023), from AIP: a row a star it fit, 562,424 of them, keyed by
  APOGEE_ID and Gaia DR3 source, not in allStar's order. one file of 249
  MB, whatever the circle. asked only by name.

  a `Star` has the 16th, 50th and 84th percentiles of its mass, distance,
  extinction at 542 nm and age, the age's 5th and 95th, its temperature,
  log g and metallicity, and StarHorse's flags as text. `Find` gives the
  rows of the Gaia sources asked.

  StarHorse gives ages only near the turn-off and on the subgiant branch:
  its age flag is NOAGE elsewhere, red giants among them, and the age is
  then not a number. a giant's mass is there all the same.

tess
: TESS's light curves at MAST: the mission's own and its high level
  science products', QLP's among them, found by MAST's CAOM cone search
  and each observation's products, each fetched as a FITS file. asked
  only by name; full-frame images are not found.

  MAST answers a question it is still working on with EXECUTING, and is
  asked again, ten times at most. the tests replay MAST's answers for
  firang's star, recorded through the adapter on 2026-11-06: sector 92,
  QLP, 200 s.

galah
: GALAH DR4's allstar, from Data Central: a row a star observed with
  HERMES on the Anglo-Australian Telescope, with the parameters and
  abundances the survey fit, lithium among them. one file of 758 MB,
  whatever the circle. asked only by name.

  the `galah` package reads it as `apogee` reads its file: a `Star` has
  its Gaia DR3 source, when the survey gave one, its parameters and
  rotation, [Fe/H], 30 elements' [X/Fe], and A(Li) or its upper limit,
  each with its uncertainty and flag, and the survey's flag_sp.

  a survey id or a flag left empty is refused, by its column, but for
  lithium's, empty in 25,364 rows; no other is empty in the file. the test
  of the file on disk runs with `-tags galahdata` and `GALAH_DR4_ALLSTAR`
  naming the file.

bsc5
: the Yale Bright Star Catalogue, fifth revised edition (Hoffleit and
  Warren 1991): 9,110 stars, near enough every one brighter than visual
  magnitude 6.5, every star a naked eye sees. one gzipped file of 574 kB,
  ybsc5.gz, from Harvard's Telescope Data Center, whatever the circle.
  asked only by name. CDS Strasbourg keeps the same catalogue as V/50; it
  did not answer from here on 2026-11-10, so the adapter asks Harvard.

  the `bsc5` package reads it by the byte columns of Warren's readme,
  ybsc5.readme: a `Star` has its HR and HD numbers, its name, its J2000
  place, its V and B-V where given, and its spectral type. the 14 rows
  with no place, novae and objects found not to be stars, have none. the
  test of the file on disk runs with `-tags bsc5data` and `BSC5_FILE`
  naming the file.

sdssv
: SDSS-V's optical spectra in DR19: the BOSS spectra whose catalogue
  place falls in the circle, as SkyServer's spAll table lists them, the
  clearest first, `--most` of them. each is a FITS file on the Science
  Archive Server, at spectra/full/FIELD/MJD/ as its name writes them.

  asked only by name, through SkyServer's SQL service; nothing to sign
  in to. BOSS's resolution is about 2,000, too low for most stars'
  lithium.

lamost
: LAMOST's spectra in DR10 v2.0, public to all since September 2024:
  the low-resolution ones in the circle, clearest in r first, and the
  medium-resolution ones, clearest in the red arm first, `--most` of
  each, each a gzipped FITS file fetched by its obsid.

  the medium-resolution red arm, 6300 to 6800 A at a resolution of
  7500, holds lithium's line. LAMOST's sky stops near declination -10.
  work using the data acknowledges the survey, in its data policy's
  words. asked only by name, through the release's TAP service.

keck
: the Keck Observatory Archive's HIRES spectra: each public science
  frame whose pointing falls in the circle, `--most` of them, its raw
  file and, where KOA reduced it, its spectra extracted by MAKEE, one a
  CCD. HIRES's echelle reaches lithium's line at 6708 A.

  asked only by name, through KOA's TAP service and its level-1 lists;
  KOA answers anyone not signed in with public frames only. work using
  them acknowledges KOA and each frame's principal investigator.

spherex
: the images NASA's SPHEREx mission has taken of a place, as IRSA holds
  them: for the circle, a cutout of each image whose footprint meets it,
  the earliest first, `--most` of them. SPHEREx's filters put a star at
  a different wavelength in each image, so the cutouts of one place hold
  its spectrum between 0.75 and 5 micron. making that spectrum is not
  done here.

  find asks IRSA's TAP service once, and names each cutout for its
  release (qr2 or qr3), its observation and detector, and the place, so
  `spherex_qr3_2026W37_2A_0136_2D2_` and what follows it is detector 2
  of observation 2026W37_2A_0136_2. the cutout is a square
  twice the radius a side, from 0.03 degrees to 0.5. it is some 4 to 5
  MB whatever its side, since IRSA does not cut the point spread
  function, so a place with a few hundred images is a gigabyte or two.
  IRSA says no length for a cutout, so fetch holds each to being a whole
  cutout: a FITS file whose headers and data add up to its length, and
  whose units are a cutout's, in order: the empty primary, IMAGE, then
  FLAGS, VARIANCE and ZODI of the image's size, the release's PSF (an
  image named PSF in QR2, a table named EPSF in QR3, either in a
  release not yet seen) and the WCS-WAVE table, and nothing after. it
  keeps none that is cut short, even where a unit ends, or is not FITS.
  the structure is as read from the 340 cutouts of both releases
  fetched on 2026-11-06. IRSA states no limit on requests; fetch takes
  them one at a time. asked only by name.

  IRSA asks that work using the data say: "This publication makes use
  of data products from the Spectro-Photometer for the History of the
  Universe, Epoch of Reionization and Ices Explorer (SPHEREx), which is
  a joint project of the Jet Propulsion Laboratory and the California
  Institute of Technology, and is funded by the National Aeronautics and
  Space Administration." and cite the release's DOI, 10.26131/IRSA652
  for QR2 and 10.26131/IRSA662 for QR3.

plates
: the photographic sky surveys of STScI's Digitized Sky Survey: for the
  circle's centre, a FITS cutout from each of POSS-I blue and red,
  POSS-II or the UK Schmidt's SERC and AAO surveys in blue, red and
  infrared, and Quick-V, whichever has a plate there. the cutout is a
  square twice the radius a side, from one arcminute to sixty, which is
  all the form allows.

  find asks each survey in turn and reads the plate's header cards from
  the start of its answer, so what it says of a plate, its id, label,
  telescope, emulsion, filter, exposure and date, is the scan's own; the
  plate id and the date are in the file's name too. fetch brings the
  whole file, header cards as the scan left them. a survey with no plate
  at the place answers with a page, not FITS; find leaves it out. no
  page is kept as a plate: a fetched file must be whole FITS blocks,
  begin as FITS does, name in its header the plate find read there, and
  reach the end of the image that header declares, whatever length the
  server said or did not say, so another plate of the same length, and
  a plate's header without its image, are refused too. the plate id
  names the plate, not the cutout: two cutouts of one plate at other
  places would both pass. a FITS string's quote written as two
  is read as one, and a string with no closing quote is refused.

  find asks each survey for the cutout at the side asked and reads its
  header from the start of the answer, then closes it. at the largest
  side, sixty arcminutes, STScI makes an image of some 26 MB for each
  of the six surveys to answer that, and find reads about 115 KB of
  each. it asks at the side asked, not the least, so that the length
  find records is the plate's own and the plate named is the one the
  fetch will bring. work using the scans acknowledges the surveys, in
  the words of STScI's acknowledging page. asked only by name.

  the form, help and acknowledging pages in `plates/sources.yaml` were
  read live on the dates it gives; no copy of them was kept, and their
  hashes were not recorded.

k2
: what Kepler's second mission, K2, holds of a place, as MAST keeps it:
  the light curves and target pixel files of the stars it watched there,
  and the full-frame images of each campaign whose field holds the
  place. a full-frame image is the whole focal plane in one file, some
  400 MB, in four forms: calibrated, raw, raw with WCS, and its
  uncertainty. `--match ffi-cal` keeps the calibrated one. asked only
  by name, through MAST's API as tess is (internal/caom).

## secrets

celestial-data signs in to some services. their secrets are named in
one place, the `secrets` package, and kept in 1Password: each person's
config says which item holds each, and op reads it each time it is
needed. no command ever says a password or a token. a refused gaia
login names the username it tried, so the person knows which login
the archive refused.

the config is `celestial-data-secrets`'s own, in the `.config` folder
of the home `HOME` names, a line a service:

```
secret ads :: op://vault/item
secret gaia :: op://vault/item
```

```sh
$ celestial-data-secrets list        # each service, its fields, its item
$ celestial-data-secrets check       # reads each, says read or why not
$ celestial-data-secrets gaia-login  # signs in to the gaia archive
$ celestial-data-secrets ads-login   # keeps the ads token
```

ads
: an API token for NASA's Astrophysics Data System, in the item's
  `credential` field. `ads-login` keeps it in `ads-token` beside the
  config, or where `ADS_SESSION` says, and `celestial-data ads`
  searches with what it kept.

gaia
: a login to the Gaia archive at ESA, in the item's `username` and
  `password` fields. `gaia-login` signs in with it, and keeps the
  session where `GAIA_SESSION` says.

op asks the person at the keyboard to allow each read, so a person
runs `check` and the logins, once. celestial-data itself never runs
op: it reads only what the logins kept.

`gaia-login` was celestial-data's own verb, with `--op` naming the
item. it is `celestial-data-secrets gaia-login` now, and the item
`--op` named is the config's `secret gaia` line.

## how it is held

everything reaches the network through `web.Web`, one interface with a
generated mock. the tests hold each adapter to the exact addresses it
asks for, against replies the archives really gave, recorded under each
package's `testdata`. the circle they asked about is in
`internal/replay/asked.yaml`.

each adapter also has a live check that asks the real archive and asks
each file it names for its length, never for the file:

```sh
$ go test -tags live ./...
```

numbers that were measured, such as where a mosaic lies, are kept with
the header card they were read from, in each package's `numbers.yaml`.
numbers that were picked say who picked them and why.

## building

```sh
$ game build
$ game check
```
