package tui

import (
	"context"
	"fmt"
	"time"

	"github.com/charmbracelet/bubbles/key"
	"github.com/charmbracelet/bubbles/table"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/nexusriot/do-droplets-tui/internal/do"
)

func (m Model) updateReservedIPs(k tea.KeyMsg, cmds []tea.Cmd) (Model, []tea.Cmd) {
	if m.busy {
		return m, cmds
	}
	switch {
	case key.Matches(k, m.keys.Refresh):
		cmds = append(cmds, m.refreshReservedIPsCmd())

	case key.Matches(k, m.keys.Create):
		m.createIPIn = newInput("Region", "fra1")
		m.createIPIn.SetValue("")
		m.createIPIn.Focus()
		m.st = stateCreateReservedIP

	case key.Matches(k, m.keys.Delete):
		if ip, ok := m.currentSelectedIP(); ok {
			m.pendingDeleteIP = ip
			m.pendingAct = actDeleteReservedIP
			m.confirmReturn = stateReservedIPs
			m.confirmText = fmt.Sprintf("Delete reserved IP %s?\nThis cannot be undone.", ip)
			m.st = stateConfirm
		}

	case key.Matches(k, m.keys.Attach):
		if ip, ok := m.currentSelectedIP(); ok {
			m.pendingAssignIP = ip
			m.st = stateAssignReservedIP
			if len(m.dropletRows) == 0 {
				cmds = append(cmds, m.refreshDropletsCmd())
			} else {
				m.assignTable.SetRows(toDropletTableRows(m.dropletRows))
			}
		}

	case key.Matches(k, m.keys.Detach):
		if ip, ok := m.currentSelectedIP(); ok {
			m.pendingUnassignIP = ip
			m.pendingAct = actUnassignReservedIP
			m.confirmReturn = stateReservedIPs
			m.confirmText = fmt.Sprintf("Unassign IP %s from its droplet?", ip)
			m.st = stateConfirm
		}
	}
	return m, cmds
}

func (m Model) updateCreateReservedIP(k tea.KeyMsg, cmds []tea.Cmd) (Model, []tea.Cmd) {
	if key.Matches(k, m.keys.Back) {
		m.st = stateReservedIPs
		return m, cmds
	}
	if m.busy {
		return m, cmds
	}
	if k.String() == "enter" {
		region := m.createIPIn.Value()
		if region == "" {
			m.errText = "region is required"
			return m, cmds
		}
		m.pendingAct = actCreateReservedIP
		m.confirmReturn = stateCreateReservedIP
		m.confirmText = fmt.Sprintf("Create reserved IP in region %q?", region)
		m.st = stateConfirm
		return m, cmds
	}
	var cmd tea.Cmd
	m.createIPIn, cmd = m.createIPIn.Update(k)
	cmds = append(cmds, cmd)
	return m, cmds
}

func (m Model) updateAssignReservedIP(k tea.KeyMsg, cmds []tea.Cmd) (Model, []tea.Cmd) {
	if key.Matches(k, m.keys.Back) {
		m.st = stateReservedIPs
		return m, cmds
	}
	if m.busy {
		return m, cmds
	}
	if key.Matches(k, m.keys.Enter) {
		i := m.assignTable.Cursor()
		if i < 0 || i >= len(m.dropletRows) {
			return m, cmds
		}
		d := m.dropletRows[i]
		m.pendingAssignDrop = d.ID
		m.pendingAct = actAssignReservedIP
		m.confirmReturn = stateReservedIPs
		m.confirmText = fmt.Sprintf("Assign IP %s to droplet %d (%s)?", m.pendingAssignIP, d.ID, d.Name)
		m.st = stateConfirm
	}
	return m, cmds
}

func (m Model) viewReservedIPs() string {
	legend := lipgloss.NewStyle().Faint(true).Render("Keys: r refresh | c create | d delete | a assign | x unassign | q quit")
	body := m.reservedIPTable.View()
	if len(m.reservedIPRows) == 0 {
		body += "\n" + lipgloss.NewStyle().Faint(true).Render("No reserved IPs. Press 'c' to create one.")
	}
	return body + "\n" + legend + "\n" + m.footer()
}

