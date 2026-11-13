package dasch_test

import (
	"testing"

	"github.com/janearc/celestial-data"
	"github.com/janearc/celestial-data/dasch"
	"github.com/janearc/celestial-data/internal/replay"
)

// the test: DASCH's own pages are an archive asked only by name, holding
// the three DR7 pages the adapter was read from, each by its address.
func TestDASCHsPagesAreHeldByAddress(t *testing.T) {
	pages, err := dasch.Pages()
	if err != nil {
		t.Fatal(err)
	}
	var asked celestialdata.ByName = pages
	if asked.Name() != "dasch-pages" || !asked.ByNameOnly() {
		t.Errorf("%q, by name only %v", asked.Name(), asked.ByNameOnly())
	}
	found, err := pages.Find(replay.Asked(t))
	if err != nil {
		t.Fatal(err)
	}
	want := map[string]string{
		"dasch-dr7-web-apis.html": "https://dasch.cfa.harvard.edu/dr7/" +
			"web-apis/",
		"dasch-dr7-data-products.html": "https://dasch.cfa.harvard.edu/" +
			"dr7/data-products/",
		"dasch-dr7-lightcurve-columns.html": "https://dasch.cfa.harvard." +
			"edu/dr7/lightcurve-columns/",
	}
	if len(found) != len(want) {
		t.Fatalf("%d pages, not %d", len(found), len(want))
	}
	for _, page := range found {
		if want[page.Name()] != page.URL() {
			t.Errorf("%s at %s", page.Name(), page.URL())
		}
	}
}
