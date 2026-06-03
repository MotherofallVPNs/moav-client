package balancer

import (
	"sync"
	"sync/atomic"
	"time"
)

// Flow is one tracked connection through the balancer. Most fields settle
// when the connection closes; BytesUp/Down tick up live.
type Flow struct {
	ID         uint64 `json:"id"`
	OpenedUnix int64  `json:"opened_unix"`
	ClosedUnix int64  `json:"closed_unix,omitempty"`
	Client     string `json:"client"`     // remote addr of the SOCKS5/HTTP client
	Dest       string `json:"dest"`       // requested destination host:port
	EndpointID string `json:"endpoint_id"` // moav endpoint chosen (empty if direct)
	Protocol   string `json:"protocol"`   // protocol of the chosen endpoint
	BytesUp    int64  `json:"bytes_up"`
	BytesDown  int64  `json:"bytes_down"`
	Result     string `json:"result"` // "open" | "ok" | "error: ..."
}

// Flows is a ring buffer of recent connections. Default cap 200.
type Flows struct {
	mu    sync.RWMutex
	buf   []*Flow
	cap   int
	next  uint64
}

// NewFlows builds a tracker with the given ring capacity (200 if 0).
func NewFlows(cap int) *Flows {
	if cap <= 0 {
		cap = 200
	}
	return &Flows{cap: cap}
}

// Begin records the start of a flow and returns its handle.
func (f *Flows) Begin(client, dest, endpointID, protocol string) *Flow {
	fl := &Flow{
		ID:         atomic.AddUint64(&f.next, 1),
		OpenedUnix: time.Now().Unix(),
		Client:     client,
		Dest:       dest,
		EndpointID: endpointID,
		Protocol:   protocol,
		Result:     "open",
	}
	f.mu.Lock()
	f.buf = append(f.buf, fl)
	if len(f.buf) > f.cap {
		f.buf = f.buf[len(f.buf)-f.cap:]
	}
	f.mu.Unlock()
	return fl
}

// End marks a flow as closed with an optional error.
func (fl *Flow) End(err error) {
	fl.ClosedUnix = time.Now().Unix()
	if err != nil {
		fl.Result = "error: " + err.Error()
	} else {
		fl.Result = "ok"
	}
}

// Add is called per-direction during the io.Copy tunnel to tally bytes.
func (fl *Flow) Add(up, down int64) {
	if up > 0 {
		atomic.AddInt64(&fl.BytesUp, up)
	}
	if down > 0 {
		atomic.AddInt64(&fl.BytesDown, down)
	}
}

// Snapshot returns a copy of the current ring, oldest first.
func (f *Flows) Snapshot() []Flow {
	f.mu.RLock()
	defer f.mu.RUnlock()
	out := make([]Flow, len(f.buf))
	for i, fl := range f.buf {
		out[i] = *fl
	}
	return out
}
