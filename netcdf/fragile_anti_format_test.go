//go:build fragile

package netcdf_test

import (
	"fmt"
	"testing"

	classic "github.com/scigolib/netcdf"
)

// hdfSignature is the eight bytes that begin an HDF5 file.
var hdfSignature = []byte{0x89, 'H', 'D', 'F', '\r', '\n', 0x1a, '\n'}

// The test at format_test.go:5 has one row where the first byte of the
// magic differs, one for a version that was never used and one for seven
// bytes of the signature. Its anti changes every byte of the magic and of
// the signature one at a time, and every version byte but the three: none
// of them may be a format. Code that looks at the first byte and the
// version only, or at the first seven of the signature, passes the test.
func TestAntiDetectFormat(t *testing.T) {
	magic := []byte("CDF\x01rest of the header")
	for at := range 4 {
		changed := append([]byte(nil), magic...)
		changed[at] ^= 0x20
		if got := classic.DetectFormat(changed); got != classic.FormatUnknown {
			t.Errorf("byte %d of the magic changed: %v, want unknown", at, got)
		}
	}
	for version := range 256 {
		changed := append([]byte(nil), magic...)
		changed[3] = byte(version)
		known := version == 1 || version == 2 || version == 5
		if got := classic.DetectFormat(changed); (got != classic.
			FormatUnknown) != known {
			t.Errorf("version byte %d is %v", version, got)
		}
	}
	for at := range hdfSignature {
		changed := append([]byte(nil), hdfSignature...)
		changed[at] ^= 0x20
		if got := classic.DetectFormat(append(changed, 0, 0)); got ==
			classic.FormatHDF5 {
			t.Errorf("byte %d of the signature changed is still HDF5", at)
		}
	}
}

// The test at format_test.go:35 names six formats. Its anti asks all 256
// numbers: the five that are formats have their own names, all different,
// and every other is Format(n). Code that named a number that is no
// format, or two the same, passes the six rows.
func TestAntiFormatString(t *testing.T) {
	known := map[classic.Format]string{classic.FormatUnknown: "unknown",
		classic.FormatClassic: "CDF-1", classic.FormatClassic64: "CDF-2",
		classic.FormatClassic64Data: "CDF-5", classic.FormatHDF5: "NetCDF-4"}
	for number := range 256 {
		want, isKnown := known[classic.Format(number)]
		if !isKnown {
			want = fmt.Sprintf("Format(%d)", number)
		}
		if got := classic.Format(number).String(); got != want {
			t.Errorf("Format(%d) is %q, want %q", number, got, want)
		}
	}
}
