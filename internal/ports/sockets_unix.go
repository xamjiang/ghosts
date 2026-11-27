//go:build darwin || linux

package ports

func scanSockets(pids []int) ([]socket, error) { return listenSockets(pids), nil }
