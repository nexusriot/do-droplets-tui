package tui

import (
	"math"
	"strings"
	"testing"
	"time"

	"github.com/nexusriot/do-droplets-tui/internal/do"
)

func TestNextImageMode(t *testing.T) {
	cases := []struct{ in, want string }{
		{"user", "distribution"},
		{"distribution", "application"},
		{"application", "user"},
		{"", "user"}, // unknown → reset
		{"tag:foo", "user"},
	}
	for _, c := range cases {
		if got := nextImageMode(c.in); got != c.want {
			t.Errorf("nextImageMode(%q) = %q want %q", c.in, got, c.want)
		}
	}
}

func TestIsUserImageMode(t *testing.T) {
	cases := map[string]bool{
		"user":         true,
		"distribution": false,
		"application":  false,
		"":             false,
		"tag:prod":     true,
	}
	for in, want := range cases {
		if got := isUserImageMode(in); got != want {
			t.Errorf("isUserImageMode(%q) = %v want %v", in, got, want)
		}
	}
}

func TestFirstRegionOf(t *testing.T) {
	cases := map[string]string{
		"":              "",
		"   ":           "",
		"nyc3":          "nyc3",
		"nyc3,fra1":     "nyc3",
		// TrimSpace runs on the whole string before the comma split, so
		// leading whitespace is stripped but the first item keeps any
		// internal trailing space before the comma.
		"  nyc3 ,fra1 ": "nyc3 ",
	}
	for in, want := range cases {
		if got := firstRegionOf(in); got != want {
			t.Errorf("firstRegionOf(%q) = %q want %q", in, got, want)
		}
	}
}

func TestSanitizeNameSuggestion(t *testing.T) {
	cases := map[string]string{
		"My App v2.0":   "my-app-v2-0",
		"  ":            "from-image",
		"":              "from-image",
		"snap_2024":     "snap-2024",
		"-dash-leading": "dash-leading",
		"trail---":      "trail",
	}
	for in, want := range cases {
		if got := sanitizeNameSuggestion(in); got != want {
			t.Errorf("sanitizeNameSuggestion(%q) = %q want %q", in, got, want)
		}
	}
}

func TestBucketize(t *testing.T) {
	// Shorter than width → returned as-is.
	in := []float64{1, 2, 3}
	got := bucketize(in, 10)
	if len(got) != len(in) {
		t.Errorf("len=%d want %d", len(got), len(in))
	}

	// 10 values into 5 buckets → averages of pairs.
	in = []float64{1, 3, 5, 7, 9, 11, 13, 15, 17, 19}
	got = bucketize(in, 5)
	want := []float64{2, 6, 10, 14, 18}
	if len(got) != len(want) {
		t.Fatalf("len=%d want %d (%v)", len(got), len(want), got)
	}
	for i := range want {
		if math.Abs(got[i]-want[i]) > 1e-9 {
			t.Errorf("bucket %d = %v want %v", i, got[i], want[i])
		}
	}
}

func TestRenderSparkline(t *testing.T) {
	// Empty input → empty output.
	if got := renderSparkline(nil, 60); got != "" {
		t.Errorf("nil → %q", got)
	}
	// Width zero → defaults to 60.
	out := renderSparkline([]float64{0, 1, 2, 3, 4, 5}, 0)
	if utf8Len(out) != 6 {
		t.Errorf("expected 6 runes, got %d in %q", utf8Len(out), out)
	}
	// Monotone ascending → final char is the highest bar.
	last := []rune(out)[5]
	if last != '█' {
		t.Errorf("last bar = %q want %q", last, '█')
	}
	// Flat series → no panic, output length matches.
	out = renderSparkline([]float64{2, 2, 2, 2, 2}, 5)
	if utf8Len(out) != 5 {
		t.Errorf("flat: len=%d", utf8Len(out))
	}
}

func TestSummarizeSamples(t *testing.T) {
	now := time.Date(2024, 1, 1, 12, 0, 0, 0, time.UTC)
	s := []do.MetricSample{
		{When: now, Value: 10},
		{When: now.Add(time.Minute), Value: 30},
		{When: now.Add(2 * time.Minute), Value: 20},
	}
	st := summarizeSamples(s)
	if st.Min != 10 || st.Max != 30 || st.Avg != 20 {
		t.Errorf("stats = %+v", st)
	}
	if st.Span != 2*time.Minute {
		t.Errorf("span = %v want 2m", st.Span)
	}

	// Empty: no panic.
	st = summarizeSamples(nil)
	if st.Span != 0 || st.Avg != 0 {
		t.Errorf("empty: %+v", st)
	}
}

// utf8Len returns the rune count of s. Strings.Builder may produce multi-byte
// block elements (▁▂…), so len() != visible width.
func utf8Len(s string) int {
	n := 0
	for range s {
		n++
	}
	return n
}

// Smoke check that the well-known alert-type prefixes still match what
// shortAlertType strips.
func TestAlertTypePresetsAreStripped(t *testing.T) {
	for _, p := range alertTypePresets {
		got := shortAlertType(p)
		if strings.HasPrefix(got, "v1/insights/") {
			t.Errorf("shortAlertType(%q) = %q — prefix not stripped", p, got)
		}
	}
}
