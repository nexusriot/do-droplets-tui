package do

import (
	"context"
	"strconv"
	"strings"

	"github.com/digitalocean/godo"
)

func (c *Client) PowerCycle(ctx context.Context, id int) error {
	_, _, err := c.godo.DropletActions.PowerCycle(ctx, id)
	return err
}

func (c *Client) PasswordReset(ctx context.Context, id int) error {
	_, _, err := c.godo.DropletActions.PasswordReset(ctx, id)
	return err
}

func (c *Client) EnableIPv6(ctx context.Context, id int) error {
	_, _, err := c.godo.DropletActions.EnableIPv6(ctx, id)
	return err
}

func (c *Client) EnablePrivateNetworking(ctx context.Context, id int) error {
	_, _, err := c.godo.DropletActions.EnablePrivateNetworking(ctx, id)
	return err
}

func (c *Client) EnableBackups(ctx context.Context, id int) error {
	_, _, err := c.godo.DropletActions.EnableBackups(ctx, id)
	return err
}

func (c *Client) DisableBackups(ctx context.Context, id int) error {
	_, _, err := c.godo.DropletActions.DisableBackups(ctx, id)
	return err
}

func (c *Client) SnapshotDroplet(ctx context.Context, id int, name string) error {
	_, _, err := c.godo.DropletActions.Snapshot(ctx, id, strings.TrimSpace(name))
	return err
}

// ResizeDroplet: sizeSlug is the new size; diskResize=true also enlarges disk
// (irreversible). Droplet must be powered off.
func (c *Client) ResizeDroplet(ctx context.Context, id int, sizeSlug string, diskResize bool) error {
	_, _, err := c.godo.DropletActions.Resize(ctx, id, strings.TrimSpace(sizeSlug), diskResize)
	return err
}

func (c *Client) RenameDroplet(ctx context.Context, id int, name string) error {
	_, _, err := c.godo.DropletActions.Rename(ctx, id, strings.TrimSpace(name))
	return err
}

// RebuildDroplet: if image parses as int, treat as image ID (snapshot/custom
// image); otherwise treat as slug (e.g. "ubuntu-24-04-x64").
func (c *Client) RebuildDroplet(ctx context.Context, id int, image string) error {
	image = strings.TrimSpace(image)
	if image == "" {
		return errEmpty("rebuild image")
	}
	if imgID, err := strconv.Atoi(image); err == nil {
		_, _, err := c.godo.DropletActions.RebuildByImageID(ctx, id, imgID)
		return err
	}
	_, _, err := c.godo.DropletActions.RebuildByImageSlug(ctx, id, image)
	return err
}

type errString string

func (e errString) Error() string { return string(e) }

func errEmpty(name string) error { return errString(name + " is required") }

type CreateFirewallReq struct {
	Name          string
	InboundRules  []FirewallRuleSpec
	OutboundRules []FirewallRuleSpec
	DropletIDs    []int
	Tags          []string
}

// FirewallRuleSpec is the flat TUI-facing form of a firewall rule. Either
// inbound or outbound; the slot decides which.
type FirewallRuleSpec struct {
	Protocol  string   // tcp | udp | icmp
	PortRange string   // "22", "80-90", or "" for icmp/all
	Addresses []string // CIDRs or IPs
	Tags      []string
	Droplets  []int
}

func (c *Client) CreateFirewall(ctx context.Context, r CreateFirewallReq) (*godo.Firewall, error) {
	req := &godo.FirewallRequest{
		Name:          strings.TrimSpace(r.Name),
		InboundRules:  toInbound(r.InboundRules),
		OutboundRules: toOutbound(r.OutboundRules),
		DropletIDs:    r.DropletIDs,
		Tags:          r.Tags,
	}
	fw, _, err := c.godo.Firewalls.Create(ctx, req)
	return fw, err
}

func (c *Client) AddFirewallDroplets(ctx context.Context, fwID string, dropletIDs ...int) error {
	_, err := c.godo.Firewalls.AddDroplets(ctx, fwID, dropletIDs...)
	return err
}

func (c *Client) RemoveFirewallDroplets(ctx context.Context, fwID string, dropletIDs ...int) error {
	_, err := c.godo.Firewalls.RemoveDroplets(ctx, fwID, dropletIDs...)
	return err
}

func (c *Client) AddFirewallTags(ctx context.Context, fwID string, tags ...string) error {
	_, err := c.godo.Firewalls.AddTags(ctx, fwID, tags...)
	return err
}

func (c *Client) RemoveFirewallTags(ctx context.Context, fwID string, tags ...string) error {
	_, err := c.godo.Firewalls.RemoveTags(ctx, fwID, tags...)
	return err
}

func (c *Client) AddFirewallRules(ctx context.Context, fwID string, inbound, outbound []FirewallRuleSpec) error {
	req := &godo.FirewallRulesRequest{
		InboundRules:  toInbound(inbound),
		OutboundRules: toOutbound(outbound),
	}
	_, err := c.godo.Firewalls.AddRules(ctx, fwID, req)
	return err
}

func (c *Client) RemoveFirewallRules(ctx context.Context, fwID string, inbound, outbound []FirewallRuleSpec) error {
	req := &godo.FirewallRulesRequest{
		InboundRules:  toInbound(inbound),
		OutboundRules: toOutbound(outbound),
	}
	_, err := c.godo.Firewalls.RemoveRules(ctx, fwID, req)
	return err
}

func toInbound(rules []FirewallRuleSpec) []godo.InboundRule {
	out := make([]godo.InboundRule, 0, len(rules))
	for _, r := range rules {
		out = append(out, godo.InboundRule{
			Protocol:  r.Protocol,
			PortRange: r.PortRange,
			Sources: &godo.Sources{
				Addresses:  r.Addresses,
				Tags:       r.Tags,
				DropletIDs: r.Droplets,
			},
		})
	}
	return out
}

func toOutbound(rules []FirewallRuleSpec) []godo.OutboundRule {
	out := make([]godo.OutboundRule, 0, len(rules))
	for _, r := range rules {
		out = append(out, godo.OutboundRule{
			Protocol:  r.Protocol,
			PortRange: r.PortRange,
			Destinations: &godo.Destinations{
				Addresses:  r.Addresses,
				Tags:       r.Tags,
				DropletIDs: r.Droplets,
			},
		})
	}
	return out
}
