package tui

import (
	"reflect"
	"strings"
	"testing"

	"github.com/nexusriot/do-droplets-tui/internal/do"
)

// freshDropletForm returns a Model with the create-droplet form initialised
// from the supplied options. Inputs are otherwise empty.
func freshDropletForm(opts Options) Model {
	m := Model{opts: opts}
	m.initDropletCreateForm()
	return m
}

func TestBuildCreateDropletReq_DefaultsAndCSVs(t *testing.T) {
	m := freshDropletForm(Options{
		DefaultRegion: "fra1",
		DefaultSize:   "s-1vcpu-1gb",
		DefaultImage:  "ubuntu-24-04-x64",
		DefaultTags:   "env-dev,team-x",
		DefaultIPv6:   true,
	})
	m.nameIn.SetValue("my-app")
	m.sshIDsIn.SetValue("100, 200, 300")
	m.vpcIn.SetValue("  ")

	req, err := m.buildCreateDropletReq()
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	if req.Name != "my-app" {
		t.Errorf("Name = %q", req.Name)
	}
	if req.Region != "fra1" {
		t.Errorf("Region = %q", req.Region)
	}
	if req.Size != "s-1vcpu-1gb" {
		t.Errorf("Size = %q", req.Size)
	}
	if req.ImageSlug != "ubuntu-24-04-x64" {
		t.Errorf("ImageSlug = %q", req.ImageSlug)
	}
	if !req.EnableIPv6 {
		t.Errorf("EnableIPv6 = false, want true")
	}
	if !reflect.DeepEqual(req.SSHKeyIDs, []int{100, 200, 300}) {
		t.Errorf("SSHKeyIDs = %v", req.SSHKeyIDs)
	}
	if !reflect.DeepEqual(req.Tags, []string{"env-dev", "team-x"}) {
		t.Errorf("Tags = %v", req.Tags)
	}
	if req.VPCUUID != "" {
		t.Errorf("VPCUUID should be trimmed empty, got %q", req.VPCUUID)
	}
}

func TestBuildCreateDropletReq_RequiresName(t *testing.T) {
	m := freshDropletForm(Options{DefaultRegion: "nyc3", DefaultSize: "x", DefaultImage: "y"})
	// nameIn defaulted to "" by init
	_, err := m.buildCreateDropletReq()
	if err == nil || !strings.Contains(err.Error(), "name") {
		t.Fatalf("err = %v, want name-required", err)
	}
}

func TestBuildCreateDropletReq_RejectsDuplicateName(t *testing.T) {
	m := freshDropletForm(Options{DefaultRegion: "nyc3", DefaultSize: "x", DefaultImage: "y"})
	m.dropletRows = []do.DropletRow{{ID: 1, Name: "Existing"}}
	m.nameIn.SetValue("existing") // case-insensitive

	_, err := m.buildCreateDropletReq()
	if err == nil || !strings.Contains(err.Error(), "already exists") {
		t.Fatalf("err = %v, want already-exists", err)
	}
}

func TestBuildCreateDropletReq_BadSSHCSVIsError(t *testing.T) {
	m := freshDropletForm(Options{DefaultRegion: "nyc3", DefaultSize: "x", DefaultImage: "y"})
	m.nameIn.SetValue("fresh")
	m.sshIDsIn.SetValue("1,not-an-int,3")

	_, err := m.buildCreateDropletReq()
	if err == nil {
		t.Fatal("want error on bad SSH CSV")
	}
}

func TestBuildCreateDropletReq_IPv6ParsedFromField(t *testing.T) {
	m := freshDropletForm(Options{DefaultRegion: "nyc3", DefaultSize: "x", DefaultImage: "y", DefaultIPv6: false})
	m.nameIn.SetValue("a")
	m.ipv6In.SetValue("TRUE") // case-insensitive

	req, err := m.buildCreateDropletReq()
	if err != nil {
		t.Fatalf("err: %v", err)
	}
	if !req.EnableIPv6 {
		t.Errorf("want EnableIPv6 true")
	}

	m.ipv6In.SetValue("anything else")
	req, err = m.buildCreateDropletReq()
	if err != nil {
		t.Fatalf("err: %v", err)
	}
	if req.EnableIPv6 {
		t.Errorf("want EnableIPv6 false")
	}
}
