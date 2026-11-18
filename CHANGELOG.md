# changelog

## unreleased

2026-11-18

- requires celestial v0.0.10, whose forced photometry the `spectrum`
  command uses.
- `earth-sources`, `earth-find` and `earth-fetch` list and bring home
  terrestrial products, each saved under a folder named for its source:
  USGS 3DEP lidar point clouds, NASA GIBS snapshots, ASF radar
  interferogram browse images, OpenSky aircraft state vectors, and
  current weather from Open-Meteo and the National Weather Service.
- the same verbs take `terrain-tiles` and `terrain-tiles-offshore`, Mapzen
  Terrarium elevation tiles, and `noaa-etopo`, a subset of NOAA's ETOPO
  2022 at 15 arc-seconds up to one degree a side.
- they take `usgs-water-quality` too: the latest USGS continuous
  observations of turbidity, salinity and suspended sediment within
  bounds, one GeoJSON snapshot for each, or for any five-digit USGS
  parameter code `--text` names. the first page of each is saved.
- netCDF is read through scigolib/netcdf at its pull request on GitHub
  until that is released.

2026-11-14

- `terrain-index` retains the USGS Hawaii elevation catalogue, and
  `terrain-fetch` brings selected products home with transfer records.
- `noaa-crm` fetches bounded Hawaii relief subsets; `documents` retains
  explicitly named primary documents with provenance.
- `refractiveindex` and `nistxray` bring optical constants and X-ray
  attenuation tables through named archives.
- `spectrum` measures a star in retained SPHEREx cutouts, using
  Celestial's forced photometry. it reads local files only.
- terrain and primary-document fetching use the shared host pacing
  and accept `--i-mean-it`, including the NOAA relief fetcher.

2026-11-11

- how to ask a server without being a nuisance is now one piece of
  code, the gentle, in the `web` package. arXiv, dasch, ADS and NTRS
  use it, and every other host is asked through it by default. see
  "how celestial-data asks a server" in the readme.
- `--i-mean-it` asks a host anyway for one run, after printing what
  the server asked for. it asks the refused request once more, and the
  override is kept in the host's record. `find`, `fetch`, `fetch-set`,
  `papers`, `ads`, `ntrs` and `ntrs-fetch` take it.
- a stop is kept across runs for dasch, ADS, NTRS and the default
  hosts: the next run asks nothing until the wait has passed.
- dasch now stops on a 403 and on a 503 with a Retry-After too, and
  asks again after a failure with no word from the server.
- NTRS answers are held to a time limit and a size limit.
- a record that cannot be kept, or no cache folder, is said once and
  the run goes on, paced in memory.
- a record written by this version is refused by an older build. to go
  back, remove the record file in the `celestial-data` folder of the
  cache folder.
