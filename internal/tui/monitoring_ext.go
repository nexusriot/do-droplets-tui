package tui

import (
	"context"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/charmbracelet/bubbles/key"
	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/nexusriot/do-droplets-tui/internal/do"
)

/* ============================================================
   Create Alert Policy form
   ============================================================

   Minimum viable surface — DO's CreateAlertPolicy payload has many
   optional fields but the core 5 are enough for the common "page me when
   CPU > 80% for 5m" use case:

     Type        e.g. v1/insights/droplet/cpu  (preset list cycled with ↑/↓
                 on the first field)
     Window      5m | 10m | 30m | 1h
     Compare     GreaterThan | LessThan
     Value       float (percent, GiB, etc.)
     Entities    CSV of droplet IDs (empty = match-by-tag instead)
     Emails      CSV
     Description free text

   Entities OR Tags must be non-empty. Slack alerts are not exposed (would
   need URL+channel pair input).
   ============================================================ */

// alertTypePresets are the common DO insight types. The form cycles through
// them with Up/Down on field 0 so the user doesn't have to memorise.
var alertTypePresets = []string{
	"v1/insights/droplet/cpu",
	"v1/insights/droplet/memory_utilization_percent",
	"v1/insights/droplet/disk_utilization_percent",
	"v1/insights/droplet/load_1",
	"v1/insights/droplet/load_5",
	"v1/insights/droplet/load_15",
	"v1/insights/droplet/public_outbound_bandwidth",
	"v1/insights/droplet/public_inbound_bandwidth",
}

var alertWindowPresets = []string{"5m", "10m", "30m", "1h"}

type alertForm struct {
	typeIdx   int
	windowIdx int
	compIn    textinput.Model
	valueIn   textinput.Model
	entIn     textinput.Model
	tagIn     textinput.Model
	emailIn   textinput.Model
	descIn    textinput.Model
	enabled   bool
	focus     int // 0..7
}

func (m *Model) initAlertForm() {
	f := &m.alertForm
	f.typeIdx = 0
	f.windowIdx = 0
	f.compIn = newInput("Compare (GreaterThan|LessThan)", "GreaterThan")
	f.compIn.SetValue("GreaterThan")
	f.valueIn = newInput("Threshold (percent / units)", "80")
	f.valueIn.SetValue("80")
	f.entIn = newInput("Droplet IDs (CSV)", "")
	f.tagIn = newInput("Tags (CSV) — alternative to droplet IDs", "")
	f.emailIn = newInput("Email recipients (CSV)", "")
	f.descIn = newInput("Description", "High CPU alert")
	f.descIn.SetValue("High CPU alert")
	f.enabled = true
	f.focus = 0
	m.blurAlertForm()
}

func (m *Model) blurAlertForm() {
	f := &m.alertForm
	f.compIn.Blur()
	f.valueIn.Blur()
	f.entIn.Blur()
	f.tagIn.Blur()
	f.emailIn.Blur()
	f.descIn.Blur()
}

func (m *Model) focusAlertOne() {
	f := &m.alertForm
	switch f.focus {
	case 2:
		f.compIn.Focus()
	case 3:
		f.valueIn.Focus()
	case 4:
		f.entIn.Focus()
	case 5:
		f.tagIn.Focus()
	case 6:
		f.emailIn.Focus()
	case 7:
		f.descIn.Focus()
	}
}

func (m Model) updateCreateAlert(k tea.KeyMsg, cmds []tea.Cmd) (Model, []tea.Cmd) {
	if key.Matches(k, m.keys.Back) {
		m.st = stateAlerts
		return m, cmds
	}
	if m.busy {
		return m, cmds
	}
	f := &m.alertForm

	// Tab cycles focus over all 8 logical fields. Up/Down on the two
	// preset fields (type=0, window=1) cycle the preset.
	switch k.String() {
	case "tab":
		m.blurAlertForm()
		f.focus = (f.focus + 1) % 8
		m.focusAlertOne()
		return m, cmds
	case "shift+tab":
		m.blurAlertForm()
		f.focus = (f.focus - 1 + 8) % 8
		m.focusAlertOne()
		return m, cmds
	case "up":
		if f.focus == 0 {
			f.typeIdx = (f.typeIdx - 1 + len(alertTypePresets)) % len(alertTypePresets)
			return m, cmds
		}
		if f.focus == 1 {
			f.windowIdx = (f.windowIdx - 1 + len(alertWindowPresets)) % len(alertWindowPresets)
			return m, cmds
		}
	case "down":
		if f.focus == 0 {
			f.typeIdx = (f.typeIdx + 1) % len(alertTypePresets)
			return m, cmds
		}
		if f.focus == 1 {
			f.windowIdx = (f.windowIdx + 1) % len(alertWindowPresets)
			return m, cmds
		}
	case " ":
		// Toggle enabled when none of the inputs are focused (avoid eating
		// space inside textinputs).
		if !anyAlertInputFocused(f) {
			f.enabled = !f.enabled
			return m, cmds
		}
	case "enter":
		if err := m.validateAlertForm(); err != nil {
			m.errText = err.Error()
			return m, cmds
		}
		m.pendingAct = actCreateAlertPolicy
		m.confirmReturn = stateCreateAlert
		m.confirmText = fmt.Sprintf(
			"Create alert?\nType: %s\nWindow: %s\nCompare: %s %s\nDroplets: %s\nTags: %s\nEmails: %s\nEnabled: %v",
			alertTypePresets[f.typeIdx], alertWindowPresets[f.windowIdx],
			strings.TrimSpace(f.compIn.Value()), strings.TrimSpace(f.valueIn.Value()),
			defaultIfEmpty(f.entIn.Value(), "(none)"),
			defaultIfEmpty(f.tagIn.Value(), "(none)"),
			defaultIfEmpty(f.emailIn.Value(), "(none)"),
			f.enabled,
		)
		m.st = stateConfirm
		return m, cmds
	}

	// Route key to focused input (fields 2..7).
	var cmd tea.Cmd
	switch f.focus {
	case 2:
		f.compIn, cmd = f.compIn.Update(k)
	case 3:
		f.valueIn, cmd = f.valueIn.Update(k)
	case 4:
		f.entIn, cmd = f.entIn.Update(k)
	case 5:
		f.tagIn, cmd = f.tagIn.Update(k)
	case 6:
		f.emailIn, cmd = f.emailIn.Update(k)
	case 7:
		f.descIn, cmd = f.descIn.Update(k)
	}
	cmds = append(cmds, cmd)
	return m, cmds
}

