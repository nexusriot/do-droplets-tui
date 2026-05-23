package tui

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/charmbracelet/bubbles/key"
	"github.com/charmbracelet/bubbles/table"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/nexusriot/do-droplets-tui/internal/do"
)

func (m Model) updateFirewalls(k tea.KeyMsg, cmds []tea.Cmd) (Model, []tea.Cmd) {
	if m.busy {
		return m, cmds
	}
	switch {
	case key.Matches(k, m.keys.Refresh):
		cmds = append(cmds, m.refreshFirewallsCmd())
	case key.Matches(k, m.keys.Enter):
		if fw, ok := m.currentSelectedFirewall(); ok {
			m.selectedFWID = fw.ID
			cmds = append(cmds, m.loadFirewallDetailsCmd(fw.ID))
		}
	case key.Matches(k, m.keys.Delete):
		if fw, ok := m.currentSelectedFirewall(); ok {
			m.pendingDeleteFW = fw.ID
			m.pendingAct = actDeleteFirewall
			m.confirmReturn = stateFirewalls
			m.confirmText = fmt.Sprintf("Delete firewall %q (%s)?\nThis cannot be undone.", fw.Name, fw.ID)
			m.st = stateConfirm
		}
	default:
		if mm, cc, handled := m.firewallsListExt(k, cmds); handled {
			return mm, cc
		}
	}
	return m, cmds
}

func (m Model) updateFirewallDetails(k tea.KeyMsg, cmds []tea.Cmd) (Model, []tea.Cmd) {
	if key.Matches(k, m.keys.Back) {
		m.st = stateFirewalls
		return m, cmds
	}
	if m.busy {
		return m, cmds
	}
	switch {
	case key.Matches(k, m.keys.Refresh):
		if m.selectedFWID != "" {
			cmds = append(cmds, m.loadFirewallDetailsCmd(m.selectedFWID))
		}
	case key.Matches(k, m.keys.Delete):
		if m.firewallDetails != nil {
			m.pendingDeleteFW = m.firewallDetails.Row.ID
			m.pendingAct = actDeleteFirewall
			m.confirmReturn = stateFirewallDetails
			m.confirmText = fmt.Sprintf("Delete firewall %q?\nThis cannot be undone.", m.firewallDetails.Row.Name)
			m.st = stateConfirm
		}
	default:
		if mm, cc, handled := m.firewallDetailsExt(k, cmds); handled {
			return mm, cc
		}
	}
	return m, cmds
}

func (m Model) viewFirewalls() string {
	legend := lipgloss.NewStyle().Faint(true).Render("Keys: r refresh | enter details | c create | d delete | q quit")
	body := m.firewallTable.View()
	if len(m.firewallRows) == 0 {
		body += "\n" + lipgloss.NewStyle().Faint(true).Render("No firewalls found.")
	}
	return body + "\n" + legend + "\n" + m.footer()
}

func (m Model) viewFirewallDetails() string {
	if m.firewallDetails == nil {
		return "No firewall loaded\n\n" + m.footer()
	}
	fw := m.firewallDetails
	var b strings.Builder
	b.WriteString(lipgloss.NewStyle().Bold(true).Render("Firewall Details") + "\n\n")
	fmt.Fprintf(&b, "ID: %s\n", fw.Row.ID)
	fmt.Fprintf(&b, "Name: %s\n", fw.Row.Name)
	fmt.Fprintf(&b, "Status: %s\n", fw.Row.Status)
	fmt.Fprintf(&b, "Attached droplets: %d\n", fw.Row.DropletCount)
	if len(fw.Tags) > 0 {
		fmt.Fprintf(&b, "Tags: %s\n", strings.Join(fw.Tags, ", "))
	}
	b.WriteString("\n")
	b.WriteString(lipgloss.NewStyle().Bold(true).Render("Inbound rules:") + "\n")
	if len(fw.Inbound) == 0 {
		b.WriteString("  (none)\n")
	}
	for _, r := range fw.Inbound {
		fmt.Fprintf(&b, "  %s\n", r)
	}
	b.WriteString("\n")
	b.WriteString(lipgloss.NewStyle().Bold(true).Render("Outbound rules:") + "\n")
	if len(fw.Outbound) == 0 {
		b.WriteString("  (none)\n")
	}
	for _, r := range fw.Outbound {
		fmt.Fprintf(&b, "  %s\n", r)
	}
	b.WriteString("\n")
	b.WriteString(lipgloss.NewStyle().Faint(true).Render(
		"Keys: esc back | r refresh | d delete | a add droplets | x remove droplets | c add rule | q quit") + "\n")
	return b.String() + m.footer()
}

func (m Model) refreshFirewallsCmd() tea.Cmd {
	m.busy = true
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(m.ctx, 30*time.Second)
		defer cancel()
		rows, err := m.api.ListFirewalls(ctx)
		if err != nil {
			return apiErrMsg{err: err}
		}
		return firewallsLoadedMsg{rows: rows}
	}
}

func (m Model) loadFirewallDetailsCmd(id string) tea.Cmd {
	m.busy = true
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(m.ctx, 30*time.Second)
		defer cancel()
		details, err := m.api.GetFirewall(ctx, id)
		if err != nil {
			return apiErrMsg{err: err}
		}
		return firewallDetailsMsg{details: details}
	}
}

func (m Model) deleteFirewallCmd(id string) tea.Cmd {
	m.busy = true
	target := "firewall:" + id
	m.logOp("firewall.delete", target, "requested")
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(m.ctx, 30*time.Second)
		defer cancel()
		if err := m.api.DeleteFirewall(ctx, id); err != nil {
			return apiErrMsg{err: err}
		}
		return apiDoneMsg{act: actDeleteFirewall, status: "Deleted firewall " + id, target: target}
	}
}

func (m Model) currentSelectedFirewall() (do.FirewallRow, bool) {
	i := m.firewallTable.Cursor()
	if i < 0 || i >= len(m.firewallRows) {
		return do.FirewallRow{}, false
	}
	return m.firewallRows[i], true
}

func toFirewallRows(rows []do.FirewallRow) []table.Row {
	out := make([]table.Row, 0, len(rows))
	for _, r := range rows {
		out = append(out, table.Row{
			r.Name,
			r.Status,
			fmt.Sprintf("%d", r.DropletCount),
			fmt.Sprintf("%d", r.InboundCount),
			fmt.Sprintf("%d", r.OutboundCount),
			r.ID,
		})
	}
	return out
}
