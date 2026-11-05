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
