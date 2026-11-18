package web

import (
	"io"
	"net/http"
	"net/url"
)

// rewriting is a Web whose errors are said again by a function.
type rewriting struct {
	inner   Web
	rewrite func(error) error
}

// Rewriting is a Web that gives what the inner Web gives, with each error
// put through rewrite, so an adapter can say a stop in its own words. A
// request that went well is given as it came, and rewrite is not called.
func Rewriting(inner Web, rewrite func(error) error) Web {
	return rewriting{inner: inner, rewrite: rewrite}
}

// said is an error put through the function, or nothing when there is
// none.
func said(err error, rewrite func(error) error) error {
	if err == nil {
		return nil
	}
	return rewrite(err)
}

// Get is the inner Web's Get, with its error rewritten.
func (wrapped rewriting) Get(address string) (io.ReadCloser, int64, error) {
	body, length, err := wrapped.inner.Get(address)
	return body, length, said(err, wrapped.rewrite)
}

// Head is the inner Web's Head, with its error rewritten.
func (wrapped rewriting) Head(address string) (int64, error) {
	length, err := wrapped.inner.Head(address)
	return length, said(err, wrapped.rewrite)
}

// Post is the inner Web's Post, with its error rewritten.
func (wrapped rewriting) Post(address string, form url.Values) (string,
	error) {
	where, err := wrapped.inner.Post(address, form)
	return where, said(err, wrapped.rewrite)
}

// rewritingDoer is a Doer whose errors are said again by a function.
type rewritingDoer struct {
	inner   Doer
	rewrite func(error) error
}

// RewritingDoer is a Doer that gives what the inner Doer gives, with each
// error put through rewrite. An answer that came is given as it came.
func RewritingDoer(inner Doer, rewrite func(error) error) Doer {
	return rewritingDoer{inner: inner, rewrite: rewrite}
}

// Do is the inner Doer's Do, with its error rewritten.
func (wrapped rewritingDoer) Do(request *http.Request) (*http.Response,
	error) {
	answer, err := wrapped.inner.Do(request)
	return answer, said(err, wrapped.rewrite)
}