func anyAlertInputFocused(f *alertForm) bool {
	return f.compIn.Focused() || f.valueIn.Focused() || f.entIn.Focused() ||
		f.tagIn.Focused() || f.emailIn.Focused() || f.descIn.Focused()
}

func (m Model) validateAlertForm() error {
	f := &m.alertForm
	v := strings.TrimSpace(f.valueIn.Value())
	if _, err := strconv.ParseFloat(v, 32); err != nil {
		return fmt.Errorf("threshold value must be a number: %v", err)
	}
	cmp := strings.TrimSpace(f.compIn.Value())
	if cmp != "GreaterThan" && cmp != "LessThan" {
		return fmt.Errorf("compare must be GreaterThan or LessThan")
	}
	if _, err := do.ParseCSVInts(f.entIn.Value()); err != nil {
		return fmt.Errorf("droplet IDs: %v", err)
	}
	ent := strings.TrimSpace(f.entIn.Value())
	tag := strings.TrimSpace(f.tagIn.Value())
	if ent == "" && tag == "" {
		return fmt.Errorf("at least one droplet ID or tag is required")
	}
	if strings.TrimSpace(f.emailIn.Value()) == "" {
		return fmt.Errorf("at least one email recipient is required")
	}
	return nil
}

func (m Model) viewCreateAlert() string {
	f := &m.alertForm

	// helper to render a field with focus indicator
	row := func(label, value string, n int) string {
		marker := "  "
		if f.focus == n {
			marker = "> "
		}
		return marker + label + ": " + value
	}

	enabled := "[ ]"
	if f.enabled {
		enabled = "[x]"
	}

	var b strings.Builder
	b.WriteString(lipgloss.NewStyle().Bold(true).Render("Create Alert Policy") + "\n\n")
	b.WriteString(lipgloss.NewStyle().Faint(true).Render(
		"Tab/Shift+Tab move | Up/Down cycle preset on type/window | "+
			"Space toggles Enabled (when no input focused) | Enter submit | Esc cancel") + "\n\n")

	b.WriteString(row("Type    ", alertTypePresets[f.typeIdx], 0) + "\n")
	b.WriteString(row("Window  ", alertWindowPresets[f.windowIdx], 1) + "\n")
	b.WriteString(row("Compare ", f.compIn.View(), 2) + "\n")
	b.WriteString(row("Value   ", f.valueIn.View(), 3) + "\n")
	b.WriteString(row("Droplets", f.entIn.View(), 4) + "\n")
	b.WriteString(row("Tags    ", f.tagIn.View(), 5) + "\n")
	b.WriteString(row("Emails  ", f.emailIn.View(), 6) + "\n")
	b.WriteString(row("Desc    ", f.descIn.View(), 7) + "\n")
	b.WriteString("\n  Enabled: " + enabled + "\n")
	return b.String()
}

func (m Model) createAlertCmd() tea.Cmd {
	f := &m.alertForm
	v, _ := strconv.ParseFloat(strings.TrimSpace(f.valueIn.Value()), 32)
	ents, _ := do.ParseCSVInts(f.entIn.Value())
	tags := splitCSV(f.tagIn.Value())
	emails := splitCSV(f.emailIn.Value())
	req := do.CreateAlertPolicyReq{
		Type:        alertTypePresets[f.typeIdx],
		Description: strings.TrimSpace(f.descIn.Value()),
		Compare:     strings.TrimSpace(f.compIn.Value()),
		Value:       float32(v),
		Window:      alertWindowPresets[f.windowIdx],
		Entities:    ents,
		Tags:        tags,
		Emails:      emails,
		Enabled:     f.enabled,
	}
	m.busy = true
	target := "alert:" + req.Type
	m.logOp("alert.create", target, "requested")
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(m.ctx, 30*time.Second)
		defer cancel()
		p, err := m.api.CreateAlertPolicy(ctx, req)
		if err != nil {
			return apiErrMsg{err: err}
		}
		return apiDoneMsg{
			act:    actCreateAlertPolicy,
			status: "Created alert policy " + p.UUID,
			target: target,
		}
	}
}

