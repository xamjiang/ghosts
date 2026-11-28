//go:build darwin || linux

package docker

import (
	"context"
	"net"
	"os"
	"path/filepath"
	"strings"
)

func dialSocket(ctx context.Context, sock string) (net.Conn, error) {
	var d net.Dialer
	return d.DialContext(ctx, "unix", sock)
}

func findSocket() string {
	if h := os.Getenv("DOCKER_HOST"); strings.HasPrefix(h, "unix://") {
		return strings.TrimPrefix(h, "unix://")
	}
	if s, ok := strings.CutPrefix(contextHost(), "unix://"); ok && s != "" {
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
