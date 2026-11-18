# refractiveindex testdata

refractiveindex-excerpt.zip holds 17 files of the refractiveindex.info
database's release zip, rii-database-2026-05-24.zip, as fetched on
2026-11-11 from GitHub (13440092 bytes, sha256
6cdebebd17fe02b6dc2b03189e15e6c689afc6619c558e1df7b9ac307b6d14eb). The
database is in the public domain: copyright and related rights waived
via CC0 1.0.

The files are whole and unchanged, under the folders they have in the
archive:

- database/catalog-nk.yml, the whole catalog of the pages with n and k
  data: its shelves, books and pages, and each page's data path.
- database/data/main/Au/nk/Johnson.yml, gold as Johnson and Christy
  1972 measured it: 49 rows of n and k, the page the tests read in full
  detail.
- database/data/main/Ag/nk/Johnson.yml, silver as Johnson and Christy
  1972 measured it: 49 rows. Silver has no other Johnson page, and no
  Rakic 1995 page; it has Rakić et al. 1998's fits, both kept here.
- database/data/main/Ag/nk/Rakic-BB.yml and Rakic-LD.yml, silver by the
  Brendel-Bormann and Lorentz-Drude models of Rakić et al. 1998.
- database/data/main/Al/nk/Rakic.yml, aluminium as Rakić 1995 fit it:
  206 rows. Aluminium has no Johnson page.
- database/data/main/Al/nk/Rakic-BB.yml and Rakic-LD.yml, aluminium by
  Rakić et al. 1998's two models.
- database/data/main/Be/nk/Rakic-BB.yml and Rakic-LD.yml, beryllium by
  Rakić et al. 1998's two models. Beryllium has no Johnson page; these
  two fits are the Johnson-and-Christy-like measurement it has none of.
- database/data/main/Ti/nk/Johnson.yml, titanium as Johnson and Christy
  1974 measured it: 49 rows.
- database/data/main/Ti/nk/Rakic-BB.yml and Rakic-LD.yml, titanium by
  Rakić et al. 1998's two models.
- database/data/main/MgAl2O4/nk/Tropf.yml, spinel as Tropf and Thomas
  1991 give it: a formula 1 page, its constants in coefficients, kept so
  the tests can refuse a formula page by its own name.
- database/data/main/MoSe2/nk/Pruszynska-Karbownik-e.yml, a tabulated n
  page, 1084 rows of wavelength and n alone, kept so the tests can read
  a page whose k is none.

The metals were asked for as the pages of Johnson, Johnson and Christy,
and Rakic's fits; what the catalog holds for each:

- gold (main/Au): Johnson (Johnson and Christy 1972), and the models
  and simulations Rakic-BB and Rakic-LD (Rakić et al. 1998). All three
  kept.
- silver (main/Ag): Johnson (Johnson and Christy 1972), Rakic-BB and
  Rakic-LD. All three kept.
- aluminium (main/Al): no Johnson page; Rakic (Rakić 1995) under
  experimental data, and Rakic-BB and Rakic-LD under models. All three
  kept.
- beryllium (main/Be): no Johnson page; Rakic-BB, Rakic-LD and
  Svechnikov (2020). The two Rakic fits kept.
- titanium (main/Ti): Johnson (Johnson and Christy 1974), and Rakic-BB
  and Rakic-LD. All three kept.

The archive was unzipped beside the zip, and the excerpt made from the
unzipped files, in the folder that holds database/:

    zip -9 refractiveindex-excerpt.zip \
      database/catalog-nk.yml \
      database/data/main/Au/nk/Johnson.yml \
      database/data/main/Au/nk/Rakic-BB.yml \
      database/data/main/Au/nk/Rakic-LD.yml \
      database/data/main/Ag/nk/Johnson.yml \
      database/data/main/Ag/nk/Rakic-BB.yml \
      database/data/main/Ag/nk/Rakic-LD.yml \
      database/data/main/Al/nk/Rakic.yml \
      database/data/main/Al/nk/Rakic-BB.yml \
      database/data/main/Al/nk/Rakic-LD.yml \
      database/data/main/Be/nk/Rakic-BB.yml \
      database/data/main/Be/nk/Rakic-LD.yml \
      database/data/main/Ti/nk/Johnson.yml \
      database/data/main/Ti/nk/Rakic-BB.yml \
      database/data/main/Ti/nk/Rakic-LD.yml \
      database/data/main/MgAl2O4/nk/Tropf.yml \
      database/data/main/MoSe2/nk/Pruszynska-Karbownik-e.yml
