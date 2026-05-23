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
)

func (m Model) updateSpaces(k tea.KeyMsg, cmds []tea.Cmd) (Model, []tea.Cmd) {
	if m.spacesClient == nil {
		return m, cmds
	}
	if m.busy {
		return m, cmds
	}
	switch {
	case key.Matches(k, m.keys.Refresh):
		cmds = append(cmds, m.refreshBucketsCmd())
	case key.Matches(k, m.keys.Create):
		m.bucketNameIn = newInput("Bucket name", "my-bucket")
		m.bucketNameIn.SetValue("")
		m.bucketNameIn.Focus()
		m.st = stateCreateBucket
	case key.Matches(k, m.keys.Delete):
		i := m.bucketTable.Cursor()
		if i >= 0 && i < len(m.bucketRows) {
			name := m.bucketRows[i].Name
			m.pendingDeleteBucket = name
			m.pendingAct = actDeleteBucket
			m.confirmReturn = stateSpaces
			m.confirmText = fmt.Sprintf("Delete bucket %q?\nAll objects will be permanently deleted.", name)
			m.st = stateConfirm
		}
	case key.Matches(k, m.keys.Enter):
		i := m.bucketTable.Cursor()
		if i >= 0 && i < len(m.bucketRows) {
			name := m.bucketRows[i].Name
			cmds = append(cmds, m.loadObjectsCmd(name))
		}
	}
	return m, cmds
}

func (m Model) updateSpaceObjects(k tea.KeyMsg, cmds []tea.Cmd) (Model, []tea.Cmd) {
	if key.Matches(k, m.keys.Back) {
		m.st = stateSpaces
		return m, cmds
	}
	if m.spacesClient == nil || m.busy {
		return m, cmds
	}
	switch {
	case key.Matches(k, m.keys.Refresh):
		cmds = append(cmds, m.loadObjectsCmd(m.selectedBucket))
	case key.Matches(k, m.keys.Delete):
		i := m.objectTable.Cursor()
		if i >= 0 && i < len(m.objectRows) {
			key := m.objectRows[i].Key
			m.pendingDeleteObjKey = key
			m.pendingAct = actDeleteObject
			m.confirmReturn = stateSpaceObjects
			m.confirmText = fmt.Sprintf("Delete object %q from bucket %q?\nThis cannot be undone.", key, m.selectedBucket)
			m.st = stateConfirm
		}
	}
	return m, cmds
}

func (m Model) updateCreateBucket(k tea.KeyMsg, cmds []tea.Cmd) (Model, []tea.Cmd) {
	if key.Matches(k, m.keys.Back) {
		m.st = stateSpaces
		return m, cmds
	}
	if m.busy {
		return m, cmds
	}
	if k.String() == "enter" {
		name := strings.TrimSpace(m.bucketNameIn.Value())
		if name == "" {
			m.errText = "bucket name is required"
			return m, cmds
		}
		m.pendingAct = actCreateBucket
		m.confirmReturn = stateCreateBucket
		m.confirmText = fmt.Sprintf("Create bucket %q?", name)
		m.st = stateConfirm
		return m, cmds
	}
	var cmd tea.Cmd
	m.bucketNameIn, cmd = m.bucketNameIn.Update(k)
	cmds = append(cmds, cmd)
	return m, cmds
}

func (m Model) viewSpaces() string {
	if m.spacesClient == nil {
		return lipgloss.NewStyle().Faint(true).Render(
			"Spaces not configured.\nAdd spaces.access_key, spaces.secret_key, spaces.region to config.json.",
		) + "\n\n" + m.footer()
	}
	legend := lipgloss.NewStyle().Faint(true).Render("Keys: r refresh | c create | d delete | enter objects | q quit")
	body := m.bucketTable.View()
	if len(m.bucketRows) == 0 {
		body += "\n" + lipgloss.NewStyle().Faint(true).Render("No buckets. Press 'c' to create one.")
	}
	return body + "\n" + legend + "\n" + m.footer()
}

