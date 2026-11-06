# keck testdata

hires-*.xml are the answers of KOA's TAP service, recorded on
2026-11-06, to the queries keck.Find makes: for the replay circle round
Sgr A*, which holds no public HIRES science frame, and for 0.01 degrees
round RA 148.49225, Dec 57.68342, at most 2. asked-*.txt are the
addresses asked. level1-*.json are KOA's level-1 lists of those two
frames, recorded the same day.

The crafted-* files are made by hand from them: a frame naming its
target only in object, an answer with progpi renamed, a frame with no
koaid, a level-1 list
that is not JSON, one with no folders, one naming a file outside its
folder, and one whose flux files sit in folders other than makee's
fits, beside a DN file.
