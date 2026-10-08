package httpx_test

import (
	"bytes"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/basecamp/once-campfire-go/internal/httpx"
	"github.com/basecamp/once-campfire-go/internal/httpx/httpxtest"
)

func TestHeaderCanonicalizesAndKeepsOrder(t *testing.T) {
	var h httpx.Header
	h.Set("content-type", "text/plain")
	h.Add("set-cookie", "a=1")
	h.Add("Set-Cookie", "b=2")
	h.Set("X-One", "1")
	if h.Get("CONTENT-TYPE") != "text/plain" || len(h.Values("Set-Cookie")) != 2 || h.Len() != 3 {
		t.Fatal("canonical lookup failed")
	}
	var seen []string
	h.Each(func(k, v string) { seen = append(seen, k+"="+v) })
	if strings.Join(seen, ",") != "Content-Type=text/plain,Set-Cookie=a=1,Set-Cookie=b=2,X-One=1" {
		t.Fatal(seen)
	}
	h.Set("Set-Cookie", "c=3")
	h.Del("x-one")
	if values := h.Values("Set-Cookie"); len(values) != 1 || values[0] != "c=3" || h.Has("X-One") {
		t.Fatal(values)
	}
	clone := h.Clone()
	clone.Set("Content-Type", "x")
	if h.Get("Content-Type") != "text/plain" {
		t.Fatal("clone shares storage")
	}
	var nilHeader *httpx.Header
	if nilHeader.Get("X") != "" || nilHeader.Len() != 0 {
		t.Fatal("nil header must read as empty")
	}
}

func TestCookiesSkipMalformedPairs(t *testing.T) {
	r := httpxtest.NewRequest("GET", "/", nil)
	r.Header.Set("Cookie", `a=1; bad name=2; b="quoted"; c=x\y; d=4`)
	r.Header.Add("Cookie", "e=5")
	got := map[string]string{}
	for _, c := range r.Cookies() {
		got[c.Name] = c.Value
	}
	if len(got) != 4 || got["a"] != "1" || got["b"] != "quoted" || got["d"] != "4" || got["e"] != "5" {
		t.Fatal(got)
	}
	if _, err := r.Cookie("c"); err != httpx.ErrNoCookie {
		t.Fatal("invalid value was accepted", err)
	}
}

func TestParseFormCombinesBodyAndQuery(t *testing.T) {
	r := httpxtest.NewRequest("POST", "/x?q=1&both=query", strings.NewReader("a=2&both=body"))
	r.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	if err := r.ParseForm(); err != nil {
		t.Fatal(err)
	}
	if r.PostForm.Get("a") != "2" || r.Form.Get("q") != "1" || strings.Join(r.Form["both"], ",") != "body,query" {
		t.Fatal(r.Form)
	}
}

func serve(t *testing.T, handler httpx.HandlerFunc) *httpxtest.Server {
	t.Helper()
	return httpxtest.NewServer(handler).Cleanup(t)
}

func TestServerResponses(t *testing.T) {
	server := serve(t, func(w httpx.ResponseWriter, r *httpx.Request) {
		switch r.URL.Path {
		case "/cookies":
			httpx.SetCookie(w, &httpx.Cookie{Name: "a", Value: "1", Path: "/"})
			httpx.SetCookie(w, &httpx.Cookie{Name: "b", Value: "2", Path: "/", HttpOnly: true})
			w.Header().Set("X-Multi", "one")
			w.Header().Add("X-Multi", "two")
			io.WriteString(w, "hello")
		case "/sniff":
			w.Write([]byte("<html><body>x</body></html>"))
		case "/empty":
		case "/head":
			w.Header().Set("Content-Length", "11")
			w.Header().Set("Content-Type", "text/plain")
			w.Write([]byte("hello world"))
		case "/redirect":
			httpx.Redirect(w, r, "/elsewhere", 302)
		case "/notmodified":
			w.Header().Set("Content-Length", "99")
			w.WriteHeader(304)
		case "/echo":
			body, _ := io.ReadAll(r.Body)
			w.Header().Set("X-Length", http.StatusText(200))
			w.Write(body)
		case "/query":
			io.WriteString(w, r.URL.EscapedPath()+"|"+r.URL.RawQuery+"|"+r.Host+"|"+r.Header.Get("X-Test"))
		default:
			httpx.NotFound(w, r)
		}
	})
	client := server.Client()
	get := func(method, path string) (*http.Response, string) {
		t.Helper()
		request, _ := http.NewRequest(method, server.URL+path, nil)
		request.Header.Set("X-Test", "value")
		response, err := client.Do(request)
		if err != nil {
			t.Fatal(err)
		}
		body, _ := io.ReadAll(response.Body)
		response.Body.Close()
		return response, string(body)
	}
	response, body := get("GET", "/cookies")
	if body != "hello" || len(response.Cookies()) != 2 || strings.Join(response.Header.Values("X-Multi"), ",") != "one,two" {
		t.Fatal(body, response.Header)
	}
	if response, _ = get("GET", "/sniff"); !strings.HasPrefix(response.Header.Get("Content-Type"), "text/html") {
		t.Fatal("content type was not sniffed", response.Header)
	}
	if response, body = get("GET", "/empty"); response.StatusCode != 200 || body != "" {
		t.Fatal(response.Status)
	}
	response, body = get("HEAD", "/head")
	if response.ContentLength != 11 || body != "" {
		t.Fatal("HEAD must advertise the GET length without a body", response.ContentLength, body)
	}
	if response, body = get("GET", "/head"); response.ContentLength != 11 || body != "hello world" {
		t.Fatal(response.ContentLength, body)
	}
	if response, body = get("GET", "/redirect"); response.Request.URL.Path != "/elsewhere" && response.StatusCode != 404 {
		t.Fatal(response.Status, body)
	}
	if response, body = get("GET", "/notmodified"); response.StatusCode != 304 || body != "" {
		t.Fatal(response.Status)
	}
	if _, body = get("GET", "/query?a=1&b=%20"); !strings.HasPrefix(body, "/query|a=1&b=%20|127.0.0.1:") || !strings.HasSuffix(body, "|value") {
		t.Fatal(body)
	}
	if response, body = get("GET", "/missing"); response.StatusCode != 404 || body != "404 page not found\n" {
		t.Fatal(response.Status, body)
	}
}

