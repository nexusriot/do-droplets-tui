package tui

import (
	"context"
	"fmt"
	"strconv"
	"time"

	"github.com/charmbracelet/bubbles/key"
	"github.com/charmbracelet/bubbles/table"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/nexusriot/do-droplets-tui/internal/do"
)

func (m Model) updateSnapshots(k tea.KeyMsg, cmds []tea.Cmd) (Model, []tea.Cmd) {
	if m.busy {
		return m, cmds
	}
	switch {
	case key.Matches(k, m.keys.Refresh):
		cmds = append(cmds, m.refreshAllSnapshotsCmd())
	case key.Matches(k, m.keys.Delete):
		i := m.snapshotTable.Cursor()
		if i >= 0 && i < len(m.allSnapshots) {
			snap := m.allSnapshots[i]
			m.pendingAct = actDeleteSnapshot
			m.confirmReturn = stateSnapshots
			m.confirmText = fmt.Sprintf("Delete snapshot %q (%s)?\nThis cannot be undone.", snap.Name, snap.ID)
			// reuse pendingDeleteSnapID
			m.pendingDeleteSnapID = snap.ID
			m.st = stateConfirm
		}
	case key.Matches(k, m.keys.Create):
		// "Create droplet from snapshot": pre-fill the existing droplet form
		// with image=<snapshot.ID> and region=<snapshot.Region>.
		i := m.snapshotTable.Cursor()
		if i >= 0 && i < len(m.allSnapshots) {
			snap := m.allSnapshots[i]
			if snap.ResourceType != "" && snap.ResourceType != "droplet" {
				m.errText = fmt.Sprintf("snapshot %q is a %s snapshot, not a droplet image — use the volume tab",
					snap.Name, snap.ResourceType)
				return m, cmds
			}
			// snap.ID is a numeric string for droplet snapshots.
			imageID, perr := strconv.Atoi(snap.ID)
			if perr != nil {
				m.errText = "snapshot ID is not numeric: " + snap.ID
				return m, cmds
			}
			m.launchDropletFromImage(imageID, snap.Name, snap.Region)
		}
	}
	return m, cmds
}

func (m Model) viewSnapshots() string {
	legend := lipgloss.NewStyle().Faint(true).Render("Keys: r refresh | c create droplet from snapshot | d delete | q quit")
	body := m.snapshotTable.View()
	if len(m.allSnapshots) == 0 {
		body += "\n" + lipgloss.NewStyle().Faint(true).Render("No snapshots found.")
	}
	return body + "\n" + legend + "\n" + m.footer()
}

func (m Model) refreshAllSnapshotsCmd() tea.Cmd {
	m.busy = true
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(m.ctx, 30*time.Second)
		defer cancel()
		rows, err := m.api.ListAllSnapshots(ctx)
		if err != nil {
			return apiErrMsg{err: err}
		}
		return allSnapshotsLoadedMsg{rows: rows}
	}
}

func toAllSnapshotRows(rows []do.SnapshotRow) []table.Row {
	out := make([]table.Row, 0, len(rows))
	for _, r := range rows {
		out = append(out, table.Row{
			r.Name,
			r.ResourceType,
			r.Region,
			strconv.FormatFloat(r.SizeGB, 'f', -1, 64),
			r.Created,
		})
	}
	return out
}
