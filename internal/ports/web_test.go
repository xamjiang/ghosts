package ports

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestLocalPageTitle(t *testing.T) {
	for _, secure := range []bool{false, true} {
		srv := httptest.NewUnstartedServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if r.URL.Path == "/" {
				http.Redirect(w, r, "/app", 302)
				return
			}
			io.WriteString(w, "<html><head><TITLE lang='zh'>Ghosts &amp;\n 本機</TITLE></head></html>")
		}))
		if secure {
			srv.StartTLS()
		} else {
			srv.Start()
		}
		title, scheme := inspectPage(context.Background(), strings.SplitN(srv.URL, "://", 2)[1])
		srv.Close()
		if scheme != strings.SplitN(srv.URL, "://", 2)[0] || title != "Ghosts & 本機" {
			t.Fatalf("TLS=%v, got %q", secure, title)
		}
	}
}
