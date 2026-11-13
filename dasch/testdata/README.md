# dasch test data

Two fixtures. Neither was fetched for the tests; each is said here as
what it is.

## querycat.json: made by hand

A catalogue search's answer, written by hand in the shape the API
answers in: a JSON list of strings, the first the column names. Its
first line names only the four columns the adapter reads
(ref_number, gsc_bin_index, ra_deg, dec_deg); the API's real answer has
more columns, in an order not recorded here.

- `419100022221405,52510421,287.50084,-22.2348` is the real source:
  APASS 419100022221405 in bin 52510421, which is Gaia DR3
  4081136684118965760, as the real query of 2026-11-08 found it (see
  below). Whether its ra and dec here are the API's own figures is not
  recorded; the query's answer was not kept.
- `419100022221777` and `419100022221888` are made up: neighbours with
  numbers no catalogue holds, placed to test the circle's cut and the
  nearest-first order.

## lightcurve.json: three lines of a real light curve

The header and two lines of the real light curve of APASS
419100022221405, taken from the file fetched on 2026-11-08: the first
a plate that covered the place and saw nothing (no ref_number), the
second a measure. All three lines are in the kept file, unchanged
(checked line for line on 2026-11-09); the header is all 64 columns.

## where the real ones are kept

The real light curve is kept whole, with its record:

- the file: `nilgai/searches/firang/data/dasch/dasch-dr7-apass-419100022221405-lightcurve.csv`,
  413,014 bytes, sha256
  d9c8b6e33c7518b0876aca1a711d3cd29932e58ee61795d4723ac488e0539e42;
  copied for the firang review into its review bundle, outside the
  repository.
- the record: `FETCHED.md` beside it. It gives the command, the adapter
  revision (dasch at 5547db9), the two POSTs and their bodies (querycat
  with refcat apass, ra 287.50086, dec -22.23481, radius 5 arcseconds;
  lightcurve with ref_number 419100022221405, gsc_bin_index 52510421),
  why it was fetched and what uses it.
- the query's own answer was not saved; the record says so. It named
  this one source.

## the pages the adapter was built from

Read 2026-11-08: DASCH DR7's pages on the web APIs, the data products
and the light curve columns. Their exact addresses, hashes, sizes and
fetch times are in `dasch/sources.yaml`; copies were fetched through the
`dasch-pages` archive on 2026-11-09 and are kept in `cited-sources/`,
in version control and out of every release. They describe the API; no
number in the adapter
is measured from them except the 3600-arcsecond widest box.

Not yet kept: Starglass's page on authentication, whose address was not
recorded, and daschlab's source (apiclient.py, lightcurves.py,
refcat.py), Harvard's own client, read at no pinned commit. Each is to
be pinned and fetched through an adapter before it is cited.
