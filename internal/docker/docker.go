// Package docker is a tiny Docker Engine API client over the local unix
// socket. It covers exactly what the monitor needs — list, stop, kill and the
// event stream — without pulling in the full Docker SDK.
package docker

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"
)

type PortMap struct {
	IP      string `json:"IP"`
	Private int    `json:"PrivatePort"`
	Public  int    `json:"PublicPort"`
	Type    string `json:"Type"`
}

type Container struct {
	ID         string
	Name       string
	Image      string
	State      string // running, paused, restarting…
	Status     string // "Up 2 hours (healthy)"
	Created    time.Time
	Ports      []PortMap // published ports only, deduplicated
	Project    string    // compose project
	Service    string    // compose service
	WorkingDir string    // compose project directory

	CPU        float64 // percent of one core, measured inside the VM
	Mem        uint64  // bytes, excluding reclaimable page cache (like `docker stats`)
	UsageReady bool
}

func (c Container) Key() string { return "c:" + c.ID }

// Health returns "healthy", "unhealthy", "starting" or "".
func (c Container) Health() string {
	switch {
	case strings.Contains(c.Status, "(unhealthy)"):
		return "unhealthy"
	case strings.Contains(c.Status, "(healthy)"):
		return "healthy"
	case strings.Contains(c.Status, "(health: starting)"):
		return "starting"
	}
	return ""
}

type Client struct {
	mu   sync.Mutex
	prev map[string]cpuSample // container ID → last CPU sample

	sock string
	http *http.Client
	// stream has no overall timeout so the event stream can stay open.
	stream *http.Client
}

// New finds the Docker socket the same way users expect the CLI to: DOCKER_HOST
// first, then the current docker context, then the usual Docker Desktop /
// OrbStack / Colima locations.
func New() *Client {
	sock := findSocket()
	dial := func(ctx context.Context, _, _ string) (net.Conn, error) {
		var d net.Dialer
		return d.DialContext(ctx, "unix", sock)
	}
	return &Client{
		prev:   map[string]cpuSample{},
		sock:   sock,
		http:   &http.Client{Timeout: 5 * time.Second, Transport: &http.Transport{DialContext: dial}},
		stream: &http.Client{Transport: &http.Transport{DialContext: dial}},
	}
}

func findSocket() string {
	if h := os.Getenv("DOCKER_HOST"); strings.HasPrefix(h, "unix://") {
		return strings.TrimPrefix(h, "unix://")
	}
	if s := contextSocket(); s != "" {
		return s
	}
	home, _ := os.UserHomeDir()
	candidates := []string{
		filepath.Join(home, ".docker/run/docker.sock"),
		filepath.Join(home, ".docker/desktop/docker.sock"), // Docker Desktop for Linux
		filepath.Join(home, ".orbstack/run/docker.sock"),
		filepath.Join(home, ".colima/default/docker.sock"),
		filepath.Join(home, ".rd/docker.sock"),
		"/var/run/docker.sock",
	}
	for _, c := range candidates {
		if fi, err := os.Stat(c); err == nil && fi.Mode()&os.ModeSocket != 0 {
			return c
		}
	}
	return "/var/run/docker.sock"
}

// contextSocket returns the socket of the docker CLI's current context
// ("docker context use"), which matters when several engines are installed,
// e.g. Docker Engine and Docker Desktop side by side on Linux.
func contextSocket() string {
	dir := os.Getenv("DOCKER_CONFIG")
	if dir == "" {
		home, _ := os.UserHomeDir()
		dir = filepath.Join(home, ".docker")
	}
	name := os.Getenv("DOCKER_CONTEXT")
	if name == "" {
		var cfg struct {
			CurrentContext string `json:"currentContext"`
		}
		if b, err := os.ReadFile(filepath.Join(dir, "config.json")); err == nil {
			json.Unmarshal(b, &cfg)
		}
		name = cfg.CurrentContext
	}
	if name == "" || name == "default" {
		return ""
	}
	// Context metadata lives in a directory named by the SHA-256 of its name.
	sum := sha256.Sum256([]byte(name))
	b, err := os.ReadFile(filepath.Join(dir, "contexts", "meta", hex.EncodeToString(sum[:]), "meta.json"))
	if err != nil {
		return ""
	}
	var meta struct {
		Endpoints struct {
			Docker struct {
				Host string `json:"Host"`
			} `json:"docker"`
		} `json:"Endpoints"`
	}
	if json.Unmarshal(b, &meta) != nil {
		return ""
	}
	h, ok := strings.CutPrefix(meta.Endpoints.Docker.Host, "unix://")
	if !ok {
		return ""
	}
	return h
}

