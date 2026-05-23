package do

import (
	"context"
	"strings"
	"time"

	"github.com/digitalocean/godo"
)

func (c *Client) ListApplicationImages(ctx context.Context) ([]ImageRow, error) {
	var out []ImageRow
	opt := &godo.ListOptions{PerPage: 200, Page: 1}
	for {
		imgs, resp, err := c.godo.Images.ListApplication(ctx, opt)
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

func (c *Client) ListImagesByTag(ctx context.Context, tag string) ([]ImageRow, error) {
	tag = strings.TrimSpace(tag)
	if tag == "" {
		return nil, errEmpty("tag")
	}
	var out []ImageRow
	opt := &godo.ListOptions{PerPage: 200, Page: 1}
	for {
		imgs, resp, err := c.godo.Images.ListByTag(ctx, tag, opt)
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

// UpdateImage renames a user image (the only field the API meaningfully
// accepts on update is Name; Distribution/Description may be ignored).
func (c *Client) UpdateImage(ctx context.Context, id int, newName string) error {
	_, _, err := c.godo.Images.Update(ctx, id, &godo.ImageUpdateRequest{
		Name: strings.TrimSpace(newName),
	})
	return err
}

// TransferImage copies a user image to another region (async; returns when
// the action has been accepted, not when the copy completes).
func (c *Client) TransferImage(ctx context.Context, id int, region string) error {
	_, _, err := c.godo.ImageActions.Transfer(ctx, id, &godo.ActionRequest{
		"type":   "transfer",
		"region": strings.TrimSpace(region),
	})
	return err
}

// ConvertImage converts a backup image into a snapshot. Only valid for
// backup-type images.
func (c *Client) ConvertImage(ctx context.Context, id int) error {
	_, _, err := c.godo.ImageActions.Convert(ctx, id)
	return err
}

type CreateAlertPolicyReq struct {
	Type        string // e.g. "v1/insights/droplet/cpu"
	Description string
	Compare     string // "GreaterThan" or "LessThan"
	Value       float32
	Window      string   // "5m", "10m", "30m", "1h"
	Entities    []int    // droplet IDs
	Tags        []string // optional tag selector
	Emails      []string
	Enabled     bool
}

func (c *Client) CreateAlertPolicy(ctx context.Context, r CreateAlertPolicyReq) (*godo.AlertPolicy, error) {
	enabled := r.Enabled
	entities := make([]string, 0, len(r.Entities))
	for _, id := range r.Entities {
		entities = append(entities, itoa(id))
	}
	req := &godo.AlertPolicyCreateRequest{
		Type:        strings.TrimSpace(r.Type),
		Description: strings.TrimSpace(r.Description),
		Compare:     godo.AlertPolicyComp(strings.TrimSpace(r.Compare)),
		Value:       r.Value,
		Window:      strings.TrimSpace(r.Window),
		Entities:    entities,
		Tags:        r.Tags,
		Alerts:      godo.Alerts{Email: r.Emails, Slack: []godo.SlackDetails{}},
		Enabled:     &enabled,
	}
	p, _, err := c.godo.Monitoring.CreateAlertPolicy(ctx, req)
	return p, err
}

// MetricSample is one (timestamp, value) point from a metric series.
type MetricSample struct {
	When  time.Time
	Value float64
}

// GetDropletCPULastHour returns one-hour worth of CPU samples for a droplet,
// flattened across all label series (DO returns one series per CPU mode —
// user/system/iowait/etc. — we sum them to "total used %").
func (c *Client) GetDropletCPULastHour(ctx context.Context, dropletID int) ([]MetricSample, error) {
	end := time.Now()
	start := end.Add(-1 * time.Hour)
	resp, _, err := c.godo.Monitoring.GetDropletCPU(ctx, &godo.DropletMetricsRequest{
		HostID: itoa(dropletID),
		Start:  start,
		End:    end,
	})
	if err != nil {
		return nil, err
	}
	return flattenCPU(resp), nil
}

// flattenCPU sums the per-mode series into a single "used CPU %" timeline,
// excluding the idle mode. DO returns CPU as counters per mode in seconds;
// to keep this dependency-light we just sum every non-idle series and treat
// the value as a relative "used" indicator. Good enough for a sparkline.
func flattenCPU(resp *godo.MetricsResponse) []MetricSample {
	if resp == nil {
		return nil
	}
	// Collect (timestamp -> sum of non-idle values).
	sums := map[int64]float64{}
	timestamps := map[int64]time.Time{}
	for _, series := range resp.Data.Result {
		mode := string(series.Metric["mode"])
		if mode == "idle" {
			continue
		}
		for _, p := range series.Values {
			ts := int64(p.Timestamp)
			sums[ts] += float64(p.Value)
			timestamps[ts] = p.Timestamp.Time()
		}
	}
	if len(sums) == 0 {
		return nil
	}
	out := make([]MetricSample, 0, len(sums))
	for ts, v := range sums {
		out = append(out, MetricSample{When: timestamps[ts], Value: v})
	}
	sortByTime(out)
	return out
}

func sortByTime(s []MetricSample) {
	// tiny insertion sort — series are short
	for i := 1; i < len(s); i++ {
		for j := i; j > 0 && s[j-1].When.After(s[j].When); j-- {
			s[j-1], s[j] = s[j], s[j-1]
		}
	}
}

func itoa(i int) string {
	if i == 0 {
		return "0"
	}
	n := i
	neg := false
	if n < 0 {
		neg = true
		n = -n
	}
	var buf [20]byte
	pos := len(buf)
	for n > 0 {
		pos--
		buf[pos] = byte('0' + n%10)
		n /= 10
	}
	if neg {
		pos--
		buf[pos] = '-'
	}
	return string(buf[pos:])
}
