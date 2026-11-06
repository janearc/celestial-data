# sdssv testdata

spall-sgra.csv and spall-three.csv are SkyServer's DR19 answers, as
recorded on 2026-11-06, to the queries sdssv.Asking makes: the replay
circle round Sgr A* (0.1 degrees, at most 3), which holds no SDSS-V
spectrum, and 0.5 degrees round RA 41.728048, Dec 0 (at most 3).

The crafted-*.csv files are made by hand, each broken one way: no
table line, a table line alone, an empty answer, a table line of two
fields, another table's line, no sn_median_all column, a quote left
open, a row shorter than its header, and spectrum
file names of two parts, not .fits, not spec-, and an empty part.
