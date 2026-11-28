//go:build darwin || linux

package main

import "time"

func runDesktopMode(time.Duration) bool { return false }
