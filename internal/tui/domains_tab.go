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

func (m Model) updateDomains(k tea.KeyMsg, cmds []tea.Cmd) (Model, []tea.Cmd) {
	if m.busy {
		return m, cmds
	}
	switch {
	case key.Matches(k, m.keys.Refresh):
		cmds = append(cmds, m.refreshDomainsCmd())
	case key.Matches(k, m.keys.Create):
		m.domainNameIn = newInput("Domain name", "example.com")
		m.domainIPIn = newInput("A-record IP (optional)", "")
		m.domainNameIn.SetValue("")
		m.domainIPIn.SetValue("")
		m.focusDomainForm = 0
		m.domainNameIn.Focus()
		m.domainIPIn.Blur()
		m.st = stateCreateDomain
	case key.Matches(k, m.keys.Delete):
		if name, ok := m.currentSelectedDomain(); ok {
			m.pendingDeleteDomain = name
			m.pendingAct = actDeleteDomain
			m.confirmReturn = stateDomains
			m.confirmText = fmt.Sprintf("Delete domain %q and all its records?\nThis cannot be undone.", name)
			m.st = stateConfirm
		}
	case key.Matches(k, m.keys.Enter):
		if name, ok := m.currentSelectedDomain(); ok {
			cmds = append(cmds, m.loadDomainRecordsCmd(name))
		}
	}
	return m, cmds
}

func (m Model) updateCreateDomain(k tea.KeyMsg, cmds []tea.Cmd) (Model, []tea.Cmd) {
	if key.Matches(k, m.keys.Back) {
		m.st = stateDomains
		return m, cmds
	}
	if m.busy {
		return m, cmds
	}
	if k.String() == "tab" || k.String() == "shift+tab" {
		if k.String() == "tab" {
			m.focusDomainForm = (m.focusDomainForm + 1) % 2
		} else {
			m.focusDomainForm = (m.focusDomainForm - 1 + 2) % 2
		}
		if m.focusDomainForm == 0 {
			m.domainNameIn.Focus()
			m.domainIPIn.Blur()
		} else {
			m.domainIPIn.Focus()
			m.domainNameIn.Blur()
		}
		return m, cmds
	}
	if k.String() == "enter" {
		name := strings.TrimSpace(m.domainNameIn.Value())
		if name == "" {
			m.errText = "domain name is required"
			return m, cmds
		}
		m.pendingAct = actCreateDomain
		m.confirmReturn = stateCreateDomain
		m.confirmText = fmt.Sprintf("Create domain %q?", name)
		m.st = stateConfirm
		return m, cmds
	}
	var cmd tea.Cmd
	if m.focusDomainForm == 0 {
		m.domainNameIn, cmd = m.domainNameIn.Update(k)
	} else {
		m.domainIPIn, cmd = m.domainIPIn.Update(k)
	}
	cmds = append(cmds, cmd)
	return m, cmds
}

func (m Model) updateDomainRecords(k tea.KeyMsg, cmds []tea.Cmd) (Model, []tea.Cmd) {
	if key.Matches(k, m.keys.Back) {
		m.st = stateDomains
		return m, cmds
	}
	if m.busy {
		return m, cmds
	}
	switch {
	case key.Matches(k, m.keys.Refresh):
		cmds = append(cmds, m.loadDomainRecordsCmd(m.selectedDomain))
	case key.Matches(k, m.keys.Create):
		m.recTypeIn = newInput("Type (A/AAAA/CNAME/MX/TXT…)", "A")
		m.recNameIn = newInput("Name (@=apex)", "@")
		m.recDataIn = newInput("Data/Value", "")
		m.recTTLIn = newInput("TTL (seconds)", "1800")
		m.recPrioIn = newInput("Priority (MX only, else 0)", "0")
		m.blurRecForm()
		m.focusRecForm = 0
		m.recTypeIn.Focus()
		m.st = stateCreateRecord
	case key.Matches(k, m.keys.Delete):
		i := m.domainRecordTable.Cursor()
		if i >= 0 && i < len(m.domainRecordRows) {
			rec := m.domainRecordRows[i]
			m.pendingDeleteRecordID = rec.ID
			m.pendingAct = actDeleteDNSRecord
			m.confirmReturn = stateDomainRecords
			m.confirmText = fmt.Sprintf("Delete %s record %q from %s?", rec.Type, rec.Name, m.selectedDomain)
			m.st = stateConfirm
		}
	}
	return m, cmds
}

