// Package noaacrm retains bounded native-grid NOAA Hawaii CRM2023 subsets.
// Endpoint, coverage, geographic axes and supported format come from the
// archive's retained WCS 1.0.0 capabilities and coverage description.
package noaacrm

import (
	"encoding/xml"
	"fmt"
	"io"
	"math"
	"net/url"
	"strconv"
	"strings"

	data "github.com/janearc/celestial-data"
	"github.com/janearc/celestial-data/web"
)

// Ceiling is the owner's authored transfer limit, in bytes, not a dataset
// measurement. A response exceeding it is closed without retaining a file.
const Ceiling int64 = 150 * 1024 * 1024

// Endpoint is the GetCoverage OnlineResource published by NOAA in the
// retained capabilities response, read 2026-11-09. Other archives, products,
// credentials and guessed fileServer URLs are not accepted here.
const Endpoint = "https://www.ngdc.noaa.gov/thredds/wcs/crm/cudem/" +
	"crm_vol10_2023.nc"

// Metadata is the subset of retained WCS metadata required to form a request.
type Metadata struct {
	West, South, East, North float64
	DX, DY                   float64
}

// Box is an authored geographic extent, in longitude/latitude degrees.
type Box struct {
	West  float64 `yaml:"West" json:"West"`
	South float64 `yaml:"South" json:"South"`
	East  float64 `yaml:"East" json:"East"`
	North float64 `yaml:"North" json:"North"`
}

// wcsCapabilities is the published WCS endpoint and version, read from
// the retained GetCapabilities response.
type wcsCapabilities struct {
	XMLName xml.Name `xml:"WCS_Capabilities"`
	Version string   `xml:"version,attr"`
	URLs    []struct {
		Href string `xml:"href,attr"`
	} `xml:"Capability>Request>GetCoverage>DCPType>HTTP>Get>OnlineResource"`
}

// ReadMetadata validates the published endpoint and native lon/lat grid.
// EPSG:0 in the response is not promoted to a known vertical or horizontal
// CRS: final TIFF tags and the product guide must be inspected separately.
func ReadMetadata(capabilities, description []byte) (Metadata, error) {
	var caps wcsCapabilities
	var desc struct {
		XMLName  xml.Name `xml:"CoverageDescription"`
		Version  string   `xml:"version,attr"`
		Offering struct {
			Name      string   `xml:"name"`
			Positions []string `xml:"lonLatEnvelope>pos"`
			CRS       string   `xml:"supportedCRSs>requestCRSs"`
			Formats   []string `xml:"supportedFormats>formats"`
			Method    string   `xml:"supportedInterpolations>interpolationMethod"`
			Offsets   []string `xml:"domainSet>spatialDomain>RectifiedGrid>offsetVector"`
		} `xml:"CoverageOffering"`
	}
	if err := xml.Unmarshal(capabilities, &caps); err != nil {
		return Metadata{}, err
	}
	if err := xml.Unmarshal(description, &desc); err != nil {
		return Metadata{}, err
	}
	if caps.Version != "1.0.0" || len(caps.URLs) != 1 ||
		caps.URLs[0].Href != Endpoint {
		return Metadata{}, fmt.Errorf("capabilities do not advertise " +
			"the " +
			"NOAA Hawaii CRM2023 endpoint")
	}
	o := desc.Offering
	if desc.Version != "1.0.0" || o.Name != "z" ||
		o.CRS != "OGC:CRS84" || o.Method != "none" ||
		!has(o.Formats, "GeoTIFF_Float") || len(o.Positions) != 2 ||
		len(o.Offsets) != 2 {
		return Metadata{}, fmt.Errorf("coverage does not advertise " +
			"native " +
			"CRS84 z GeoTIFF_Float")
	}
	lo, err := pair(o.Positions[0])
	if err != nil {
		return Metadata{}, err
	}
	hi, err := pair(o.Positions[1])
	if err != nil {
		return Metadata{}, err
	}
	x, err := pair(o.Offsets[0])
	if err != nil {
		return Metadata{}, err
	}
	y, err := pair(o.Offsets[1])
	if err != nil {
		return Metadata{}, err
	}
	if x[0] <= 0 || y[1] <= 0 || x[1] != 0 || y[0] != 0 ||
		lo[0] >= hi[0] || lo[1] >= hi[1] {
		return Metadata{}, fmt.Errorf("coverage has an unsupported " +
			"geographic grid")
	}
	return Metadata{lo[0], lo[1], hi[0], hi[1], x[0], y[1]}, nil
}

// has reports whether values contains value.
func has(values []string, value string) bool {
	for _, v := range values {
		if v == value {
			return true
		}
	}
	return false
}