func (m Model) viewSpaceObjects() string {
	h := lipgloss.NewStyle().Bold(true).Render("Objects — " + m.selectedBucket)
	legend := lipgloss.NewStyle().Faint(true).Render("Keys: r refresh | d delete | esc back | q quit")
	body := m.objectTable.View()
	if len(m.objectRows) == 0 {
		body += "\n" + lipgloss.NewStyle().Faint(true).Render("No objects in this bucket.")
	}
	return h + "\n\n" + body + "\n" + legend + "\n" + m.footer()
}

func (m Model) viewCreateBucket() string {
	return lipgloss.NewStyle().Bold(true).Render("Create Bucket") + "\n\n" +
		lipgloss.NewStyle().Faint(true).Render("Enter submit | Esc cancel") + "\n\n" +
		m.bucketNameIn.View() + "\n"
}

func (m Model) refreshBucketsCmd() tea.Cmd {
	if m.spacesClient == nil {
		return nil
	}
	m.busy = true
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(m.ctx, 30*time.Second)
		defer cancel()
		rows, err := m.spacesClient.ListBuckets(ctx)
		if err != nil {
			return apiErrMsg{err: err}
		}
		return bucketsLoadedMsg{rows: rows}
	}
}

func (m Model) loadObjectsCmd(bucket string) tea.Cmd {
	if m.spacesClient == nil {
		return nil
	}
	m.busy = true
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(m.ctx, 30*time.Second)
		defer cancel()
		rows, err := m.spacesClient.ListObjects(ctx, bucket, "")
		if err != nil {
			return apiErrMsg{err: err}
		}
		return objectsLoadedMsg{bucket: bucket, rows: rows}
	}
}

func (m Model) createBucketCmd() tea.Cmd {
	if m.spacesClient == nil {
		return nil
	}
	name := strings.TrimSpace(m.bucketNameIn.Value())
	m.busy = true
	target := "spaces:bucket:" + name
	m.logOp("spaces.bucket.create", target, "requested")
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(m.ctx, 30*time.Second)
		defer cancel()
		if err := m.spacesClient.CreateBucket(ctx, name); err != nil {
			return apiErrMsg{err: err}
		}
		return apiDoneMsg{act: actCreateBucket, status: "Created bucket " + name, target: target}
	}
}

func (m Model) deleteBucketCmd(name string) tea.Cmd {
	if m.spacesClient == nil {
		return nil
	}
	m.busy = true
	target := "spaces:bucket:" + name
	m.logOp("spaces.bucket.delete", target, "requested")
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(m.ctx, 30*time.Second)
		defer cancel()
		if err := m.spacesClient.DeleteBucket(ctx, name); err != nil {
			return apiErrMsg{err: err}
		}
		return apiDoneMsg{act: actDeleteBucket, status: "Deleted bucket " + name, target: target}
	}
}

func (m Model) deleteObjectCmd(bucket, key string) tea.Cmd {
	if m.spacesClient == nil {
		return nil
	}
	m.busy = true
	target := fmt.Sprintf("spaces:%s/%s", bucket, key)
	m.logOp("spaces.object.delete", target, "requested")
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(m.ctx, 30*time.Second)
		defer cancel()
		if err := m.spacesClient.DeleteObject(ctx, bucket, key); err != nil {
			return apiErrMsg{err: err}
		}
		return apiDoneMsg{act: actDeleteObject, status: "Deleted " + key, target: target}
	}
}

func toBucketRows(rows []SpacesBucketRow) []table.Row {
	out := make([]table.Row, 0, len(rows))
	for _, r := range rows {
		out = append(out, table.Row{r.Name, r.Created})
	}
	return out
}

func toObjectRows(rows []SpacesObjectRow) []table.Row {
	out := make([]table.Row, 0, len(rows))
	for _, r := range rows {
		out = append(out, table.Row{
			r.Key,
			humanBytes(r.SizeBytes),
			r.LastModified,
		})
	}
	return out
}

func humanBytes(b int64) string {
	switch {
	case b >= 1<<30:
		return fmt.Sprintf("%.1f GB", float64(b)/(1<<30))
	case b >= 1<<20:
		return fmt.Sprintf("%.1f MB", float64(b)/(1<<20))
	case b >= 1<<10:
		return fmt.Sprintf("%.1f KB", float64(b)/(1<<10))
	default:
		return fmt.Sprintf("%d B", b)
	}
}
