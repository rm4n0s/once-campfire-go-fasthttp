package httpx

import (
	"io"
	"mime"
	"net/http"
	"path/filepath"
	"strconv"
	"time"
)

// ServeContent replies with content, honoring Range and conditional requests
// exactly as net/http.ServeContent does. Plain full-body GETs, which dominate,
// skip the generic machinery.
func ServeContent(w ResponseWriter, r *Request, name string, modtime time.Time, content io.ReadSeeker) {
	if modtime.IsZero() && !conditional(r.Header) {
		if size, err := content.Seek(0, io.SeekEnd); err == nil {
			if _, err = content.Seek(0, io.SeekStart); err == nil {
				serveFull(w, r, name, size, content)
				return
			}
		}
	}
	serveStandard(w, r, name, modtime, content)
}

func conditional(h *Header) bool {
	return h.Has("Range") || h.Has("If-Match") || h.Has("If-None-Match") || h.Has("If-Modified-Since") ||
		h.Has("If-Unmodified-Since") || h.Has("If-Range")
}

func serveFull(w ResponseWriter, r *Request, name string, size int64, content io.ReadSeeker) {
	h := w.Header()
	if !h.Has("Content-Type") {
		ctype := mime.TypeByExtension(filepath.Ext(name))
		if ctype == "" {
			var sniff [512]byte
			n, _ := io.ReadFull(content, sniff[:])
			ctype = DetectContentType(sniff[:n])
			content.Seek(0, io.SeekStart)
		}
		h.Set("Content-Type", ctype)
	}
	h.Set("Accept-Ranges", "bytes")
	if h.Get("Content-Encoding") == "" {
		h.Set("Content-Length", strconv.FormatInt(size, 10))
	}
	w.WriteHeader(StatusOK)
	if r.Method != "HEAD" {
		CopyN(w, content, size)
	}
}

// serveStandard runs the standard library's implementation over a small adapter.
func serveStandard(w ResponseWriter, r *Request, name string, modtime time.Time, content io.ReadSeeker) {
	request := &http.Request{Method: r.Method, Header: make(http.Header, 2), URL: r.URL}
	for _, key := range []string{"Range", "If-Match", "If-None-Match", "If-Modified-Since", "If-Unmodified-Since", "If-Range"} {
		if values := r.Header.Values(key); values != nil {
			request.Header[key] = values
		}
	}
	adapter := &standardWriter{w: w, header: make(http.Header, w.Header().Len()+4)}
	w.Header().Each(func(key, value string) { adapter.header.Add(key, value) })
	http.ServeContent(adapter, request, name, modtime, content)
}

type standardWriter struct {
	w       ResponseWriter
	header  http.Header
	started bool
}

func (s *standardWriter) Header() http.Header { return s.header }

func (s *standardWriter) WriteHeader(code int) {
	if s.started {
		return
	}
	s.started = true
	dest := s.w.Header()
	dest.Reset()
	for key, values := range s.header {
		dest.SetValues(key, values)
	}
	s.w.WriteHeader(code)
}

func (s *standardWriter) Write(p []byte) (int, error) {
	if !s.started {
		s.WriteHeader(StatusOK)
	}
	return s.w.Write(p)
}

// ReadFrom lets io.CopyN reach the stream-capable writer.
func (s *standardWriter) ReadFrom(src io.Reader) (int64, error) {
	if !s.started {
		s.WriteHeader(StatusOK)
	}
	if limited, ok := src.(*io.LimitedReader); ok && !isPipe(limited.R) {
		n := limited.N
		if err := CopyN(s.w, limited.R, n); err != nil {
			return 0, err
		}
		limited.N = 0
		return n, nil
	}
	return io.Copy(struct{ io.Writer }{s.w}, src)
}

// A pipe is fed by a goroutine that dies with ServeContent, so it cannot outlive the handler.
func isPipe(r io.Reader) bool {
	_, ok := r.(*io.PipeReader)
	return ok
}
