// Package httpx is the HTTP server layer: a net/http-shaped request and
// response API running on fasthttp. Handlers, middleware and tests keep the
// familiar ResponseWriter/Request composition, while connections, parsing and
// serialization belong to fasthttp.
//
// Only value types and pure helpers (Cookie, status text, time formats,
// content sniffing, ServeContent's range logic) come from net/http; no
// net/http server or connection runs here.
package httpx

import (
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"path"
	"strings"
	"time"
)

// Re-exported value types and pure helpers.
type (
	Cookie         = http.Cookie
	MaxBytesError  = http.MaxBytesError
	SameSite       = http.SameSite
	ConnectionFunc func(net.Conn)
)

const (
	SameSiteLaxMode = http.SameSiteLaxMode
	TimeFormat      = http.TimeFormat

	StatusOK                           = http.StatusOK
	StatusFound                        = http.StatusFound
	StatusNotModified                  = http.StatusNotModified
	StatusNotFound                     = http.StatusNotFound
	StatusSwitchingProtocols           = http.StatusSwitchingProtocols
	StatusRequestedRangeNotSatisfiable = http.StatusRequestedRangeNotSatisfiable
)

func StatusText(code int) string               { return http.StatusText(code) }
func ParseTime(text string) (time.Time, error) { return http.ParseTime(text) }
func DetectContentType(data []byte) string     { return http.DetectContentType(data) }

// MaxBytesReader limits how much of body may be read. Exceeding it returns a *MaxBytesError.
func MaxBytesReader(body io.ReadCloser, n int64) io.ReadCloser {
	return http.MaxBytesReader(nil, body, n)
}

// ResponseWriter assembles a response. As in net/http, headers are mutable
// until WriteHeader or the first Write.
type ResponseWriter interface {
	Header() *Header
	Write([]byte) (int, error)
	WriteHeader(status int)
}

// Handler responds to a request.
type Handler interface {
	ServeHTTP(w ResponseWriter, r *Request)
}

// HandlerFunc adapts a function to a Handler.
type HandlerFunc func(w ResponseWriter, r *Request)

func (f HandlerFunc) ServeHTTP(w ResponseWriter, r *Request) { f(w, r) }

// Wrapper is implemented by ResponseWriters that decorate another.
type Wrapper interface{ Unwrap() ResponseWriter }

// Flusher is implemented by writers that can push buffered data.
type Flusher interface{ Flush() }

// Hijacker is implemented by writers that can hand the connection to fn after
// the response head is written. The handler must return after hijacking; fn
// then owns the connection and runs on its own goroutine.
type Hijacker interface {
	Hijack(fn func(net.Conn)) error
}

// ReaderSender is implemented by writers that can stream a reader as the
// whole body without buffering it. It reports whether it took the stream; if
// so, it also owns closing src when src is an io.Closer.
type ReaderSender interface {
	SendReader(src io.Reader, size int64) bool
}

// Flush flushes the nearest writer in the wrapper chain that can.
func Flush(w ResponseWriter) {
	for {
		if f, ok := w.(Flusher); ok {
			f.Flush()
			return
		}
		u, ok := w.(Wrapper)
		if !ok {
			return
		}
		w = u.Unwrap()
	}
}

// Hijack takes over the connection through the nearest hijackable writer.
func Hijack(w ResponseWriter, fn func(net.Conn)) error {
	for {
		if h, ok := w.(Hijacker); ok {
			return h.Hijack(fn)
		}
		u, ok := w.(Wrapper)
		if !ok {
			return fmt.Errorf("httpx: response writer cannot be hijacked")
		}
		w = u.Unwrap()
	}
}

