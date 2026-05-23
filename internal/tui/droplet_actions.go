package tui

import (
	"context"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/charmbracelet/bubbles/key"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

/* ============================================================
   Extended droplet actions
   ============================================================

   These all dispatch from stateDetails (per-droplet) only. They use
   CAPITAL-letter keys so they never collide with vim-style navigation
   (lowercase j/k) and don't fire from text-input states.

     S  snapshot droplet
     E  resize droplet (must be off; defaults to flexible/CPU+RAM only,
        space toggles disk resize on the form)
     B  rebuild droplet (from image slug or numeric image ID)
     M  rename (Modify) droplet
     I  enable IPv6
     V  enable priVate networking
     W  password reset (Web console password)
     Y  power cYcle
     U  enable/disable backUps  (the action is toggled per current state)

   ============================================================ */

func (m Model) updateDetailsExt(k tea.KeyMsg, cmds []tea.Cmd) (Model, []tea.Cmd, bool) {
	// Returns (model, cmds, handled). Caller (updateDetails) does its own
	// routing for the original keys; we just slot in the new actions here.
	id := m.selectedDropletID
	switch k.String() {
	case "S":
		m.dropSnapNameIn = newInput("Snapshot name", fmt.Sprintf("droplet-%d-snap", id))
		m.dropSnapNameIn.Focus()
		m.st = stateDropletSnapName
		return m, cmds, true

	case "E":
		m.dropResizeIn = newInput("New size slug", "s-2vcpu-2gb")
		m.dropResizeDisk = false
		m.dropResizeIn.Focus()
		m.st = stateDropletResize
		return m, cmds, true

	case "B":
		m.dropRebuildIn = newInput("Image slug or ID", "ubuntu-24-04-x64")
		m.dropRebuildIn.Focus()
		m.st = stateDropletRebuild
		return m, cmds, true

	case "M":
		m.dropRenameIn = newInput("New name", "")
		m.dropRenameIn.Focus()
		m.st = stateDropletRename
		return m, cmds, true

	case "I":
		m.pendingAct = actEnableIPv6
		m.confirmReturn = stateDetails
		m.confirmText = fmt.Sprintf("Enable IPv6 on droplet %d?", id)
		m.st = stateConfirm
		return m, cmds, true

	case "V":
		m.pendingAct = actEnablePrivateNet
		m.confirmReturn = stateDetails
		m.confirmText = fmt.Sprintf("Enable private networking on droplet %d?", id)
		m.st = stateConfirm
		return m, cmds, true

	case "W":
		m.pendingAct = actPasswordReset
		m.confirmReturn = stateDetails
		m.confirmText = fmt.Sprintf("Reset root password on droplet %d?\n(new password emailed to your account)", id)
		m.st = stateConfirm
		return m, cmds, true

	case "Y":
		m.pendingAct = actPowerCycle
		m.confirmReturn = stateDetails
		m.confirmText = fmt.Sprintf("Power-cycle (hard reset) droplet %d?", id)
		m.st = stateConfirm
		return m, cmds, true

	case "G":
		// Graph: open metrics overlay and start loading CPU samples.
		m.dropletMetrics = nil
		m.st = stateDropletMetrics
		cmds = append(cmds, m.loadDropletMetricsCmd(id))
		return m, cmds, true

	case "U":
		// Toggle: figure out current state.
		enable := true
		if m.dropletDetails != nil && m.dropletDetails.BackupIDs != nil && len(m.dropletDetails.BackupIDs) > 0 {
			enable = false
		}
		if enable {
			m.pendingAct = actEnableBackups
			m.confirmText = fmt.Sprintf("Enable automatic backups on droplet %d?", id)
		} else {
			m.pendingAct = actDisableBackups
			m.confirmText = fmt.Sprintf("Disable automatic backups on droplet %d?", id)
		}
		m.confirmReturn = stateDetails
		m.st = stateConfirm
		return m, cmds, true
	}
	return m, cmds, false
}

func (m Model) updateDropletSnapName(k tea.KeyMsg, cmds []tea.Cmd) (Model, []tea.Cmd) {
	if key.Matches(k, m.keys.Back) {
		m.st = stateDetails
		return m, cmds
	}
	if m.busy {
		return m, cmds
	}
	if k.String() == "enter" {
		name := strings.TrimSpace(m.dropSnapNameIn.Value())
		if name == "" {
			m.errText = "snapshot name is required"
			return m, cmds
		}
		m.pendingDropletSnap = name
		m.pendingAct = actSnapshotDroplet
		m.confirmReturn = stateDropletSnapName
		m.confirmText = fmt.Sprintf("Snapshot droplet %d as %q?", m.selectedDropletID, name)
		m.st = stateConfirm
		return m, cmds
	}
	var cmd tea.Cmd
	m.dropSnapNameIn, cmd = m.dropSnapNameIn.Update(k)
	cmds = append(cmds, cmd)
	return m, cmds
}

func (m Model) updateDropletResize(k tea.KeyMsg, cmds []tea.Cmd) (Model, []tea.Cmd) {
	if key.Matches(k, m.keys.Back) {
		m.st = stateDetails
		return m, cmds
	}
	if m.busy {
		return m, cmds
	}
	if k.String() == " " {
		m.dropResizeDisk = !m.dropResizeDisk
		return m, cmds
	}
	if k.String() == "enter" {
		size := strings.TrimSpace(m.dropResizeIn.Value())
		if size == "" {
			m.errText = "size slug is required"
			return m, cmds
		}
		m.pendingDropletNewSize = size
		m.pendingAct = actResizeDroplet
		m.confirmReturn = stateDropletResize
		warn := ""
		if m.dropResizeDisk {
			warn = "\n\nWARNING: disk resize is IRREVERSIBLE — you cannot downgrade later."
		}
		m.confirmText = fmt.Sprintf("Resize droplet %d to %q (disk=%v)?\nDroplet must be powered off.%s",
			m.selectedDropletID, size, m.dropResizeDisk, warn)
		m.st = stateConfirm
		return m, cmds
	}
	var cmd tea.Cmd
	m.dropResizeIn, cmd = m.dropResizeIn.Update(k)
	cmds = append(cmds, cmd)
	return m, cmds
}

func (m Model) updateDropletRename(k tea.KeyMsg, cmds []tea.Cmd) (Model, []tea.Cmd) {
	if key.Matches(k, m.keys.Back) {
		m.st = stateDetails
		return m, cmds
	}
	if m.busy {
		return m, cmds
	}
	if k.String() == "enter" {
		name := strings.TrimSpace(m.dropRenameIn.Value())
		if name == "" {
			m.errText = "new name is required"
			return m, cmds
		}
		m.pendingDropletNewName = name
		m.pendingAct = actRenameDroplet
		m.confirmReturn = stateDropletRename
		m.confirmText = fmt.Sprintf("Rename droplet %d to %q?", m.selectedDropletID, name)
		m.st = stateConfirm
		return m, cmds
	}
	var cmd tea.Cmd
	m.dropRenameIn, cmd = m.dropRenameIn.Update(k)
	cmds = append(cmds, cmd)
	return m, cmds
}

func (m Model) updateDropletRebuild(k tea.KeyMsg, cmds []tea.Cmd) (Model, []tea.Cmd) {
	if key.Matches(k, m.keys.Back) {
		m.st = stateDetails
		return m, cmds
	}
	if m.busy {
		return m, cmds
	}
	if k.String() == "enter" {
		img := strings.TrimSpace(m.dropRebuildIn.Value())
		if img == "" {
			m.errText = "image slug or ID is required"
			return m, cmds
		}
		m.pendingDropletRebuild = img
		m.pendingAct = actRebuildDroplet
		m.confirmReturn = stateDropletRebuild
		m.confirmText = fmt.Sprintf("Rebuild droplet %d from image %q?\nAll data on the droplet WILL BE LOST.",
			m.selectedDropletID, img)
		m.st = stateConfirm
		return m, cmds
	}
	var cmd tea.Cmd
	m.dropRebuildIn, cmd = m.dropRebuildIn.Update(k)
	cmds = append(cmds, cmd)
	return m, cmds
}

func (m Model) viewDropletSnapName() string {
	return lipgloss.NewStyle().Bold(true).Render(fmt.Sprintf("Snapshot droplet %d", m.selectedDropletID)) + "\n\n" +
		lipgloss.NewStyle().Faint(true).Render("Enter submit | Esc cancel") + "\n\n" +
		m.dropSnapNameIn.View() + "\n"
}

func (m Model) viewDropletResize() string {
	disk := "[ ]"
	if m.dropResizeDisk {
		disk = "[x]"
	}
	return lipgloss.NewStyle().Bold(true).Render(fmt.Sprintf("Resize droplet %d", m.selectedDropletID)) + "\n\n" +
		lipgloss.NewStyle().Faint(true).Render("Enter submit | Space toggle disk resize | Esc cancel") + "\n\n" +
		m.dropResizeIn.View() + "\n" +
		"Resize disk too: " + disk + "    (irreversible if checked)\n"
}

func (m Model) viewDropletRename() string {
	return lipgloss.NewStyle().Bold(true).Render(fmt.Sprintf("Rename droplet %d", m.selectedDropletID)) + "\n\n" +
		lipgloss.NewStyle().Faint(true).Render("Enter submit | Esc cancel") + "\n\n" +
		m.dropRenameIn.View() + "\n"
}

func (m Model) viewDropletRebuild() string {
	return lipgloss.NewStyle().Bold(true).Render(fmt.Sprintf("Rebuild droplet %d", m.selectedDropletID)) + "\n\n" +
		lipgloss.NewStyle().Faint(true).Render("Enter submit | Esc cancel") + "\n\n" +
		m.dropRebuildIn.View() + "\n"
}

func (m Model) runExtDropletActionCmd(a actionKind) tea.Cmd {
	id := m.selectedDropletID
	target := fmt.Sprintf("droplet:%d", id)
	m.logOp(actToString(a), target, "requested")
	m.busy = true
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(m.ctx, 90*time.Second)
		defer cancel()

		var err error
		switch a {
		case actPowerCycle:
			err = m.api.PowerCycle(ctx, id)
		case actPasswordReset:
			err = m.api.PasswordReset(ctx, id)
		case actEnableIPv6:
			err = m.api.EnableIPv6(ctx, id)
		case actEnablePrivateNet:
			err = m.api.EnablePrivateNetworking(ctx, id)
		case actEnableBackups:
			err = m.api.EnableBackups(ctx, id)
		case actDisableBackups:
			err = m.api.DisableBackups(ctx, id)
		case actSnapshotDroplet:
			err = m.api.SnapshotDroplet(ctx, id, m.pendingDropletSnap)
		case actResizeDroplet:
			err = m.api.ResizeDroplet(ctx, id, m.pendingDropletNewSize, m.dropResizeDisk)
		case actRenameDroplet:
			err = m.api.RenameDroplet(ctx, id, m.pendingDropletNewName)
		case actRebuildDroplet:
			err = m.api.RebuildDroplet(ctx, id, m.pendingDropletRebuild)
		default:
			err = fmt.Errorf("unknown extended droplet action")
		}
		if err != nil {
			return apiErrMsg{err: err}
		}
		return apiDoneMsg{act: a, status: extActionLabel(a) + " ok", target: target}
	}
}

func extActionLabel(a actionKind) string {
	switch a {
	case actPowerCycle:
		return "Power-cycled droplet"
	case actPasswordReset:
		return "Password reset issued"
	case actEnableIPv6:
		return "Enabled IPv6"
	case actEnablePrivateNet:
		return "Enabled private networking"
	case actEnableBackups:
		return "Enabled backups"
	case actDisableBackups:
		return "Disabled backups"
	case actSnapshotDroplet:
		return "Snapshot scheduled"
	case actResizeDroplet:
		return "Resize scheduled"
	case actRenameDroplet:
		return "Renamed droplet"
	case actRebuildDroplet:
		return "Rebuild scheduled"
	}
	return "ok"
}

// helper for displaying optional numeric value
func intStr(v int) string { return strconv.Itoa(v) }
