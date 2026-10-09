package ports

import "testing"

func BenchmarkScan(b *testing.B) {
	s := NewScanner()
	s.Scan()
	for b.Loop() {
		s.Scan()
	}
}

func BenchmarkSockets(b *testing.B) {
	t, _ := readProcTable()
	for b.Loop() {
		listenSockets(t.own)
	}
}

func BenchmarkProcTable(b *testing.B) {
	for b.Loop() {
		readProcTable()
	}
}
