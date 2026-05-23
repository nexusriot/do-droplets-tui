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

// inTextInputState reports whether the current state owns one or more
// textinput.Model widgets that the user might be typing into. Used to gate
// single-letter globals (like `q` quit) so they don't fire mid-typing.
func inTextInputState(st state) bool {
	switch st {
	case stateCreateDroplet, stateCreateVolume, stateCreateReservedIP,
		stateCreateDomain, stateCreateRecord, stateCreateBucket,
		stateCreateVPC, stateResizeVolume, stateSnapshotName, stateAI,
		stateDropletSnapName, stateDropletResize, stateDropletRename,
		stateDropletRebuild, stateCreateFirewall, stateFirewallAddRule,
		stateImageTagFilter, stateImageRename, stateImageTransfer,
		stateCreateAlert:
		return true
	default:
		return false
	}
}

// canSwitchTabNow is the runtime-state-aware gate. It allows tab switching
// whenever tabSwitchAllowed(state) does, AND also when we're inside stateAI
// with no input focused — that way the user can leave the AI tab with a
// single key (e.g. `1`) once they've pressed Esc once to unfocus, instead
// of being forced through stateDroplets.
func (m Model) canSwitchTabNow() bool {
	if tabSwitchAllowed(m.st) {
		return true
	}
	if m.st == stateAI && !m.aiPromptIn.Focused() && !m.aiSystemIn.Focused() {
		return true
	}
	return false
}

// tabSwitchAllowed reports whether 1/2/l tab switching is allowed in a state.
// Only the top-level list/log screens accept it; modal screens do not.
func tabSwitchAllowed(st state) bool {
	switch st {
	case stateDroplets, stateVolumes, stateOpsLog,
		stateSnapshots, stateReservedIPs, stateFirewalls, stateDomains, stateSpaces,
		stateAccount, stateVPCs, stateImages, stateAlerts:
		// NOTE: stateAI is intentionally excluded — it owns text inputs and
		// would swallow the user's prompt characters into tab switching.
		// Esc/q work to leave the AI tab when its inputs are unfocused.
		return true
	default:
		return false
	}
}

func (m Model) currentSelectedVolume() (do.VolumeRow, bool) {
	i := m.volumeTable.Cursor()
	if i < 0 || i >= len(m.volumeRows) {
		return do.VolumeRow{}, false
	}
	return m.volumeRows[i], true
}

func (m Model) currentSelectedSnapshotID() (string, bool) {
	i := m.volSnapTable.Cursor()
	if i < 0 || i >= len(m.volSnapshots) {
		return "", false
	}
	return m.volSnapshots[i].ID, true
}

func (m Model) updateVolumeDetails(k tea.KeyMsg, cmds []tea.Cmd) (Model, []tea.Cmd) {
	if key.Matches(k, m.keys.Back) {
		m.st = stateVolumes
		return m, cmds
	}
	if m.busy || m.volumeDetails == nil {
		return m, cmds
	}

	switch {
	case key.Matches(k, m.keys.Refresh):
		cmds = append(cmds, m.loadVolumeDetailsCmd(m.volumeDetails.ID))

	case key.Matches(k, m.keys.Snapshot):
		m.pendingSnapVolID = m.volumeDetails.ID
		m.snapNameIn = newInput("Snapshot name", m.volumeDetails.Name+"-snap")
		m.snapNameIn.Focus()
		m.st = stateSnapshotName

	case key.Matches(k, m.keys.Delete):
		if id, ok := m.currentSelectedSnapshotID(); ok {
			m.pendingDeleteSnapID = id
			m.pendingAct = actDeleteSnapshot
			m.confirmReturn = stateVolumeDetails
			m.confirmText = fmt.Sprintf("Delete snapshot %s?\nThis cannot be undone.", id)
			m.st = stateConfirm
		}
	}
	return m, cmds
}