func (m Model) updateCreateRecord(k tea.KeyMsg, cmds []tea.Cmd) (Model, []tea.Cmd) {
	if key.Matches(k, m.keys.Back) {
		m.st = stateDomainRecords
		return m, cmds
	}
	if m.busy {
		return m, cmds
	}
	const recFields = 5
	if k.String() == "tab" || k.String() == "shift+tab" {
		m.blurRecForm()
		if k.String() == "tab" {
			m.focusRecForm = (m.focusRecForm + 1) % recFields
		} else {
			m.focusRecForm = (m.focusRecForm - 1 + recFields) % recFields
		}
		m.focusRecOne()
		return m, cmds
	}
	if k.String() == "enter" {
		recType := strings.TrimSpace(m.recTypeIn.Value())
		if recType == "" {
			m.errText = "record type is required"
			return m, cmds
		}
		ttl, _ := strconv.Atoi(strings.TrimSpace(m.recTTLIn.Value()))
		if ttl <= 0 {
			ttl = 1800
		}
		prio, _ := strconv.Atoi(strings.TrimSpace(m.recPrioIn.Value()))
		m.pendingAct = actCreateDNSRecord
		m.confirmReturn = stateCreateRecord
		m.confirmText = fmt.Sprintf("Create %s record %q → %q in %s?",
			recType, m.recNameIn.Value(), m.recDataIn.Value(), m.selectedDomain)
		m.st = stateConfirm
		_ = prio
		return m, cmds
	}
	var cmd tea.Cmd
	switch m.focusRecForm {
	case 0:
		m.recTypeIn, cmd = m.recTypeIn.Update(k)
	case 1:
		m.recNameIn, cmd = m.recNameIn.Update(k)
	case 2:
		m.recDataIn, cmd = m.recDataIn.Update(k)
	case 3:
		m.recTTLIn, cmd = m.recTTLIn.Update(k)
	case 4:
		m.recPrioIn, cmd = m.recPrioIn.Update(k)
	}
	cmds = append(cmds, cmd)
	return m, cmds
}

func (m *Model) blurRecForm() {
	m.recTypeIn.Blur()
	m.recNameIn.Blur()
	m.recDataIn.Blur()
	m.recTTLIn.Blur()
	m.recPrioIn.Blur()
}

func (m *Model) focusRecOne() {
	switch m.focusRecForm {
	case 0:
		m.recTypeIn.Focus()
	case 1:
		m.recNameIn.Focus()
	case 2:
		m.recDataIn.Focus()
	case 3:
		m.recTTLIn.Focus()
	case 4:
		m.recPrioIn.Focus()
	}
}

func (m Model) viewDomains() string {
	legend := lipgloss.NewStyle().Faint(true).Render("Keys: r refresh | c create | d delete | enter records | q quit")
	body := m.domainTable.View()
	if len(m.domainRows) == 0 {
		body += "\n" + lipgloss.NewStyle().Faint(true).Render("No domains found. Press 'c' to create one.")
	}
	return body + "\n" + legend + "\n" + m.footer()
}

func (m Model) viewCreateDomain() string {
	var b strings.Builder
	b.WriteString(lipgloss.NewStyle().Bold(true).Render("Create Domain") + "\n\n")
	b.WriteString(lipgloss.NewStyle().Faint(true).Render("Tab/Shift+Tab move | Enter submit | Esc cancel") + "\n\n")
	b.WriteString(m.domainNameIn.View() + "\n")
	b.WriteString(m.domainIPIn.View() + "\n")
	return b.String()
}

func (m Model) viewDomainRecords() string {
	h := lipgloss.NewStyle().Bold(true).Render("DNS Records — " + m.selectedDomain)
	legend := lipgloss.NewStyle().Faint(true).Render("Keys: r refresh | c new record | d delete | esc back | q quit")
	body := m.domainRecordTable.View()
	if len(m.domainRecordRows) == 0 {
		body += "\n" + lipgloss.NewStyle().Faint(true).Render("No records. Press 'c' to create one.")
	}
	return h + "\n\n" + body + "\n" + legend + "\n" + m.footer()
}

func (m Model) viewCreateRecord() string {
	var b strings.Builder
	b.WriteString(lipgloss.NewStyle().Bold(true).Render("Create DNS Record — "+m.selectedDomain) + "\n\n")
	b.WriteString(lipgloss.NewStyle().Faint(true).Render("Tab/Shift+Tab move | Enter submit | Esc cancel") + "\n\n")
	b.WriteString(m.recTypeIn.View() + "\n")
	b.WriteString(m.recNameIn.View() + "\n")
	b.WriteString(m.recDataIn.View() + "\n")
	b.WriteString(m.recTTLIn.View() + "\n")
	b.WriteString(m.recPrioIn.View() + "\n")
	return b.String()
}

