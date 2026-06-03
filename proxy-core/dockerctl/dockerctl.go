// Package dockerctl talks to the Docker Engine API over the local Unix socket
// at /var/run/docker.sock. We use it to start/stop sidecar containers when
// the user toggles a sidecar endpoint on/off from the dashboard.
//
// The socket must be mounted into proxy-core for any of this to work; if it's
// absent the package's calls return ErrSocketUnavailable and the caller is
// expected to fall back to "endpoint-pool-only" semantics (the sidecar may
// keep running in the background; it just won't be dialed).
package dockerctl

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"net/http"
	"net/url"
	"os"
	"strings"
	"time"
)

// ErrSocketUnavailable means /var/run/docker.sock isn't mounted or readable.
var ErrSocketUnavailable = errors.New("dockerctl: /var/run/docker.sock unavailable")

const socketPath = "/var/run/docker.sock"

// Available returns true if the docker socket is readable.
func Available() bool {
	if _, err := os.Stat(socketPath); err != nil {
		return false
	}
	return true
}

// Client is a thin Engine API client. Zero-value is usable.
type Client struct {
	http *http.Client
}

// New returns a Client that talks to the local docker socket.
func New() *Client {
	return &Client{
		http: &http.Client{
			Timeout: 8 * time.Second,
			Transport: &http.Transport{
				DialContext: func(_ context.Context, _, _ string) (net.Conn, error) {
					return net.Dial("unix", socketPath)
				},
			},
		},
	}
}

// FindContainerByService returns the first container whose docker-compose
// service label matches name. Empty string + no error means "not found".
func (c *Client) FindContainerByService(ctx context.Context, name string) (string, error) {
	if !Available() {
		return "", ErrSocketUnavailable
	}
	filters := url.QueryEscape(fmt.Sprintf(`{"label":["com.docker.compose.service=%s"]}`, name))
	req, _ := http.NewRequestWithContext(ctx, "GET", "http://docker/containers/json?all=true&filters="+filters, nil)
	resp, err := c.http.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		return "", fmt.Errorf("docker list: %s", resp.Status)
	}
	var containers []struct {
		ID    string   `json:"Id"`
		Names []string `json:"Names"`
		State string   `json:"State"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&containers); err != nil {
		return "", err
	}
	if len(containers) == 0 {
		return "", nil
	}
	return containers[0].ID, nil
}

// Stop sends a stop request to the container. Returns nil if already stopped.
func (c *Client) Stop(ctx context.Context, id string) error {
	if !Available() {
		return ErrSocketUnavailable
	}
	req, _ := http.NewRequestWithContext(ctx, "POST", "http://docker/containers/"+id+"/stop?t=5", nil)
	resp, err := c.http.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode == 204 || resp.StatusCode == 304 {
		return nil
	}
	return fmt.Errorf("docker stop %s: %s", id, resp.Status)
}

// Start (re)starts the container. Returns nil if already running.
func (c *Client) Start(ctx context.Context, id string) error {
	if !Available() {
		return ErrSocketUnavailable
	}
	req, _ := http.NewRequestWithContext(ctx, "POST", "http://docker/containers/"+id+"/start", nil)
	resp, err := c.http.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode == 204 || resp.StatusCode == 304 {
		return nil
	}
	return fmt.Errorf("docker start %s: %s", id, resp.Status)
}

// Restart issues a stop+start in one Engine API call. Used by handle reload
// to hot-cycle proxy-core / singbox / xray after config edits.
func (c *Client) Restart(ctx context.Context, id string) error {
	if !Available() {
		return ErrSocketUnavailable
	}
	req, _ := http.NewRequestWithContext(ctx, "POST", "http://docker/containers/"+id+"/restart?t=5", nil)
	resp, err := c.http.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode == 204 {
		return nil
	}
	return fmt.Errorf("docker restart %s: %s", id, resp.Status)
}

// SidecarDockerService maps a SidecarManager "sidecar_kind" to its
// docker-compose service name. Keep this in lock-step with the services
// declared in docker-compose.yml.
func SidecarDockerService(kind string) string {
	switch strings.ToLower(kind) {
	case "masterdns":
		return "masterdns"
	case "amneziawg":
		return "amneziawg"
	case "trusttunnel":
		return "trusttunnel"
	case "psiphon":
		return "psiphon"
	case "tor":
		return "tor"
	case "dnstt":
		return "dns-tunnels"
	default:
		return kind
	}
}