func (m Model) viewVolumeDetails() string {
	if m.volumeDetails == nil {
		return "No volume loaded\n\n" + m.footer()
	}
	v := m.volumeDetails
	var b strings.Builder
	b.WriteString(lipgloss.NewStyle().Bold(true).Render("Volume Details") + "\n\n")
	region := ""
	if v.Region != nil {
		region = v.Region.Slug
	}
	fmt.Fprintf(&b, "ID: %s\n", v.ID)
	fmt.Fprintf(&b, "Name: %s\n", v.Name)
	fmt.Fprintf(&b, "Region: %s\n", region)
	fmt.Fprintf(&b, "Size: %d GB\n", v.SizeGigaBytes)
	fmt.Fprintf(&b, "Description: %s\n", v.Description)
	attached := "-"
	if len(v.DropletIDs) > 0 {
		parts := make([]string, 0, len(v.DropletIDs))
		for _, id := range v.DropletIDs {
			parts = append(parts, strconv.Itoa(id))
		}
		attached = strings.Join(parts, ", ")
	}
	fmt.Fprintf(&b, "Attached droplets: %s\n", attached)
	fmt.Fprintf(&b, "Created: %s\n\n", v.CreatedAt.Format("2006-01-02 15:04:05"))

	b.WriteString(lipgloss.NewStyle().Bold(true).Render("Snapshots") + "\n")
	b.WriteString(m.volSnapTable.View() + "\n")
	if len(m.volSnapshots) == 0 {
		b.WriteString(lipgloss.NewStyle().Faint(true).Render("No snapshots. Press 'n' to create one.") + "\n")
	}
	b.WriteString(lipgloss.NewStyle().Faint(true).Render("Keys: esc back | r refresh | n new snapshot | d delete snapshot | q quit") + "\n")
	return b.String() + m.footer()
}

func (m Model) updateAttachVolume(k tea.KeyMsg, cmds []tea.Cmd) (Model, []tea.Cmd) {
	if key.Matches(k, m.keys.Back) {
		m.st = stateVolumes
		return m, cmds
	}
	if m.busy {
		return m, cmds
	}
	if key.Matches(k, m.keys.Enter) {
		i := m.attachTable.Cursor()
		if i < 0 || i >= len(m.dropletRows) {
			return m, cmds
		}
		d := m.dropletRows[i]
		m.pendingAttachDropID = d.ID
		m.pendingAct = actAttachVolume
		m.confirmReturn = stateVolumes
		m.confirmText = fmt.Sprintf("Attach volume %s to droplet %d (%s)?", m.pendingAttachVolID, d.ID, d.Name)
		m.st = stateConfirm
	}
	return m, cmds
}

func (m Model) viewAttachVolume() string {
	h := lipgloss.NewStyle().Bold(true).Render("Attach Volume — pick a droplet")
	legend := lipgloss.NewStyle().Faint(true).Render("enter=attach to selected  esc=cancel")
	body := m.attachTable.View()
	if len(m.dropletRows) == 0 {
		body += "\n" + lipgloss.NewStyle().Faint(true).Render("No droplets loaded.")
	}
	return h + "\n\n" + body + "\n\n" + legend
}

func (m Model) updateResizeVolume(k tea.KeyMsg, cmds []tea.Cmd) (Model, []tea.Cmd) {
	if key.Matches(k, m.keys.Back) {
		m.st = stateVolumes
		return m, cmds
	}
	if m.busy {
		return m, cmds
	}
	if k.String() == "enter" {
		gb, err := strconv.ParseInt(strings.TrimSpace(m.resizeIn.Value()), 10, 64)
		if err != nil || gb <= 0 {
			m.errText = "new size must be a positive integer"
			return m, cmds
		}
		m.pendingResizeGB = gb
		m.pendingAct = actResizeVolume
		m.confirmReturn = stateResizeVolume
		m.confirmText = fmt.Sprintf("Resize volume %s to %d GB?\nVolumes can only grow, not shrink.", m.pendingResizeVolID, gb)
		m.st = stateConfirm
		return m, cmds
	}
	var cmd tea.Cmd
	m.resizeIn, cmd = m.resizeIn.Update(k)
	cmds = append(cmds, cmd)
	return m, cmds
}

func (m Model) viewResizeVolume() string {
	var b strings.Builder
	b.WriteString(lipgloss.NewStyle().Bold(true).Render("Resize Volume") + "\n\n")
	b.WriteString(lipgloss.NewStyle().Faint(true).Render("Enter submit (confirm) | Esc cancel") + "\n\n")
	b.WriteString(m.resizeIn.View() + "\n")
	return b.String()
}

