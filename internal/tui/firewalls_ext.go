package tui

import (
	"context"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/charmbracelet/bubbles/key"
	"github.com/charmbracelet/bubbles/table"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/nexusriot/do-droplets-tui/internal/do"
)

func (m *Model) initFirewallCreateForm() {
	m.fwNameIn = newInput("Name", "my-fw")
	m.fwInRulesIn = newInput("Inbound rules (proto:ports CSV)", "tcp:22,tcp:80,tcp:443")
	m.fwOutRulesIn = newInput("Outbound rules", "tcp:all,udp:all,icmp:")
	m.fwDropletIDsIn = newInput("Droplet IDs (CSV)", "")
	m.fwTagsIn = newInput("Tags (CSV)", "")
	m.focusFWForm = 0
	m.blurFWForm()
	m.fwNameIn.Focus()
}

func (m *Model) blurFWForm() {
	m.fwNameIn.Blur()
	m.fwInRulesIn.Blur()
	m.fwOutRulesIn.Blur()
	m.fwDropletIDsIn.Blur()
	m.fwTagsIn.Blur()
}

func (m *Model) focusFWOne() {
	switch m.focusFWForm {
	case 0:
		m.fwNameIn.Focus()
	case 1:
		m.fwInRulesIn.Focus()
	case 2:
		m.fwOutRulesIn.Focus()
	case 3:
		m.fwDropletIDsIn.Focus()
	case 4:
		m.fwTagsIn.Focus()
	}
}

func (m Model) updateCreateFirewall(k tea.KeyMsg, cmds []tea.Cmd) (Model, []tea.Cmd) {
	if key.Matches(k, m.keys.Back) {
		m.st = stateFirewalls
		return m, cmds
	}
	if m.busy {
		return m, cmds
	}
	if k.String() == "tab" || k.String() == "shift+tab" {
		m.blurFWForm()
		if k.String() == "tab" {
			m.focusFWForm = (m.focusFWForm + 1) % 5
		} else {
			m.focusFWForm = (m.focusFWForm - 1 + 5) % 5
		}
		m.focusFWOne()
		return m, cmds
	}
	if k.String() == "enter" {
		name := strings.TrimSpace(m.fwNameIn.Value())
		if name == "" {
			m.errText = "name is required"
			return m, cmds
		}
		// Validate rules parse before opening confirm.
		if _, err := parseRuleCSV(m.fwInRulesIn.Value()); err != nil {
			m.errText = "inbound: " + err.Error()
			return m, cmds
		}
		if _, err := parseRuleCSV(m.fwOutRulesIn.Value()); err != nil {
			m.errText = "outbound: " + err.Error()
			return m, cmds
		}
		if _, err := do.ParseCSVInts(m.fwDropletIDsIn.Value()); err != nil {
			m.errText = "droplet IDs: " + err.Error()
			return m, cmds
		}
		m.pendingAct = actCreateFirewall
		m.confirmReturn = stateCreateFirewall
		m.confirmText = fmt.Sprintf("Create firewall %q?\nInbound: %s\nOutbound: %s\nDroplets: %s\nTags: %s",
			name,
			defaultIfEmpty(m.fwInRulesIn.Value(), "(none)"),
			defaultIfEmpty(m.fwOutRulesIn.Value(), "(none)"),
			defaultIfEmpty(m.fwDropletIDsIn.Value(), "(none)"),
			defaultIfEmpty(m.fwTagsIn.Value(), "(none)"))
		m.st = stateConfirm
		return m, cmds
	}
	var cmd tea.Cmd
	switch m.focusFWForm {
	case 0:
		m.fwNameIn, cmd = m.fwNameIn.Update(k)
	case 1:
		m.fwInRulesIn, cmd = m.fwInRulesIn.Update(k)
	case 2:
		m.fwOutRulesIn, cmd = m.fwOutRulesIn.Update(k)
	case 3:
		m.fwDropletIDsIn, cmd = m.fwDropletIDsIn.Update(k)
	case 4:
		m.fwTagsIn, cmd = m.fwTagsIn.Update(k)
	}
	cmds = append(cmds, cmd)
	return m, cmds
}

