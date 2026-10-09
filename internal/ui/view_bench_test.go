package ui

import (
	"testing"

	tea "charm.land/bubbletea/v2"

	"github.com/xamjiang/ghosts/internal/docker"
)

func BenchmarkView(b *testing.B) {
	m := New(Options{}, docker.New())
	snap, _ := m.scanner.Scan()
	m.update(tea.WindowSizeMsg{Width: 150, Height: 30})
	m.update(portsMsg{snap: snap})
	m.update(dockerMsg{err: nil})
	m.showAll = true
	m.rebuild()
	b.ResetTimer()
	for b.Loop() {
		_ = m.View()
	}
}