func (m Model) updateSnapshotName(k tea.KeyMsg, cmds []tea.Cmd) (Model, []tea.Cmd) {
	if key.Matches(k, m.keys.Back) {
		m.st = stateVolumeDetails
		return m, cmds
	}
	if m.busy {
		return m, cmds
	}
	if k.String() == "enter" {
		name := strings.TrimSpace(m.snapNameIn.Value())
		if name == "" {
			m.errText = "snapshot name is required"
			return m, cmds
		}
		m.pendingSnapName = name
		m.pendingAct = actCreateSnapshot
		m.confirmReturn = stateSnapshotName
		m.confirmText = fmt.Sprintf("Create snapshot %q of volume %s?", name, m.pendingSnapVolID)
		m.st = stateConfirm
		return m, cmds
	}
	var cmd tea.Cmd
	m.snapNameIn, cmd = m.snapNameIn.Update(k)
	cmds = append(cmds, cmd)
	return m, cmds
}

func (m Model) viewSnapshotName() string {
	var b strings.Builder
	b.WriteString(lipgloss.NewStyle().Bold(true).Render("Create Volume Snapshot") + "\n\n")
	b.WriteString(lipgloss.NewStyle().Faint(true).Render("Enter submit (confirm) | Esc cancel") + "\n\n")
	b.WriteString(m.snapNameIn.View() + "\n")
	return b.String()
}

func (m Model) loadVolumeDetailsCmd(id string) tea.Cmd {
	m.busy = true
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(m.ctx, 30*time.Second)
		defer cancel()
		v, err := m.api.GetVolume(ctx, id)
		if err != nil {
			return apiErrMsg{err: err}
		}
		snaps, err := m.api.ListVolumeSnapshots(ctx, id)
		if err != nil {
			return apiErrMsg{err: err}
		}
		return volumeDetailsMsg{v: v, snaps: snaps}
	}
}

func (m Model) attachVolumeCmd(volID string, dropletID int) tea.Cmd {
	m.busy = true
	target := fmt.Sprintf("volume:%s droplet:%d", volID, dropletID)
	m.logOp("volume.attach", target, "requested")
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(m.ctx, 60*time.Second)
		defer cancel()
		if err := m.api.AttachVolume(ctx, volID, dropletID); err != nil {
			return apiErrMsg{err: err}
		}
		return apiDoneMsg{act: actAttachVolume, status: "Volume attached", target: target}
	}
}

func (m Model) detachVolumeCmd(volID string, dropletID int) tea.Cmd {
	m.busy = true
	target := fmt.Sprintf("volume:%s droplet:%d", volID, dropletID)
	m.logOp("volume.detach", target, "requested")
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(m.ctx, 60*time.Second)
		defer cancel()
		if err := m.api.DetachVolume(ctx, volID, dropletID); err != nil {
			return apiErrMsg{err: err}
		}
		return apiDoneMsg{act: actDetachVolume, status: "Volume detached", target: target}
	}
}

func (m Model) resizeVolumeCmd(volID, region string, sizeGB int64) tea.Cmd {
	m.busy = true
	target := "volume:" + volID
	m.logOp("volume.resize", target, "requested")
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(m.ctx, 60*time.Second)
		defer cancel()
		if err := m.api.ResizeVolume(ctx, volID, region, sizeGB); err != nil {
			return apiErrMsg{err: err}
		}
		return apiDoneMsg{act: actResizeVolume, status: fmt.Sprintf("Volume resized to %d GB", sizeGB), target: target}
	}
}

func (m Model) createSnapshotCmd(volID, name string) tea.Cmd {
	m.busy = true
	target := "volume:" + volID
	m.logOp("snapshot.create", target, "requested")
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(m.ctx, 60*time.Second)
		defer cancel()
		s, err := m.api.CreateVolumeSnapshot(ctx, volID, name)
		if err != nil {
			return apiErrMsg{err: err}
		}
		return apiDoneMsg{act: actCreateSnapshot, status: fmt.Sprintf("Created snapshot %q (id=%s)", s.Name, s.ID), target: target}
	}
}

func (m Model) deleteSnapshotCmd(id string) tea.Cmd {
	m.busy = true
	target := "snapshot:" + id
	m.logOp("snapshot.delete", target, "requested")
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(m.ctx, 60*time.Second)
		defer cancel()
		if err := m.api.DeleteSnapshot(ctx, id); err != nil {
			return apiErrMsg{err: err}
		}
		return apiDoneMsg{act: actDeleteSnapshot, status: "Deleted snapshot " + id, target: target}
	}
}

func toSnapshotRows(rows []do.SnapshotRow) []table.Row {
	out := make([]table.Row, 0, len(rows))
	for _, r := range rows {
		out = append(out, table.Row{
			r.Name,
			strconv.FormatFloat(r.SizeGB, 'f', -1, 64),
			r.Created,
			r.ID,
		})
	}
	return out
}
