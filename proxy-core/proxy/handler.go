package proxy

import (
	"io"
	"log"
	"net"
	"net/http"

	"github.com/MotherofallVPNs/moav-client/proxy-core/plugins"
)

type httpHandler struct {
	router *router
}

// ServeHTTP handles HTTP CONNECT tunneling and plain HTTP forwarding.
func (h *httpHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if r.Method == http.MethodConnect {
		h.handleConnect(w, r)
		return
	}
	// TODO: plain HTTP forwarding (non-CONNECT)
	http.Error(w, "only CONNECT supported", http.StatusMethodNotAllowed)
}

func (h *httpHandler) handleConnect(w http.ResponseWriter, r *http.Request) {
	host, portStr, err := net.SplitHostPort(r.Host)
	if err != nil {
		host = r.Host
		portStr = "443"
	}
	port := 443
	if p, err2 := net.LookupPort("tcp", portStr); err2 == nil {
		port = p
	}

	rt := h.router.decide(r.Context(), host, port, "tcp")
	if rt.decision == plugins.DecisionBlock {
		log.Printf("BLOCK %s:%d (plugin engine)", host, port)
		// Close without response — the CONNECT request is dropped.
		if hijacker, ok := w.(http.Hijacker); ok {
			if conn, _, herr := hijacker.Hijack(); herr == nil {
				conn.Close()
			}
		}
		return
	}
	if rt.decision == plugins.DecisionDirect {
		log.Printf("DIRECT %s:%d (plugin engine)%s", host, port, viaAddrs(rt))
	}

	dst, derr := h.router.dial(r.Context(), rt, "tcp", host, port)
	if derr != nil {
		log.Printf("CONNECT dial %s: %v", r.Host, derr)
		http.Error(w, "Bad Gateway", http.StatusBadGateway)
		return
	}
	defer dst.Close()
	w.WriteHeader(http.StatusOK)
	hijacker, ok := w.(http.Hijacker)
	if !ok {
		http.Error(w, "hijacking not supported", http.StatusInternalServerError)
		return
	}
	clientConn, _, herr := hijacker.Hijack()
	if herr != nil {
		log.Printf("CONNECT hijack: %v", herr)
		return
	}
	defer clientConn.Close()
	tunnel(clientConn, dst)
}

// tunnel bidirectionally copies between two connections.
func tunnel(a, b net.Conn) {
	done := make(chan struct{}, 2)
	copy := func(dst, src net.Conn) {
		io.Copy(dst, src) //nolint:errcheck
		done <- struct{}{}
	}
	go copy(a, b)
	go copy(b, a)
	<-done
}
