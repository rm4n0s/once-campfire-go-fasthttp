package httpx

import (
	"bytes"
	"context"
	"crypto/tls"
	"errors"
	"io"
	"log/slog"
	"net"
	"net/url"
	"runtime/debug"
	"strconv"
	"sync"
	"sync/atomic"
	"time"

	"github.com/valyala/fasthttp"
)

// Server serves a Handler through fasthttp.
type Server struct {
	Handler Handler
	// Raw, when set, serves requests instead of Handler. It exists for
	// net/http handlers bridged with fasthttpadaptor.
	Raw                                    fasthttp.RequestHandler
	ReadTimeout, WriteTimeout, IdleTimeout time.Duration
	// MaxRequestBody bounds a request body. Bodies larger than the read buffer
	// stream to the handler instead of being held in memory.
	MaxRequestBody int64
	// MaxHeaderBytes bounds the request line and headers.
	MaxHeaderBytes int

	once     sync.Once
	fs       *fasthttp.Server
	shutdown atomic.Bool
}

func (s *Server) init() {
	s.once.Do(func() {
		header := s.MaxHeaderBytes
		if header <= 0 {
			header = 32 << 10
		}
		limit := s.MaxRequestBody
		if limit <= 0 {
			limit = 1 << 40
		}
		s.fs = &fasthttp.Server{
			Handler:                       s.dispatch,
			NoDefaultServerHeader:         true,
			NoDefaultContentType:          true,
			ReadTimeout:                   s.ReadTimeout,
			WriteTimeout:                  s.WriteTimeout,
			IdleTimeout:                   s.IdleTimeout,
			ReadBufferSize:                header,
			WriteBufferSize:               16 << 10,
			MaxRequestBodySize:            int(min(limit, int64(1<<62))),
			StreamRequestBody:             true,
			DisablePreParseMultipartForm:  true,
			TCPKeepalive:                  true,
			CloseOnShutdown:               true,
			KeepHijackedConns:             true,
			Logger:                        slogPrinter{},
			DisableHeaderNamesNormalizing: false,
		}
	})
}

type slogPrinter struct{}

func (slogPrinter) Printf(format string, args ...any) {
	slog.Debug("fasthttp", "message", format, "args", args)
}

// Serve accepts connections from l until it is shut down.
func (s *Server) Serve(l net.Listener) error {
	s.init()
	err := s.fs.Serve(l)
	if err == nil && s.shutdown.Load() {
		return ErrServerClosed
	}
	return err
}

// ServeTLS terminates TLS with cfg (HTTP/1.1 only) on l.
func (s *Server) ServeTLS(l net.Listener, cfg *tls.Config) error {
	return s.Serve(tls.NewListener(l, cfg))
}

// Shutdown stops accepting and waits for active requests, up to ctx.
func (s *Server) Shutdown(ctx context.Context) error {
	s.init()
	s.shutdown.Store(true)
	return s.fs.ShutdownWithContext(ctx)
}

// Close stops serving without waiting.
func (s *Server) Close() error {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	s.Shutdown(ctx)
	return nil
}

// ErrServerClosed is returned by Serve after Shutdown.
var ErrServerClosed = errors.New("httpx: server closed")

var writers = sync.Pool{New: func() any { return new(writer) }}

func (s *Server) dispatch(ctx *fasthttp.RequestCtx) {
	if s.Raw != nil {
		s.Raw(ctx)
		return
	}
	s.Handle(ctx)
}

// Handle serves one fasthttp request through the Handler.
func (s *Server) Handle(ctx *fasthttp.RequestCtx) {
	r, err := newRequest(ctx)
	if err != nil {
		ctx.Error("Bad Request", fasthttp.StatusBadRequest)
		return
	}
	w := writers.Get().(*writer)
	w.reset(ctx)
	defer func() {
		if recovered := recover(); recovered != nil {
			slog.Error("http handler panic", "path", r.URL.Path, "panic", recovered, "stack", string(debug.Stack()))
			if w.status == 0 && !w.hijacked {
				ctx.Error("Internal Server Error", fasthttp.StatusInternalServerError)
			}
		}
		w.closeNow()
		w.release()
		writers.Put(w)
	}()
	s.Handler.ServeHTTP(w, r)
	w.finish()
}

