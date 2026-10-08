package usage

import (
	"encoding/json"
	"strings"
	"testing"
	"time"

	"gopkg.in/yaml.v3"
)

func TestWindowJSONDurationSeconds(t *testing.T) {
	pct := 12.5
	b, err := json.Marshal(Window{Label: "5h", UsedPercent: &pct, Duration: 5 * time.Hour})
	if err != nil {
		t.Fatal(err)
	}
	got := string(b)
	for _, want := range []string{`"duration_seconds":18000`, `"label":"5h"`, `"used_percent":12.5`} {
		if !strings.Contains(got, want) {
			t.Fatalf("JSON = %s, want it to contain %s", got, want)
		}
	}
	if strings.Contains(got, "Duration") {
		t.Fatalf("JSON = %s, want no raw Duration field", got)
	}
}

func TestWindowJSONOmitsUnknownDuration(t *testing.T) {
	b, err := json.Marshal(Window{Label: "Credits"})
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(b), "duration_seconds") {
		t.Fatalf("JSON = %s, want no duration_seconds without a Duration", b)
	}
}

func TestReportJSONCarriesWindowDuration(t *testing.T) {
	r := Report{Provider: "Claude", Windows: []Window{{Label: "Weekly", Duration: 7 * 24 * time.Hour}}}
	b, err := json.Marshal(r)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(b), `"duration_seconds":604800`) {
		t.Fatalf("JSON = %s, want duration_seconds 604800 on the window", b)
	}
}

func TestWindowYAMLDurationSeconds(t *testing.T) {
	b, err := yaml.Marshal(Report{Provider: "Codex", Windows: []Window{
		{Label: "Weekly", Duration: 7 * 24 * time.Hour},
		{Label: "Credits"},
	}})
	if err != nil {
		t.Fatal(err)
	}
	got := string(b)
	if !strings.Contains(got, "label: Weekly\n      duration_seconds: 604800\n") {
		t.Fatalf("YAML =\n%s\nwant label and duration_seconds: 604800 on the Weekly window", got)
	}
	if strings.Count(got, "duration_seconds") != 1 {
		t.Fatalf("YAML =\n%s\nwant duration_seconds only on the window with a Duration", got)
	}
}