func (m Model) loadDropletMetricsCmd(id int) tea.Cmd {
	m.busy = true
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(m.ctx, 30*time.Second)
		defer cancel()
		samples, err := m.api.GetDropletCPULastHour(ctx, id)
		if err != nil {
			return apiErrMsg{err: err}
		}
		return dropletMetricsLoadedMsg{samples: samples}
	}
}

func (m Model) updateDropletMetrics(k tea.KeyMsg, cmds []tea.Cmd) (Model, []tea.Cmd) {
	if key.Matches(k, m.keys.Back) {
		m.st = stateDetails
		return m, cmds
	}
	if m.busy {
		return m, cmds
	}
	if key.Matches(k, m.keys.Refresh) {
		cmds = append(cmds, m.loadDropletMetricsCmd(m.selectedDropletID))
	}
	return m, cmds
}

func (m Model) viewDropletMetrics() string {
	var b strings.Builder
	b.WriteString(lipgloss.NewStyle().Bold(true).Render(
		fmt.Sprintf("Droplet %d — CPU last hour", m.selectedDropletID)) + "\n\n")
	if len(m.dropletMetrics) == 0 {
		b.WriteString(lipgloss.NewStyle().Faint(true).Render("(no data — press r to load)") + "\n")
	} else {
		b.WriteString(renderSparkline(samplesToValues(m.dropletMetrics), 60) + "\n\n")
		stats := summarizeSamples(m.dropletMetrics)
		b.WriteString(fmt.Sprintf("min %.2f  avg %.2f  max %.2f   (%d samples over %s)\n",
			stats.Min, stats.Avg, stats.Max, len(m.dropletMetrics), stats.Span))
		first := m.dropletMetrics[0].When
		last := m.dropletMetrics[len(m.dropletMetrics)-1].When
		b.WriteString(fmt.Sprintf("from %s\nto   %s\n",
			first.Format("15:04:05"), last.Format("15:04:05")))
	}
	b.WriteString("\n" + lipgloss.NewStyle().Faint(true).Render("Keys: esc back | r refresh") + "\n")
	return b.String()
}

func samplesToValues(s []do.MetricSample) []float64 {
	out := make([]float64, len(s))
	for i, p := range s {
		out[i] = p.Value
	}
	return out
}

/* renderSparkline: braille-light eight-level bar graph in width cols. */
func renderSparkline(vals []float64, width int) string {
	if len(vals) == 0 {
		return ""
	}
	if width <= 0 {
		width = 60
	}
	// down-sample to `width` buckets by averaging.
	buckets := bucketize(vals, width)

	min, max := buckets[0], buckets[0]
	for _, v := range buckets {
		if v < min {
			min = v
		}
		if v > max {
			max = v
		}
	}
	rng := max - min
	if rng == 0 {
		rng = 1
	}
	// 8 levels (Unicode block elements).
	bars := []rune{' ', '▁', '▂', '▃', '▄', '▅', '▆', '▇', '█'}
	var b strings.Builder
	for _, v := range buckets {
		idx := int(((v-min)/rng)*float64(len(bars)-1) + 0.5)
		if idx < 0 {
			idx = 0
		}
		if idx >= len(bars) {
			idx = len(bars) - 1
		}
		b.WriteRune(bars[idx])
	}
	return b.String()
}

func bucketize(vals []float64, width int) []float64 {
	if len(vals) <= width {
		return vals
	}
	out := make([]float64, width)
	step := float64(len(vals)) / float64(width)
	for i := 0; i < width; i++ {
		from := int(float64(i) * step)
		to := int(float64(i+1) * step)
		if to > len(vals) {
			to = len(vals)
		}
		if from == to {
			out[i] = vals[from]
			continue
		}
		var sum float64
		for j := from; j < to; j++ {
			sum += vals[j]
		}
		out[i] = sum / float64(to-from)
	}
	return out
}

type sampleStats struct {
	Min, Max, Avg float64
	Span          time.Duration
}

func summarizeSamples(s []do.MetricSample) sampleStats {
	var min, max, sum float64
	if len(s) > 0 {
		min = s[0].Value
		max = s[0].Value
	}
	for _, p := range s {
		if p.Value < min {
			min = p.Value
		}
		if p.Value > max {
			max = p.Value
		}
		sum += p.Value
	}
	avg := 0.0
	if n := len(s); n > 0 {
		avg = sum / float64(n)
	}
	span := time.Duration(0)
	if len(s) > 1 {
		span = s[len(s)-1].When.Sub(s[0].When)
	}
	return sampleStats{Min: min, Max: max, Avg: avg, Span: span.Round(time.Second)}
}