// pair parses two finite coordinates from text.
func pair(text string) ([2]float64, error) {
	var result [2]float64
	words := strings.Fields(text)
	if len(words) != 2 {
		return result, fmt.Errorf("expected geographic coordinate pair")
	}
	for i, word := range words {
		n, err := strconv.ParseFloat(word, 64)
		if err != nil || !finite(n) {
			return result, fmt.Errorf(
				"invalid geographic coordinate")
		}
		result[i] = n
	}
	return result, nil
}

// finite reports whether n is neither NaN nor an infinity.
func finite(n float64) bool {
	return !math.IsNaN(n) && !math.IsInf(n, 0)
}

// Dimensions is a conservative native-grid dimension ceiling, allowing three
// extra edge samples for archive bbox snapping. It is not a resampling request.
func (b Box) Dimensions(m Metadata) (int, int, error) {
	for _, n := range []float64{b.West, b.South, b.East, b.North,
		m.West, m.South, m.East, m.North, m.DX, m.DY} {
		if !finite(n) {
			return 0, 0, fmt.Errorf("non-finite box or grid")
		}
	}
	if m.DX <= 0 || m.DY <= 0 || b.West >= b.East ||
		b.South >= b.North || b.West < m.West || b.East > m.East ||
		b.South < m.South || b.North > m.North {
		return 0, 0, fmt.Errorf("box is not inside the described " +
			"Hawaii native grid")
	}
	w := math.Ceil((b.East-b.West)/m.DX) + 3
	h := math.Ceil((b.North-b.South)/m.DY) + 3
	// Four bytes per float32 sample plus a conservative authored 1 MiB
	// metadata allowance. The actual transfer has an independent hard
	// limit.
	if w*h*4+1024*1024 > float64(Ceiling) {
		return 0, 0, fmt.Errorf("native subset estimate exceeds " +
			"150 MiB " +
			"transfer budget")
	}
	return int(w), int(h), nil
}

// Product makes a bbox-only native-grid WCS request. This parameter shape is
// published in Unidata's TDS WCS reference, Examples/GetCoverage. We request
// the archive-advertised Float format; no width, stride, reprojection or
// interpolation is invented. The response must prove its own dimensions.
func (b Box) Product(m Metadata, name string) (*data.Product, error) {
	w, h, err := b.Dimensions(m)
	if err != nil {
		return nil, err
	}
	q := url.Values{
		"service":  {"WCS"},
		"version":  {"1.0.0"},
		"request":  {"GetCoverage"},
		"coverage": {"z"},
		"format":   {"GeoTIFF_Float"},
		"crs":      {"OGC:CRS84"},
		"bbox": {fmt.Sprintf("%s,%s,%s,%s", number(b.West),
			number(b.South),
			number(b.East), number(b.North))},
	}
	a, _ := data.Named("noaacrm")
	p, err := a.Product("crm_vol10_2023:z", name,
		Endpoint+"?"+q.Encode(), data.Unsized,
		"NOAA Hawaii CRM2023 native float32 topobathymetry subset")
	return p.Checked(tiffCheck{w, h, m.DX, m.DY}), err
}

// number writes n without adding precision beyond its float64 value.
func number(n float64) string {
	return strconv.FormatFloat(n, 'g', -1, 64)
}

// Fetch uses the common atomic, length-checked Fetch with an archive-specific
// response-size guard. Existing products and partial files are never replaced.
func Fetch(get web.Web, b Box, m Metadata, name, into string) (string, error) {
	p, err := b.Product(m, name)
	if err != nil {
		return "", err
	}
	return data.Fetch(limitedWeb{get, Ceiling}, p, into)
}

type limitedWeb struct {
	web.Web
	limit int64
}

// Get closes a response whose declared size exceeds the request limit.
func (w limitedWeb) Get(address string) (io.ReadCloser, int64, error) {
	body, size, err := w.Web.Get(address)
	if err != nil {
		return nil, size, err
	}
	if size > w.limit {
		body.Close()
		return nil, size, fmt.Errorf("server declared %d bytes, over "+
			"%d-byte budget", size, w.limit)
	}
	return &limitedBody{ReadCloser: body, remaining: w.limit}, size, nil
}

type limitedBody struct {
	io.ReadCloser
	remaining int64
}

// Read enforces the byte ceiling, probing once to distinguish an exact EOF.
func (b *limitedBody) Read(p []byte) (int, error) {
	if b.remaining == 0 {
		// A probe distinguishes an exact EOF from excess.
		// It reads one excess byte at most; none is written.
		var probe [1]byte
		n, err := b.ReadCloser.Read(probe[:])
		if n > 0 {
			return 0, fmt.Errorf("response exceeds byte budget")
		}
		return 0, err
	}
	if int64(len(p)) > b.remaining {
		p = p[:b.remaining]
	}
	n, err := b.ReadCloser.Read(p)
	b.remaining -= int64(n)
	return n, err
}
