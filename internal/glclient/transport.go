package glclient

import (
	"errors"
	"io"
	"net/http"
)

// ErrBodyTooLarge is returned when a response body exceeds the configured cap.
var ErrBodyTooLarge = errors.New("response body too large")

// capBody fails reads once more than left bytes have been delivered.
type capBody struct {
	io.ReadCloser
	left int64
}

func (c *capBody) Read(p []byte) (int, error) {
	if c.left <= 0 {
		return 0, ErrBodyTooLarge
	}
	if int64(len(p)) > c.left {
		p = p[:c.left]
	}
	n, err := c.ReadCloser.Read(p)
	c.left -= int64(n)
	return n, err
}

// capRT wraps a RoundTripper so every response body is bounded by max bytes.
type capRT struct {
	rt  http.RoundTripper
	max int64
}

func (r capRT) RoundTrip(req *http.Request) (*http.Response, error) {
	resp, err := r.rt.RoundTrip(req)
	if err != nil {
		return resp, err
	}
	resp.Body = &capBody{ReadCloser: resp.Body, left: r.max}
	return resp, nil
}