func (m Model) refreshDomainsCmd() tea.Cmd {
	m.busy = true
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(m.ctx, 30*time.Second)
		defer cancel()
		rows, err := m.api.ListDomains(ctx)
		if err != nil {
			return apiErrMsg{err: err}
		}
		return domainsLoadedMsg{rows: rows}
	}
}

func (m Model) loadDomainRecordsCmd(domain string) tea.Cmd {
	m.busy = true
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(m.ctx, 30*time.Second)
		defer cancel()
		rows, err := m.api.ListDomainRecords(ctx, domain)
		if err != nil {
			return apiErrMsg{err: err}
		}
		return domainRecordsLoadedMsg{domain: domain, rows: rows}
	}
}

func (m Model) createDomainCmd() tea.Cmd {
	name := strings.TrimSpace(m.domainNameIn.Value())
	ip := strings.TrimSpace(m.domainIPIn.Value())
	m.busy = true
	target := "domain:" + name
	m.logOp("domain.create", target, "requested")
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(m.ctx, 30*time.Second)
		defer cancel()
		d, err := m.api.CreateDomain(ctx, do.CreateDomainReq{Name: name, IPAddress: ip})
		if err != nil {
			return apiErrMsg{err: err}
		}
		return apiDoneMsg{act: actCreateDomain, status: "Created domain " + d.Name, target: target}
	}
}

func (m Model) deleteDomainCmd(name string) tea.Cmd {
	m.busy = true
	target := "domain:" + name
	m.logOp("domain.delete", target, "requested")
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(m.ctx, 30*time.Second)
		defer cancel()
		if err := m.api.DeleteDomain(ctx, name); err != nil {
			return apiErrMsg{err: err}
		}
		return apiDoneMsg{act: actDeleteDomain, status: "Deleted domain " + name, target: target}
	}
}

func (m Model) createDNSRecordCmd() tea.Cmd {
	recType := strings.TrimSpace(m.recTypeIn.Value())
	name := strings.TrimSpace(m.recNameIn.Value())
	data := strings.TrimSpace(m.recDataIn.Value())
	ttl, _ := strconv.Atoi(strings.TrimSpace(m.recTTLIn.Value()))
	prio, _ := strconv.Atoi(strings.TrimSpace(m.recPrioIn.Value()))
	domain := m.selectedDomain
	if ttl <= 0 {
		ttl = 1800
	}
	m.busy = true
	target := "domain:" + domain + " " + recType
	m.logOp("dns.create", target, "requested")
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(m.ctx, 30*time.Second)
		defer cancel()
		r, err := m.api.CreateDomainRecord(ctx, do.CreateRecordReq{
			Domain:   domain,
			Type:     recType,
			Name:     name,
			Data:     data,
			TTL:      ttl,
			Priority: prio,
		})
		if err != nil {
			return apiErrMsg{err: err}
		}
		return apiDoneMsg{act: actCreateDNSRecord, status: fmt.Sprintf("Created %s record (id=%d)", r.Type, r.ID), target: target}
	}
}

func (m Model) deleteDNSRecordCmd(domain string, id int) tea.Cmd {
	m.busy = true
	target := fmt.Sprintf("domain:%s record:%d", domain, id)
	m.logOp("dns.delete", target, "requested")
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(m.ctx, 30*time.Second)
		defer cancel()
		if err := m.api.DeleteDomainRecord(ctx, domain, id); err != nil {
			return apiErrMsg{err: err}
		}
		return apiDoneMsg{act: actDeleteDNSRecord, status: fmt.Sprintf("Deleted record %d from %s", id, domain), target: target}
	}
}

func (m Model) currentSelectedDomain() (string, bool) {
	i := m.domainTable.Cursor()
	if i < 0 || i >= len(m.domainRows) {
		return "", false
	}
	return m.domainRows[i].Name, true
}

func toDomainRows(rows []do.DomainRow) []table.Row {
	out := make([]table.Row, 0, len(rows))
	for _, r := range rows {
		out = append(out, table.Row{r.Name, strconv.Itoa(r.TTL)})
	}
	return out
}

func toDomainRecordRows(rows []do.DomainRecordRow) []table.Row {
	out := make([]table.Row, 0, len(rows))
	for _, r := range rows {
		out = append(out, table.Row{r.Type, r.Name, r.Data, strconv.Itoa(r.TTL)})
	}
	return out
}