// CopyN writes n bytes of src as the response body. A writer that can stream
// (the server's own) sends the reader directly after the handler returns, so
// sources must stay open until then: register them with DeferClose.
// Compressors and recorders receive the bytes through Write instead.
func CopyN(w ResponseWriter, src io.Reader, n int64) error {
	for {
		if s, ok := w.(ReaderSender); ok {
			if s.SendReader(src, n) {
				return nil
			}
			break
		}
		u, ok := w.(Wrapper)
		if !ok || !passesThrough(w) {
			break
		}
		w = u.Unwrap()
	}
	_, err := io.CopyN(w, src, n)
	return err
}

// PassThrough is implemented by wrappers that forward bytes unchanged, so a
// stream may bypass them.
type PassThrough interface{ PassesThrough() bool }

func passesThrough(w ResponseWriter) bool {
	p, ok := w.(PassThrough)
	return ok && p.PassesThrough()
}

// DeferClose arranges for c to be closed after the response is fully sent. It
// reports false when the writer cannot defer, in which case the caller closes c.
func DeferClose(w ResponseWriter, c io.Closer) bool {
	for {
		if d, ok := w.(interface{ DeferClose(io.Closer) bool }); ok {
			return d.DeferClose(c)
		}
		u, ok := w.(Wrapper)
		if !ok {
			return false
		}
		w = u.Unwrap()
	}
}

// Error replies with a plain-text error, as net/http.Error does.
func Error(w ResponseWriter, message string, code int) {
	h := w.Header()
	h.Del("Content-Length")
	h.Set("Content-Type", "text/plain; charset=utf-8")
	h.Set("X-Content-Type-Options", "nosniff")
	w.WriteHeader(code)
	io.WriteString(w, message+"\n")
}

func NotFound(w ResponseWriter, r *Request) { Error(w, "404 page not found", StatusNotFound) }

// SetCookie adds a Set-Cookie header.
func SetCookie(w ResponseWriter, cookie *Cookie) {
	if v := cookie.String(); v != "" {
		w.Header().Add("Set-Cookie", v)
	}
}

// Redirect replies with a redirect to url, resolving relative references.
func Redirect(w ResponseWriter, r *Request, target string, code int) {
	if u, err := url.Parse(target); err == nil && u.Scheme == "" && u.Host == "" {
		oldpath := r.URL.EscapedPath()
		if oldpath == "" {
			oldpath = "/"
		}
		if target == "" || target[0] != '/' {
			olddir, _ := path.Split(oldpath)
			target = olddir + target
		}
		var query string
		if i := strings.Index(target, "?"); i != -1 {
			target, query = target[:i], target[i:]
		}
		trailing := strings.HasSuffix(target, "/")
		target = path.Clean(target)
		if trailing && !strings.HasSuffix(target, "/") {
			target += "/"
		}
		target += query
	}
	h := w.Header()
	hadContentType := h.Has("Content-Type")
	h.Set("Location", hexEscapeNonASCII(target))
	if !hadContentType && (r.Method == "GET" || r.Method == "HEAD") {
		h.Set("Content-Type", "text/html; charset=utf-8")
	}
	w.WriteHeader(code)
	if !hadContentType && r.Method == "GET" {
		io.WriteString(w, "<a href=\""+htmlEscape(target)+"\">"+StatusText(code)+"</a>.\n\n")
	}
}

var htmlReplacer = strings.NewReplacer("&", "&amp;", "<", "&lt;", ">", "&gt;", `"`, "&#34;", "'", "&#39;")

func htmlEscape(s string) string { return htmlReplacer.Replace(s) }

func hexEscapeNonASCII(s string) string {
	newLen := 0
	for i := 0; i < len(s); i++ {
		if s[i] >= 0x80 {
			newLen += 3
		} else {
			newLen++
		}
	}
	if newLen == len(s) {
		return s
	}
	b := make([]byte, 0, newLen)
	for i := 0; i < len(s); i++ {
		if s[i] >= 0x80 {
			b = append(b, '%')
			b = append(b, "0123456789ABCDEF"[s[i]>>4], "0123456789ABCDEF"[s[i]&15])
		} else {
			b = append(b, s[i])
		}
	}
	return string(b)
}
