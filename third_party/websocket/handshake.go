package websocket

import (
	"bufio"
	"net"
	"net/http"
)

// ServerRequest is the part of an upgrade request the handshake reads. It lets
// servers that are not built on net/http (here, fasthttp) reuse Accept's request
// verification and negotiation.
type ServerRequest struct {
	Method string
	Host   string
	// Header uses canonical MIME keys, as http.Header does.
	Header map[string][]string
}

// ServerHandshake is the outcome of validating an upgrade request. When Err is
// nil, Header holds the headers of the 101 response; otherwise Status and Err
// describe the rejection.
type ServerHandshake struct {
	Status int
	Header map[string][]string
	Err    error

	subprotocol string
	copts       *compressionOptions
	opts        *AcceptOptions
}

// NewServerHandshake performs Accept's verification, origin check, subprotocol
// and permessage-deflate negotiation without touching a connection.
func NewServerHandshake(req ServerRequest, opts *AcceptOptions) *ServerHandshake {
	w := &handshakeWriter{header: http.Header{}}
	r := &http.Request{Method: req.Method, Host: req.Host, Header: http.Header(req.Header), Proto: "HTTP/1.1", ProtoMajor: 1, ProtoMinor: 1}
	if code, err := verifyClientRequest(w, r); err != nil {
		return &ServerHandshake{Status: code, Header: w.header, Err: err}
	}
	opts = opts.cloneWithDefaults()
	if !opts.InsecureSkipVerify {
		if err := authenticateOrigin(r, opts.OriginPatterns); err != nil {
			return &ServerHandshake{Status: http.StatusForbidden, Header: w.header, Err: err}
		}
	}
	w.header.Set("Upgrade", "websocket")
	w.header.Set("Connection", "Upgrade")
	w.header.Set("Sec-WebSocket-Accept", secWebSocketAccept(r.Header.Get("Sec-WebSocket-Key")))
	h := &ServerHandshake{Status: http.StatusSwitchingProtocols, Header: w.header, opts: opts}
	if subprotocol := selectSubprotocol(r, opts.Subprotocols); subprotocol != "" {
		w.header.Set("Sec-WebSocket-Protocol", subprotocol)
		h.subprotocol = subprotocol
	}
	if copts, ok := selectDeflate(websocketExtensions(r.Header), opts.CompressionMode); ok {
		w.header.Set("Sec-WebSocket-Extensions", copts.String())
		h.copts = copts
	}
	return h
}

// Conn wraps the upgraded connection once the 101 response has been sent.
func (h *ServerHandshake) Conn(conn net.Conn) *Conn {
	return newConn(connConfig{
		subprotocol:    h.subprotocol,
		rwc:            conn,
		copts:          h.copts,
		flateThreshold: h.opts.CompressionThreshold,
		onPingReceived: h.opts.OnPingReceived,
		onPongReceived: h.opts.OnPongReceived,
		br:             bufio.NewReader(conn),
		bw:             bufio.NewWriter(conn),
	})
}

// handshakeWriter collects the headers verifyClientRequest sets on rejection.
type handshakeWriter struct{ header http.Header }

func (w *handshakeWriter) Header() http.Header         { return w.header }
func (w *handshakeWriter) Write(p []byte) (int, error) { return len(p), nil }
func (w *handshakeWriter) WriteHeader(int)             {}