func (m Model) viewCreateFirewall() string {
	var b strings.Builder
	b.WriteString(lipgloss.NewStyle().Bold(true).Render("Create Firewall") + "\n\n")
	b.WriteString(lipgloss.NewStyle().Faint(true).Render(
		"Tab/Shift+Tab move | Enter submit | Esc cancel\n"+
			"Rule format: proto:ports per entry, CSV. Ports = \"22\", \"80-90\", or \"all\" (\"\" for icmp).\n"+
			"Sources default to 0.0.0.0/0 and ::/0 (any).") + "\n\n")
	b.WriteString(m.fwNameIn.View() + "\n")
	b.WriteString(m.fwInRulesIn.View() + "\n")
	b.WriteString(m.fwOutRulesIn.View() + "\n")
	b.WriteString(m.fwDropletIDsIn.View() + "\n")
	b.WriteString(m.fwTagsIn.View() + "\n")
	return b.String()
}

func (m Model) updateFirewallPicker(k tea.KeyMsg, cmds []tea.Cmd, add bool) (Model, []tea.Cmd) {
	if key.Matches(k, m.keys.Back) {
		m.st = stateFirewallDetails
		return m, cmds
	}
	if m.busy {
		return m, cmds
	}
	switch k.String() {
	case " ":
		i := m.fwPickerTable.Cursor()
		if i >= 0 && i < len(m.dropletRows) {
			id := m.dropletRows[i].ID
			m.fwPickerSelected[id] = !m.fwPickerSelected[id]
			m.fwPickerTable.SetRows(toFWPickerRows(m.dropletRows, m.fwPickerSelected))
		}
	case "enter":
		ids := selectedFromPicker(m.fwPickerSelected)
		if len(ids) == 0 {
			m.errText = "no droplets selected (space to toggle)"
			return m, cmds
		}
		if add {
			m.pendingAct = actAddFirewallDroplets
			m.confirmText = fmt.Sprintf("Add %d droplet(s) to firewall %q?",
				len(ids), m.firewallDetails.Row.Name)
		} else {
			m.pendingAct = actRemoveFirewallDroplets
			m.confirmText = fmt.Sprintf("Remove %d droplet(s) from firewall %q?",
				len(ids), m.firewallDetails.Row.Name)
		}
		m.confirmReturn = stateFirewallDetails
		m.st = stateConfirm
	}
	return m, cmds
}

func (m Model) viewFirewallPicker(add bool) string {
	verb := "Add to"
	if !add {
		verb = "Remove from"
	}
	h := lipgloss.NewStyle().Bold(true).Render(
		fmt.Sprintf("%s firewall %q — pick droplets", verb, m.firewallDetails.Row.Name))
	legend := lipgloss.NewStyle().Faint(true).Render("space toggle | enter confirm | esc cancel")
	return h + "\n\n" + m.fwPickerTable.View() + "\n\n" + legend
}

func (m Model) modifyFirewallDropletsCmd(add bool) tea.Cmd {
	fwID := m.firewallDetails.Row.ID
	fwName := m.firewallDetails.Row.Name
	ids := selectedFromPicker(m.fwPickerSelected)
	m.fwPickerSelected = map[int]bool{} // reset
	m.busy = true
	verb := "add"
	if !add {
		verb = "remove"
	}
	target := fmt.Sprintf("firewall:%s droplets:%d", fwID, len(ids))
	m.logOp("firewall."+verb+"_droplets", target, "requested")
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(m.ctx, 30*time.Second)
		defer cancel()
		var err error
		if add {
			err = m.api.AddFirewallDroplets(ctx, fwID, ids...)
		} else {
			err = m.api.RemoveFirewallDroplets(ctx, fwID, ids...)
		}
		if err != nil {
			return apiErrMsg{err: err}
		}
		act := actAddFirewallDroplets
		if !add {
			act = actRemoveFirewallDroplets
		}
		return apiDoneMsg{
			act:    act,
			status: fmt.Sprintf("%s %d droplets %s firewall %s", titleCase(verb), len(ids), prep(add), fwName),
			target: target,
		}
	}
}

