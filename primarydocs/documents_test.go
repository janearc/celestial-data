package primarydocs

import (
	"io"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

type fakeWeb struct {
	body   string
	length int64
	calls  int
}

func (f *fakeWeb) Get(string) (io.ReadCloser, int64, error) {
	f.calls++
	return io.NopCloser(strings.NewReader(f.body)), f.length, nil
}
func (*fakeWeb) Head(string) (int64, error) {
	panic("unexpected HEAD")
}

func (*fakeWeb) Post(string, url.Values) (string, error) {
	panic("unused")
}

func TestProductValidation(t *testing.T) {
	good := Document{"paper", "paper.pdf",
		"https://example.edu/paper.pdf", "Paper", "pdf"}
	for _, mutate := range []func(*Document){
		func(d *Document) { d.URL = "http://example.edu/paper.pdf" },
		func(d *Document) {
			d.URL = "https://secret@archive/paper.pdf"
		},
		func(d *Document) { d.Name = "../paper.pdf" },
		func(d *Document) { d.Kind = "zip" },
		func(d *Document) { d.ID = "" },
	} {
		d := good
		mutate(&d)
		if _, err := d.Product(); err == nil {
			t.Fatalf("accepted %#v", d)
		}
	}
}

func TestCheckPDFIntegrity(t *testing.T) {
	for _, body := range []string{"<html>denied</html>", "%PDF-1.7\ntruncated"} {
		p := filepath.Join(t.TempDir(), "document")
		os.WriteFile(p, []byte(body), 0600)
		if err := documentCheck("pdf").Check(p); err == nil {
			t.Fatal("accepted invalid PDF")
		}
	}
	p := filepath.Join(t.TempDir(), "document")
	os.WriteFile(p, []byte("%PDF-1.7\nsynthetic fixture\n%%EOF\n"), 0600)
	if err := documentCheck("pdf").Check(p); err != nil {
		t.Fatal(err)
	}
}

func TestCheckHTMLChallenge(t *testing.T) {
	p := filepath.Join(t.TempDir(), "document")
	os.WriteFile(p, []byte("<!doctype html><html><title>Just a "+
		"moment...</title></html>"), 0600)
	if err := documentCheck("html").Check(p); err == nil {
		t.Fatal("accepted challenge")
	}
}

func TestFetchRejectsTruncationAndCleansPart(t *testing.T) {
	d := Document{"paper", "paper.pdf",
		"https://example.edu/paper.pdf", "Paper", "pdf"}
	for _, f := range []*fakeWeb{
		{body: "%PDF-1.7\n%%EOF", length: 100},
		{body: "<html>denied</html>", length: -1},
	} {
		into := t.TempDir()
		if _, err := Fetch(f, d, into); err == nil {
			t.Fatal("accepted malformed response")
		}
		files, _ := os.ReadDir(into)
		if len(files) != 0 {
			t.Fatal("left partial product")
		}
	}
	f := &fakeWeb{body: "%PDF-1.7\nfixture\n%%EOF\n", length: -1}
	into := t.TempDir()
	if _, err := Fetch(f, d, into); err != nil {
		t.Fatal(err)
	}
	if _, err := Fetch(f, d, into); err == nil {
		t.Fatal("overwrote retained source")
	}
	if f.calls != 1 {
		t.Fatal("asked network for existing source")
	}
}
