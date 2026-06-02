package proxy

import (
	"io"
	"log"
	"net"
	"net/http"

	"github.com/ibeezhan/moav-client/proxy-core/balancer"
	"github.com/ibeezhan/moav-client/proxy-core/plugins"
)

type httpHandler struct {
	balancer *balancer.Balancer
	engine   *plugins.Engine
	torrent  *plugins.TorrentBlocker
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

	decision := h.decide(host, port, "tcp")
	switch decision {
	case plugins.DecisionBlock:
		log.Printf("BLOCK %s:%d (plugin engine)", host, port)
		// Close without response — the CONNECT request is dropped.
		hijacker, ok := w.(http.Hijacker)
		if ok {
			conn, _, herr := hijacker.Hijack()
			if herr == nil {
				conn.Close()
			}
		}
		return

	case plugins.DecisionDirect:
		log.Printf("DIRECT %s:%d (plugin engine)", host, port)
		dst, derr := net.Dial("tcp", r.Host)
		if derr != nil {
			log.Printf("CONNECT direct dial %s: %v", r.Host, derr)
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

	default: // DecisionProxy
		dst, derr := h.balancer.DialContext("tcp", r.Host)
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
}

// decide applies TorrentBlocker first, then the Engine rule list.
func (h *httpHandler) decide(host string, port int, proto string) plugins.Decision {
	if h.torrent != nil && h.torrent.Match(host, port, proto) {
		return plugins.DecisionBlock
	}
	if h.engine != nil {
		return h.engine.Evaluate(host, port, proto)
	}
	return plugins.DecisionProxy
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