func (m Model) updateFirewallAddRule(k tea.KeyMsg, cmds []tea.Cmd) (Model, []tea.Cmd) {
	if key.Matches(k, m.keys.Back) {
		m.st = stateFirewallDetails
		return m, cmds
	}
	if m.busy {
		return m, cmds
	}
	if k.String() == "tab" || k.String() == "shift+tab" {
		m.blurFWRuleForm()
		if k.String() == "tab" {
			m.focusFWRule = (m.focusFWRule + 1) % 3
		} else {
			m.focusFWRule = (m.focusFWRule - 1 + 3) % 3
		}
		m.focusFWRuleOne()
		return m, cmds
	}
	// 'd' on the FIRST keystroke before any input: toggle direction. But once
	// the user has typed anything we don't want to steal characters. Keep it
	// simple: dedicated Ctrl+D toggle.
	if k.String() == "ctrl+d" {
		if m.fwAddRuleDir == "in" {
			m.fwAddRuleDir = "out"
		} else {
			m.fwAddRuleDir = "in"
		}
		return m, cmds
	}
	if k.String() == "enter" {
		proto := strings.ToLower(strings.TrimSpace(m.fwAddRuleProto.Value()))
		ports := strings.TrimSpace(m.fwAddRulePorts.Value())
		src := strings.TrimSpace(m.fwAddRuleSrc.Value())
		if proto == "" {
			m.errText = "protocol is required (tcp|udp|icmp)"
			return m, cmds
		}
		addrs := splitCSV(src)
		if len(addrs) == 0 {
			addrs = []string{"0.0.0.0/0", "::/0"}
		}
		spec := do.FirewallRuleSpec{Protocol: proto, PortRange: ports, Addresses: addrs}
		// stash on model
		if m.fwAddRuleDir == "out" {
			m.pendingFwInRules = nil
			m.pendingFwOutRules = []do.FirewallRuleSpec{spec}
		} else {
			m.pendingFwInRules = []do.FirewallRuleSpec{spec}
			m.pendingFwOutRules = nil
		}
		m.pendingAct = actAddFirewallRules
		m.confirmReturn = stateFirewallAddRule
		m.confirmText = fmt.Sprintf("Add %s rule  %s:%s  sources/dest=%s  to firewall %q?",
			m.fwAddRuleDir, proto, defaultIfEmpty(ports, "(any)"),
			strings.Join(addrs, ","), m.firewallDetails.Row.Name)
		m.st = stateConfirm
		return m, cmds
	}
	var cmd tea.Cmd
	switch m.focusFWRule {
	case 0:
		m.fwAddRuleProto, cmd = m.fwAddRuleProto.Update(k)
	case 1:
		m.fwAddRulePorts, cmd = m.fwAddRulePorts.Update(k)
	case 2:
		m.fwAddRuleSrc, cmd = m.fwAddRuleSrc.Update(k)
	}
	cmds = append(cmds, cmd)
	return m, cmds
}

func (m *Model) blurFWRuleForm() {
	m.fwAddRuleProto.Blur()
	m.fwAddRulePorts.Blur()
	m.fwAddRuleSrc.Blur()
}

func (m *Model) focusFWRuleOne() {
	switch m.focusFWRule {
	case 0:
		m.fwAddRuleProto.Focus()
	case 1:
		m.fwAddRulePorts.Focus()
	case 2:
		m.fwAddRuleSrc.Focus()
	}
}

