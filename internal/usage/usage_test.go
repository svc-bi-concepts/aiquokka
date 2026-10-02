package usage

import (
	"testing"
	"time"
)

// TestWindowResetsInUnknown covers the "we don't know when it resets" case:
// a zero ResetsAt must yield the -1 sentinel, not a duration measured from
// the zero time.Time.
func TestWindowResetsInUnknown(t *testing.T) {
	now := time.Date(2026, time.October, 2, 12, 0, 0, 0, time.UTC)

	if got := (Window{}).ResetsIn(now); got != -1 {
		t.Fatalf("zero-value Window.ResetsIn(now) = %v, want -1", got)
	}
	if got := (Window{Label: "5h", UsedPercent: ptr(50.0)}).ResetsIn(now); got != -1 {
		t.Fatalf("Window with populated fields but zero ResetsAt: ResetsIn(now) = %v, want -1", got)
	}
}

// TestWindowResetsIn is the table-driven check for known reset times.
func TestWindowResetsIn(t *testing.T) {
	now := time.Date(2026, time.October, 2, 12, 0, 0, 0, time.UTC)
	tests := []struct {
		name     string
		resetsAt time.Time
		want     time.Duration
	}{
		{"resets in five hours", now.Add(5 * time.Hour), 5 * time.Hour},
		{"resets in 90 minutes", now.Add(90 * time.Minute), 90 * time.Minute},
		{"resets in under a second", now.Add(250 * time.Millisecond), 250 * time.Millisecond},
		{"resets in 25 hours (crosses a day)", now.Add(25 * time.Hour), 25 * time.Hour},
		{"resets exactly now", now, 0},
		// Sub-second boundary cases. time.Duration is an integer count of
		// nanoseconds, so ResetsAt == now-1ns exercises the exact value of d
		// (-1ns) where "d < 0" and "d < -1" diverge: the contract demands 0,
		// never a tiny negative duration. now-2ns pins the neighbour value and
		// now+1ns pins the smallest positive result.
		{"reset time passed by one nanosecond", now.Add(-time.Nanosecond), 0},
		{"reset time passed by two nanoseconds", now.Add(-2 * time.Nanosecond), 0},
		{"resets in exactly one nanosecond", now.Add(time.Nanosecond), time.Nanosecond},
		{"reset time already passed by a second", now.Add(-time.Second), 0},
		{"reset time passed hours ago", now.Add(-3 * time.Hour), 0},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			w := Window{Label: "5h", ResetsAt: tt.resetsAt}
			if got := w.ResetsIn(now); got != tt.want {
				t.Fatalf("Window{ResetsAt: %v}.ResetsIn(%v) = %v, want %v",
					tt.resetsAt, now, got, tt.want)
			}
		})
	}
}

func ptr(f float64) *float64 { return &f }
