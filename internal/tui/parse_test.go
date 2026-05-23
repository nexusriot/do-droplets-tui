package tui

import (
	"reflect"
	"strings"
	"testing"

	"github.com/nexusriot/do-droplets-tui/internal/do"
)

func TestSplitCSV(t *testing.T) {
	cases := []struct {
		in   string
		want []string
	}{
		{"", nil},
		{"a", []string{"a"}},
		{"a,b,c", []string{"a", "b", "c"}},
		{" a , b ,c ", []string{"a", "b", "c"}},
		{",a,,b,", []string{"a", "b"}},
	}
	for _, c := range cases {
		got := splitCSV(c.in)
		if !reflect.DeepEqual(got, c.want) {
			t.Errorf("splitCSV(%q) = %v want %v", c.in, got, c.want)
		}
	}
}

func TestSignedBalance(t *testing.T) {
	cases := []struct {
		raw, creditLabel, owedLabel, wantLabel, wantDisplay string
	}{
		{"-15.66", "Prepayments", "Amount due", "Prepayments", "$15.66"},
		{"0", "Prepayments", "Amount due", "Amount due", "$0"},
		{"5.00", "Prepayments", "Amount due", "Amount due", "$5.00"},
		{"   ", "Prepayments", "Amount due", "Prepayments", "-"},
		{" -3.50 ", "Credit", "Owed", "Credit", "$3.50"},
	}
	for _, c := range cases {
		l, d := signedBalance(c.raw, c.creditLabel, c.owedLabel)
		if l != c.wantLabel || d != c.wantDisplay {
			t.Errorf("signedBalance(%q) = (%q,%q) want (%q,%q)",
				c.raw, l, d, c.wantLabel, c.wantDisplay)
		}
	}
}

func TestDispMoney(t *testing.T) {
	if got, want := dispMoney(""), "-"; got != want {
		t.Errorf("empty: got %q want %q", got, want)
	}
	if got, want := dispMoney("5.00"), "$5.00"; got != want {
		t.Errorf("got %q want %q", got, want)
	}
}

func TestShortAlertType(t *testing.T) {
	cases := map[string]string{
		"v1/insights/droplet/cpu":      "droplet/cpu",
		"v1/insights/lbaas/throughput": "lbaas/throughput",
		"droplet/memory":               "droplet/memory", // no prefix → unchanged
		"":                             "",
	}
	for in, want := range cases {
		if got := shortAlertType(in); got != want {
			t.Errorf("shortAlertType(%q) = %q want %q", in, got, want)
		}
	}
}

func TestDefaultIfEmpty(t *testing.T) {
	if got := defaultIfEmpty("", "X"); got != "X" {
		t.Errorf("empty: got %q want %q", got, "X")
	}
	if got := defaultIfEmpty("   ", "X"); got != "X" {
		t.Errorf("whitespace: got %q want %q", got, "X")
	}
	if got := defaultIfEmpty("hi", "X"); got != "hi" {
		t.Errorf("non-empty: got %q want %q", got, "hi")
	}
}

func TestParseRuleCSV(t *testing.T) {
	t.Run("empty", func(t *testing.T) {
		rules, err := parseRuleCSV("")
		if err != nil {
			t.Fatalf("err: %v", err)
		}
		if rules != nil {
			t.Fatalf("want nil rules, got %v", rules)
		}
	})

	t.Run("single tcp port", func(t *testing.T) {
		rules, err := parseRuleCSV("tcp:22")
		if err != nil {
			t.Fatalf("err: %v", err)
		}
		want := []do.FirewallRuleSpec{{
			Protocol:  "tcp",
			PortRange: "22",
			Addresses: []string{"0.0.0.0/0", "::/0"},
		}}
		if !reflect.DeepEqual(rules, want) {
			t.Fatalf("got %+v want %+v", rules, want)
		}
	})

	t.Run("multi mixed", func(t *testing.T) {
		rules, err := parseRuleCSV("tcp:22, tcp:80-90 , udp:53, icmp:")
		if err != nil {
			t.Fatalf("err: %v", err)
		}
		if len(rules) != 4 {
			t.Fatalf("want 4 rules, got %d", len(rules))
		}
		if rules[0].Protocol != "tcp" || rules[0].PortRange != "22" {
			t.Errorf("rules[0] = %+v", rules[0])
		}
		if rules[1].PortRange != "80-90" {
			t.Errorf("rules[1] ports = %q want %q", rules[1].PortRange, "80-90")
		}
		if rules[3].Protocol != "icmp" || rules[3].PortRange != "" {
			t.Errorf("rules[3] = %+v", rules[3])
		}
	})

	t.Run("case-insensitive proto", func(t *testing.T) {
		rules, err := parseRuleCSV("TCP:443")
		if err != nil {
			t.Fatalf("err: %v", err)
		}
		if rules[0].Protocol != "tcp" {
			t.Errorf("proto = %q want %q", rules[0].Protocol, "tcp")
		}
	})

	t.Run("bad protocol", func(t *testing.T) {
		_, err := parseRuleCSV("sctp:22")
		if err == nil {
			t.Fatal("want error")
		}
		if !strings.Contains(err.Error(), "sctp") {
			t.Errorf("err = %v, want to mention sctp", err)
		}
	})

	t.Run("skips empty entries", func(t *testing.T) {
		rules, err := parseRuleCSV("tcp:22,,tcp:80,")
		if err != nil {
			t.Fatalf("err: %v", err)
		}
		if len(rules) != 2 {
			t.Errorf("want 2 rules, got %d", len(rules))
		}
	})
}

func TestSelectedFromPicker(t *testing.T) {
	sel := map[int]bool{
		1: true,
		2: false,
		3: true,
	}
	got := selectedFromPicker(sel)
	// order is map-iteration-dependent — sort before compare
	if len(got) != 2 {
		t.Fatalf("len=%d want 2", len(got))
	}
	set := map[int]bool{}
	for _, id := range got {
		set[id] = true
	}
	if !set[1] || !set[3] || set[2] {
		t.Errorf("selected set = %v want {1,3}", set)
	}
}

func TestTitleCase(t *testing.T) {
	// titleCase is only called with ASCII verbs in the codebase ("add",
	// "remove"). It is intentionally byte-oriented, so multibyte inputs
	// would be mangled — not tested.
	cases := map[string]string{
		"":    "",
		"x":   "X",
		"add": "Add",
		"ADD": "ADD",
	}
	for in, want := range cases {
		if got := titleCase(in); got != want {
			t.Errorf("titleCase(%q) = %q want %q", in, got, want)
		}
	}
}

func TestPrep(t *testing.T) {
	if got := prep(true); got != "to" {
		t.Errorf("prep(true) = %q", got)
	}
	if got := prep(false); got != "from" {
		t.Errorf("prep(false) = %q", got)
	}
}