func (m Model) viewFirewallAddRule() string {
	dir := m.fwAddRuleDir
	if dir == "" {
		dir = "in"
	}
	var b strings.Builder
	b.WriteString(lipgloss.NewStyle().Bold(true).Render(
		fmt.Sprintf("Add rule to firewall %q  [direction: %s]", m.firewallDetails.Row.Name, dir)) + "\n\n")
	b.WriteString(lipgloss.NewStyle().Faint(true).Render(
		"Tab move | Ctrl+D toggle in/out | Enter submit | Esc cancel\n"+
			"Sources/dest empty = 0.0.0.0/0 + ::/0. Protocol: tcp | udp | icmp.") + "\n\n")
	b.WriteString(m.fwAddRuleProto.View() + "\n")
	b.WriteString(m.fwAddRulePorts.View() + "\n")
	b.WriteString(m.fwAddRuleSrc.View() + "\n")
	return b.String()
}

func (m Model) modifyFirewallRulesCmd(add bool) tea.Cmd {
	fwID := m.firewallDetails.Row.ID
	fwName := m.firewallDetails.Row.Name
	in := m.pendingFwInRules
	out := m.pendingFwOutRules
	m.pendingFwInRules = nil
	m.pendingFwOutRules = nil
	m.busy = true
	verb := "add"
	if !add {
		verb = "remove"
	}
	target := "firewall:" + fwID
	m.logOp("firewall."+verb+"_rules", target, "requested")
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(m.ctx, 30*time.Second)
		defer cancel()
		var err error
		if add {
			err = m.api.AddFirewallRules(ctx, fwID, in, out)
		} else {
			err = m.api.RemoveFirewallRules(ctx, fwID, in, out)
		}
		if err != nil {
			return apiErrMsg{err: err}
		}
		act := actAddFirewallRules
		if !add {
			act = actRemoveFirewallRules
		}
		return apiDoneMsg{
			act:    act,
			status: fmt.Sprintf("%sed rules on firewall %s", titleCase(verb), fwName),
			target: target,
		}
	}
}

func (m Model) createFirewallCmd() tea.Cmd {
	name := strings.TrimSpace(m.fwNameIn.Value())
	in, _ := parseRuleCSV(m.fwInRulesIn.Value())
	out, _ := parseRuleCSV(m.fwOutRulesIn.Value())
	dropletIDs, _ := do.ParseCSVInts(m.fwDropletIDsIn.Value())
	tags := splitCSV(m.fwTagsIn.Value())
	req := do.CreateFirewallReq{
		Name:          name,
		InboundRules:  in,
		OutboundRules: out,
		DropletIDs:    dropletIDs,
		Tags:          tags,
	}
	m.busy = true
	target := "firewall:" + name
	m.logOp("firewall.create", target, "requested")
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(m.ctx, 60*time.Second)
		defer cancel()
		fw, err := m.api.CreateFirewall(ctx, req)
		if err != nil {
			return apiErrMsg{err: err}
		}
		return apiDoneMsg{
			act:    actCreateFirewall,
			status: fmt.Sprintf("Created firewall %q (id=%s)", fw.Name, fw.ID),
			target: target,
		}
	}
}

/* ============================================================
   Rule CSV parser
   ============================================================

   Accepts entries like:
     tcp:22
     tcp:80-90
     tcp:all
     udp:53
     icmp:        (no port range)

   Sources default to 0.0.0.0/0 + ::/0 (matches the DO control-panel
   "any source" default). The granular per-rule source UI is the
   stateFirewallAddRule form instead.

   ============================================================ */

