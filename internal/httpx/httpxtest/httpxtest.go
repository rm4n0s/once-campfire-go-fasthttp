// Package httpxtest provides in-memory requests and recorders for httpx handlers,
// and a loopback fasthttp server for tests that need real connections.
package httpxtest

import (
	"bytes"
	"context"
	"io"
	"net"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/basecamp/once-campfire-go/internal/httpx"
)

// NewRequest builds a server request the way net/http/httptest does.
func NewRequest(method, target string, body io.Reader) *httpx.Request {
	if method == "" {
		method = "GET"
	}
	u, err := url.ParseRequestURI(target)
	if err != nil {
		panic("httpxtest: invalid target " + target + ": " + err.Error())
	}
	r := &httpx.Request{
		Method:     strings.ToUpper(method),
		URL:        u,
		Proto:      "HTTP/1.1",
		ProtoMajor: 1,
		ProtoMinor: 1,
		Header:     new(httpx.Header),
		Host:       "example.com",
		RemoteAddr: "192.0.2.1:1234",
		RequestURI: target,
		Body:       io.NopCloser(bytes.NewReader(nil)),
	}
	if u.Host != "" {
		r.Host = u.Host
	}
	if body != nil {
		switch v := body.(type) {
		case *bytes.Buffer:
			r.ContentLength = int64(v.Len())
		case *bytes.Reader:
			r.ContentLength = int64(v.Len())
		case *strings.Reader:
			r.ContentLength = int64(v.Len())
		default:
			r.ContentLength = -1
		}
		if rc, ok := body.(io.ReadCloser); ok {
			r.Body = rc
		} else {
			r.Body = io.NopCloser(body)
		}
	}
	return r
}

// Recorder collects a response.
type Recorder struct {
	Code        int
	Body        *bytes.Buffer
	Flushed     bool
	header      *httpx.Header
	snapshot    *httpx.Header
	wroteHeader bool
}

func NewRecorder() *Recorder {
	return &Recorder{Code: 200, Body: new(bytes.Buffer), header: new(httpx.Header)}
}

func (r *Recorder) Header() *httpx.Header { return r.header }

func (r *Recorder) WriteHeader(code int) {
	if r.wroteHeader {
		return
	}
	if code < 100 || code > 999 {
		panic("invalid WriteHeader code")
	}
	r.Code, r.wroteHeader = code, true
	r.snapshot = r.header.Clone()
}

func (r *Recorder) Write(p []byte) (int, error) {
	if !r.wroteHeader {
		if !r.header.Has("Content-Type") && len(p) > 0 {
			r.header.Set("Content-Type", httpx.DetectContentType(p))
		}
		r.WriteHeader(200)
	}
	return r.Body.Write(p)
}

func (r *Recorder) WriteString(s string) (int, error) { return r.Write([]byte(s)) }
func (r *Recorder) Flush() {
	if !r.wroteHeader {
		r.WriteHeader(200)
	}
	r.Flushed = true
}

// Result returns the headers as sent, which later header edits do not affect.
func (r *Recorder) Result() *Result {
	if !r.wroteHeader {
		r.WriteHeader(200)
	}
	return &Result{StatusCode: r.Code, Header: r.snapshot, Body: io.NopCloser(bytes.NewReader(r.Body.Bytes()))}
}

type Result struct {
	StatusCode int
	Header     *httpx.Header
	Body       io.ReadCloser
}

// Serve runs h against a request and returns the recording.
func Serve(h httpx.Handler, r *httpx.Request) *Recorder {
	w := NewRecorder()
	h.ServeHTTP(w, r)
	return w
}

// Server is a loopback fasthttp server.
type Server struct {
	URL      string
	Addr     string
	listener net.Listener
	server   *httpx.Server
	done     chan struct{}
	once     sync.Once
	client   *http.Client
}

// NewServer starts h on a loopback port; Close stops it.
func NewServer(h httpx.Handler) *Server {
	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		panic(err)
	}
	s := &Server{URL: "http://" + l.Addr().String(), Addr: l.Addr().String(), listener: l, server: &httpx.Server{Handler: h}, done: make(chan struct{})}
	go func() { s.server.Serve(opaqueListener{l}); close(s.done) }()
	return s
}

// opaqueListener hides *net.TCPConn from fasthttp. For large responses fasthttp
// uses writev on a TCP connection, a syscall the race detector does not treat as
// a synchronization edge, so tests that read handler state after a response
// would report races that the network ordering rules out. Without the concrete
// type fasthttp writes through syscall.Write, which the detector understands.
type opaqueListener struct{ net.Listener }

func (l opaqueListener) Accept() (net.Conn, error) {
	c, err := l.Listener.Accept()
	if err != nil {
		return nil, err
	}
	return struct{ net.Conn }{c}, nil
}

// Client returns an HTTP client for the server. Tests drive the server through
// real connections with net/http's client.
func (s *Server) Client() *http.Client {
	s.once.Do(func() { s.client = &http.Client{Transport: &http.Transport{}} })
	return s.client
}

func (s *Server) Close() {
	if s.client != nil {
		s.client.CloseIdleConnections()
	}
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	s.server.Shutdown(ctx)
	s.listener.Close()
	<-s.done
}

// Cleanup closes the server when the test ends.
func (s *Server) Cleanup(t testing.TB) *Server { t.Cleanup(s.Close); return s }
