# nistxray testdata

The files here are real fetches of NIST's X-Ray Mass Attenuation
Coefficients (Standard Reference Database 126) pages, made on
2026-11-11. Each page came from
https://physics.nist.gov/PhysRefData/XrayMassCoef/, as the paths below
name it. The data is copyright, not public domain; in NIST's own words:
"© 1989, 1990, 1996 copyright by the U.S. Secretary of Commerce on
behalf of the United States of America. All rights reserved. NIST
reserves the right to charge for these data in the future."

The pages are whole and unchanged, as the wire brought them:
Cloudflare's injected script line included, which is why no page's
bytes can be summed twice. What a page is held to is the sha256 of its
content, the page with that one line removed; the pinned sums of all
144 pages are in ../manifest.yaml.

- z82-a.html, lead, Z 82, from ElemTab/z82.html: 12168 bytes, sha256
  21430c7a9153ceae39b168d07cbf056f9a296132036729162fd7c948d4c5befb,
  content sha256
  0881af817c92b801dd3031a336b766d9d8cf98f9a291f7f0bc817cbbde1bd4fd.
  The page the tests read in full detail.
- z82-b.html, lead again, from the same address, a second fetch:
  12168 bytes, sha256
  4e6822c36386eaf6ffa167221778da66c59fe091d2ace7c79f886add9d6b30e9,
  and the same content sha256. Kept with z82-a.html so a test can show
  the two fetches differ on the wire — in exactly the one line
  Cloudflare injects — and agree in the sum of their content.
- z13.html, aluminum, Z 13, from ElemTab/z13.html: 9144 bytes,
  content sha256
  fa01e6704d901bb9e85c72f9ba6e2581744cda0a8988a4bdb2948820ead71f6b.
  A second element, with a single K edge.
- water.html, water, from ComTab/water.html: 8546 bytes, content
  sha256
  a6d7b3fc22608bcce821ae3f52ea91b67bc1a78ddaa6bb9f2a588bfd8c9f73c1.
  A compound, with no edge rows.
- tab1.html, Table 1, the material constants of the elements: 13272
  bytes, content sha256
  b65abc587fc6e2337aa784117ca60d0255c574dc7b9d33af41b2699ad46bd6a0.
  Kept for the constants reader, which reads lead's row, and for the
  test that refuses a page that holds no table of rows.
- tab2.html, Table 2, the material constants and composition of the
  compounds and mixtures: 16232 bytes, content sha256
  85a14470eca079ea615139e238ee6fea0eda4e4ace365eb64c3fc5526b56a269.
  Kept for the constants reader, which reads water's row.
- tab3.html, Table 3, the index of the element pages: 12611 bytes,
  content sha256
  fd97e5a34534bb9946f850e9d51279ab7c1e4d3477200446434cc75315f95609.
  Kept as a page with no table of rows, which the reader refuses.

The manifest (../manifest.yaml) was made the same day, from the same
kind of fetches: every one of the 144 pages — the 92 element pages
ElemTab/z01.html to z92.html, the 48 compound pages ComTab/*.html that
tab4.html indexes, and tab1.html to tab4.html — was fetched twice, as
two separate requests. Every pair differed on the wire in exactly the
one line carrying "window.__CF$cv$params=", and every pair agreed in
the sha256 of its content, with that line removed; both fetches also
gave the same length. The manifest records the length and the content
sum of the first fetch of each pair, the element's Z, and the display
name NIST's own index gives the page: Table 3's link text for an
element, Table 4's for a compound. The pages' parent is not one file
but the 144 addresses themselves, under
https://physics.nist.gov/PhysRefData/XrayMassCoef/.

The commands that fetched these files, run in an empty folder:

    curl -o z82-a.html https://physics.nist.gov/PhysRefData/XrayMassCoef/ElemTab/z82.html
    curl -o z82-b.html https://physics.nist.gov/PhysRefData/XrayMassCoef/ElemTab/z82.html
    curl -o z13.html https://physics.nist.gov/PhysRefData/XrayMassCoef/ElemTab/z13.html
    curl -o water.html https://physics.nist.gov/PhysRefData/XrayMassCoef/ComTab/water.html
    curl -o tab1.html https://physics.nist.gov/PhysRefData/XrayMassCoef/tab1.html
    curl -o tab2.html https://physics.nist.gov/PhysRefData/XrayMassCoef/tab2.html
    curl -o tab3.html https://physics.nist.gov/PhysRefData/XrayMassCoef/tab3.html

The manifest was made by fetching every page the same two ways, in the
two folders a/ and b/, and recording the first fetch of each pair:

    for z in $(seq -w 1 92); do
      curl -o a/z$z.html https://physics.nist.gov/PhysRefData/XrayMassCoef/ElemTab/z$z.html
      curl -o b/z$z.html https://physics.nist.gov/PhysRefData/XrayMassCoef/ElemTab/z$z.html
    done
    for c in $(the 48 names tab4.html indexes); do
      curl -o a/$c.html https://physics.nist.gov/PhysRefData/XrayMassCoef/ComTab/$c.html
      curl -o b/$c.html https://physics.nist.gov/PhysRefData/XrayMassCoef/ComTab/$c.html
    done
    for t in tab1 tab2 tab3 tab4; do
      curl -o a/$t.html https://physics.nist.gov/PhysRefData/XrayMassCoef/$t.html
      curl -o b/$t.html https://physics.nist.gov/PhysRefData/XrayMassCoef/$t.html
    done

    # then, for each page: check the two fetches agree in length and
    # in the sha256 of the content, each taken as the page with every
    # line carrying window.__CF$cv$params= removed, and differ in no
    # other line; and write the manifest's entry from the first fetch
    python3 - <<'PYEOF'
    import hashlib, re, os
    MARK = b"window.__CF$cv$params="
    def content(raw):
        return b"\n".join(l for l in raw.split(b"\n") if MARK not in l)
    ...
    PYEOF

The 48 names tab4.html indexes are a150, adipose, air, alanine, b100,
bakelite, blood, bone, brain, breast, c552, calcium, ceric, cesium,
concrete, concreteba, eye, fluoride, fricke, gadolinium, gafchromic,
gallium, glass, kodak, lithium, lithiumflu, lung, magnesium, mercuric,
muscle, mylar, nylonfilm, ovary, photoemul, pmma, polyethylene,
polystyrene, polyvinyl, pyrex, teflon, telluride, temethane, tepropane,
testis, tissue, tissue4, vinyl and water.
