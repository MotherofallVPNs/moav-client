package balancer

import (
	"errors"
	"io"
	"net"
	"reflect"
	"strconv"
	"sync"
	"testing"

	"github.com/MotherofallVPNs/moav-client/proxy-core/subscription"
)

// ep is a terse constructor for a live-by-default endpoint.
func ep(id string, opts ...func(*subscription.Endpoint)) subscription.Endpoint {
	e := subscription.Endpoint{ID: id, Enabled: true, Status: "ok", LatencyMs: -1}
	for _, o := range opts {
		o(&e)
	}
	return e
}

func latency(ms int64) func(*subscription.Endpoint) { return func(e *subscription.Endpoint) { e.LatencyMs = ms } }
func prio(p int) func(*subscription.Endpoint)        { return func(e *subscription.Endpoint) { e.Priority = p } }
func disabled() func(*subscription.Endpoint)         { return func(e *subscription.Endpoint) { e.Enabled = false } }
func status(s string) func(*subscription.Endpoint)   { return func(e *subscription.Endpoint) { e.Status = s } }

func TestNewAndStrategyName(t *testing.T) {
	b := New(StrategyLatency)
	if b.StrategyName() != "latency" {
		t.Fatalf("StrategyName = %q, want latency", b.StrategyName())
	}
	if b.Stats() == nil || b.Flows() == nil {
		t.Fatal("New must initialise Stats and Flows")
	}
	b.SetStrategy(StrategyPriority)
	if b.StrategyName() != "priority" {
		t.Fatalf("after SetStrategy: %q, want priority", b.StrategyName())
	}
}

func TestSetEndpointsSnapshotIsCopy(t *testing.T) {
	b := New(StrategyLatency)
	b.SetEndpoints([]subscription.Endpoint{ep("a"), ep("b")})
	snap := b.Endpoints()
	if len(snap) != 2 {
		t.Fatalf("Endpoints len = %d, want 2", len(snap))
	}
	// Mutating the snapshot must not affect the balancer's internal pool.
	snap[0].ID = "mutated"
	if b.Endpoints()[0].ID != "a" {
		t.Fatal("Endpoints() must return a defensive copy")
	}
}

func TestPatchEndpoint(t *testing.T) {
	b := New(StrategyPriority)
	b.SetEndpoints([]subscription.Endpoint{ep("a", prio(5))})

	yes := true
	got, ok := b.PatchEndpoint("a", EndpointPatch{Enabled: &yes, Priority: intp(9)})
	if !ok {
		t.Fatal("PatchEndpoint returned ok=false for an existing id")
	}
	if got.Priority != 9 || !got.Enabled {
		t.Fatalf("patch not applied: %+v", got)
	}
	if b.Endpoints()[0].Priority != 9 {
		t.Fatal("patch must mutate the stored endpoint")
	}

	// Unknown id → false, and nothing changes.
	if _, ok := b.PatchEndpoint("nope", EndpointPatch{Priority: intp(1)}); ok {
		t.Fatal("PatchEndpoint on unknown id must return ok=false")
	}
	// nil patch fields are left untouched.
	if _, ok := b.PatchEndpoint("a", EndpointPatch{}); !ok {
		t.Fatal("empty patch on existing id should still match")
	}
	if b.Endpoints()[0].Priority != 9 {
		t.Fatal("nil patch fields must not change existing values")
	}
}

func intp(i int) *int { return &i }

func TestPickFiltersDownEndpoints(t *testing.T) {
	b := New(StrategyLatency)
	// none live → ErrNoEndpoints
	b.SetEndpoints([]subscription.Endpoint{ep("a", disabled()), ep("b", status("timeout"))})
	if _, err := b.Pick(); !errors.Is(err, ErrNoEndpoints) {
		t.Fatalf("Pick with no live endpoints: err = %v, want ErrNoEndpoints", err)
	}
	// empty pool → ErrNoEndpoints
	b.SetEndpoints(nil)
	if _, err := b.Pick(); !errors.Is(err, ErrNoEndpoints) {
		t.Fatalf("Pick on empty pool: err = %v, want ErrNoEndpoints", err)
	}
}

func TestPickLatency(t *testing.T) {
	b := New(StrategyLatency)
	b.SetEndpoints([]subscription.Endpoint{
		ep("slow", latency(300)),
		ep("fast", latency(20)),
		ep("mid", latency(120)),
		ep("down", latency(1), disabled()), // fastest but disabled → ignored
	})
	got, err := b.Pick()
	if err != nil {
		t.Fatal(err)
	}
	if got.ID != "fast" {
		t.Fatalf("latency strategy picked %q, want fast", got.ID)
	}
}

