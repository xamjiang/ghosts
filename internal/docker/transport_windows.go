package docker

import (
	"context"
	"fmt"
	"net"
	"os"
	"strings"

	"github.com/Microsoft/go-winio"
)

func findSocket() string {
	host := os.Getenv("DOCKER_HOST")
	if host == "" {
		host = contextHost()
	}
	if host == "" {
		return `\\.\pipe\docker_engine`
	}
	path, ok := strings.CutPrefix(host, "npipe://")
	path = strings.ReplaceAll(path, "/", `\`)
	if !ok || !strings.HasPrefix(strings.ToLower(path), `\\.\pipe\`) {
		return ""
	}
	return path
}

func dialSocket(ctx context.Context, sock string) (net.Conn, error) {
	if sock == "" {
		return nil, fmt.Errorf("Docker context must use a local Windows named pipe")
	}
	return winio.DialPipeContext(ctx, sock)
}
