package web

import (
	"github.com/basecamp/once-campfire-go/internal/httpx"
	"github.com/basecamp/once-campfire-go/internal/httpx/httpxtest"
	"testing"
)

func TestCompletedResponseValidators(t *testing.T) {
	for _, method := range []string{"GET", "HEAD"} {
		request := httpxtest.NewRequest(method, "/up", nil)
		first := httpxtest.NewRecorder()
		writer := &responseBuffer{ResponseWriter: first}
		writer.Write([]byte("<p>complete"))
		writer.Write([]byte(" body</p>"))
		writer.finish(request)
		if first.Header().Get("ETag") == "" || first.Header().Get("Cache-Control") != "max-age=0, private, must-revalidate" {
			t.Fatal(first.Header())
		}
		if method == "HEAD" && first.Body.Len() != 0 {
			t.Fatal("HEAD body", first.Body.String())
		}
		request.Header.Set("If-None-Match", first.Header().Get("ETag"))
		second := httpxtest.NewRecorder()
		writer = &responseBuffer{ResponseWriter: second}
		writer.Write([]byte("<p>complete body</p>"))
		writer.finish(request)
		if second.Code != httpx.StatusNotModified || second.Body.Len() != 0 {
			t.Fatal(second.Code, second.Body.String())
		}
		changed := httpxtest.NewRecorder()
		writer = &responseBuffer{ResponseWriter: changed}
		writer.Write([]byte("changed"))
		writer.finish(request)
		if changed.Code != 200 {
			t.Fatal("stale validator accepted")
		}
	}
}