func TestPickPriority(t *testing.T) {
	b := New(StrategyPriority)
	b.SetEndpoints([]subscription.Endpoint{
		ep("c", prio(3)),
		ep("a", prio(1)),
		ep("b", prio(2)),
	})
	got, err := b.Pick()
	if err != nil {
		t.Fatal(err)
	}
	if got.ID != "a" {
		t.Fatalf("priority strategy picked %q, want a (lowest Priority)", got.ID)
	}
}

func TestPickWeightedStaysInLiveSet(t *testing.T) {
	b := New(StrategyWeighted)
	b.SetEndpoints([]subscription.Endpoint{
		ep("a", prio(1)),
		ep("b", prio(5)),
		ep("down", disabled()),
	})
	live := map[string]bool{"a": true, "b": true}
	for i := 0; i < 100; i++ {
		got, err := b.Pick()
		if err != nil {
			t.Fatal(err)
		}
		if !live[got.ID] {
			t.Fatalf("weighted picked non-live/absent endpoint %q", got.ID)
		}
	}
}

func TestPickDefaultStrategyPicksFirst(t *testing.T) {
	b := New(Strategy("bogus"))
	b.SetEndpoints([]subscription.Endpoint{ep("first"), ep("second")})
	got, err := b.Pick()
	if err != nil {
		t.Fatal(err)
	}
	if got.ID != "first" {
		t.Fatalf("unknown strategy should pick the first live endpoint, got %q", got.ID)
	}
}

func TestWeightedRandom(t *testing.T) {
	// Single endpoint is always returned.
	only := []subscription.Endpoint{ep("solo", prio(7))}
	if weightedRandom(only).ID != "solo" {
		t.Fatal("weightedRandom with one endpoint must return it")
	}
	// A zero/negative priority is weighted as 1 (not skipped), so every
	// endpoint remains reachable.
	eps := []subscription.Endpoint{ep("x", prio(0)), ep("y", prio(-3)), ep("z", prio(2))}
	seen := map[string]bool{}
	for i := 0; i < 500; i++ {
		seen[weightedRandom(eps).ID] = true
	}
	for _, id := range []string{"x", "y", "z"} {
		if !seen[id] {
			t.Fatalf("weightedRandom never returned %q — a weight is being dropped", id)
		}
	}
}

func TestStats(t *testing.T) {
	s := NewStats()
	s.RecordDial("a", nil)
	s.RecordDial("a", errors.New("boom"))
	s.RecordFailover("a")
	s.RecordTraffic("a", 100, 250)
	s.RecordTraffic("a", 0, 50) // up=0 ignored, down accumulates

	snap := s.Snapshot()
	if len(snap) != 1 {
		t.Fatalf("Snapshot len = %d, want 1", len(snap))
	}
	e := snap[0]
	if e.Dials != 2 || e.DialErrors != 1 || e.Failovers != 1 {
		t.Fatalf("counters wrong: %+v", e)
	}
	if e.BytesUp != 100 || e.BytesDown != 300 {
		t.Fatalf("traffic wrong: up=%d down=%d, want 100/300", e.BytesUp, e.BytesDown)
	}
	if e.LastError != "boom" || e.LastErrorUnix == 0 || e.LastDialUnix == 0 {
		t.Fatalf("error/time fields not set: %+v", e)
	}
}

func TestFlows(t *testing.T) {
	f := NewFlows(0) // 0 → default 200
	fl := f.Begin("1.2.3.4:5", "example.com:443", "epA", "vless")
	if fl.ID != 1 || fl.Result != "open" {
		t.Fatalf("Begin: id=%d result=%q, want 1/open", fl.ID, fl.Result)
	}
	fl.Add(10, 20)
	fl.Add(5, 0)
	fl.End(nil)
	if fl.BytesUp != 15 || fl.BytesDown != 20 {
		t.Fatalf("flow bytes up=%d down=%d, want 15/20", fl.BytesUp, fl.BytesDown)
	}
	if fl.Result != "ok" || fl.ClosedUnix == 0 {
		t.Fatalf("End(nil) → result=%q closed=%d", fl.Result, fl.ClosedUnix)
	}

	errFlow := f.Begin("c", "d", "e", "p")
	errFlow.End(errors.New("reset"))
	if errFlow.Result != "error: reset" {
		t.Fatalf("End(err) result = %q", errFlow.Result)
	}
	if got := f.Snapshot(); len(got) != 2 {
		t.Fatalf("Snapshot len = %d, want 2", len(got))
	}
}

func TestFlowsRingBufferCap(t *testing.T) {
	f := NewFlows(3)
	for i := 0; i < 10; i++ {
		f.Begin("c", "d", "e", "p")
	}
	snap := f.Snapshot()
	if len(snap) != 3 {
		t.Fatalf("ring cap not enforced: len = %d, want 3", len(snap))
	}
	// Oldest-first, and only the last 3 IDs (8,9,10) survive.
	if snap[0].ID != 8 || snap[2].ID != 10 {
		t.Fatalf("ring kept wrong entries: first=%d last=%d, want 8..10", snap[0].ID, snap[2].ID)
	}
}

