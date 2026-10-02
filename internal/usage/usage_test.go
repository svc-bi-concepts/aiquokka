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

func TestWindowPace(t *testing.T) {
	now := time.Date(2026, time.October, 2, 17, 30, 0, 0, time.UTC)

	tests := []struct {
		name     string
		duration time.Duration
		resetsAt time.Time
		want     float64
	}{
		{
			name:     "zero duration returns -1",
			duration: 0,
			resetsAt: now.Add(5 * time.Hour),
			want:     -1,
		},
		{
			name:     "negative duration returns -1",
			duration: -time.Hour,
			resetsAt: now.Add(5 * time.Hour),
			want:     -1,
		},
		{
			name:     "unknown reset time returns -1",
			duration: 5 * time.Hour,
			resetsAt: time.Time{},
			want:     -1,
		},
		{
			name:     "one nanosecond duration is a valid window",
			duration: time.Nanosecond,
			resetsAt: now.Add(time.Nanosecond),
			want:     0,
		},
		{
			name:     "now exactly at window start returns 0",
			duration: 4 * time.Hour,
			resetsAt: now.Add(4 * time.Hour),
			want:     0,
		},
		{
			name:     "now before window start clamps to 0",
			duration: 4 * time.Hour,
			resetsAt: now.Add(6 * time.Hour), // window start is now+2h, frac = -0.5
			want:     0,
		},
		{
			name:     "quarter of window elapsed returns 0.25",
			duration: 4 * time.Hour,
			resetsAt: now.Add(3 * time.Hour), // window start is now-1h, elapsed = 1h of 4h
			want:     0.25,
		},
		{
			name:     "one nanosecond after window start",
			duration: time.Second,
			resetsAt: now.Add(time.Second - time.Nanosecond), // window start is now-1ns
			want:     1e-9,
		},
		{
			name:     "one second before reset returns 0.9",
			duration: 10 * time.Second,
			resetsAt: now.Add(time.Second), // 9s of 10s elapsed
			want:     0.9,
		},
		{
			name:     "now exactly at reset returns 1",
			duration: 4 * time.Hour,
			resetsAt: now,
			want:     1,
		},
		{
			name:     "now after reset clamps to 1",
			duration: 4 * time.Hour,
			resetsAt: now.Add(-time.Hour), // frac = 1.25
			want:     1,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			w := Window{Label: "5h", Duration: tt.duration, ResetsAt: tt.resetsAt}
			if got := w.Pace(now); got != tt.want {
				t.Fatalf("Window.Pace(now) = %v, want %v", got, tt.want)
			}
		})
	}
}
