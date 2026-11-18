# NOAA Hawaii CRM2023 WCS subsets

This adapter fetches bounded, native-grid float32 GeoTIFF subsets from
NOAA's Hawaii CRM2023 archive. The authored box and byte ceiling bound each
request; the response must prove its TIFF dimensions and georeferencing.
The authored transfer ceiling is 150 MiB.

The adapter does not fetch the full NetCDF archive, invent a TIFF filename,
reproject heights or substitute sea for nodata. It uses retained WCS metadata
and local fixtures, with no network call needed to validate that evidence.

See [implementation and evidence](implementation.md) for archive metadata,
fixture hashes, validation limits, and the command's audit requirements.
