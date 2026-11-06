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
  and asks nothing more. nothing is retried. the wait is kept in the
  record, and a run before it has passed asks nothing and says when it
  will have: the Retry-After's seconds or date, or five minutes
  (`rest-unsaid`) when arXiv said none. any other failure of the API
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
  $ celestial-data gaia-login --op op://Personal/gaia
  $ celestial-data fetch --archive gaia --ra 0 --dec 0 --radius 180 \
      --into ~/sky
  ```

  `gaia-login` reads the login's username and password with op, which
  asks the person at the keyboard to allow it, so a person runs it. the
  session it keeps is readable by its owner alone, and fetch sends it
  when `GAIA_SESSION` names it.

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