func methodString(b []byte) string {
	switch string(b) {
	case "GET":
		return "GET"
	case "POST":
		return "POST"
	case "HEAD":
		return "HEAD"
	case "PUT":
		return "PUT"
	case "PATCH":
		return "PATCH"
	case "DELETE":
		return "DELETE"
	case "OPTIONS":
		return "OPTIONS"
	}
	return string(b)
}

var commonKeys = func() map[string]string {
	keys := map[string]string{}
	for _, name := range []string{
		"Accept", "Accept-Encoding", "Accept-Language", "Authorization", "Cache-Control", "Connection",
		"Content-Length", "Content-Type", "Cookie", "Forwarded", "Host", "If-Modified-Since", "If-None-Match",
		"If-Range", "Origin", "Range", "Referer", "Sec-Fetch-Dest", "Sec-Fetch-Mode", "Sec-Fetch-Site",
		"Sec-Fetch-User", "Sec-Websocket-Extensions", "Sec-Websocket-Key", "Sec-Websocket-Protocol",
		"Sec-Websocket-Version", "Turbo-Frame", "Upgrade", "Upgrade-Insecure-Requests", "User-Agent",
		"X-Csrf-Token", "X-Forwarded-For", "X-Forwarded-Host", "X-Forwarded-Port", "X-Forwarded-Proto",
		"X-Forwarded-Ssl", "X-Request-Start", "X-Requested-With",
	} {
		keys[name] = CanonicalKey(name)
	}
	return keys
}()

func newRequest(ctx *fasthttp.RequestCtx) (*Request, error) {
	header := &ctx.Request.Header
	requestURI := string(header.RequestURI())
	u, err := url.ParseRequestURI(requestURI)
	if err != nil {
		return nil, err
	}
	r := &Request{
		Method:     methodString(header.Method()),
		URL:        u,
		Proto:      "HTTP/1.1",
		ProtoMajor: 1,
		ProtoMinor: 1,
		Host:       string(ctx.Host()),
		RequestURI: requestURI,
		RemoteAddr: ctx.RemoteAddr().String(),
		Header:     new(Header),
	}
	if header.IsHTTP11() {
		r.Proto = "HTTP/1.1"
	} else {
		r.Proto, r.ProtoMinor = "HTTP/1.0", 0
	}
	if ctx.IsTLS() {
		r.TLS = ctx.TLSConnectionState()
	}
	// Header values are copied once into a single string; fields are substrings.
	var values []byte
	type span struct {
		key        string
		start, end int
	}
	var spans []span
	for key, value := range header.All() {
		name, ok := commonKeys[string(key)]
		if !ok {
			name = CanonicalKey(string(key))
		}
		start := len(values)
		values = append(values, value...)
		spans = append(spans, span{name, start, len(values)})
	}
	all := string(values)
	r.Header.fields = make([]field, 0, len(spans))
	for _, sp := range spans {
		value := all[sp.start:sp.end]
		if f := r.Header.find(sp.key); f != nil {
			f.more = append(f.more, value)
			continue
		}
		r.Header.fields = append(r.Header.fields, field{key: sp.key, value: value})
	}
	// fasthttp reports -1 for chunked bodies and -2 when a request names no length.
	switch n := header.ContentLength(); {
	case n == -1:
		r.ContentLength = -1
		r.TransferEncoding = []string{"chunked"}
	case n > 0:
		r.ContentLength = int64(n)
	}
	switch {
	case ctx.Request.IsBodyStream():
		r.Body = io.NopCloser(ctx.RequestBodyStream())
	case len(ctx.PostBody()) > 0:
		r.Body = io.NopCloser(bytes.NewReader(ctx.PostBody()))
	default:
		r.Body = noBody{}
	}
	return r, nil
}

type noBody struct{}

func (noBody) Read([]byte) (int, error) { return 0, io.EOF }
func (noBody) Close() error             { return nil }

// Bodies below this size are copied into the response buffer: cheaper than a stream.
const streamThreshold = 256 << 10

// writer is the fasthttp-backed ResponseWriter. Bodies accumulate in the
// response buffer, and fasthttp sends them with an exact Content-Length once
// the handler returns. Large files use SendReader instead of buffering.
type writer struct {
	ctx       *fasthttp.RequestCtx
	header    Header
	status    int
	head      bool
	declared  int64
	written   int64
	body      io.Writer
	hijacked  bool
	streaming bool
	closers   []io.Closer
}