func TestRequestBodiesStreamAndEscapedPaths(t *testing.T) {
	big := bytes.Repeat([]byte("0123456789abcdef"), 1<<20) // 16 MiB, far above the read buffer
	server := serve(t, func(w httpx.ResponseWriter, r *httpx.Request) {
		if r.URL.Path == "/a/b c" && r.URL.EscapedPath() == "/a%2Fb/b%20c" {
			// path and escaped path stay distinct, as in net/http
		}
		n, err := io.Copy(io.Discard, r.Body)
		if err != nil || n != int64(len(big)) || r.ContentLength != int64(len(big)) {
			httpx.Error(w, "short body", 500)
			return
		}
		io.WriteString(w, r.URL.Path+"|"+r.URL.EscapedPath())
	})
	response, err := server.Client().Post(server.URL+"/a%2Fb/b%20c", "application/octet-stream", bytes.NewReader(big))
	if err != nil {
		t.Fatal(err)
	}
	body, _ := io.ReadAll(response.Body)
	response.Body.Close()
	if response.StatusCode != 200 || string(body) != "/a/b/b c|/a%2Fb/b%20c" {
		t.Fatal(response.Status, string(body))
	}
}

func TestServeContentMatchesNetHTTP(t *testing.T) {
	content := []byte(strings.Repeat("abcdefghij", 100))
	modified := time.Date(2024, 1, 2, 3, 4, 5, 0, time.UTC)
	cases := []struct {
		name     string
		header   map[string]string
		modified time.Time
	}{
		{"plain", nil, time.Time{}},
		{"range", map[string]string{"Range": "bytes=10-19"}, time.Time{}},
		{"multi", map[string]string{"Range": "bytes=0-4,10-14"}, time.Time{}},
		{"unsatisfiable", map[string]string{"Range": "bytes=5000-6000"}, time.Time{}},
		{"modified", nil, modified},
		{"since", map[string]string{"If-Modified-Since": modified.Format(http.TimeFormat)}, modified},
		{"etag", map[string]string{"If-None-Match": `"x"`}, time.Time{}},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			ours := httpxtest.NewRecorder()
			r := httpxtest.NewRequest("GET", "/file.txt", nil)
			for k, v := range c.header {
				r.Header.Set(k, v)
			}
			ours.Header().Set("Content-Type", "text/plain")
			httpx.ServeContent(ours, r, "file.txt", c.modified, bytes.NewReader(content))

			theirs := newStd()
			request, _ := http.NewRequest("GET", "/file.txt", nil)
			for k, v := range c.header {
				request.Header.Set(k, v)
			}
			theirs.header.Set("Content-Type", "text/plain")
			http.ServeContent(theirs, request, "file.txt", c.modified, bytes.NewReader(content))

			ourBody, theirBody := ours.Body.String(), theirs.body.String()
			if c.name == "multi" {
				// The part boundary is random; compare bodies with it masked.
				for _, pair := range []struct {
					header string
					body   *string
				}{{ours.Header().Get("Content-Type"), &ourBody}, {theirs.header.Get("Content-Type"), &theirBody}} {
					_, boundary, _ := strings.Cut(pair.header, "boundary=")
					*pair.body = strings.ReplaceAll(*pair.body, boundary, "BOUNDARY")
				}
			}
			if ours.Code != theirs.code || ourBody != theirBody {
				t.Fatalf("status/body differ: %d %d\n%q\n%q", ours.Code, theirs.code, ourBody, theirBody)
			}
			for key := range theirs.header {
				if key == "Content-Type" && strings.HasPrefix(ours.Header().Get(key), "multipart/byteranges") {
					continue
				}
				if want := theirs.header.Get(key); ours.Header().Get(key) != want {
					t.Fatalf("%s: %q != %q", key, ours.Header().Get(key), want)
				}
			}
		})
	}
}