func (m Model) viewCreateReservedIP() string {
	return lipgloss.NewStyle().Bold(true).Render("Create Reserved IP") + "\n\n" +
		lipgloss.NewStyle().Faint(true).Render("Enter submit | Esc cancel") + "\n\n" +
		m.createIPIn.View() + "\n"
}

func (m Model) viewAssignReservedIP() string {
	h := lipgloss.NewStyle().Bold(true).Render(fmt.Sprintf("Assign IP %s — pick a droplet", m.pendingAssignIP))
	legend := lipgloss.NewStyle().Faint(true).Render("enter=assign  esc=cancel")
	return h + "\n\n" + m.assignTable.View() + "\n\n" + legend
}

func (m Model) refreshReservedIPsCmd() tea.Cmd {
	m.busy = true
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(m.ctx, 30*time.Second)
		defer cancel()
		rows, err := m.api.ListReservedIPs(ctx)
		if err != nil {
			return apiErrMsg{err: err}
		}
		return reservedIPsLoadedMsg{rows: rows}
	}
}

func (m Model) createReservedIPCmd() tea.Cmd {
	region := m.createIPIn.Value()
	m.busy = true
	target := "ip:new@" + region
	m.logOp("ip.create", target, "requested")
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(m.ctx, 30*time.Second)
		defer cancel()
		ip, err := m.api.CreateReservedIP(ctx, do.CreateReservedIPReq{Region: region})
		if err != nil {
			return apiErrMsg{err: err}
		}
		return apiDoneMsg{act: actCreateReservedIP, status: "Created IP " + ip.IP, target: target}
	}
}

func (m Model) deleteReservedIPCmd(ip string) tea.Cmd {
	m.busy = true
	target := "ip:" + ip
	m.logOp("ip.delete", target, "requested")
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(m.ctx, 30*time.Second)
		defer cancel()
		if err := m.api.DeleteReservedIP(ctx, ip); err != nil {
			return apiErrMsg{err: err}
		}
		return apiDoneMsg{act: actDeleteReservedIP, status: "Deleted IP " + ip, target: target}
	}
}

func (m Model) assignReservedIPCmd(ip string, dropletID int) tea.Cmd {
	m.busy = true
	target := fmt.Sprintf("ip:%s droplet:%d", ip, dropletID)
	m.logOp("ip.assign", target, "requested")
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(m.ctx, 30*time.Second)
		defer cancel()
		if err := m.api.AssignReservedIP(ctx, ip, dropletID); err != nil {
			return apiErrMsg{err: err}
		}
		return apiDoneMsg{act: actAssignReservedIP, status: "Assigned IP " + ip, target: target}
	}
}

func (m Model) unassignReservedIPCmd(ip string) tea.Cmd {
	m.busy = true
	target := "ip:" + ip
	m.logOp("ip.unassign", target, "requested")
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(m.ctx, 30*time.Second)
		defer cancel()
		if err := m.api.UnassignReservedIP(ctx, ip); err != nil {
			return apiErrMsg{err: err}
		}
		return apiDoneMsg{act: actUnassignReservedIP, status: "Unassigned IP " + ip, target: target}
	}
}

func (m Model) currentSelectedIP() (string, bool) {
	i := m.reservedIPTable.Cursor()
	if i < 0 || i >= len(m.reservedIPRows) {
		return "", false
	}
	return m.reservedIPRows[i].IP, true
}

func toReservedIPRows(rows []do.ReservedIPRow) []table.Row {
	out := make([]table.Row, 0, len(rows))
	for _, r := range rows {
		droplet := "-"
		if r.DropletName != "" {
			droplet = fmt.Sprintf("%s (%d)", r.DropletName, r.DropletID)
		}
		out = append(out, table.Row{r.IP, r.Region, droplet})
	}
	return out
}
