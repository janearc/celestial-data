package noaacrm

import (
	"bytes"
	"encoding/binary"
	"io"
	"math"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

const capabilities = `<WCS_Capabilities version="1.0.0"><Capability><Request><GetCoverage><DCPType><HTTP><Get><OnlineResource xmlns:xlink="http://www.w3.org/1999/xlink" xlink:href="` + Endpoint + `"/></Get></HTTP></DCPType></GetCoverage></Request></Capability></WCS_Capabilities>`
const description = `<CoverageDescription version="1.0.0"><CoverageOffering><name>z</name><lonLatEnvelope><pos>-161 18</pos><pos>-153.5 23</pos></lonLatEnvelope><domainSet><spatialDomain><RectifiedGrid><offsetVector>0.00027777777 0</offsetVector><offsetVector>0 0.00027777777</offsetVector></RectifiedGrid></spatialDomain></domainSet><supportedCRSs><requestCRSs>OGC:CRS84</requestCRSs></supportedCRSs><supportedFormats><formats>GeoTIFF_Float</formats></supportedFormats><supportedInterpolations><interpolationMethod>none</interpolationMethod></supportedInterpolations></CoverageOffering></CoverageDescription>`

func TestPublishedMetadataAndExactRequest(t *testing.T) {
	m, err := ReadMetadata([]byte(capabilities), []byte(description))
	if err != nil {
		t.Fatal(err)
	}
	p, err := (Box{-156.2, 18.8, -154.7, 20.4}).Product(m, "big-island.tif")
	if err != nil {
		t.Fatal(err)
	}
	u, _ := url.Parse(p.URL())
	want := "bbox=-156.2%2C18.8%2C-154.7%2C20.4&coverage=z&crs=OGC%3ACRS84&format=GeoTIFF_Float&request=GetCoverage&service=WCS&version=1.0.0"
	if u.RawQuery != want || u.Scheme+"://"+u.Host+u.Path != Endpoint {
		t.Fatalf("unexpected request: %s", p.URL())
	}
	for _, bad := range []string{strings.Replace(capabilities, "ngdc.noaa.gov", "example.org", 1), "<ExceptionReport/>"} {
		if _, err := ReadMetadata([]byte(bad), []byte(description)); err == nil {
			t.Fatal("accepted non-advertised endpoint")
		}
	}
	for _, old := range []string{"GeoTIFF_Float", "OGC:CRS84", "none", "<name>z</name>"} {
		if _, err := ReadMetadata([]byte(capabilities), []byte(strings.Replace(description, old, "other", 1))); err == nil {
			t.Fatal("accepted incompatible coverage")
		}
	}
}

func TestBoxRefusesFullArchiveAndInvalidCoordinates(t *testing.T) {
	m, _ := ReadMetadata([]byte(capabilities), []byte(description))
	for _, b := range []Box{{-161, 18, -153.5, 23}, {-162, 18.8, -154.7, 20.4}, {-156.2, 20.4, -154.7, 18.8}, {math.NaN(), 18.8, -154.7, 20.4}} {
		if _, err := b.Product(m, "out.tif"); err == nil {
			t.Fatalf("accepted invalid/oversized box %#v", b)
		}
	}
	// Independent small-grid edge accounting: 4 by 8 intervals plus the
	// documented three-sample conservative allowance gives 7 by 11.
	w, h, err := (Box{1, 1, 2, 3}).Dimensions(Metadata{0, 0, 4, 4, .25, .25})
	if err != nil || w != 7 || h != 11 {
		t.Fatalf("dimensions %d %d: %v", w, h, err)
	}
}

type fakeWeb struct {
	body   []byte
	length int64
	calls  int
	closed bool
}

type fakeBody struct {
	*bytes.Reader
	closed *bool
}

func (b fakeBody) Close() error { *b.closed = true; return nil }
func (f *fakeWeb) Get(string) (io.ReadCloser, int64, error) {
	f.calls++
	return fakeBody{bytes.NewReader(f.body), &f.closed}, f.length, nil
}
func (*fakeWeb) Head(string) (int64, error)              { panic("unexpected HEAD") }
func (*fakeWeb) Post(string, url.Values) (string, error) { panic("unexpected POST") }

func TestBothDeclaredAndStreamingByteLimits(t *testing.T) {
	for _, test := range []struct {
		body string
		size int64
		ok   bool
	}{{"12345678", 8, true}, {"12345678", -1, true}, {"123456789", -1, false}, {"small", 900, false}} {
		f := &fakeWeb{body: []byte(test.body), length: test.size}
		body, _, err := (limitedWeb{f, 8}).Get("https://fixture.invalid")
		if err == nil {
			var got []byte
			got, err = io.ReadAll(body)
			body.Close()
			if len(got) > 8 {
				t.Fatal("returned bytes over budget")
			}
		}
		if (err == nil) != test.ok || !f.closed {
			t.Fatalf("case %#v: err=%v closed=%t", test, err, f.closed)
		}
	}
}

// fixtureTIFF is a hand-authored 2x2 float32 image with one strip and known
// scale, tiepoint and geographic key. It does not use the production parser
// to compute its expected values. Pixel values are 1,2,3,4, without compression.
func fixtureTIFF(order binary.ByteOrder) []byte {
	b := make([]byte, 274)
	copy(b, "II")
	if order == binary.BigEndian {
		copy(b, "MM")
	}
	order.PutUint16(b[2:], 42)
	order.PutUint32(b[4:], 8)
	order.PutUint16(b[8:], 13)
	tags := [][4]uint32{{256, 4, 1, 2}, {257, 4, 1, 2}, {258, 3, 1, 32}, {259, 3, 1, 1}, {262, 3, 1, 1}, {273, 4, 1, 258}, {277, 3, 1, 1}, {278, 4, 1, 2}, {279, 4, 1, 16}, {339, 3, 1, 3}, {33550, 12, 3, 170}, {33922, 12, 6, 194}, {34735, 3, 8, 242}}
	for i, tag := range tags {
		e := b[10+i*12:]
		order.PutUint16(e, uint16(tag[0]))
		order.PutUint16(e[2:], uint16(tag[1]))
		order.PutUint32(e[4:], tag[2])
		if tag[1] == 3 && tag[2] == 1 {
			order.PutUint16(e[8:], uint16(tag[3]))
		} else {
			order.PutUint32(e[8:], tag[3])
		}
	}
	for i, n := range []float64{.25, .25, 0, 0, 0, 0, -156, 20, 0} {
		order.PutUint64(b[170+i*8:], math.Float64bits(n))
	}
	for i, n := range []uint16{1, 1, 0, 1, 2048, 0, 1, 4326} {
		order.PutUint16(b[242+i*2:], n)
	}
	for i, n := range []float32{1, 2, 3, 4} {
		order.PutUint32(b[258+i*4:], math.Float32bits(n))
	}
	return b
}

func TestTIFFMetadataBothOrdersAndTruncation(t *testing.T) {
	for _, order := range []binary.ByteOrder{binary.LittleEndian, binary.BigEndian} {
		path := filepath.Join(t.TempDir(), "fixture.tif")
		os.WriteFile(path, fixtureTIFF(order), 0600)
		info, err := Inspect(path)
		if err != nil || info.Width != 2 || info.Height != 2 || info.Tags[33550].Values[0] != .25 || info.Tags[33922].Values[4] != 20 || info.Tags[34735].Values[7] != 4326 {
			t.Fatalf("unexpected metadata %#v: %v", info, err)
		}
		if err := (tiffCheck{2, 2, .25, .25}).Check(path); err != nil {
			t.Fatal(err)
		}
		if err := (tiffCheck{1, 2, .25, .25}).Check(path); err == nil {
			t.Fatal("accepted excessive dimensions")
		}
		os.Truncate(path, 273)
		if err := (tiffCheck{2, 2, .25, .25}).Check(path); err == nil {
			t.Fatal("accepted incomplete pixel strip")
		}
	}
}

func TestFetchAtomicFailuresAndExistingFile(t *testing.T) {
	m := Metadata{-161, 18, -153.5, 23, .25, .25}
	b := Box{-156, 19, -155, 20}
	for _, f := range []*fakeWeb{{[]byte("<ServiceExceptionReport/>"), -1, 0, false}, {fixtureTIFF(binary.LittleEndian), 500, 0, false}} {
		into := t.TempDir()
		if _, err := Fetch(f, b, m, "out.tif", into); err == nil {
			t.Fatal("accepted failed/truncated response")
		}
		files, _ := os.ReadDir(into)
		if len(files) != 0 {
			t.Fatal("left partial product")
		}
	}
	f := &fakeWeb{body: fixtureTIFF(binary.BigEndian), length: 274}
	into := t.TempDir()
	if _, err := Fetch(f, b, m, "out.tif", into); err != nil {
		t.Fatal(err)
	}
	if _, err := Fetch(f, b, m, "out.tif", into); err == nil || f.calls != 1 {
		t.Fatal("existing source fetched again or overwritten")
	}
}
