package lyrics

import (
	"testing"
	"time"
)

func TestActiveLineIndex(t *testing.T) {
	lines := []Line{
		{Start: 5 * time.Second, Text: "first"},
		{Start: 10 * time.Second, Text: "second"},
		{Start: 20 * time.Second, Text: "third"},
	}

	cases := []struct {
		name string
		pos  time.Duration
		want int
	}{
		{"before the first line", 2 * time.Second, -1},
		{"exactly on a start", 10 * time.Second, 1},
		{"between two lines", 12 * time.Second, 1},
		{"after the last line", time.Hour, 2},
		{"at zero", 0, -1},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := ActiveLineIndex(lines, tc.pos); got != tc.want {
				t.Errorf("ActiveLineIndex(%v) = %d, want %d", tc.pos, got, tc.want)
			}
		})
	}

	if got := ActiveLineIndex(nil, time.Minute); got != -1 {
		t.Errorf("ActiveLineIndex(nil) = %d, want -1", got)
	}
}
