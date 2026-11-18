# NOAA Hawaii CRM2023 implementation and evidence

## Archive evidence

The endpoint path is:
`https://www.ngdc.noaa.gov/thredds/wcs/crm/cudem/crm_vol10_2023.nc`

The GetCapabilities URL appends this query suffix:
`?service=WCS&version=1.0.0&request=GetCapabilities`

DescribeCoverage uses this suffix:
`?service=WCS&version=1.0.0&request=DescribeCoverage&coverage=z`
Both queries were checked against NOAA's WCS metadata on 2026-11-09.

The exact GetCoverage OnlineResource appears in the retained capabilities
response. DescribeCoverage names coverage `z`, CRS84 axes, positive native
x/y offsets, GeoTIFF_Float, and no interpolation.

Unidata's TDS WCS reference documents bbox-only native-grid requests and
distinguishes Float data samples from eight-bit grayscale GeoTIFF.

[Unidata TDS WCS reference][wcs]

[wcs]: https://docs.unidata.ucar.edu/tds/current/userguide/wcs_ref.html

The two NOAA metadata responses are copied into testdata from the adapter's
retained research originals. No network request was made to create the
fixtures.

| Fixture | Bytes |
| --- | ---: |
| noaa-capabilities.xml | 1701 |
| noaa-description.xml | 2227 |

The capabilities SHA-256 is
`e860fa5bbeaf38bace64909037b0609e36605f3b37487698d01b9905361addb9`.
The description SHA-256 is
`a70bd6e83f9438b9fd75c5899237460b8953ea37d832a5bd338f949c1c0abe14`.

Original provenance is in the owner's research directory,
`scratch-tools/water-research-2026-11-09/sources/FETCHED.md`, runs 009/010.
The metadata is a NOAA US-government work.

The separately retained Hawaii CRM guide and NCML document source units,
vertical datum, and bounds. They are not inferred from EPSG:0 in the WCS XML.

## Request and response limits

The integrated `celestial-data noaa-crm` command uses Pzm; its `tiff` command
reads a retained TIFF's first-directory metadata. No standalone noaacrm
command is built. Authored JSON/YAML is parsed strictly by yamlfile.

The fetch command requires a full source commit, exact command file, purpose,
and consumer. It flushes FETCHED.md before the first GET.

The command uses the common atomic Fetch and an additional declared and
streamed byte guard. Its authored ceiling is 150 MiB. The guard consumes at
most one probe byte beyond that limit to distinguish EOF from an oversized
unknown-length response. Excess bytes are never written or retained.

TIFF checks require bounded native dimensions, unchanged scale, single-band
float32 samples, georeferencing tags, and complete declared strips. This is
directory integrity, not raster decoding. An independent libtiff read and
source-specific metadata interpretation remain required before use.

## Checks

Focused Game targets are `check-noaacrm` and `vet-noaacrm`. They cover the
archive-advertised request, invalid and full-archive boxes, strict
configuration, and no-write dry runs.

They also cover declared and streaming limits, hand-authored little- and
big-endian TIFF metadata, truncated strips, server error bodies, cleanup,
and refusal to fetch an existing source. No graphics or renderer checks are
included.
