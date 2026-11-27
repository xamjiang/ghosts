package docker

import (
	"context"
	"fmt"
	"github.com/Microsoft/go-winio"
	"io"
	"net/http"
	"os"
	"testing"
	"time"
)

func TestWindowsEnginePipe(t *testing.T) {
	name := fmt.Sprintf("ghosts-test-%d-%d", os.Getpid(), time.Now().UnixNano())
	ln, err := winio.ListenPipe(`\\.\pipe\`+name, nil)
	if err != nil {
		t.Fatal(err)
	}
	t.Setenv("DOCKER_HOST", "npipe:////./pipe/"+name)
	calls := make(chan string, 8)
	server := http.Server{Handler: http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls <- r.Method + " " + r.URL.Path
		switch r.URL.Path {
		case "/containers/json":
			io.WriteString(w, `[{"Id":"test","Names":["/web"],"State":"running","Labels":{"com.docker.compose.project":"demo"},"Ports":[{"PublicPort":5173,"PrivatePort":80,"Type":"tcp"}]}]`)
		case "/containers/test/stats":
			io.WriteString(w, `{"num_procs":1,"memory_stats":{"privateworkingset":123456}}`)
		case "/events":
			io.WriteString(w, `{"Action":"start"}`)
		default:
			w.WriteHeader(http.StatusNoContent)
		}
	})}
	go server.Serve(ln)
	defer server.Close()
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	client := New()
	rows, err := client.List(ctx)
	if err != nil || len(rows) != 1 || rows[0].Project != "demo" || len(rows[0].Ports) != 1 {
		t.Fatalf("list: %+v %v", rows, err)
	}
	client.FillStats(ctx, rows)
	if rows[0].Mem != 123456 {
		t.Fatal(rows[0].Mem)
	}
	connected, notified := false, false
	client.Watch(ctx, func() { connected = true }, func() { notified = true })
	if !connected || !notified {
		t.Error("missing event")
	}
	if err := client.Stop(ctx, "test"); err != nil {
		t.Fatal(err)
	}
	if err := client.Kill(ctx, "test"); err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"GET /containers/json", "GET /containers/test/stats", "GET /events", "POST /containers/test/stop", "POST /containers/test/kill"} {
		if got := <-calls; got != want {
			t.Errorf("got %s, want %s", got, want)
		}
	}
}
