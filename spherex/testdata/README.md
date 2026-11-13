# spherex testdata

images-three.xml is the answer of IRSA's TAP service, recorded on
2026-11-06, to the query spherex.Find makes for 0.01 degrees round
RA 287.50086, Dec -22.23481, at most 3. It is whole, not trimmed: the
query asks for three rows. Its three images are all QR2.

cutout-qr3-D2-image.fits is real: the first 34,560 bytes of the cutout
IRSA's image server made on 2026-11-06 of the QR3 image
level2_2026W37_2A_0136_2D2_spx_l2b-v27-2026-259.fits, 0.05 degrees round
the same place. They are its two first units, the empty primary header
and the IMAGE extension, 29 by 29 pixels, and end where its next unit,
FLAGS, begins, so they are a whole FITS file by themselves. The rest of
that cutout (FLAGS, VARIANCE, ZODI, EPSF and WCS-WAVE) is 3.9 MB and is
not kept here. A fetch holds a file to being a whole cutout, so these
two units alone are refused: a test holds that.

The answers the tests give that are neither of these are made in the
tests: tables with other rows, columns left out, whole cutouts of QR2
and QR3 crafted unit by unit, and cutouts cut short, extended or
altered card by card.

What a whole cutout holds, the units the check asks for, was read from
the headers of 340 cutouts of both releases, fetched on 2026-11-06
round the same place. They are kept outside git and outside this
repository. No FETCHED record was written when they were fetched, and
they are not yet kept under the sources policy; that record is owed
before they are used as evidence.