func (c *Client) do(ctx context.Context, method, path string) (*http.Response, error) {
	req, err := http.NewRequestWithContext(ctx, method, "http://docker"+path, nil)
	if err != nil {
		return nil, err
	}
	return c.http.Do(req)
}

// List returns running containers sorted by compose project, then name.
func (c *Client) List(ctx context.Context) ([]Container, error) {
	resp, err := c.do(ctx, http.MethodGet, "/containers/json")
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, apiError(resp)
	}
	var raw []struct {
		ID      string            `json:"Id"`
		Names   []string          `json:"Names"`
		Image   string            `json:"Image"`
		State   string            `json:"State"`
		Status  string            `json:"Status"`
		Created int64             `json:"Created"`
		Ports   []PortMap         `json:"Ports"`
		Labels  map[string]string `json:"Labels"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&raw); err != nil {
		return nil, err
	}
	out := make([]Container, 0, len(raw))
	for _, r := range raw {
		ct := Container{
			ID:         r.ID,
			Image:      r.Image,
			State:      r.State,
			Status:     r.Status,
			Created:    time.Unix(r.Created, 0),
			Project:    r.Labels["com.docker.compose.project"],
			Service:    r.Labels["com.docker.compose.service"],
			WorkingDir: r.Labels["com.docker.compose.project.working_dir"],
		}
		if len(r.Names) > 0 {
			ct.Name = strings.TrimPrefix(r.Names[0], "/")
		}
		seen := map[[2]int]bool{}
		for _, p := range r.Ports {
			k := [2]int{p.Public, p.Private}
			if p.Public == 0 || seen[k] {
				continue
			}
			seen[k] = true
			ct.Ports = append(ct.Ports, p)
		}
		sort.Slice(ct.Ports, func(i, j int) bool { return ct.Ports[i].Public < ct.Ports[j].Public })
		out = append(out, ct)
	}
	sort.SliceStable(out, func(i, j int) bool {
		if out[i].Project != out[j].Project {
			// Standalone containers sort after compose projects.
			if out[i].Project == "" || out[j].Project == "" {
				return out[j].Project == ""
			}
			return out[i].Project < out[j].Project
		}
		return out[i].Name < out[j].Name
	})
	return out, nil
}

// Stop gracefully stops a container (SIGTERM, then SIGKILL after 10s).
func (c *Client) Stop(ctx context.Context, id string) error {
	return c.post(ctx, "/containers/"+url.PathEscape(id)+"/stop?t=10", 15*time.Second)
}

// Kill stops a container immediately.
func (c *Client) Kill(ctx context.Context, id string) error {
	return c.post(ctx, "/containers/"+url.PathEscape(id)+"/kill", 5*time.Second)
}

func (c *Client) post(ctx context.Context, path string, timeout time.Duration) error {
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, "http://docker"+path, nil)
	if err != nil {
		return err
	}
	resp, err := c.stream.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	switch resp.StatusCode {
	case http.StatusNoContent, http.StatusNotModified, http.StatusOK:
		return nil
	case http.StatusConflict: // kill on an already-stopped container
		return nil
	}
	return apiError(resp)
}

// Watch streams container events and calls notify on each one. It returns when
// the stream ends or ctx is cancelled; callers reconnect as they see fit.
// onConnect is called once the stream is established.
func (c *Client) Watch(ctx context.Context, onConnect, notify func()) error {
	filters := url.QueryEscape(`{"type":["container"]}`)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, "http://docker/events?filters="+filters, nil)
	if err != nil {
		return err
	}
	resp, err := c.stream.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return apiError(resp)
	}
	onConnect()
	dec := json.NewDecoder(resp.Body)
	for {
		var ev struct {
			Action string `json:"Action"`
		}
		if err := dec.Decode(&ev); err != nil {
			return err
		}
		// exec_* and top events are noise for a monitor.
		if strings.HasPrefix(ev.Action, "exec_") || ev.Action == "top" {
			continue
		}
		notify()
	}
}

func apiError(resp *http.Response) error {
	b, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
	var m struct {
		Message string `json:"message"`
	}
	if json.Unmarshal(b, &m) == nil && m.Message != "" {
		return fmt.Errorf("docker: %s", m.Message)
	}
	return fmt.Errorf("docker: %s", resp.Status)
}

type cpuSample struct {
	at    time.Time
	total uint64  // cumulative CPU ns
	cpu   float64 // last computed rate
	ready bool
}

// FillStats adds CPU and memory usage to running containers. It uses
// one-shot stats (a few ms each, fetched in parallel) instead of the default
// endpoint, which blocks for a full second per container to compute a delta.
// CPU is computed against the previous call, so it is ready from the second
// call on.
func (c *Client) FillStats(ctx context.Context, cs []Container) {
	ctx, cancel := context.WithTimeout(ctx, 3*time.Second)
	defer cancel()
	var wg sync.WaitGroup
	for i := range cs {
		if cs[i].State != "running" {
			continue
		}
		wg.Add(1)
		go func(ct *Container) {
			defer wg.Done()
			c.fillOne(ctx, ct)
		}(&cs[i])
	}
	wg.Wait()

	c.mu.Lock()
	defer c.mu.Unlock()
	live := map[string]bool{}
	for _, ct := range cs {
		live[ct.ID] = true
	}
	for id := range c.prev {
		if !live[id] {
			delete(c.prev, id)
		}
	}
}

func (c *Client) fillOne(ctx context.Context, ct *Container) {
	resp, err := c.do(ctx, http.MethodGet, "/containers/"+url.PathEscape(ct.ID)+"/stats?stream=false&one-shot=true")
	if err != nil {
		return
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return
	}
	var st struct {
		CPU struct {
			Usage struct {
				Total uint64 `json:"total_usage"`
			} `json:"cpu_usage"`
		} `json:"cpu_stats"`
		Mem struct {
			Usage uint64            `json:"usage"`
			Stats map[string]uint64 `json:"stats"`
		} `json:"memory_stats"`
	}
	if json.NewDecoder(resp.Body).Decode(&st) != nil {
		return
	}
	now := time.Now()
	mem := st.Mem.Usage
	cache := st.Mem.Stats["inactive_file"] // cgroup v2
	if cache == 0 {
		cache = st.Mem.Stats["total_inactive_file"] // cgroup v1
	}
	if cache < mem {
		mem -= cache
	}
	ct.Mem = mem

	c.mu.Lock()
	defer c.mu.Unlock()
	p, ok := c.prev[ct.ID]
	switch {
	case ok && st.CPU.Usage.Total >= p.total && now.Sub(p.at) < time.Second:
		// Called again too soon (an event burst): reuse the last rate so the
		// number stays steady instead of jumping on a tiny interval.
		ct.CPU, ct.UsageReady = p.cpu, p.ready
	case ok && st.CPU.Usage.Total >= p.total:
		dt := now.Sub(p.at)
		ct.CPU = float64(st.CPU.Usage.Total-p.total) / float64(dt.Nanoseconds()) * 100
		ct.UsageReady = true
		c.prev[ct.ID] = cpuSample{now, st.CPU.Usage.Total, ct.CPU, true}
	default:
		c.prev[ct.ID] = cpuSample{at: now, total: st.CPU.Usage.Total}
	}
}
