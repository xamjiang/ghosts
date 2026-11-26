package ports

import "testing"

func TestNewPortTransitions(t *testing.T) {
	s := NewScanner()
	for _, step := range []struct {
		rows []Listener
		want []bool
	}{
		{[]Listener{{Port: 3000}}, []bool{false}},
		{[]Listener{{Port: 3000}, {Port: 5173}, {Port: 5173}, {Port: 9999, Kind: KindSystem}}, []bool{false, true, false, false}},
		{[]Listener{{Port: 5173, PID: 10}}, []bool{false}},
		{nil, nil},
		{[]Listener{{Port: 5173}}, []bool{true}},
	} {
		s.markNewPorts(step.rows)
		for i, row := range step.rows {
			if row.NewPort != step.want[i] {
				t.Fatalf("port %d: got %v, want %v", row.Port, row.NewPort, step.want[i])
			}
		}
	}
}
