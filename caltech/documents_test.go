package caltech

import (
	"io"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/janearc/celestial-data/mocks"
	"github.com/janearc/celestial-data/primarydocs"
	"go.uber.org/mock/gomock"
)

func TestCheckCaltechDocuments(t *testing.T) {
	good := primarydocs.Document{ID: "solar", Name: "solar.html", URL: "https://www.spitzer.caltech.edu/mission/the-solar-panel-assembly", Title: "Solar panel", Kind: "html"}
	if err := Check(good); err != nil {
		t.Fatal(err)
	}
	credential := url.URL{Scheme: "https", Host: "caltech.edu", Path: "/p", User: url.User("example")}
	for _, bad := range []string{"https://caltech.edu.attacker.invalid/p", "https://attacker.invalid/p", "http://caltech.edu/p", credential.String(), "https://caltech.edu/p#part"} {
		document := good
		document.URL = bad
		if err := Check(document); err == nil {
			t.Errorf("accepted %s", bad)
		}
	}
	good.Name = "../solar.html"
	if err := Check(good); err == nil {
		t.Fatal("accepted unsafe filename")
	}
}

func TestFetchKeepsExactBytesAndDoesNotOverwrite(t *testing.T) {
	get := mocks.NewMockWeb(gomock.NewController(t))
	document := primarydocs.Document{ID: "solar", Name: "solar.html", URL: "https://www.spitzer.caltech.edu/mission/the-solar-panel-assembly", Title: "Solar panel", Kind: "html"}
	body := "<!doctype html><html><meta name=\"csrf-token\" content=\"fixture\"><p>392 solar cells</p></html>"
	get.EXPECT().Get(document.URL).Return(io.NopCloser(strings.NewReader(body)), int64(len(body)), nil)
	dir := t.TempDir()
	path, err := Fetch(get, document, dir)
	if err != nil {
		t.Fatal(err)
	}
	got, err := os.ReadFile(path)
	if err != nil || string(got) != body {
		t.Fatalf("got %q, %v", got, err)
	}
	if _, err := Fetch(get, document, dir); err == nil {
		t.Fatal("overwrote document")
	}
	if path != filepath.Join(dir, document.Name) {
		t.Fatalf("unexpected path %s", path)
	}
}

func TestFetchRejectsForeignHostWithoutRequest(t *testing.T) {
	get := mocks.NewMockWeb(gomock.NewController(t))
	document := primarydocs.Document{ID: "solar", Name: "solar.html", URL: "https://example.org/solar", Title: "Solar panel", Kind: "html"}
	if _, err := Fetch(get, document, t.TempDir()); err == nil {
		t.Fatal("accepted foreign host")
	}
}
