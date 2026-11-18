# Hawaii elevation catalogue

This adapter retains the USGS National Map product response for an authored
rectangle around Hawaii island. The query in hawaii.yaml requests the archive's
1/3 arc-second elevation product group in GeoTIFF form. It does not establish
complete island coverage, a height datum or a coastline mask by itself.

The public archive needs no astronomical login. The command uses the existing
Celestial-data fetch path and refuses to replace retained catalogue bytes.
Malformed replies remain available for inspection, but the command fails
rather than accepting them as catalogues.

    game build
    bin/celestial-data terrain-index --into /path/to/terrain

Record the command, adapter revision, purpose and retained files in FETCHED.md
before downloading. Select terrain products from the retained response, then
check their coverage, metadata, dimensions and datum before scene staging.
