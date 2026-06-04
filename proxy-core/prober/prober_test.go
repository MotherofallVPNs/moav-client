package prober

import (
	"bytes"
	"log"
	"strings"
	"testing"

	"github.com/ibeezhan/moav-client/proxy-core/subscription"
)

// TestProbeAll_SkipsDisabled asserts that disabled endpoints are passed
// through unchanged AND excluded from the cycle summary's numerator and
// denominator. This is the contract the dashboard relies on so a toggled-off
// row doesn't masquerade as unhealthy in the Debug tab.
func TestProbeAll_SkipsDisabled(t *testing.T) {
	var buf bytes.Buffer
	prev := log.Writer()
	log.SetOutput(&buf)
	defer log.SetOutput(prev)

	p := New()

	// One disabled endpoint with a nonsense address. If we don't short-circuit,
	// the dial will time out and Status will be set. We assert that it stays
	// untouched ("preserved-status") below.
	disabled := subscription.Endpoint{
		ID:       "vless:disabled.example:443",
		Name:     "off-endpoint",
		Protocol: "vless",
		Address:  "disabled.example:443",
		Status:   "preserved-status",
		Enabled:  false,
	}

	results := p.ProbeAll([]subscription.Endpoint{disabled})
	if got := results[0].Status; got != "preserved-status" {
		t.Errorf("disabled endpoint status: want preserved-status, got %q", got)
	}

	out := buf.String()
	if strings.Contains(out, "endpoints unhealthy") {
		t.Errorf("cycle summary should not flag disabled endpoint as unhealthy; got: %s", out)
	}
	if !strings.Contains(out, "1 disabled") && !strings.Contains(out, "(1 disabled)") {
		// When ALL inputs are disabled there's no probedCount > 0, so no
		// summary line is emitted at all. That's also fine — silent vs
		// "(1 disabled)" appended to "all N ok". Either is acceptable.
		if strings.Contains(out, "probe cycle:") {
			t.Errorf("cycle summary missing disabled-count suffix when present; got: %s", out)
		}
	}
}
