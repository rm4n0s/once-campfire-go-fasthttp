package httpx

import (
	"context"
	"crypto/tls"
	"errors"
	"io"
	"mime"
	"mime/multipart"
	"net/url"
	"strings"
)

var (
	ErrNoCookie    = errors.New("http: named cookie not present")
	ErrMissingFile = errors.New("http: no such file")
)

// Request is a server request. It carries the fields of net/http.Request the
// application reads, built from a fasthttp request without sharing its buffers.
type Request struct {
	Method           string
	URL              *url.URL
	Proto            string
	ProtoMajor       int
	ProtoMinor       int
	Header           *Header
	Body             io.ReadCloser
	ContentLength    int64
	TransferEncoding []string
	Host             string
	Form             url.Values
	PostForm         url.Values
	MultipartForm    *multipart.Form
	RemoteAddr       string
	RequestURI       string
	TLS              *tls.ConnectionState

	ctx        context.Context
	pathValues []pathValue
}

type pathValue struct{ name, value string }

// Context returns the request context; it is never nil.
func (r *Request) Context() context.Context {
	if r.ctx != nil {
		return r.ctx
	}
	return context.Background()
}

// WithContext returns a shallow copy using ctx.
func (r *Request) WithContext(ctx context.Context) *Request {
	if ctx == nil {
		panic("nil context")
	}
	c := new(Request)
	*c = *r
	c.ctx = ctx
	return c
}

// Clone returns a deep copy of the headers, URL and path values.
func (r *Request) Clone(ctx context.Context) *Request {
	c := r.WithContext(ctx)
	c.Header = r.Header.Clone()
	if r.URL != nil {
		u := *r.URL
		c.URL = &u
	}
	c.pathValues = append([]pathValue(nil), r.pathValues...)
	return c
}

func (r *Request) PathValue(name string) string {
	for _, p := range r.pathValues {
		if p.name == name {
			return p.value
		}
	}
	return ""
}

func (r *Request) SetPathValue(name, value string) {
	for i := range r.pathValues {
		if r.pathValues[i].name == name {
			r.pathValues[i].value = value
			return
		}
	}
	r.pathValues = append(r.pathValues, pathValue{name, value})
}

func (r *Request) UserAgent() string { return r.Header.Get("User-Agent") }

func (r *Request) ProtoAtLeast(major, minor int) bool {
	return r.ProtoMajor > major || r.ProtoMajor == major && r.ProtoMinor >= minor
}

// Cookie returns the first cookie named name, tolerating malformed siblings the
// way net/http does.
func (r *Request) Cookie(name string) (*Cookie, error) {
	if name == "" {
		return nil, ErrNoCookie
	}
	var found *Cookie
	scanCookies(r.Header, name, func(c *Cookie) bool { found = c; return false })
	if found == nil {
		return nil, ErrNoCookie
	}
	return found, nil
}

// Cookies returns every well-formed cookie in the request.
func (r *Request) Cookies() []*Cookie {
	var cookies []*Cookie
	scanCookies(r.Header, "", func(c *Cookie) bool { cookies = append(cookies, c); return true })
	return cookies
}

// AddCookie appends a name=value pair to the Cookie header.
func (r *Request) AddCookie(c *Cookie) {
	pair := c.Name + "=" + c.Value
	if old := r.Header.Get("Cookie"); old != "" {
		r.Header.Set("Cookie", old+"; "+pair)
	} else {
		r.Header.Set("Cookie", pair)
	}
}

// ParseForm populates PostForm from an urlencoded body and Form from both that
// and the query. Multipart bodies are parsed by the caller.
func (r *Request) ParseForm() error {
	var err error
	if r.PostForm == nil {
		if r.Method == "POST" || r.Method == "PUT" || r.Method == "PATCH" {
			r.PostForm, err = parsePostForm(r)
		}
		if r.PostForm == nil {
			r.PostForm = make(url.Values)
		}
	}
	if r.Form == nil {
		r.Form = make(url.Values, len(r.PostForm))
		for key, values := range r.PostForm {
			r.Form[key] = append(r.Form[key], values...)
		}
		if r.URL != nil {
			query, e := url.ParseQuery(r.URL.RawQuery)
			if err == nil {
				err = e
			}
			for key, values := range query {
				r.Form[key] = append(r.Form[key], values...)
			}
		}
	}
	return err
}

func parsePostForm(r *Request) (url.Values, error) {
	if r.Body == nil {
		return nil, errors.New("missing form body")
	}
	kind, _, err := mime.ParseMediaType(r.Header.Get("Content-Type"))
	if err != nil && r.Header.Get("Content-Type") != "" {
		return nil, err
	}
	if kind != "application/x-www-form-urlencoded" {
		return nil, nil
	}
	raw, err := io.ReadAll(io.LimitReader(r.Body, 10<<20+1))
	if err != nil {
		return nil, err
	}
	if len(raw) > 10<<20 {
		return nil, errors.New("http: POST too large")
	}
	return url.ParseQuery(string(raw))
}

// FormValue returns the first value for key from Form, parsing it on demand.
func (r *Request) FormValue(key string) string {
	if r.Form == nil {
		r.ParseForm()
	}
	return r.Form.Get(key)
}

// FormFile returns the last uploaded file for a field.
func (r *Request) FormFile(key string) (multipart.File, *multipart.FileHeader, error) {
	if r.MultipartForm == nil {
		return nil, nil, ErrMissingFile
	}
	files := r.MultipartForm.File[key]
	if len(files) == 0 {
		return nil, nil, ErrMissingFile
	}
	file, err := files[len(files)-1].Open()
	return file, files[len(files)-1], err
}

// scanCookies visits well-formed name=value pairs in Cookie headers. Pairs with
// invalid names or values are skipped, as net/http skips them.
func scanCookies(h *Header, name string, visit func(*Cookie) bool) {
	f := h.find("Cookie")
	if f == nil {
		return
	}
	lines := append([]string{f.value}, f.more...)
	for _, line := range lines {
		line = strings.TrimSpace(line)
		for len(line) > 0 {
			var part string
			part, line, _ = strings.Cut(line, ";")
			part = strings.TrimSpace(part)
			if part == "" {
				continue
			}
			key, value, _ := strings.Cut(part, "=")
			key = strings.TrimSpace(key)
			if !validCookieName(key) || name != "" && name != key {
				continue
			}
			quoted := false
			if len(value) > 1 && value[0] == '"' && value[len(value)-1] == '"' {
				value, quoted = value[1:len(value)-1], true
			}
			if !validCookieValue(value) {
				continue
			}
			if !visit(&Cookie{Name: key, Value: value, Quoted: quoted}) {
				return
			}
		}
	}
}

func validCookieValue(value string) bool {
	for i := 0; i < len(value); i++ {
		if b := value[i]; b < 0x20 || b >= 0x7f || b == '"' || b == ';' || b == '\\' {
			return false
		}
	}
	return true
}

func validCookieName(name string) bool {
	if name == "" {
		return false
	}
	for i := 0; i < len(name); i++ {
		b := name[i]
		switch {
		case b >= 'a' && b <= 'z', b >= 'A' && b <= 'Z', b >= '0' && b <= '9':
		case strings.IndexByte("!#$%&'*+-.^_`|~", b) >= 0:
		default:
			return false
		}
	}
	return true
}