func (w *writer) reset(ctx *fasthttp.RequestCtx) {
	w.ctx = ctx
	w.header.Reset()
	w.status, w.written, w.declared = 0, 0, -1
	w.head = ctx.Request.Header.IsHead()
	w.body = nil
	w.hijacked, w.streaming = false, false
	w.closers = nil
}

func (w *writer) release() { w.ctx, w.body = nil, nil }

// DeferClose closes c once the response has been sent, including any streamed body.
func (w *writer) DeferClose(c io.Closer) bool {
	w.closers = append(w.closers, c)
	return true
}

// streamBody closes files the response was reading once fasthttp finishes sending it.
type streamBody struct {
	io.Reader
	closers []io.Closer
}

func (s *streamBody) Close() error {
	for _, c := range s.closers {
		c.Close()
	}
	s.closers = nil
	return nil
}

func (w *writer) closeNow() {
	for _, c := range w.closers {
		c.Close()
	}
	w.closers = nil
}

func (w *writer) Header() *Header { return &w.header }

func (w *writer) WriteHeader(code int) {
	if w.status != 0 {
		return
	}
	if code < 200 && code != StatusSwitchingProtocols {
		return
	}
	w.status = code
	resp := &w.ctx.Response
	resp.SetStatusCode(code)
	h := &resp.Header
	w.header.Each(func(key, value string) {
		switch key {
		case "Content-Length":
			if n, err := strconv.ParseInt(value, 10, 64); err == nil && n >= 0 {
				w.declared = n
			}
		case "Content-Type":
			h.SetContentType(value)
		default:
			h.Add(key, value)
		}
	})
	if w.head && w.declared >= 0 {
		h.SetContentLength(int(w.declared))
	}
}

func (w *writer) Write(p []byte) (int, error) {
	if w.status == 0 {
		if !w.header.Has("Content-Type") && len(p) > 0 {
			w.header.Set("Content-Type", DetectContentType(p))
		}
		w.WriteHeader(StatusOK)
	}
	if w.streaming {
		return 0, errors.New("httpx: write after body stream")
	}
	w.written += int64(len(p))
	if w.head {
		return len(p), nil
	}
	if w.body == nil {
		w.body = w.ctx.Response.BodyWriter()
	}
	return w.body.Write(p)
}

// WriteString avoids a []byte conversion for string bodies.
func (w *writer) WriteString(s string) (int, error) {
	if w.status == 0 {
		return w.Write([]byte(s))
	}
	if w.streaming {
		return 0, errors.New("httpx: write after body stream")
	}
	w.written += int64(len(s))
	if w.head {
		return len(s), nil
	}
	if w.body == nil {
		w.body = w.ctx.Response.BodyWriter()
	}
	return io.WriteString(w.body, s)
}

func (w *writer) Flush() {}

// SendReader streams size bytes of src after the head is written. Files
// registered with DeferClose stay open until fasthttp has sent the stream.
func (w *writer) SendReader(src io.Reader, size int64) bool {
	if w.status == 0 || w.streaming || w.written != 0 || size < streamThreshold && !w.head {
		return false
	}
	if w.head {
		w.ctx.Response.Header.SetContentLength(int(size))
		return true
	}
	w.streaming = true
	w.ctx.Response.SetBodyStream(&streamBody{Reader: src, closers: w.closers}, int(size))
	w.closers = nil
	return true
}

// Hijack hands the connection to fn once the response head has been sent.
func (w *writer) Hijack(fn func(net.Conn)) error {
	if w.hijacked {
		return errors.New("httpx: connection already hijacked")
	}
	w.hijacked = true
	if w.status == 0 {
		w.ctx.HijackSetNoResponse(true)
	}
	w.ctx.Hijack(func(c net.Conn) {
		// fasthttp neither closes nor recycles a kept connection: fn's owner does.
		defer c.Close()
		c.SetDeadline(time.Time{})
		fn(c)
	})
	return nil
}

func (w *writer) finish() {
	if w.status == 0 && !w.hijacked {
		w.WriteHeader(StatusOK)
	}
	if w.head && w.declared < 0 && !w.streaming {
		w.ctx.Response.Header.SetContentLength(int(w.written))
	}
	w.closeNow()
}
