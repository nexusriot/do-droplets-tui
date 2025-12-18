package do

import (
	"context"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/digitalocean/godo"
	"golang.org/x/oauth2"
)

type Client struct {
	godo *godo.Client
}

func New(token string) *Client {
	ts := oauth2.StaticTokenSource(&oauth2.Token{AccessToken: token})
	httpClient := oauth2.NewClient(context.Background(), ts)

	// Optional: small transport defaults
	httpClient.Timeout = 30 * time.Second

	return &Client{godo: godo.NewClient(httpClient)}
}

type DropletRow struct {
	ID     int
	Name   string
	Region string
	Size   string
	Status string
	IPv4   string
	Tags   []string
}

func (c *Client) ListDroplets(ctx context.Context) ([]DropletRow, error) {
	var out []DropletRow
	opt := &godo.ListOptions{PerPage: 200, Page: 1}

	for {
		ds, resp, err := c.godo.Droplets.List(ctx, opt)
		if err != nil {
			return nil, err
		}
		for _, d := range ds {
			out = append(out, DropletRow{
				ID:     d.ID,
				Name:   d.Name,
				Region: d.Region.Slug,
				Size:   d.SizeSlug,
				Status: d.Status,
				IPv4:   firstIPv4(d.Networks),
				Tags:   d.Tags,
			})
		}
		if resp == nil || resp.Links == nil || resp.Links.IsLastPage() {
			break
		}
		page, err := resp.Links.CurrentPage()
		if err != nil {
			break
		}
		opt.Page = page + 1
	}
	return out, nil
}

func (c *Client) GetDroplet(ctx context.Context, id int) (*godo.Droplet, error) {
	d, _, err := c.godo.Droplets.Get(ctx, id)
	return d, err
}

func (c *Client) PowerOn(ctx context.Context, id int) error {
	_, _, err := c.godo.DropletActions.PowerOn(ctx, id)
	return err
}

func (c *Client) PowerOff(ctx context.Context, id int) error {
	_, _, err := c.godo.DropletActions.PowerOff(ctx, id)
	return err
}

func (c *Client) Shutdown(ctx context.Context, id int) error {
	_, _, err := c.godo.DropletActions.Shutdown(ctx, id)
	return err
}

func (c *Client) Reboot(ctx context.Context, id int) error {
	_, _, err := c.godo.DropletActions.Reboot(ctx, id)
	return err
}

func (c *Client) DeleteDroplet(ctx context.Context, id int) error {
	_, err := c.godo.Droplets.Delete(ctx, id)
	return err
}

type CreateDropletReq struct {
	Name       string
	Region     string
	Size       string
	ImageSlug  string
	SSHKeyIDs  []int
	Tags       []string
	VPCUUID    string
	EnableIPv6 bool
}

func (c *Client) CreateDroplet(ctx context.Context, r CreateDropletReq) (*godo.Droplet, error) {
	if strings.TrimSpace(r.Name) == "" {
		return nil, fmt.Errorf("name is required")
	}
	if r.Region == "" {
		r.Region = "fra1"
	}
	if r.Size == "" {
		r.Size = "s-1vcpu-1gb"
	}
	if r.ImageSlug == "" {
		r.ImageSlug = "ubuntu-24-04-x64"
	}

	keys := make([]godo.DropletCreateSSHKey, 0, len(r.SSHKeyIDs))
	for _, id := range r.SSHKeyIDs {
		keys = append(keys, godo.DropletCreateSSHKey{ID: id})
	}

	req := &godo.DropletCreateRequest{
		Name:   r.Name,
		Region: r.Region,
		Size:   r.Size,
		Tags:   r.Tags,
		IPv6:   r.EnableIPv6,
		VPCUUID: func() string {
			return strings.TrimSpace(r.VPCUUID)
		}(),
		SSHKeys: keys,
		Image: godo.DropletCreateImage{
			Slug: r.ImageSlug,
		},
	}

	d, _, err := c.godo.Droplets.Create(ctx, req)
	return d, err
}

func ParseCSVInts(s string) ([]int, error) {
	s = strings.TrimSpace(s)
	if s == "" {
		return nil, nil
	}
	parts := strings.Split(s, ",")
	out := make([]int, 0, len(parts))
	for _, p := range parts {
		p = strings.TrimSpace(p)
		if p == "" {
			continue
		}
		v, err := strconv.Atoi(p)
		if err != nil {
			return nil, fmt.Errorf("bad int %q: %w", p, err)
		}
		out = append(out, v)
	}
	return out, nil
}

func firstIPv4(n *godo.Networks) string {
	if n == nil {
		return ""
	}
	for _, v := range n.V4 {
		if v.IPAddress != "" {
			return v.IPAddress
		}
	}
	return ""
}
