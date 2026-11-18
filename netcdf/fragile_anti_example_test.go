//go:build fragile

package netcdf_test

import (
	"fmt"
	"testing"

	classic "github.com/scigolib/netcdf"
)

// The example at example_test.go:9 prints CDF-2 for a header of version
// 2, which code that answered CDF-2 for every header would also print.
// Its anti: the same header with the version byte 1 and 5 prints the
// other two names, and each of the three is told from the others.
func TestAntiExampleDetectFormat(t *testing.T) {
	for _, each := range []struct {
		version byte
		want    string
	}{{1, "CDF-1"}, {2, "CDF-2"}, {5, "CDF-5"}} {
		header := []byte("CDF\x00rest of the header")
		header[3] = each.version
		if got := fmt.Sprint(classic.DetectFormat(header)); got != each.want {
			t.Errorf("version %d prints %s, want %s", each.version, got,
				each.want)
		}
	}
}
