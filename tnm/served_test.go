package tnm

import (
	"io"
	"strings"
	"testing"

	celestialdata "github.com/janearc/celestial-data"
	"github.com/janearc/celestial-data/mocks"
	"go.uber.org/mock/gomock"
)

func TestServedTerrainTransferStillChecksObservedLength(t *testing.T) {
	archive, err := celestialdata.NewFixed("usgs-tnm",
		celestialdata.NewFile("test", "tile.tif", "https://prd-tnm.s3.amazonaws.com/tile.tif", 3, "fixture"))
	if err != nil {
		t.Fatal(err)
	}
	products, err := archive.Find(nil)
	if err != nil {
		t.Fatal(err)
	}
	get := mocks.NewMockWeb(gomock.NewController(t))
	get.EXPECT().Get(products[0].URL()).DoAndReturn(
		func(string) (io.ReadCloser, int64, error) {
			return io.NopCloser(strings.NewReader("tile")), int64(4), nil
		}).Times(2)
	observed, err := ServedProduct(products[0], 4)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := celestialdata.Fetch(get, observed, t.TempDir()); err != nil {
		t.Fatal(err)
	}
	wrong, err := ServedProduct(products[0], 5)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := celestialdata.Fetch(get, wrong, t.TempDir()); err == nil {
		t.Fatal("GET differing from observed transfer size accepted")
	}
}
