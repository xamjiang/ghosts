// The native menu bar app links this scanner into its executable.
package main

import "C"
import (
	"crypto/tls"
	"encoding/json"
	"net/http"
	"time"

	"github.com/xamjiang/ghosts/internal/ports"
)

var scanner = ports.NewScanner()

type row struct {
	Title   string `json:"title"`
	ID      string `json:"id"`
	Name    string `json:"name"`
	Address string `json:"address"`
	Service string `json:"service"`
	Memory  uint64 `json:"memory"`
}

//export ghostsSnapshot
func ghostsSnapshot() *C.char {
	snap, err := scanner.Scan()
	if err != nil {
		return nil
	}
	rows := make([]row, 0, len(snap.Listeners))
	for _, l := range snap.Listeners {
		if l.Kind != ports.KindDev {
			continue
		}
		rows = append(rows, row{
			Title: l.Title, ID: l.Key(), Name: l.Name, Address: l.Address(),
			Service: l.Service(), Memory: l.Usage.Mem,
		})
	}
	data, err := json.Marshal(rows)
	if err != nil {
		return nil
	}
	return C.CString(string(data))
}

// Check only on a user click. Keep redirects and certificate decisions in the browser.
//
//export ghostsWebScheme
func ghostsWebScheme(address *C.char) C.int {
	host := C.GoString(address)
	transport := &http.Transport{DisableKeepAlives: true, TLSClientConfig: &tls.Config{InsecureSkipVerify: true}}
	client := &http.Client{
		Transport: transport, Timeout: 2 * time.Second,
		CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse },
	}
	for i, scheme := range []string{"https", "http"} {
		response, err := client.Head(scheme + "://" + host + "/")
		if err == nil {
			response.Body.Close()
			return C.int(i + 1)
		}
	}
	return 0
}

func main() {}
