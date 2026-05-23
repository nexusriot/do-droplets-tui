package do

import (
	"context"
	"fmt"
	"strings"

	"github.com/digitalocean/godo"
)

func (c *Client) ListAllSnapshots(ctx context.Context) ([]SnapshotRow, error) {
	var out []SnapshotRow
	opt := &godo.ListOptions{PerPage: 200, Page: 1}
	for {
		snaps, resp, err := c.godo.Snapshots.List(ctx, opt)
		if err != nil {
			return nil, err
		}
		for _, s := range snaps {
			out = append(out, snapshotToRow(s))
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

type ReservedIPRow struct {
	IP          string
	Region      string
	DropletID   int
	DropletName string
}

type CreateReservedIPReq struct {
	Region    string
	DropletID int // 0 = unassigned on create
}

func (c *Client) ListReservedIPs(ctx context.Context) ([]ReservedIPRow, error) {
	var out []ReservedIPRow
	opt := &godo.ListOptions{PerPage: 200, Page: 1}
	for {
		ips, resp, err := c.godo.ReservedIPs.List(ctx, opt)
		if err != nil {
			return nil, err
		}
		for _, r := range ips {
			row := ReservedIPRow{IP: r.IP}
			if r.Region != nil {
				row.Region = r.Region.Slug
			}
			if r.Droplet != nil {
				row.DropletID = r.Droplet.ID
				row.DropletName = r.Droplet.Name
			}
			out = append(out, row)
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

func (c *Client) CreateReservedIP(ctx context.Context, req CreateReservedIPReq) (*godo.ReservedIP, error) {
	region := strings.TrimSpace(req.Region)
	if region == "" {
		return nil, fmt.Errorf("region is required")
	}
	cr := &godo.ReservedIPCreateRequest{Region: region}
	if req.DropletID > 0 {
		cr.DropletID = req.DropletID
	}
	ip, _, err := c.godo.ReservedIPs.Create(ctx, cr)
	return ip, err
}

func (c *Client) DeleteReservedIP(ctx context.Context, ip string) error {
	ip = strings.TrimSpace(ip)
	if ip == "" {
		return fmt.Errorf("IP is empty")
	}
	_, err := c.godo.ReservedIPs.Delete(ctx, ip)
	return err
}

func (c *Client) AssignReservedIP(ctx context.Context, ip string, dropletID int) error {
	_, _, err := c.godo.ReservedIPActions.Assign(ctx, strings.TrimSpace(ip), dropletID)
	return err
}

func (c *Client) UnassignReservedIP(ctx context.Context, ip string) error {
	_, _, err := c.godo.ReservedIPActions.Unassign(ctx, strings.TrimSpace(ip))
	return err
}

type FirewallRow struct {
	ID            string
	Name          string
	Status        string
	DropletCount  int
	InboundCount  int
	OutboundCount int
}

type FirewallDetails struct {
	Row      FirewallRow
	Inbound  []string
	Outbound []string
	Tags     []string
	Droplets []int
}

func (c *Client) ListFirewalls(ctx context.Context) ([]FirewallRow, error) {
	var out []FirewallRow
	opt := &godo.ListOptions{PerPage: 200, Page: 1}
	for {
		fws, resp, err := c.godo.Firewalls.List(ctx, opt)
		if err != nil {
			return nil, err
		}
		for _, fw := range fws {
			out = append(out, FirewallRow{
				ID:            fw.ID,
				Name:          fw.Name,
				Status:        fw.Status,
				DropletCount:  len(fw.DropletIDs),
				InboundCount:  len(fw.InboundRules),
				OutboundCount: len(fw.OutboundRules),
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

func (c *Client) GetFirewall(ctx context.Context, id string) (*FirewallDetails, error) {
	fw, _, err := c.godo.Firewalls.Get(ctx, strings.TrimSpace(id))
	if err != nil {
		return nil, err
	}
	d := &FirewallDetails{
		Row: FirewallRow{
			ID:            fw.ID,
			Name:          fw.Name,
			Status:        fw.Status,
			DropletCount:  len(fw.DropletIDs),
			InboundCount:  len(fw.InboundRules),
			OutboundCount: len(fw.OutboundRules),
		},
		Tags:     fw.Tags,
		Droplets: fw.DropletIDs,
	}
	for _, r := range fw.InboundRules {
		src := ""
		if len(r.Sources.Addresses) > 0 {
			src = strings.Join(r.Sources.Addresses, ",")
		} else if len(r.Sources.Tags) > 0 {
			src = "tag:" + strings.Join(r.Sources.Tags, ",")
		} else {
			src = "any"
		}
		d.Inbound = append(d.Inbound, fmt.Sprintf("%s %s:%s ← %s", r.Protocol, r.PortRange, r.PortRange, src))
	}
	for _, r := range fw.OutboundRules {
		dst := ""
		if len(r.Destinations.Addresses) > 0 {
			dst = strings.Join(r.Destinations.Addresses, ",")
		} else if len(r.Destinations.Tags) > 0 {
			dst = "tag:" + strings.Join(r.Destinations.Tags, ",")
		} else {
			dst = "any"
		}
		d.Outbound = append(d.Outbound, fmt.Sprintf("%s %s → %s", r.Protocol, r.PortRange, dst))
	}
	return d, nil
}

func (c *Client) DeleteFirewall(ctx context.Context, id string) error {
	id = strings.TrimSpace(id)
	if id == "" {
		return fmt.Errorf("firewall id is empty")
	}
	_, err := c.godo.Firewalls.Delete(ctx, id)
	return err
}

type DomainRow struct {
	Name string
	TTL  int
}

type DomainRecordRow struct {
	ID       int
	Type     string
	Name     string
	Data     string
	TTL      int
	Priority int
}

type CreateDomainReq struct {
	Name      string
	IPAddress string // optional
}

type CreateRecordReq struct {
	Domain   string
	Type     string
	Name     string
	Data     string
	TTL      int
	Priority int
}

func (c *Client) ListDomains(ctx context.Context) ([]DomainRow, error) {
	var out []DomainRow
	opt := &godo.ListOptions{PerPage: 200, Page: 1}
	for {
		domains, resp, err := c.godo.Domains.List(ctx, opt)
		if err != nil {
			return nil, err
		}
		for _, d := range domains {
			out = append(out, DomainRow{Name: d.Name, TTL: d.TTL})
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

func (c *Client) CreateDomain(ctx context.Context, req CreateDomainReq) (*godo.Domain, error) {
	name := strings.TrimSpace(req.Name)
	if name == "" {
		return nil, fmt.Errorf("domain name is required")
	}
	cr := &godo.DomainCreateRequest{
		Name:      name,
		IPAddress: strings.TrimSpace(req.IPAddress),
	}
	d, _, err := c.godo.Domains.Create(ctx, cr)
	return d, err
}

func (c *Client) DeleteDomain(ctx context.Context, name string) error {
	name = strings.TrimSpace(name)
	if name == "" {
		return fmt.Errorf("domain name is empty")
	}
	_, err := c.godo.Domains.Delete(ctx, name)
	return err
}

func (c *Client) ListDomainRecords(ctx context.Context, domain string) ([]DomainRecordRow, error) {
	domain = strings.TrimSpace(domain)
	var out []DomainRecordRow
	opt := &godo.ListOptions{PerPage: 200, Page: 1}
	for {
		recs, resp, err := c.godo.Domains.Records(ctx, domain, opt)
		if err != nil {
			return nil, err
		}
		for _, r := range recs {
			out = append(out, DomainRecordRow{
				ID:       r.ID,
				Type:     r.Type,
				Name:     r.Name,
				Data:     r.Data,
				TTL:      r.TTL,
				Priority: r.Priority,
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

func (c *Client) CreateDomainRecord(ctx context.Context, req CreateRecordReq) (*godo.DomainRecord, error) {
	if strings.TrimSpace(req.Domain) == "" {
		return nil, fmt.Errorf("domain is required")
	}
	if strings.TrimSpace(req.Type) == "" {
		return nil, fmt.Errorf("record type is required")
	}
	ttl := req.TTL
	if ttl <= 0 {
		ttl = 1800
	}
	r, _, err := c.godo.Domains.CreateRecord(ctx, req.Domain, &godo.DomainRecordEditRequest{
		Type:     strings.TrimSpace(req.Type),
		Name:     strings.TrimSpace(req.Name),
		Data:     strings.TrimSpace(req.Data),
		TTL:      ttl,
		Priority: req.Priority,
	})
	return r, err
}

func (c *Client) DeleteDomainRecord(ctx context.Context, domain string, id int) error {
	domain = strings.TrimSpace(domain)
	if domain == "" {
		return fmt.Errorf("domain is required")
	}
	_, err := c.godo.Domains.DeleteRecord(ctx, domain, id)
	return err
}
