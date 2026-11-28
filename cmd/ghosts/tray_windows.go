package main

import (
	"encoding/json"
	"flag"
	"github.com/xamjiang/ghosts/internal/ports"
	"io"
	"os"
	"time"
)

var trayFeed = flag.Bool("tray-feed", false, "stream development listeners to the Windows notification area app")

type trayRow struct {
	Name, Address, Title, Source, Service, URL string
	Memory                                     uint64
	NewPort                                    bool
}

func runDesktopMode(interval time.Duration) bool {
	if !*trayFeed {
		return false
	}
	// The tray owns stdin. Closing it also ends this worker if the tray crashes.
	go func() { io.Copy(io.Discard, os.Stdin); os.Exit(0) }()
	enc := json.NewEncoder(os.Stdout)
	scanner := ports.NewScanner()
	for {
		msg := struct {
			Rows  []trayRow
			Error string
		}{Rows: []trayRow{}}
		snap, err := scanner.Scan()
		if err != nil {
			msg.Error = err.Error()
		} else {
			for _, l := range snap.Listeners {
				if l.Kind != ports.KindDev {
					continue
				}
				r := trayRow{Name: l.Name, Address: l.Address(), Title: l.Title, Source: l.Source, Service: l.Service(), Memory: l.Usage.Mem, NewPort: l.NewPort}
				if l.WebScheme != "" {
					r.URL = l.WebScheme + "://" + r.Address
				}
				msg.Rows = append(msg.Rows, r)
			}
		}
		if enc.Encode(msg) != nil {
			return true
		}
		time.Sleep(max(interval, 250*time.Millisecond))
	}
}
