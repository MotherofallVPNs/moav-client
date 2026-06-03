// Package logbus is an in-process pub/sub log bus that wraps the stdlib
// logger. Every log line written by proxy-core (or sing-box stderr captured
// elsewhere) lands in a bounded ring buffer with a parsed level + timestamp,
// AND is fanned out over a channel to live subscribers so the API can
// stream events to WebSocket clients.
package logbus

import (
	"bytes"
	"log"
	"strings"
	"sync"
	"time"
)

// Event is one structured log record.
type Event struct {
	Ts      int64  `json:"ts"`      // unix-ms
	Level   string `json:"level"`   // info | warn | error
	Source  string `json:"source"`  // "proxy-core" | "singbox" | ...
	Message string `json:"message"`
}

// Bus is a fan-out hub plus a ring buffer.
type Bus struct {
	mu       sync.RWMutex
	ring     []Event
	maxRing  int
	subsMu   sync.RWMutex
	subs     map[chan Event]struct{}
}

// New creates a bus retaining ring size events in memory.
func New(ring int) *Bus {
	if ring <= 0 {
		ring = 500
	}
	return &Bus{
		ring:    make([]Event, 0, ring),
		maxRing: ring,
		subs:    make(map[chan Event]struct{}),
	}
}

// Publish appends and fans out an event.
func (b *Bus) Publish(ev Event) {
	if ev.Ts == 0 {
		ev.Ts = time.Now().UnixMilli()
	}
	if ev.Level == "" {
		ev.Level = "info"
	}
	b.mu.Lock()
	if len(b.ring) >= b.maxRing {
		copy(b.ring, b.ring[1:])
		b.ring[len(b.ring)-1] = ev
	} else {
		b.ring = append(b.ring, ev)
	}
	b.mu.Unlock()

	b.subsMu.RLock()
	for ch := range b.subs {
		select {
		case ch <- ev:
		default: // slow subscriber, drop
		}
	}
	b.subsMu.RUnlock()
}

// Snapshot returns a copy of the current ring (oldest first).
func (b *Bus) Snapshot() []Event {
	b.mu.RLock()
	defer b.mu.RUnlock()
	out := make([]Event, len(b.ring))
	copy(out, b.ring)
	return out
}

// Subscribe returns a channel for incoming events and a function to release it.
func (b *Bus) Subscribe(buf int) (<-chan Event, func()) {
	if buf <= 0 {
		buf = 16
	}
	ch := make(chan Event, buf)
	b.subsMu.Lock()
	b.subs[ch] = struct{}{}
	b.subsMu.Unlock()
	return ch, func() {
		b.subsMu.Lock()
		delete(b.subs, ch)
		b.subsMu.Unlock()
		close(ch)
	}
}

// CapturingWriter is an io.Writer that splits incoming bytes by newline and
// publishes each line to a Bus. Plug it into log.SetOutput to capture every
// stdlib log call. We keep a passthrough so the original stderr still shows
// the log line — useful for `docker logs`.
type CapturingWriter struct {
	Bus        *Bus
	Source     string
	Passthrough []byte // assigned externally; CapturingWriter.Write writes to this too
	pass        func(p []byte) (int, error)
	buf         bytes.Buffer
	mu          sync.Mutex
}

// NewCapturingWriter wires a Bus and a passthrough write function (typically
// os.Stderr.Write so docker logs still works).
func NewCapturingWriter(bus *Bus, source string, pass func(p []byte) (int, error)) *CapturingWriter {
	return &CapturingWriter{Bus: bus, Source: source, pass: pass}
}

func (w *CapturingWriter) Write(p []byte) (int, error) {
	w.mu.Lock()
	w.buf.Write(p)
	for {
		line, err := w.buf.ReadBytes('\n')
		if err != nil {
			// no full line yet; put what we read back
			w.buf.Write(line)
			break
		}
		s := strings.TrimRight(string(line), "\r\n")
		if s != "" {
			w.Bus.Publish(Event{
				Level:   classifyLevel(s),
				Source:  w.Source,
				Message: stripDatePrefix(s),
			})
		}
	}
	w.mu.Unlock()

	if w.pass != nil {
		return w.pass(p)
	}
	return len(p), nil
}

