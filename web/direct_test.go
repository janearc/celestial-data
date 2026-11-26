package web_test

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/janearc/celestial-data/web"
)

func TestDirectRefusesRedirectWithoutAskingItsDestination(t *testing.T) {
	called := false
	target := httptest.NewServer(http.HandlerFunc(func(answer http.ResponseWriter, request *http.Request) { called = true }))
	defer target.Close()
	source := httptest.NewServer(http.HandlerFunc(func(answer http.ResponseWriter, request *http.Request) {
		http.Redirect(answer, request, target.URL, http.StatusFound)
	}))
	defer source.Close()
	client := web.Direct(time.Second, time.Second, "test")
	if body, _, err := client.Get(source.URL); err == nil {
		body.Close()
		t.Fatal("accepted redirect")
	}
	if called {
		t.Fatal("asked redirect destination")
	}
}
