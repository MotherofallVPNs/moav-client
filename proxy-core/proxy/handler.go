package proxy

import (
	"io"
	"log"
	"net"
	"net/http"

	"github.com/ibeezhan/moav-client/proxy-core/balancer"
)

type httpHandler struct {
	balancer *balancer.Balancer
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
	dst, err := h.balancer.DialContext("tcp", r.Host)
	if err != nil {
		log.Printf("CONNECT dial %s: %v", r.Host, err)
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
	clientConn, _, err := hijacker.Hijack()
	if err != nil {
		log.Printf("CONNECT hijack: %v", err)
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
