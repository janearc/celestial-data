package main

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

// redirectPatience is how long the redirect test's client waits for
// its one answer. It's defined: far longer than a local server takes.
const redirectPatience = 10 * time.Second

// Anti-test: the client ADS is asked through follows no redirect. It
// followed every one, so a search was as many exchanges as the server
// chose, and its Authorization went wherever a redirect sent it. Both
// servers are on this machine.
func TestADSIsAskedOnceWithNoRedirectFollowed(t *testing.T) {
	reached := false
	away := httptest.NewServer(http.HandlerFunc(
		func(http.ResponseWriter, *http.Request) { reached = true }))
	t.Cleanup(away.Close)
	there := httptest.NewServer(http.RedirectHandler(away.URL,
		http.StatusFound))
	t.Cleanup(there.Close)
	request, err := http.NewRequest(http.MethodGet, there.URL, nil)
	if err != nil {
		t.Fatal(err)
	}
	request.Header.Set("Authorization", "Bearer "+adsToken)
	answer, err := adsClient(redirectPatience).Do(request)
	if err != nil {
		t.Fatal(err)
	}
	answer.Body.Close()
	if answer.StatusCode != http.StatusFound || reached {
		t.Errorf("status %d; the redirect's target reached: %v",
			answer.StatusCode, reached)
	}
}
