package plates_test

import (
	"strings"
	"testing"

	"github.com/janearc/celestial-data/plates"
)

// The sizes the image tests are held to, each defined: a file the images
// fit in, and an image of ten by three pixels. A block is plates_test's.
const (
	roomy      = 1 << 20 // defined: a file far longer than these images
	tenByThree = 30      // defined: the pixels of a ten by three image
)

// the test: an image's bytes are its pixels times the bytes of one, for
// every BITPIX the FITS standard allows, 4.4.1.1; a length is read in
// base ten.
func TestAnImageIsItsPixelsTimesAPixelsBytes(t *testing.T) {
	for bitpix, width := range map[string]int64{"8": 1, "16": 2, "32": 4,
		"64": 8, "-32": 4, "-64": 8} {
		got, err := plates.ImageBytes(map[string]string{"BITPIX": bitpix,
			"NAXIS": "2", "NAXIS1": "10", "NAXIS2": "3"}, roomy)
		if err != nil || got != width*tenByThree {
			t.Errorf("BITPIX %s: %d bytes, %v; not %d", bitpix, got, err,
				width*tenByThree)
		}
	}
	if _, err := plates.ImageBytes(map[string]string{"BITPIX": "24",
		"NAXIS": "0"}, roomy); err == nil {
		t.Error("BITPIX 24, which the standard does not allow, was sized")
	}
}

// the test: an axis may be of no length, and an image as long as the
// file is not longer than it.
//
// Anti-test: one byte longer than the file is refused.
func TestAnImageMayFillTheFileAndNoMore(t *testing.T) {
	cards := func(length string) map[string]string {
		return map[string]string{"BITPIX": "8", "NAXIS": "1",
			"NAXIS1": length}
	}
	if got, err := plates.ImageBytes(cards("0"), roomy); err != nil ||
		got != 0 {
		t.Errorf("an axis of no length: %d, %v", got, err)
	}
	if got, err := plates.ImageBytes(cards("100"), 100); err != nil ||
		got != 100 {
		t.Errorf("an image as long as the file: %d, %v", got, err)
	}
	if _, err := plates.ImageBytes(map[string]string{"BITPIX": "16",
		"NAXIS": "1", "NAXIS1": "100"}, 100); err == nil ||
		!strings.Contains(err.Error(), "longer than the file") {
		t.Errorf("an image longer than the file: %v", err)
	}
}

// the test: 999 axes is a count of axes, and their lengths are then
// what is read; 1000 is not.
func TestNineHundredAndNinetyNineAxesAreTheMost(t *testing.T) {
	_, err := plates.ImageBytes(map[string]string{"BITPIX": "8",
		"NAXIS": "999"}, roomy)
	if err == nil || !strings.Contains(err.Error(), "NAXIS1") {
		t.Errorf("999 axes: %v; the first axis's length is what is wrong",
			err)
	}
	_, err = plates.ImageBytes(map[string]string{"BITPIX": "8",
		"NAXIS": "1000"}, roomy)
	if err == nil || !strings.Contains(err.Error(), "not a count of axes") {
		t.Errorf("1000 axes: %v", err)
	}
}

// the test: a length is made up to whole blocks: none stays none, one
// byte is a block, a block is a block, and a byte more is two.
func TestALengthIsMadeUpToWholeBlocks(t *testing.T) {
	for length, want := range map[int64]int64{0: 0, 1: block,
		block: block, block + 1: 2 * block} {
		if got := plates.Padded(length); got != want {
			t.Errorf("%d bytes padded to %d, not %d", length, got, want)
		}
	}
}