// /api/flows copies flows while connections close and count bytes; run with
// -race. Before the fix, Snapshot read ClosedUnix/Result while End wrote them.
func TestFlowsSnapshotConcurrentWithEnd(t *testing.T) {
	f := NewFlows(50)
	stop := make(chan struct{})
	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		for {
			select {
			case <-stop:
				return
			default:
				for _, fl := range f.Snapshot() {
					_ = fl.Result + strconv.FormatInt(fl.ClosedUnix+fl.BytesUp+fl.BytesDown, 10)
				}
			}
		}
	}()
	for i := 0; i < 200; i++ {
		fl := f.Begin("c", "d", "e", "p")
		fl.Add(1, 2)
		if i%2 == 0 {
			fl.End(nil)
		} else {
			fl.End(errors.New("reset"))
		}
	}
	close(stop)
	wg.Wait()

	for _, fl := range f.Snapshot() {
		if fl.Result == "open" || fl.ClosedUnix == 0 || fl.BytesUp != 1 || fl.BytesDown != 2 {
			t.Fatalf("flow %d not settled: %+v", fl.ID, fl)
		}
	}
}

// /api/stats copies the counters while tunnels count bytes and dials land;
// run with -race. Before the fix, Snapshot copied each EndpointStat whole
// while countingConn.Read/Write updated it with sync/atomic.
func TestStatsSnapshotConcurrentWithTraffic(t *testing.T) {
	const writes = 500
	b := New(StrategyLatency)
	local, remote := net.Pipe()
	defer remote.Close()
	conn := &countingConn{Conn: local, id: "a", stats: b.Stats(), flow: b.Flows().Begin("c", "d", "a", "p")}

	stop := make(chan struct{})
	var readers, peers sync.WaitGroup
	readers.Add(1)
	go func() {
		defer readers.Done()
		for {
			select {
			case <-stop:
				return
			default:
				for _, st := range b.Stats().Snapshot() {
					_ = st.LastError + strconv.FormatInt(st.Dials+st.DialErrors+st.Failovers+st.BytesUp+st.BytesDown+st.LastDialUnix+st.LastErrorUnix, 10)
				}
			}
		}
	}()
	// The far end echoes, so Write (up) and Read (down) both count.
	peers.Add(2)
	go func() {
		defer peers.Done()
		_, _ = io.Copy(remote, remote)
	}()
	go func() {
		defer peers.Done()
		_, _ = io.Copy(io.Discard, conn)
	}()

	for i := 0; i < writes; i++ {
		if _, err := conn.Write([]byte("x")); err != nil {
			t.Fatalf("write %d: %v", i, err)
		}
		b.Stats().RecordDial("a", errors.New("boom"))
		b.Stats().RecordFailover("a")
	}
	// Closing our end stops the echo; closing the far end then stops the reader.
	_ = conn.Close()
	_ = remote.Close()
	peers.Wait()
	close(stop)
	readers.Wait()

	snap := b.Stats().Snapshot()
	if len(snap) != 1 {
		t.Fatalf("Snapshot len = %d, want 1", len(snap))
	}
	st := snap[0]
	if st.ID != "a" || st.Dials != writes || st.DialErrors != writes || st.Failovers != writes || st.BytesUp != writes {
		t.Fatalf("counters wrong: %+v", st)
	}
	// net.Pipe is unbuffered: only the last echo can be cut off by Close.
	if st.BytesDown < writes-1 || st.BytesDown > writes {
		t.Fatalf("bytes down = %d, want %d or %d", st.BytesDown, writes-1, writes)
	}
	if st.LastError != "boom" || st.LastErrorUnix == 0 || st.LastDialUnix == 0 {
		t.Fatalf("error/time fields not set: %+v", st)
	}
}

// Snapshot copies EndpointStat field by field, so a field added to the struct
// but not to Snapshot would silently read as zero in /api/stats.
func TestStatsSnapshotCopiesEveryField(t *testing.T) {
	s := NewStats()
	e := s.entry("a")
	v := reflect.ValueOf(e).Elem()
	for i := 0; i < v.NumField(); i++ {
		switch f := v.Field(i); f.Kind() {
		case reflect.Int64:
			f.SetInt(int64(i + 1))
		case reflect.String:
			f.SetString("f" + strconv.Itoa(i))
		default:
			t.Fatalf("EndpointStat.%s: unhandled kind %s, extend this test", v.Type().Field(i).Name, f.Kind())
		}
	}
	snap := s.Snapshot()
	if len(snap) != 1 || snap[0] != *e {
		t.Fatalf("Snapshot = %+v, want %+v", snap, *e)
	}
}