type std struct {
	header http.Header
	code   int
	body   bytes.Buffer
}

func newStd() *std                         { return &std{header: http.Header{}, code: 200} }
func (s *std) Header() http.Header         { return s.header }
func (s *std) WriteHeader(code int)        { s.code = code }
func (s *std) Write(p []byte) (int, error) { return s.body.Write(p) }

func TestLargeFilesStreamAndAreClosed(t *testing.T) {
	path := filepath.Join(t.TempDir(), "big.bin")
	data := bytes.Repeat([]byte("stream-me!"), 200_000) // 2 MB, above the streaming threshold
	if err := os.WriteFile(path, data, 0o600); err != nil {
		t.Fatal(err)
	}
	closed := make(chan struct{}, 1)
	server := serve(t, func(w httpx.ResponseWriter, r *httpx.Request) {
		file, err := os.Open(path)
		if err != nil {
			httpx.Error(w, err.Error(), 500)
			return
		}
		if !httpx.DeferClose(w, closer{file, closed}) {
			t.Error("server writer must defer close")
		}
		w.Header().Set("Content-Type", "application/octet-stream")
		w.Header().Set("Content-Length", "2000000")
		w.WriteHeader(200)
		if r.Method != "HEAD" {
			httpx.CopyN(w, file, int64(len(data)))
		}
	})
	response, err := server.Client().Get(server.URL)
	if err != nil {
		t.Fatal(err)
	}
	body, err := io.ReadAll(response.Body)
	response.Body.Close()
	if err != nil || !bytes.Equal(body, data) {
		t.Fatal("body differs", len(body), err)
	}
	select {
	case <-closed:
	case <-time.After(2 * time.Second):
		t.Fatal("file was not closed after the stream")
	}
	head, err := server.Client().Head(server.URL)
	if err != nil || head.ContentLength != int64(len(data)) {
		t.Fatal("HEAD length", head, err)
	}
	head.Body.Close()
	select {
	case <-closed:
	case <-time.After(2 * time.Second):
		t.Fatal("file was not closed after HEAD")
	}
}

type closer struct {
	*os.File
	done chan struct{}
}

func (c closer) Close() error {
	c.File.Close()
	select {
	case c.done <- struct{}{}:
	default:
	}
	return nil
}

func TestPanicIsolation(t *testing.T) {
	server := serve(t, func(w httpx.ResponseWriter, r *httpx.Request) {
		if r.URL.Path == "/boom" {
			panic("boom")
		}
		io.WriteString(w, "fine")
	})
	response, err := server.Client().Get(server.URL + "/boom")
	if err != nil {
		t.Fatal(err)
	}
	response.Body.Close()
	if response.StatusCode != 500 {
		t.Fatal(response.Status)
	}
	response, err = server.Client().Get(server.URL + "/ok")
	if err != nil || response.StatusCode != 200 {
		t.Fatal("server did not survive a handler panic", err)
	}
	response.Body.Close()
}

func TestRequestFraming(t *testing.T) {
	server := serve(t, func(w httpx.ResponseWriter, r *httpx.Request) {
		body, _ := io.ReadAll(r.Body)
		io.WriteString(w, strings.Join([]string{r.Method, itoa(r.ContentLength), strings.Join(r.TransferEncoding, "+"), string(body)}, "|"))
	})
	do := func(method string, body io.Reader) string {
		t.Helper()
		request, _ := http.NewRequest(method, server.URL, body)
		response, err := server.Client().Do(request)
		if err != nil {
			t.Fatal(err)
		}
		defer response.Body.Close()
		data, _ := io.ReadAll(response.Body)
		return string(data)
	}
	if got := do("GET", nil); got != "GET|0||" {
		t.Fatal("bodiless GET must report a zero length:", got)
	}
	if got := do("POST", strings.NewReader("abc")); got != "POST|3||abc" {
		t.Fatal(got)
	}
	// An unknown-length body is sent chunked.
	if got := do("POST", io.NopCloser(strings.NewReader("chunky"))); got != "POST|-1|chunked|chunky" {
		t.Fatal(got)
	}
}

func itoa(n int64) string { return strconv.FormatInt(n, 10) }
