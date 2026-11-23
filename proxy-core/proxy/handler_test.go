package proxy

import (
	"bufio"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/MotherofallVPNs/moav-client/proxy-core/plugins"
)

// A client may send tunnel bytes right behind the CONNECT request (or right
// after the 200). Those bytes sit in the server's read buffer at hijack time
// and must reach the destination, not be dropped with the buffer.
func TestHTTPConnect_ForwardsBytesSentWithTheRequest(t *testing.T) {
	echo := startEcho(t, "127.0.0.1:0")
	rt := newRouter(nil, plugins.NewEngine([]plugins.Rule{
		rule("ip_cidr", "127.0.0.0/8", plugins.DecisionDirect),
	}), nil)
	srv := httptest.NewServer(&httpHandler{router: rt})
	t.Cleanup(srv.Close)

	c := dialRaw(t, srv.Listener.Addr().String())
	target := echo.addr.String()
	mustWrite(t, c, []byte(fmt.Sprintf("CONNECT %s HTTP/1.1\r\nHost: %s\r\n\r\npayload", target, target)))
	br := bufio.NewReader(c)
	resp, err := http.ReadResponse(br, &http.Request{Method: http.MethodConnect})
	if err != nil || resp.StatusCode != http.StatusOK {
		t.Fatalf("CONNECT response = %v, %v", resp, err)
	}
	got := make([]byte, len("payload"))
	if _, err := io.ReadFull(br, got); err != nil || string(got) != "payload" {
		t.Fatalf("echo of early bytes = %q, %v; want %q", got, err, "payload")
	}
}
