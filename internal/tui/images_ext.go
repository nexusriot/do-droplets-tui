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

func (m Model) updateImageRename(k tea.KeyMsg, cmds []tea.Cmd) (Model, []tea.Cmd) {
	if key.Matches(k, m.keys.Back) {
		m.st = stateImages
		return m, cmds
	}
	if m.busy {
		return m, cmds
	}
	if k.String() == "enter" {
		name := strings.TrimSpace(m.imgRenameIn.Value())
		if name == "" {
			m.errText = "name is required"
			return m, cmds
		}
		m.pendingImgNewName = name
		m.pendingAct = actRenameImage
		m.confirmReturn = stateImageRename
		m.confirmText = fmt.Sprintf("Rename image %d to %q?", m.pendingImgID, name)
		m.st = stateConfirm
		return m, cmds
	}
	var cmd tea.Cmd
	m.imgRenameIn, cmd = m.imgRenameIn.Update(k)
	cmds = append(cmds, cmd)
	return m, cmds
}

func (m Model) updateImageTransfer(k tea.KeyMsg, cmds []tea.Cmd) (Model, []tea.Cmd) {
	if key.Matches(k, m.keys.Back) {
		m.st = stateImages
		return m, cmds
	}
	if m.busy {
		return m, cmds
	}
	if k.String() == "enter" {
		region := strings.TrimSpace(m.imgTransferIn.Value())
		if region == "" {
			m.errText = "region is required"
			return m, cmds
		}
		m.pendingImgRegion = region
		m.pendingAct = actTransferImage
		m.confirmReturn = stateImageTransfer
		m.confirmText = fmt.Sprintf("Transfer image %d to region %q?\n(Async — completion shown in DO control panel.)",
			m.pendingImgID, region)
		m.st = stateConfirm
		return m, cmds
	}
	var cmd tea.Cmd
	m.imgTransferIn, cmd = m.imgTransferIn.Update(k)
	cmds = append(cmds, cmd)
	return m, cmds
}

func (m Model) updateImageTagFilter(k tea.KeyMsg, cmds []tea.Cmd) (Model, []tea.Cmd) {
	if key.Matches(k, m.keys.Back) {
		m.st = stateImages
		return m, cmds
	}
	if m.busy {
		return m, cmds
	}
	if k.String() == "enter" {
		tag := strings.TrimSpace(m.imgTagFilterIn.Value())
		if tag == "" {
			m.errText = "tag is required"
			return m, cmds
		}
		m.imagesTag = tag
		m.imagesMode = "tag:" + tag
		cmds = append(cmds, m.refreshImagesCmd(m.imagesMode))
		m.st = stateImages
		return m, cmds
	}
	var cmd tea.Cmd
	m.imgTagFilterIn, cmd = m.imgTagFilterIn.Update(k)
	cmds = append(cmds, cmd)
	return m, cmds
}

func (m Model) viewImageRename() string {
	return lipgloss.NewStyle().Bold(true).Render(fmt.Sprintf("Rename image %d", m.pendingImgID)) + "\n\n" +
		lipgloss.NewStyle().Faint(true).Render("Enter submit | Esc cancel") + "\n\n" +
		m.imgRenameIn.View() + "\n"
}

func (m Model) viewImageTransfer() string {
	return lipgloss.NewStyle().Bold(true).Render(fmt.Sprintf("Transfer image %d", m.pendingImgID)) + "\n\n" +
		lipgloss.NewStyle().Faint(true).Render("Enter submit | Esc cancel") + "\n\n" +
		m.imgTransferIn.View() + "\n"
}

func (m Model) viewImageTagFilter() string {
	return lipgloss.NewStyle().Bold(true).Render("Filter images by tag") + "\n\n" +
		lipgloss.NewStyle().Faint(true).Render("Enter submit | Esc cancel") + "\n\n" +
		m.imgTagFilterIn.View() + "\n"
}

func (m Model) renameImageCmd() tea.Cmd {
	id := m.pendingImgID
	name := m.pendingImgNewName
	m.busy = true
	target := "image:" + strconv.Itoa(id)
	m.logOp("image.rename", target, "requested")
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(m.ctx, 30*time.Second)
		defer cancel()
		if err := m.api.UpdateImage(ctx, id, name); err != nil {
			return apiErrMsg{err: err}
		}
		return apiDoneMsg{act: actRenameImage, status: "Renamed image to " + name, target: target}
	}
}

func (m Model) transferImageCmd() tea.Cmd {
	id := m.pendingImgID
	region := m.pendingImgRegion
	m.busy = true
	target := fmt.Sprintf("image:%d region:%s", id, region)
	m.logOp("image.transfer", target, "requested")
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(m.ctx, 30*time.Second)
		defer cancel()
		if err := m.api.TransferImage(ctx, id, region); err != nil {
			return apiErrMsg{err: err}
		}
		return apiDoneMsg{act: actTransferImage, status: "Transfer scheduled", target: target}
	}
}

func (m Model) convertImageCmd() tea.Cmd {
	id := m.pendingImgID
	m.busy = true
	target := "image:" + strconv.Itoa(id)
	m.logOp("image.convert", target, "requested")
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(m.ctx, 30*time.Second)
		defer cancel()
		if err := m.api.ConvertImage(ctx, id); err != nil {
			return apiErrMsg{err: err}
		}
		return apiDoneMsg{act: actConvertImage, status: "Image converted to snapshot", target: target}
	}
}

func (m *Model) launchDropletFromImage(imageID int, imageName, region string) {
	m.initDropletCreateForm()
	m.imageIn.SetValue(strconv.Itoa(imageID))
	if region != "" {
		m.regionIn.SetValue(region)
	}
	// Seed a sensible default name from the image.
	if imageName != "" {
		clean := sanitizeNameSuggestion(imageName)
		m.nameIn.SetValue(clean + "-droplet")
	}
	m.status = fmt.Sprintf("Create droplet from image %d (%s)", imageID, imageName)
	m.st = stateCreateDroplet
}

func sanitizeNameSuggestion(s string) string {
	var b strings.Builder
	prevDash := false
	for _, r := range strings.ToLower(strings.TrimSpace(s)) {
		switch {
		case r >= 'a' && r <= 'z', r >= '0' && r <= '9':
			b.WriteRune(r)
			prevDash = false
		default:
			if !prevDash && b.Len() > 0 {
				b.WriteByte('-')
				prevDash = true
			}
		}
	}
	out := strings.TrimRight(b.String(), "-")
	if out == "" {
		out = "from-image"
	}
	return out
}
