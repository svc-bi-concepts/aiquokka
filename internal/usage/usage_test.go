package usage

import (
	"testing"
	"time"
)

func TestWindowResetsIn(t *testing.T) {
	now := time.Date(2026, time.October, 2, 17, 30, 0, 0, time.UTC)

	tests := []struct {
		name     string
		resetsAt time.Time
		want     time.Duration
	}{
		{
			name:     "unknown reset returns -1",
			resetsAt: time.Time{},
			want:     -1,
		},
		{
			name:     "reset in the future returns remaining time",
			resetsAt: now.Add(90 * time.Minute),
			want:     90 * time.Minute,
		},
		{
			name:     "reset in the future by a sub-second remainder",
			resetsAt: now.Add(1500 * time.Millisecond),
			want:     1500 * time.Millisecond,
		},
		{
			name:     "reset exactly now returns 0",
			resetsAt: now,
			want:     0,
		},
		{
			name:     "reset in the past returns 0, never negative",
			resetsAt: now.Add(-time.Hour),
			want:     0,
		},
		{
			name:     "reset one nanosecond in the past returns 0",
			resetsAt: now.Add(-time.Nanosecond),
			want:     0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			w := Window{Label: "5h", ResetsAt: tt.resetsAt}
			if got := w.ResetsIn(now); got != tt.want {
				t.Fatalf("Window.ResetsIn(now) = %v, want %v", got, tt.want)
			}
		})
	}
}
