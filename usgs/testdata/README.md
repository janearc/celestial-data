# usgs testdata

splib07a-excerpt.zip holds three files of the USGS Spectral Library
Version 7 ASCII archive, ASCIIdata_splib07a.zip, as fetched on 2026-11-02
from ScienceBase (21812828 bytes, sha256
d232645740869a82aafcad5839448c50b1dc72965ce042d1374f29b7a798a91c). The
library is a US Geological Survey data release, in the public domain,
https://doi.org/10.5066/F7RR1WDJ.

The files are whole and unchanged, under the folders they have in the
archive:

- ASCIIdata_splib07a/splib07a_Wavelengths_BECK_Beckman_0.2-3.0_microns.txt,
  the 481 channels, in micrometres, of the Beckman instrument.
- ASCIIdata_splib07a/ChapterL_Liquids/
  splib07a_Seawater_Open_Ocean_SW2_lwch_BECKa_AREF.txt, a spectrum with
  a measurement in every channel.
- ASCIIdata_splib07a/ChapterV_Vegetation/
  splib07a_Grass_dry.5+.5green_AMX28_BECKa_AREF.txt, a spectrum with 14
  channels that hold -1.2300000e+034, the value for no measurement: lines
  2 to 10, 55 to 57 and 240 to 241 of the file.

The archive was unzipped beside the zip, and the excerpt made from the
unzipped files, in the folder that holds ASCIIdata_splib07a/:

    zip -9 splib07a-excerpt.zip \
      ASCIIdata_splib07a/splib07a_Wavelengths_BECK_Beckman_0.2-3.0_microns.txt \
      ASCIIdata_splib07a/ChapterL_Liquids/splib07a_Seawater_Open_Ocean_SW2_lwch_BECKa_AREF.txt \
      "ASCIIdata_splib07a/ChapterV_Vegetation/splib07a_Grass_dry.5+.5green_AMX28_BECKa_AREF.txt"
