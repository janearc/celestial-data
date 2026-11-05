//go:build live

package irsa_test

import (
	"bufio"
	"strings"
	"testing"

	"github.com/janearc/celestial-data/internal/replay"
	"github.com/janearc/celestial-data/irsa"
)

// The live checks ask the archive itself. They run only with go test
// -tags live, since they need the network.
//
// The mosaics are asked for their length, never for the files.
func TestLiveFindsMosaicsThatAreThere(t *testing.T) {
	get := replay.Live(t)
	glimpse, err := irsa.NewGlimpse(get, irsa.GlimpseII)
	if err != nil {
		t.Fatal(err)
	}
	products, err := glimpse.Find(replay.Asked(t))
	if err != nil || len(products) != bands {
		t.Fatalf("%d products, %v", len(products), err)
	}
	for _, product := range products {
		length, err := get.Head(product.URL())
		if err != nil || length <= unsized {
			t.Errorf("%s: %d bytes, %v", product.URL(), length, err)
		}
	}
}

// The catalogue's query is asked for real. Only its first line is read,
// which names the columns.
func TestLiveCatalogueAnswersWithItsColumns(t *testing.T) {
	catalogue, err := irsa.NewCatalogue("galcen_psc", "*")
	if err != nil {
		t.Fatal(err)
	}
	products, err := catalogue.Find(replay.Asked(t))
	if err != nil {
		t.Fatal(err)
	}
	body, _, err := replay.Live(t).Get(products[first].URL())
	if err != nil {
		t.Fatal(err)
	}
	defer body.Close()
	header, err := bufio.NewReader(body).ReadString('\n')
	columns := strings.Split(strings.TrimSpace(header), ",")
	if err != nil || !hasBoth(columns, "ra", "dec") {
		t.Errorf("%q, %v", header, err)
	}
}

// hasBoth is whether a list holds two names.
func hasBoth(names []string, one, other string) bool {
	seen := strings.Join(names, " ")
	return strings.Contains(" "+seen+" ", " "+one+" ") &&
		strings.Contains(" "+seen+" ", " "+other+" ")
}