func parseRuleCSV(s string) ([]do.FirewallRuleSpec, error) {
	s = strings.TrimSpace(s)
	if s == "" {
		return nil, nil
	}
	var out []do.FirewallRuleSpec
	for _, entry := range strings.Split(s, ",") {
		entry = strings.TrimSpace(entry)
		if entry == "" {
			continue
		}
		parts := strings.SplitN(entry, ":", 2)
		proto := strings.ToLower(strings.TrimSpace(parts[0]))
		if proto != "tcp" && proto != "udp" && proto != "icmp" {
			return nil, fmt.Errorf("bad protocol %q (want tcp|udp|icmp)", proto)
		}
		ports := ""
		if len(parts) == 2 {
			ports = strings.TrimSpace(parts[1])
		}
		out = append(out, do.FirewallRuleSpec{
			Protocol:  proto,
			PortRange: ports,
			Addresses: []string{"0.0.0.0/0", "::/0"},
		})
	}
	return out, nil
}

func (m Model) firewallsListExt(k tea.KeyMsg, cmds []tea.Cmd) (Model, []tea.Cmd, bool) {
	switch {
	case key.Matches(k, m.keys.Create):
		m.initFirewallCreateForm()
		m.st = stateCreateFirewall
		return m, cmds, true
	}
	return m, cmds, false
}

func (m Model) firewallDetailsExt(k tea.KeyMsg, cmds []tea.Cmd) (Model, []tea.Cmd, bool) {
	if m.firewallDetails == nil {
		return m, cmds, false
	}
	switch {
	case key.Matches(k, m.keys.Attach):
		// Add droplets to this firewall.
		m.fwPickerSelected = map[int]bool{}
		m.fwPickerTable.SetRows(toFWPickerRows(m.dropletRows, m.fwPickerSelected))
		if len(m.dropletRows) == 0 {
			cmds = append(cmds, m.refreshDropletsCmd())
		}
		m.st = stateFirewallAddDroplets
		return m, cmds, true

	case key.Matches(k, m.keys.Detach):
		// Remove droplets from this firewall — we still show ALL droplets
		// here (the API doesn't include "attached droplets" in the details
		// payload as a flat list, just a count). User selects from the same
		// table; an unattached droplet "remove" is a no-op server-side.
		m.fwPickerSelected = map[int]bool{}
		m.fwPickerTable.SetRows(toFWPickerRows(m.dropletRows, m.fwPickerSelected))
		if len(m.dropletRows) == 0 {
			cmds = append(cmds, m.refreshDropletsCmd())
		}
		m.st = stateFirewallRemoveDroplets
		return m, cmds, true

	case key.Matches(k, m.keys.Create):
		// `c` on details = add rule (Create rule).
		m.fwAddRuleProto = newInput("Protocol (tcp|udp|icmp)", "tcp")
		m.fwAddRulePorts = newInput("Ports (\"22\" | \"80-90\" | \"all\" | \"\")", "22")
		m.fwAddRuleSrc = newInput("Sources/dest CIDR CSV (empty = any)", "")
		m.fwAddRuleDir = "in"
		m.focusFWRule = 0
		m.blurFWRuleForm()
		m.fwAddRuleProto.Focus()
		m.st = stateFirewallAddRule
		return m, cmds, true
	}
	return m, cmds, false
}

func toFWPickerRows(rows []do.DropletRow, sel map[int]bool) []table.Row {
	out := make([]table.Row, 0, len(rows))
	for _, r := range rows {
		mark := " "
		if sel[r.ID] {
			mark = "✓"
		}
		out = append(out, table.Row{
			mark + " " + strconv.Itoa(r.ID),
			r.Name,
			r.Region,
			r.Size,
			r.Status,
			r.IPv4,
		})
	}
	return out
}

func selectedFromPicker(sel map[int]bool) []int {
	var ids []int
	for id, ok := range sel {
		if ok {
			ids = append(ids, id)
		}
	}
	return ids
}

func defaultIfEmpty(s, def string) string {
	if strings.TrimSpace(s) == "" {
		return def
	}
	return s
}

func titleCase(s string) string {
	if s == "" {
		return ""
	}
	return strings.ToUpper(s[:1]) + s[1:]
}

func prep(add bool) string {
	if add {
		return "to"
	}
	return "from"
}
