// Package usgs fetches and reads the USGS Spectral Library Version 7: the
// measured reflectance of minerals, soils, water, plants and made things,
// from the ultraviolet into the infrared.
//
// The library is a US Geological Survey data release, in the public
// domain. Work using it cites Kokaly and others, 2017, USGS Spectral
// Library Version 7 Data, https://doi.org/10.5066/F7RR1WDJ.
package usgs

import (
	"archive/zip"
	"bufio"
	_ "embed"
	"fmt"
	"io"
	"math"
	"path"
	"strconv"
	"strings"

	"github.com/janearc/celestial-data"
)

// adapterName is the archive's name, as the command line takes it.
const adapterName = "usgs"

// The library's ASCII archive, as ScienceBase keeps it.
const (
	// doi names the data release.
	doi = "10.5066/F7RR1WDJ"
	// archiveName is the archive of the measured spectra in ASCII.
	archiveName = "ASCIIdata_splib07a.zip"
	// address is where ScienceBase serves it, by its item and name.
	address = "https://www.sciencebase.gov/catalog/file/get/" +
		"586e8c88e4b0f5ce109fccae?name=" + archiveName
	// prefix begins the name of every measured spectrum's file.
	prefix = "splib07a_"
)

// The numbers the library gives, by their names in numbers.yaml.
const (
	// noData is what a file writes where a channel has no measurement.
	noData = "no-data"
	// archiveSize is the archive's length in bytes, as served.
	archiveSize = "archive-size"
)

// numbersFile is the numbers file that holds the library's numbers.
const numbersFile = "numbers.yaml"

//go:embed numbers.yaml
var numbersRaw []byte

// measured is the numbers file, read.
func measured() *celestialdata.Measured {
	return celestialdata.NewMeasured(numbersFile, numbersRaw)
}

// New is the archive of the library, asked by name only. It holds the
// library's measured spectra as one archive to fetch, whatever the circle.
func New() (*celestialdata.Fixed, error) {
	size := int64(measured().Value(archiveSize))
	about := "USGS Spectral Library Version 7, public domain, cite " +
		"Kokaly and others 2017, doi:" + doi
	library := celestialdata.NewFile(doi, archiveName, address, size, about)
	return celestialdata.NewFixed(adapterName, library)
}

// refuse is a refusal that begins with the archive's name.
func refuse(format string, args ...any) error {
	return celestialdata.Refuse(adapterName+" "+format, args...)
}

// wavelengthFiles are the files of each instrument's channels, in
// micrometres, by the code a spectrum's name gives its instrument.
var wavelengthFiles = map[string]string{
	"BECK":   "splib07a_Wavelengths_BECK_Beckman_0.2-3.0_microns.txt",
	"ASD":    "splib07a_Wavelengths_ASD_0.35-2.5_microns_2151_ch.txt",
	"NIC4":   "splib07a_Wavelengths_NIC4_Nicolet_1.12-216microns.txt",
	"AVIRIS": "splib07a_Wavelengths_AVIRIS_1996_0.37-2.5_microns.txt",
}

// Spectrum is one measured reflectance, by wavelength.
//
//ninja:vow practice a library: the viewer in slayer draws spectra
type Spectrum struct {
	name        string
	nanometres  []float64
	reflectance []float64
}

// Name is the spectrum's name as the library gives it.
//
//ninja:vow practice a library: the viewer in slayer draws spectra
func (spectrum Spectrum) Name() string { return spectrum.name }

// Read is a spectrum from the library's archive, by its name: its file's
// name without the folder, the prefix or ".txt", as
// "Seawater_Open_Ocean_SW2_lwch_BECKa_AREF".
//
// Channels with no measurement are left out. It refuses a name the
// archive does not hold and an instrument it does not know.
//
//ninja:vow practice a library: the viewer in slayer calls it
func Read(archive io.ReaderAt, size int64, name string) (*Spectrum, error) {
	gone := measured().Value(noData)
	files, err := zip.NewReader(archive, size)
	if err != nil {
		return nil, fmt.Errorf("%s: %w", adapterName, err)
	}
	spectrumFile, wavelengthFile, err := filesOf(files.File, name)
	if err != nil {
		return nil, err
	}
	values, err := column(spectrumFile)
	if err != nil {
		return nil, err
	}
	microns, err := column(wavelengthFile)
	if err != nil {
		return nil, err
	}
	if len(values) != len(microns) {
		return nil, refuse("spectrum %s has %d values for %d channels",
			name, len(values), len(microns))
	}
	return measure(name, gone, values, microns), nil
}

