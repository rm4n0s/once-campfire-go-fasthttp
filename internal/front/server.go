package front

import (
	"context"
	"crypto/tls"
	"encoding/base64"
	"errors"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/basecamp/once-campfire-go/internal/httpx"
	"github.com/valyala/fasthttp/fasthttpadaptor"
	"golang.org/x/crypto/acme"
	"golang.org/x/crypto/acme/autocert"
)

// server builds a fasthttp listener. fasthttp speaks HTTP/1.1 only: H2 and H2C
// are not offered, and TLS negotiates http/1.1 (plus the ACME challenge protocol).
func (c Config) server(handler httpx.Handler) *httpx.Server {
	return &httpx.Server{Handler: handler, ReadTimeout: c.ReadTimeout, WriteTimeout: c.WriteTimeout, IdleTimeout: c.IdleTimeout, MaxHeaderBytes: 64 << 10}
}
func bodyLimit(next httpx.Handler, limit int64) httpx.Handler {
	if limit <= 0 {
		return next
	}
	return httpx.HandlerFunc(func(w httpx.ResponseWriter, r *httpx.Request) {
		if r.ContentLength > limit {
			httpx.Error(w, "Request Entity Too Large", 413)
			return
		}
		r.Body = httpx.MaxBytesReader(r.Body, limit)
		next.ServeHTTP(w, r)
	})
}
func forward(next httpx.Handler, c Config) httpx.Handler {
	return httpx.HandlerFunc(func(w httpx.ResponseWriter, r *httpx.Request) {
		r = r.Clone(r.Context())
		host, _, _ := net.SplitHostPort(r.RemoteAddr)
		if !c.ForwardHeaders {
			for _, name := range []string{"X-Forwarded-For", "X-Forwarded-Host", "X-Forwarded-Port", "X-Forwarded-Proto", "Forwarded"} {
				r.Header.Del(name)
			}
		}
		if previous := r.Header.Get("X-Forwarded-For"); previous != "" {
			r.Header.Set("X-Forwarded-For", previous+", "+host)
		} else {
			r.Header.Set("X-Forwarded-For", host)
		}
		if r.Header.Get("X-Forwarded-Host") == "" {
			r.Header.Set("X-Forwarded-Host", r.Host)
		}
		if r.Header.Get("X-Forwarded-Proto") == "" {
			scheme := "http"
			if r.TLS != nil {
				scheme = "https"
			}
			r.Header.Set("X-Forwarded-Proto", scheme)
		}
		r.Header.Set("X-Request-Start", strconv.FormatInt(time.Now().UnixMilli(), 10))
		started := time.Now()
		next.ServeHTTP(w, r)
		if c.LogRequests {
			slog.Info("request", "method", r.Method, "path", r.URL.Path, "duration", time.Since(started))
		}
	})
}

// Serve hosts the application listener and the public HTTP/TLS listeners in one
// process. The ACME manager is Go's autocert, also used by Thruster.
func Serve(ctx context.Context, c Config, app httpx.Handler) error {
	app = bodyLimit(Deflate(app), c.MaxRequestBody)
	public := forward(PublicCompression(NewCache(c.CacheSize, c.MaxCacheItemSize).Handler(app), c), c)
	type listening struct {
		server   *httpx.Server
		listener net.Listener
		tls      *tls.Config
	}
	var servers []listening
	add := func(address string, server *httpx.Server, config *tls.Config) error {
		listener, err := net.Listen("tcp", address)
		if err != nil {
			return err
		}
		servers = append(servers, listening{server, listener, config})
		return nil
	}
	defer func() {
		for _, s := range servers {
			s.listener.Close()
		}
	}()
	if c.TargetPort != c.HTTPPort && (len(c.Domains) == 0 || c.TargetPort != c.HTTPSPort) {
		if err := add(net.JoinHostPort(c.TargetBind, strconv.Itoa(c.TargetPort)), c.server(app), nil); err != nil {
			return err
		}
	}
	if len(c.Domains) == 0 {
		if err := add(":"+strconv.Itoa(c.HTTPPort), c.server(public), nil); err != nil {
			return err
		}
	} else {
		manager := &autocert.Manager{Prompt: autocert.AcceptTOS, Cache: autocert.DirCache(c.StoragePath), HostPolicy: autocert.HostWhitelist(c.Domains...), Client: &acme.Client{DirectoryURL: c.ACMEDirectory}}
		if c.EABKeyID != "" {
			key, err := base64.RawURLEncoding.DecodeString(strings.TrimRight(c.EABKey, "="))
			if err != nil {
				return fmt.Errorf("invalid EAB_HMAC_KEY: %w", err)
			}
			manager.ExternalAccountBinding = &acme.ExternalAccountBinding{KID: c.EABKeyID, Key: key}
		}
		config := manager.TLSConfig()
		config.MinVersion = tls.VersionTLS12
		config.NextProtos = []string{"http/1.1", acme.ALPNProto}
		if err := add(":"+strconv.Itoa(c.HTTPSPort), c.server(public), config); err != nil {
			return err
		}
		// autocert answers HTTP-01 challenges with a net/http handler; the
		// redirect is its fallback for every other request.
		redirect := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			host := r.Host
			if h, _, err := net.SplitHostPort(host); err == nil {
				host = h
			}
			if c.HTTPSPort != 443 {
				host = net.JoinHostPort(host, strconv.Itoa(c.HTTPSPort))
			}
			http.Redirect(w, r, "https://"+host+r.URL.RequestURI(), 301)
		})
		redirector := c.server(nil)
		redirector.Raw = fasthttpadaptor.NewFastHTTPHandler(manager.HTTPHandler(redirect))
		if err := add(":"+strconv.Itoa(c.HTTPPort), redirector, nil); err != nil {
			return err
		}
	}
	results := make(chan error, len(servers))
	for _, s := range servers {
		slog.Info("listening", "address", s.listener.Addr(), "tls", s.tls != nil)
		go func() {
			if s.tls != nil {
				results <- s.server.ServeTLS(s.listener, s.tls)
			} else {
				results <- s.server.Serve(s.listener)
			}
		}()
	}
	var result error
	select {
	case <-ctx.Done():
	case result = <-results:
	}
	shutdown, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	stopped := make(chan error, len(servers))
	for _, s := range servers {
		go func() {
			err := s.server.Shutdown(shutdown)
			if err != nil {
				s.server.Close()
			}
			stopped <- err
		}()
	}
	for range servers {
		result = errors.Join(result, <-stopped)
	}
	if errors.Is(result, httpx.ErrServerClosed) {
		return nil
	}
	return result
}
