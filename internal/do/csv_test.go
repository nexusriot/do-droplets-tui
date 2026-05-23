package do

import (
	"reflect"
	"testing"

	"github.com/digitalocean/godo"
)

func TestParseCSVInts(t *testing.T) {
	cases := []struct {
		name    string
		in      string
		want    []int
		wantErr bool
	}{
		{"empty", "", nil, false},
		{"whitespace only", "   ", nil, false},
		{"single", "42", []int{42}, false},
		{"multi", "1,2,3", []int{1, 2, 3}, false},
		{"padded", " 1 , 2 ,3 ", []int{1, 2, 3}, false},
		{"trailing comma", "1,2,", []int{1, 2}, false},
		{"embedded empties", "1,,2", []int{1, 2}, false},
		{"negative ok", "1,-2,3", []int{1, -2, 3}, false},
		{"bad token", "1,foo,3", nil, true},
		{"bad token only", "abc", nil, true},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got, err := ParseCSVInts(c.in)
			if (err != nil) != c.wantErr {
				t.Fatalf("err=%v wantErr=%v", err, c.wantErr)
			}
			if err == nil && !reflect.DeepEqual(got, c.want) {
				t.Fatalf("got %v want %v", got, c.want)
			}
		})
	}
}

func TestFirstIPv4(t *testing.T) {
	if got := firstIPv4(nil); got != "" {
		t.Fatalf("nil networks: got %q want %q", got, "")
	}

	empty := &godo.Networks{}
	if got := firstIPv4(empty); got != "" {
		t.Fatalf("empty V4: got %q want %q", got, "")
	}

	n := &godo.Networks{V4: []godo.NetworkV4{
		{IPAddress: ""},
		{IPAddress: "10.0.0.1"},
		{IPAddress: "203.0.113.5"},
	}}
	if got, want := firstIPv4(n), "10.0.0.1"; got != want {
		t.Fatalf("got %q want %q", got, want)
	}
}
