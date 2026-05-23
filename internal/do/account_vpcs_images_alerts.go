package do

import (
	"context"
	"strings"

	"github.com/digitalocean/godo"
)

type AccountInfo struct {
	Email           string
	Name            string
	UUID            string
	Status          string
	StatusMessage   string
	EmailVerified   bool
	DropletLimit    int
	FloatingIPLimit int
	ReservedIPLimit int
	VolumeLimit     int
	TeamName        string
	TeamUUID        string
}

type BalanceInfo struct {
	MonthToDateBalance string
	AccountBalance     string
	MonthToDateUsage   string
	GeneratedAt        string
}

func (c *Client) GetAccount(ctx context.Context) (*AccountInfo, error) {
	a, _, err := c.godo.Account.Get(ctx)
	if err != nil {
		return nil, err
	}
	out := &AccountInfo{
		Email:           a.Email,
		Name:            a.Name,
		UUID:            a.UUID,
		Status:          a.Status,
		StatusMessage:   a.StatusMessage,
		EmailVerified:   a.EmailVerified,
		DropletLimit:    a.DropletLimit,
		FloatingIPLimit: a.FloatingIPLimit,
		ReservedIPLimit: a.ReservedIPLimit,
		VolumeLimit:     a.VolumeLimit,
	}
	if a.Team != nil {
		out.TeamName = a.Team.Name
		out.TeamUUID = a.Team.UUID
	}
	return out, nil
}

func (c *Client) GetBalance(ctx context.Context) (*BalanceInfo, error) {
	b, _, err := c.godo.Balance.Get(ctx)
	if err != nil {
		return nil, err
	}
	return &BalanceInfo{
		MonthToDateBalance: b.MonthToDateBalance,
		AccountBalance:     b.AccountBalance,
		MonthToDateUsage:   b.MonthToDateUsage,
		GeneratedAt:        b.GeneratedAt.Format("2006-01-02 15:04:05 MST"),
	}, nil
}

type VPCRow struct {
	ID          string
	Name        string
	Region      string
	IPRange     string
	Description string
	Default     bool
	Created     string
}

type CreateVPCReq struct {
	Name        string
	Region      string
	IPRange     string // optional
	Description string
}

func (c *Client) ListVPCs(ctx context.Context) ([]VPCRow, error) {
	var out []VPCRow
	opt := &godo.ListOptions{PerPage: 200, Page: 1}
	for {
		vpcs, resp, err := c.godo.VPCs.List(ctx, opt)
		if err != nil {
			return nil, err
		}
		for _, v := range vpcs {
			out = append(out, VPCRow{
				ID:          v.ID,
				Name:        v.Name,
				Region:      v.RegionSlug,
				IPRange:     v.IPRange,
				Description: v.Description,
				Default:     v.Default,
				Created:     v.CreatedAt.Format("2006-01-02 15:04:05"),
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

func (c *Client) CreateVPC(ctx context.Context, r CreateVPCReq) (*godo.VPC, error) {
	req := &godo.VPCCreateRequest{
		Name:        strings.TrimSpace(r.Name),
		RegionSlug:  strings.TrimSpace(r.Region),
		IPRange:     strings.TrimSpace(r.IPRange),
		Description: strings.TrimSpace(r.Description),
	}
	v, _, err := c.godo.VPCs.Create(ctx, req)
	return v, err
}

func (c *Client) DeleteVPC(ctx context.Context, id string) error {
	_, err := c.godo.VPCs.Delete(ctx, id)
	return err
}

type ImageRow struct {
	ID           int
	Name         string
	Distribution string
	Slug         string
	Type         string
	Public       bool
	MinDiskGB    int
	SizeGB       float64
	Created      string
	Regions      string
	Status       string
}

func imageToRow(im godo.Image) ImageRow {
	return ImageRow{
		ID:           im.ID,
		Name:         im.Name,
		Distribution: im.Distribution,
		Slug:         im.Slug,
		Type:         im.Type,
		Public:       im.Public,
		MinDiskGB:    im.MinDiskSize,
		SizeGB:       im.SizeGigaBytes,
		Created:      im.Created,
		Regions:      strings.Join(im.Regions, ","),
		Status:       im.Status,
	}
}

func (c *Client) ListUserImages(ctx context.Context) ([]ImageRow, error) {
	var out []ImageRow
	opt := &godo.ListOptions{PerPage: 200, Page: 1}
	for {
		imgs, resp, err := c.godo.Images.ListUser(ctx, opt)
		if err != nil {
			return nil, err
		}
		for _, im := range imgs {
			out = append(out, imageToRow(im))
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

func (c *Client) ListDistributionImages(ctx context.Context) ([]ImageRow, error) {
	var out []ImageRow
	opt := &godo.ListOptions{PerPage: 200, Page: 1}
	for {
		imgs, resp, err := c.godo.Images.ListDistribution(ctx, opt)
		if err != nil {
			return nil, err
		}
		for _, im := range imgs {
			out = append(out, imageToRow(im))
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

func (c *Client) DeleteImage(ctx context.Context, id int) error {
	_, err := c.godo.Images.Delete(ctx, id)
	return err
}

type AlertPolicyRow struct {
	UUID        string
	Type        string
	Description string
	Compare     string
	Value       float32
	Window      string
	Enabled     bool
	Entities    string
	Tags        string
	Emails      string
	SlackChans  int
}

func (c *Client) ListAlertPolicies(ctx context.Context) ([]AlertPolicyRow, error) {
	var out []AlertPolicyRow
	opt := &godo.ListOptions{PerPage: 200, Page: 1}
	for {
		pols, resp, err := c.godo.Monitoring.ListAlertPolicies(ctx, opt)
		if err != nil {
			return nil, err
		}
		for _, p := range pols {
			out = append(out, AlertPolicyRow{
				UUID:        p.UUID,
				Type:        p.Type,
				Description: p.Description,
				Compare:     string(p.Compare),
				Value:       p.Value,
				Window:      p.Window,
				Enabled:     p.Enabled,
				Entities:    strings.Join(p.Entities, ","),
				Tags:        strings.Join(p.Tags, ","),
				Emails:      strings.Join(p.Alerts.Email, ","),
				SlackChans:  len(p.Alerts.Slack),
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

func (c *Client) DeleteAlertPolicy(ctx context.Context, uuid string) error {
	_, err := c.godo.Monitoring.DeleteAlertPolicy(ctx, uuid)
	return err
}
