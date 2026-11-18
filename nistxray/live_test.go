//go:build live

package nistxray_test

import (
	"path/filepath"
	"testing"

	"github.com/janearc/celestial-data/internal/replay"
	"github.com/janearc/celestial-data/nistxray"
)

// The live check asks the archive itself. It runs only with go test
// -tags live, since it needs the network. It asks each of the 144
// pages for its length with a HEAD, never for the page: the pages'
// bytes are Cloudflare's to rewrite, and what they are held to is the
// sum of their content, which only a GET would bring.
func TestLiveEveryPageIsStillThere(t *testing.T) {
	get := replay.Live(t)
	archive, err := nistxray.New()
	if err != nil {
		t.Fatal(err)
	}
	found, err := archive.Find(replay.Asked(t))
	if err != nil || len(found) != pages {
		t.Fatalf("%d products, %v", len(found), err)
	}
	for _, product := range found {
		length, err := get.Head(product.URL())
		if err != nil {
			t.Errorf("%s: %v", product.URL(), err)
			continue
		}
		// NIST says no length for a HEAD; when it does say one, the
		// page it would serve is the length the manifest measured,
		// Cloudflare's injected line included, for the line is of a
		// fixed width.
		if length > 0 && length != bytesOf(t, product.Name()) {
			t.Errorf("%s: the server says %d bytes, the manifest said %d",
				product.URL(), length, bytesOf(t, product.Name()))
		}
	}
}

// bytesOf is the length the manifest measured a page, by the name the
// archive keeps it by.
func bytesOf(t *testing.T, name string) int64 {
	t.Helper()
	for _, each := range pinned(t) {
		if name == filepath.Base(each.path) {
			return each.bytes
		}
	}
	t.Fatalf("no page named %s", name)
	return 0
}