// classifyLevel inspects an unstructured log line and returns info/warn/error.
//
// Rules of thumb:
//   - **error** is reserved for SYSTEM-level failures (fatal/panic, no
//     healthy endpoints anywhere, can't load config, can't bind a listener).
//     A single endpoint being unhealthy in a probe pass is NOT an error —
//     the balancer just routes around it.
//   - **warn** covers transient / recoverable conditions the operator
//     should notice (one dial failed and we're retrying, fall-back to
//     direct, deprecation notices).
//   - everything else (probe results, normal lifecycle, traffic) is **info**.
//
// Heuristics order matters — we check the most specific patterns first.
func classifyLevel(s string) string {
	low := strings.ToLower(s)

	// "probe cycle:" is our roll-up emitted at the end of each pass. We
	// classify it explicitly first so the embedded WARN keyword survives.
	if strings.HasPrefix(low, "probe cycle:") {
		if strings.Contains(low, "warn") {
			return "warn"
		}
		return "info"
	}

	// Individual probe lines are always info regardless of the per-endpoint
	// status. They report on a peer's health, not on proxy-core's own health.
	if strings.HasPrefix(low, "probe ") || strings.Contains(low, " probe ") &&
		(strings.Contains(low, "status=ok") || strings.Contains(low, "status=error") ||
			strings.Contains(low, "status=timeout")) {
		return "info"
	}

	switch {
	// Transient / recoverable conditions are warns even if they mention
	// "fail" / "no healthy" etc. — check these FIRST so the fallback log
	// lines don't bubble up to error.
	case strings.Contains(low, "warn") ||
		strings.Contains(low, "deprecat") ||
		strings.Contains(low, "falling back") ||
		strings.Contains(low, "fall back") ||
		strings.Contains(low, "trying next endpoint") ||
		strings.Contains(low, "failover") ||
		strings.Contains(low, "succeeded after") ||
		strings.Contains(low, "skipping") ||
		strings.Contains(low, "all candidates failed") || // we still try direct after
		strings.Contains(low, "no healthy endpoint") ||
		strings.Contains(low, "dial through ") && strings.Contains(low, "failed"):
		return "warn"

	// True system-level failures: process can't proceed.
	case strings.HasPrefix(low, "fatal:") ||
		strings.Contains(low, " fatal:") ||
		strings.HasPrefix(low, "panic:") ||
		strings.Contains(low, " panic:") ||
		strings.HasPrefix(low, "load config:") ||
		strings.HasPrefix(low, "could not load") ||
		strings.Contains(low, "listen:") && strings.Contains(low, "address already in use") ||
		strings.Contains(low, "listen:") && strings.Contains(low, "permission denied") ||
		strings.HasPrefix(low, "http listen:") ||
		strings.HasPrefix(low, "socks5 listen:") ||
		strings.HasPrefix(low, "api listen:"):
		return "error"

	default:
		return "info"
	}
}

// stripDatePrefix removes the stdlib "2026/06/03 00:00:00 " timestamp because
// we attach our own structured Ts. Keeps lines compact in the UI.
func stripDatePrefix(s string) string {
	if len(s) >= 20 && s[4] == '/' && s[7] == '/' && s[10] == ' ' {
		return strings.TrimSpace(s[20:])
	}
	return s
}

// Default is a process-global bus used when callers don't want to plumb one.
var Default = New(500)

// Helper functions for code that wants to publish directly.
func Info(source, msg string)  { Default.Publish(Event{Level: "info", Source: source, Message: msg}) }
func Warn(source, msg string)  { Default.Publish(Event{Level: "warn", Source: source, Message: msg}) }
func Error(source, msg string) { Default.Publish(Event{Level: "error", Source: source, Message: msg}) }

// CaptureStdLog wires the package-global stdlib logger so every log.Printf
// from anywhere in the process is mirrored to bus. Returns the previous
// output so callers can restore it.
func CaptureStdLog(bus *Bus, source string, pass func(p []byte) (int, error)) {
	log.SetOutput(NewCapturingWriter(bus, source, pass))
}
