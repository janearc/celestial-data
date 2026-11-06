// Package web is the one way celestial-data reaches an archive: a GET, a
// HEAD and a POST. A client keeps the cookies its answers set, so a session
// signed in goes with what it asks. Everything goes through Web, so a test
// can hold an adapter to the exact addresses it asks for.
package web

import (
	"fmt"
	"io"
	"net"
	"net/http"
	"net/http/cookiejar"
	"net/url"
	"strings"
	"time"
)

// failed is the length given beside an error. It's defined: the error
// says what happened, and no bytes came.
const failed = 0

// Web is what an adapter asks of the network.
type Web interface {
	// Get opens what a URL holds. length is the number of bytes the
	// server said it would send, or -1 when it did not say.
	Get(url string) (body io.ReadCloser, length int64, err error)
	// Head is the length of what a URL holds, as its server tells it.
	// It is -1 when the server does not say.
	Head(url string) (length int64, err error)
	// Post sends a form to an address and gives the address its answer
	// sends on to, or none. It does not go on to it.
	Post(address string, form url.Values) (where string, err error)
}

// Client is Web over HTTP. Its zero value is not ready; New makes one.
type Client struct {
	http  *http.Client
	post  *http.Client
	agent string
}

// New makes a Client. The server gets wait to connect and to begin its
// answer. The whole request gets whole, up to its last byte. Zero is no
// limit. That suits a large file on a slow link.
func New(wait, whole time.Duration, agent string) Client {
	dialer := &net.Dialer{Timeout: wait}
	transport := &http.Transport{
		Proxy:                 http.ProxyFromEnvironment,
		DialContext:           dialer.DialContext,
		TLSHandshakeTimeout:   wait,
		ResponseHeaderTimeout: wait,
	}
	jar, _ := cookiejar.New(nil) // its error is always nil
	return Client{
		http: &http.Client{Transport: transport, Timeout: whole,
			Jar: jar},
		post: &http.Client{Transport: transport, Timeout: whole,
			Jar: jar, CheckRedirect: stay},
		agent: agent}
}

// stay answers a redirect by going no further, so Post can say where it
// pointed.
func stay(*http.Request, []*http.Request) error {
	return http.ErrUseLastResponse
}

// Get is Web's Get over HTTP. Any answer but a success is an error that
// names the URL and the status. Its body is closed.
func (client Client) Get(url string) (io.ReadCloser, int64, error) {
	answer, err := client.ask(http.MethodGet, url)
	if err != nil {
		return nil, failed, err
	}
	return answer.Body, answer.ContentLength, nil
}

// Head is Web's Head over HTTP.
func (client Client) Head(url string) (int64, error) {
	answer, err := client.ask(http.MethodHead, url)
	if err != nil {
		return failed, err
	}
	answer.Body.Close()
	return answer.ContentLength, nil
}

// Post is Web's Post over HTTP. An answer that is neither a success nor
// a redirect is an error that names the address and the status, and
// never the form, which may hold a password.
func (client Client) Post(address string, form url.Values) (string,
	error) {
	request, err := http.NewRequest(http.MethodPost, address,
		strings.NewReader(form.Encode()))
	if err != nil {
		return "", err
	}
	request.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	request.Header.Set("User-Agent", client.agent)
	answer, err := client.post.Do(request)
	if err != nil {
		return "", err
	}
	answer.Body.Close()
	if answer.StatusCode < http.StatusOK ||
		answer.StatusCode >= http.StatusBadRequest {
		return "", fmt.Errorf("POST %s: %s", address, answer.Status)
	}
	// the client has refused a Location it cannot read already, so the
	// one thing left to go wrong is that there is none.
	where, err := answer.Location()
	if err != nil {
		return "", nil
	}
	return where.String(), nil
}

// Cookies are the cookies the client holds for an address: those its
// answers set, and those Keep gave it.
func (client Client) Cookies(address string) ([]*http.Cookie, error) {
	where, err := url.Parse(address)
	if err != nil {
		return nil, err
	}
	return client.http.Jar.Cookies(where), nil
}

// Keep gives the client cookies to send to an address, as a session
// signed in before would be.
func (client Client) Keep(address string, cookies []*http.Cookie) error {
	where, err := url.Parse(address)
	if err != nil {
		return err
	}
	client.http.Jar.SetCookies(where, cookies)
	return nil
}

// ask sends one request and refuses an answer that is not a success.
func (client Client) ask(method, url string) (*http.Response, error) {
	request, err := http.NewRequest(method, url, nil)
	if err != nil {
		return nil, err
	}
	request.Header.Set("User-Agent", client.agent)
	answer, err := client.http.Do(request)
	if err != nil {
		return nil, err
	}
	if answer.StatusCode < http.StatusOK ||
		answer.StatusCode >= http.StatusMultipleChoices {
		answer.Body.Close()
		return nil, &Status{method: method, url: url,
			status: answer.Status, code: answer.StatusCode,
			retryAfter: answer.Header.Get("Retry-After")}
	}
	return answer, nil
}

// Status is an answer that was not a success, as Get and Head refuse
// it: the request's method and address, the status as the server wrote
// it and its code, and its Retry-After header as written, or none. A
// caller that must stop when a server asks it to wait reads Code.
type Status struct {
	method, url, status string
	code                int
	retryAfter          string
}

// Error is the method, the address and the status.
func (status *Status) Error() string {
	return fmt.Sprintf("%s %s: %s", status.method, status.url,
		status.status)
}

// Code is the status's code, as 429 for Too Many Requests.
func (status *Status) Code() int { return status.code }

// RetryAfter is how long the server asked us to wait, as its Retry-After
// header wrote it: seconds or a date. It is empty when it asked none.
func (status *Status) RetryAfter() string { return status.retryAfter }

// Seconds is a count of seconds as a wait New takes. The waits celestial-data
// picks are kept in numbers files as seconds.
func Seconds(count float64) time.Duration {
	return time.Duration(count * float64(time.Second))
}
