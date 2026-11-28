package ports

import (
	"context"
	"crypto/tls"
	"io"
	"net"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"
	"unicode"

	"golang.org/x/net/html"
)

var serviceNames = map[string]string{
	"postgres": "PostgreSQL", "postmaster": "PostgreSQL", "mysqld": "MySQL",
	"mariadbd": "MariaDB", "redis-server": "Redis", "mongod": "MongoDB",
	"sshd": "SSH", "memcached": "Memcached", "adb": "ADB",
}

func (l Listener) Service() string { return serviceNames[strings.ToLower(l.Name)] }

func (l Listener) Address() string {
	if len(l.Addrs) == 0 {
		return ""
	}
	host := l.Addrs[0]
	for _, addr := range l.Addrs {
		if addr == "*" {
			host = "localhost"
			break
		}
		if net.ParseIP(addr).IsLoopback() {
			host = addr
			break
		}
	}
	return net.JoinHostPort(host, strconv.Itoa(l.Port))
}

type titleEntry struct {
	title, scheme    string
	updated, started time.Time
	address          string
}

// Called under Scanner.mu. Refresh only expired entries, with limited concurrency
// and one deadline for the entire scan, even when many listeners don't speak HTTP.
func (s *Scanner) readTitles(rows []Listener) {
	now := time.Now()
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	next := make(map[string]titleEntry)
	var workers sync.WaitGroup
	slots := make(chan struct{}, 4)
	updated := make([]bool, len(rows))
	for i := range rows {
		l := &rows[i]
		if l.Kind != KindDev || l.Service() != "" || l.Address() == "" {
			continue
		}
		key := l.Key()
		if prev, ok := s.titles[key]; ok && prev.started.Equal(l.Started) && prev.address == l.Address() && now.Sub(prev.updated) < time.Minute {
			l.Title, l.WebScheme = prev.title, prev.scheme
			next[key] = prev
			continue
		}
		select {
		case slots <- struct{}{}:
			workers.Add(1)
			go func() {
				defer workers.Done()
				defer func() { <-slots }()
				l.Title, l.WebScheme = inspectPage(ctx, l.Address())
				updated[i] = true
			}()
		case <-ctx.Done():
		}
	}
	workers.Wait()
	for i, l := range rows {
		if updated[i] {
			next[l.Key()] = titleEntry{l.Title, l.WebScheme, now, l.Started, l.Address()}
		}
	}
	s.titles = next
}

// Requests target a socket identified by the local process scanner. Allow local
// development certificates, and keep redirects on the same scheme and endpoint.
var titleClient = &http.Client{
	Timeout:   750 * time.Millisecond,
	Transport: &http.Transport{DisableKeepAlives: true, TLSClientConfig: &tls.Config{InsecureSkipVerify: true}},
	CheckRedirect: func(req *http.Request, via []*http.Request) error {
		if len(via) > 3 || req.URL.Host != via[0].URL.Host || req.URL.Scheme != via[0].URL.Scheme {
			return http.ErrUseLastResponse
		}
		return nil
	},
}

func inspectPage(ctx context.Context, address string) (title, schemeUsed string) {
	for _, scheme := range []string{"https", "http"} {
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, scheme+"://"+address+"/", nil)
		if err != nil {
			return "", ""
		}
		res, err := titleClient.Do(req)
		if err != nil {
			continue
		}
		defer res.Body.Close()
		if res.StatusCode < 200 || res.StatusCode >= 300 {
			return "", scheme
		}
		z := html.NewTokenizer(io.LimitReader(res.Body, 128<<10))
		for {
			switch z.Next() {
			case html.ErrorToken:
				return "", scheme
			case html.StartTagToken:
				name, _ := z.TagName()
				switch string(name) {
				case "body":
					return "", scheme
				case "title":
					if z.Next() != html.TextToken {
						return "", scheme
					}
					plain := strings.Map(func(r rune) rune {
						if unicode.IsControl(r) || unicode.In(r, unicode.Cf) {
							return ' '
						}
						return r
					}, string(z.Text()))
					runes := []rune(strings.Join(strings.Fields(plain), " "))
					return string(runes[:min(len(runes), 200)]), scheme
				}
			}
		}
	}
	return "", ""
}
