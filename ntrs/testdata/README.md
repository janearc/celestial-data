# ntrs/testdata

These answers are crafted for the tests, in the shape NTRS's own API
description gives (https://ntrs.nasa.gov/api/openapi-yaml): a search
answers `results`, `aggregations` and `stats`, and each citation lists
its `downloads`, each with `links`. None was recorded from NTRS. The
ids, titles and authors are made up, and the ids are of a year that has
not come, so none is a real citation.

- two-citations.json: a search that found two of 115. The first has
  three authors out of sequence, a text download before its PDF, and a
  PDF; the second has one author and no download.
- bare-citation.json: a search that found three citations that lack
  what the others have: no title and an author with no name; a date
  that is no year; a PDF with no link at all, then one with only an
  original link; no date; a date too short to hold a year; no
  authors, publications or downloads at all.
- none-found.json: a search that found nothing.
- no-results.json: valid JSON without the results NTRS gives.
- not-json.txt: not JSON at all.
- citation.json: one citation, as a lookup of its id answers, whose
  PDF is not named by its id.
- citation-no-pdf.json: one citation that holds no download.
- citation-elsewhere.json: one citation whose PDF address points into
  another citation's folder.