// filesOf is a spectrum's file and its instrument's wavelength file, from
// those the archive holds.
func filesOf(files []*zip.File, name string) (spectrumFile,
	wavelengthFile *zip.File, err error) {
	instrument, err := instrumentOf(name)
	if err != nil {
		return nil, nil, err
	}
	for _, file := range files {
		switch path.Base(file.Name) {
		case prefix + name + ".txt":
			spectrumFile = file
		case wavelengthFiles[instrument]:
			wavelengthFile = file
		}
	}
	if spectrumFile == nil {
		return nil, nil, refuse("holds no spectrum %s", name)
	}
	if wavelengthFile == nil {
		return nil, nil, refuse("holds no channels for %s", name)
	}
	return spectrumFile, wavelengthFile, nil
}

// measure is the spectrum of a name, from its values and the channels'
// micrometres. A channel either file gives as gone is left out.
func measure(name string, gone float64, values, microns []float64) *Spectrum {
	spectrum := &Spectrum{name: name}
	for i, value := range values {
		if value <= gone || microns[i] <= gone {
			continue
		}
		spectrum.nanometres = append(spectrum.nanometres,
			microns[i]*nanometresAMicron)
		spectrum.reflectance = append(spectrum.reflectance, value)
	}
	return spectrum
}

// nanometresAMicron is the nanometres in a micrometre. It's defined.
const nanometresAMicron = 1000

// instrumentOf is the instrument a spectrum's name says measured it: the
// code before its last part, as BECKa in ..._BECKa_AREF.
func instrumentOf(name string) (string, error) {
	parts := strings.Split(name, "_")
	if len(parts) >= minimumParts {
		code := parts[len(parts)-minimumParts]
		for instrument := range wavelengthFiles {
			if strings.HasPrefix(code, instrument) {
				return instrument, nil
			}
		}
	}
	return "", refuse("knows no instrument in the name %s", name)
}

// minimumParts is how many parts a name has at least: the instrument's
// code and the kind of reflectance after it. It's defined by the
// library's names.
const minimumParts = 2

// column is the numbers of a library file: one a line, after its title.
func column(file *zip.File) ([]float64, error) {
	opened, err := file.Open()
	if err != nil {
		return nil, unread(file, err)
	}
	defer opened.Close()
	lines := bufio.NewScanner(opened)
	lines.Scan()
	var numbers []float64
	for lines.Scan() {
		text := strings.TrimSpace(lines.Text())
		number, err := strconv.ParseFloat(text, bits)
		if err != nil {
			return nil, unread(file, err)
		}
		numbers = append(numbers, number)
	}
	if err := lines.Err(); err != nil {
		return nil, unread(file, err)
	}
	return numbers, nil
}

// unread is why a file in the archive would not read.
func unread(file *zip.File, err error) error {
	return fmt.Errorf("%s: %s: %w", adapterName, file.Name, err)
}

// bits is a float64's width, for ParseFloat. It's defined.
const bits = 64

// The places At looks and the value it gives where it finds none.
const (
	// oneBack is how far back a channel's neighbour is. It's defined.
	oneBack = 1
	// noValue is what At gives with false. It's defined: it says nothing.
	noValue = 0
)

// At is the reflectance at a wavelength in nanometres, drawn straight
// between the channels either side. A channel with no measurement is left
// out, so the line is drawn across it. There is none outside the measured
// span, and none for a wavelength that is not a number.
//
//ninja:vow practice a library: the viewer in slayer draws spectra
func (spectrum Spectrum) At(nm float64) (float64, bool) {
	if math.IsNaN(nm) {
		return noValue, false
	}
	for i := oneBack; i < len(spectrum.nanometres); i++ {
		low := spectrum.nanometres[i-oneBack]
		high := spectrum.nanometres[i]
		if nm < low || nm > high {
			continue
		}
		before := spectrum.reflectance[i-oneBack]
		if high == low {
			return before, true
		}
		after := spectrum.reflectance[i]
		share := (nm - low) / (high - low)
		return before + share*(after-before), true
	}
	return noValue, false
}
