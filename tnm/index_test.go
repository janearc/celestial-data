package tnm

import (
	"net/url"
	"os"
	"path/filepath"
	"testing"
)

func TestHawaiiIndexKeepsQueryAndRejectsBrokenReplies(t *testing.T) {
	product, err := HawaiiIndex()
	if err != nil {
		t.Fatal(err)
	}
	u, err := url.Parse(product.URL())
	if err != nil {
		t.Fatal(err)
	}
	if u.Scheme != "https" || u.Host != "tnmaccess.nationalmap.gov" ||
		u.Query().Get("bbox") != "-156.2,18.8,-154.7,20.4" {
		t.Fatal("unexpected archive query", u)
	}
	for _, raw := range []string{"<html>error</html>", "{", "{}"} {
		p := filepath.Join(t.TempDir(), "reply.json")
		if err := os.WriteFile(p, []byte(raw), 0600); err != nil {
			t.Fatal(err)
		}
		if err := (indexCheck{}).Check(p); err == nil {
			t.Fatal("invalid reply accepted", raw)
		}
	}
	p := filepath.Join(t.TempDir(), "reply.json")
	if err := os.WriteFile(p, []byte("{\"items\":[]}"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := (indexCheck{}).Check(p); err != nil {
		t.Fatal(err)
	}
}
