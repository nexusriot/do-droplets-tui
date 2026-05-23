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

func (m Model) updateAccount(k tea.KeyMsg, cmds []tea.Cmd) (Model, []tea.Cmd) {
	if m.busy {
		return m, cmds
	}
	if key.Matches(k, m.keys.Refresh) {
		cmds = append(cmds, m.refreshAccountCmd())
	}
	return m, cmds
}

func (m Model) viewAccount() string {
	legend := lipgloss.NewStyle().Faint(true).Render("Keys: r refresh | q quit")
	bold := lipgloss.NewStyle().Bold(true)

	if m.accountInfo == nil {
		return bold.Render("Account") + "\n\n" +
			lipgloss.NewStyle().Faint(true).Render("Press r to load") + "\n\n" + legend + "\n" + m.footer()
	}

	a := m.accountInfo
	verified := "no"
	if a.EmailVerified {
		verified = "yes"
	}
	team := "-"
	if a.TeamName != "" {
		team = fmt.Sprintf("%s (%s)", a.TeamName, a.TeamUUID)
	}

	out := bold.Render("Account") + "\n\n"
	out += fmt.Sprintf("Email:           %s (verified: %s)\n", a.Email, verified)
	if a.Name != "" {
		out += fmt.Sprintf("Name:            %s\n", a.Name)
	}
	out += fmt.Sprintf("UUID:            %s\n", a.UUID)
	out += fmt.Sprintf("Status:          %s", a.Status)
	if a.StatusMessage != "" {
		out += "  — " + a.StatusMessage
	}
	out += "\n"
	out += fmt.Sprintf("Team:            %s\n", team)
	out += "\n"
	out += fmt.Sprintf("Droplet limit:    %d\n", a.DropletLimit)
	out += fmt.Sprintf("Volume limit:     %d\n", a.VolumeLimit)
	out += fmt.Sprintf("Reserved IPs:     %d\n", a.ReservedIPLimit)
	out += fmt.Sprintf("Floating IPs:     %d\n", a.FloatingIPLimit)

	out += "\n" + bold.Render("Balance") + "\n\n"
	if m.balanceInfo == nil {
		out += lipgloss.NewStyle().Faint(true).Render("(balance not loaded)") + "\n"
	} else {
		b := m.balanceInfo
		// DO API convention: account_balance / month_to_date_balance are
		// "amount owed". Negative = customer has prepaid credit. We flip the
		// sign so the display matches the web console (Prepayments / Remaining).
		acctLabel, acct := signedBalance(b.AccountBalance, "Prepayments", "Account balance due")
		mtdLabel, mtd := signedBalance(b.MonthToDateBalance, "Remaining prepayments", "MTD balance due")
		out += fmt.Sprintf("%-22s  %s\n", acctLabel+":", acct)
		out += fmt.Sprintf("%-22s  %s\n", "Month-to-date usage:", dispMoney(b.MonthToDateUsage))
		out += fmt.Sprintf("%-22s  %s\n", mtdLabel+":", mtd)
		out += fmt.Sprintf("%-22s  %s\n", "Generated at:", b.GeneratedAt)
	}

	return out + "\n" + legend + "\n" + m.footer()
}

func dispMoney(s string) string {
	if s == "" {
		return "-"
	}
	return "$" + s
}

// signedBalance interprets a DO "amount owed" string. Negative means the
// customer has a credit; we flip the sign and relabel to match the web UI
// ("Prepayments" / "Remaining prepayments"). Positive means the customer
// owes DO money and we keep the original label.
func signedBalance(raw, creditLabel, owedLabel string) (label, display string) {
	s := strings.TrimSpace(raw)
	if s == "" {
		return creditLabel, "-"
	}
	if strings.HasPrefix(s, "-") {
		// DO convention: negative "amount owed" = customer credit/prepayment.
		return creditLabel, "$" + strings.TrimPrefix(s, "-")
	}
	return owedLabel, "$" + s
}

func (m Model) refreshAccountCmd() tea.Cmd {
	m.busy = true
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(m.ctx, 30*time.Second)
		defer cancel()
		acc, err := m.api.GetAccount(ctx)
		if err != nil {
			return apiErrMsg{err: err}
		}
		bal, err := m.api.GetBalance(ctx)
		if err != nil {
			// Balance may be unavailable for some account types (e.g. team
			// member tokens). Still show account info.
			return accountLoadedMsg{acc: acc, bal: nil}
		}
		return accountLoadedMsg{acc: acc, bal: bal}
	}
}

func (m Model) updateVPCs(k tea.KeyMsg, cmds []tea.Cmd) (Model, []tea.Cmd) {
	if m.busy {
		return m, cmds
	}
	switch {
	case key.Matches(k, m.keys.Refresh):
		cmds = append(cmds, m.refreshVPCsCmd())

	case key.Matches(k, m.keys.Create):
		m.initVPCCreateForm()
		m.st = stateCreateVPC

	case key.Matches(k, m.keys.Delete):
		if v, ok := m.currentSelectedVPC(); ok {
			if v.Default {
				m.errText = "cannot delete the default VPC"
				return m, cmds
			}
			m.pendingDeleteVP = v.ID
			m.pendingAct = actDeleteVPC
			m.confirmReturn = stateVPCs
			m.confirmText = fmt.Sprintf("Delete VPC %q (%s)?\nThis cannot be undone.", v.Name, v.ID)
			m.st = stateConfirm
		}
	}
	return m, cmds
}

func (m Model) updateCreateVPC(k tea.KeyMsg, cmds []tea.Cmd) (Model, []tea.Cmd) {
	if key.Matches(k, m.keys.Back) {
		m.st = stateVPCs
		return m, cmds
	}
	if m.busy {
		return m, cmds
	}
	if k.String() == "tab" || k.String() == "shift+tab" {
		m.blurVPCForm()
		if k.String() == "tab" {
			m.focusVPCForm = (m.focusVPCForm + 1) % 4
		} else {
			m.focusVPCForm = (m.focusVPCForm - 1 + 4) % 4
		}
		m.focusVPCOne()
		return m, cmds
	}
	if k.String() == "enter" {
		name := trim(m.vpcNameIn.Value())
		region := trim(m.vpcRegionIn.Value())
		if name == "" || region == "" {
			m.errText = "name and region are required"
			return m, cmds
		}
		m.pendingAct = actCreateVPC
		m.confirmReturn = stateCreateVPC
		m.confirmText = fmt.Sprintf("Create VPC %q in %s (ip_range=%q)?",
			name, region, trim(m.vpcIPRangeIn.Value()))
		m.st = stateConfirm
		return m, cmds
	}
	var cmd tea.Cmd
	switch m.focusVPCForm {
	case 0:
		m.vpcNameIn, cmd = m.vpcNameIn.Update(k)
	case 1:
		m.vpcRegionIn, cmd = m.vpcRegionIn.Update(k)
	case 2:
		m.vpcIPRangeIn, cmd = m.vpcIPRangeIn.Update(k)
	case 3:
		m.vpcDescIn, cmd = m.vpcDescIn.Update(k)
	}
	cmds = append(cmds, cmd)
	return m, cmds
}

func (m Model) viewVPCs() string {
	legend := lipgloss.NewStyle().Faint(true).Render("Keys: r refresh | c create | d delete | q quit")
	body := m.vpcTable.View()
	if len(m.vpcRows) == 0 {
		body += "\n" + lipgloss.NewStyle().Faint(true).Render("No VPCs (or none loaded). Press 'r' to refresh.")
	}
	return body + "\n" + legend + "\n" + m.footer()
}

func (m Model) viewCreateVPC() string {
	var s string
	s += lipgloss.NewStyle().Bold(true).Render("Create VPC") + "\n\n"
	s += lipgloss.NewStyle().Faint(true).Render("Tab/Shift+Tab move | Enter submit | Esc cancel") + "\n\n"
	s += m.vpcNameIn.View() + "\n"
	s += m.vpcRegionIn.View() + "\n"
	s += m.vpcIPRangeIn.View() + "\n"
	s += m.vpcDescIn.View() + "\n"
	return s
}

func (m Model) refreshVPCsCmd() tea.Cmd {
	m.busy = true
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(m.ctx, 30*time.Second)
		defer cancel()
		rows, err := m.api.ListVPCs(ctx)
		if err != nil {
			return apiErrMsg{err: err}
		}
		return vpcsLoadedMsg{rows: rows}
	}
}

func (m Model) createVPCCmd() tea.Cmd {
	req := do.CreateVPCReq{
		Name:        trim(m.vpcNameIn.Value()),
		Region:      trim(m.vpcRegionIn.Value()),
		IPRange:     trim(m.vpcIPRangeIn.Value()),
		Description: trim(m.vpcDescIn.Value()),
	}
	m.busy = true
	target := "vpc:" + req.Name
	m.logOp("vpc.create", target, "requested")
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(m.ctx, 30*time.Second)
		defer cancel()
		v, err := m.api.CreateVPC(ctx, req)
		if err != nil {
			return apiErrMsg{err: err}
		}
		return apiDoneMsg{act: actCreateVPC, status: "Created VPC " + v.Name, target: target}
	}
}

func (m Model) deleteVPCCmd(id string) tea.Cmd {
	m.busy = true
	target := "vpc:" + id
	m.logOp("vpc.delete", target, "requested")
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(m.ctx, 30*time.Second)
		defer cancel()
		if err := m.api.DeleteVPC(ctx, id); err != nil {
			return apiErrMsg{err: err}
		}
		return apiDoneMsg{act: actDeleteVPC, status: "Deleted VPC " + id, target: target}
	}
}

func (m Model) currentSelectedVPC() (do.VPCRow, bool) {
	i := m.vpcTable.Cursor()
	if i < 0 || i >= len(m.vpcRows) {
		return do.VPCRow{}, false
	}
	return m.vpcRows[i], true
}

func toVPCRows(rows []do.VPCRow) []table.Row {
	out := make([]table.Row, 0, len(rows))
	for _, r := range rows {
		def := ""
		if r.Default {
			def = "✓"
		}
		out = append(out, table.Row{r.Name, r.Region, r.IPRange, def, r.ID})
	}
	return out
}

func (m *Model) initVPCCreateForm() {
	m.vpcNameIn = newInput("Name", "my-vpc")
	m.vpcRegionIn = newInput("Region", m.opts.DefaultRegion)
	m.vpcIPRangeIn = newInput("IP range (CIDR, optional)", "10.10.10.0/24")
	m.vpcDescIn = newInput("Description (optional)", "")
	m.vpcRegionIn.SetValue(m.opts.DefaultRegion)
	m.focusVPCForm = 0
	m.blurVPCForm()
	m.vpcNameIn.Focus()
}

func (m *Model) blurVPCForm() {
	m.vpcNameIn.Blur()
	m.vpcRegionIn.Blur()
	m.vpcIPRangeIn.Blur()
	m.vpcDescIn.Blur()
}

func (m *Model) focusVPCOne() {
	switch m.focusVPCForm {
	case 0:
		m.vpcNameIn.Focus()
	case 1:
		m.vpcRegionIn.Focus()
	case 2:
		m.vpcIPRangeIn.Focus()
	case 3:
		m.vpcDescIn.Focus()
	}
}

func (m Model) updateImages(k tea.KeyMsg, cmds []tea.Cmd) (Model, []tea.Cmd) {
	if m.busy {
		return m, cmds
	}
	switch {
	case key.Matches(k, m.keys.Refresh):
		cmds = append(cmds, m.refreshImagesCmd(m.imagesMode))

	case key.Matches(k, m.keys.ToggleImages):
		// Cycle: user → distribution → application → user
		m.imagesMode = nextImageMode(m.imagesMode)
		cmds = append(cmds, m.refreshImagesCmd(m.imagesMode))

	case k.String() == "f":
		// Tag filter.
		m.imgTagFilterIn = newInput("Tag", "production")
		if m.imagesTag != "" {
			m.imgTagFilterIn.SetValue(m.imagesTag)
		}
		m.imgTagFilterIn.Focus()
		m.st = stateImageTagFilter

	case key.Matches(k, m.keys.Delete):
		if !isUserImageMode(m.imagesMode) {
			m.errText = "cannot delete " + m.imagesMode + " images"
			return m, cmds
		}
		if im, ok := m.currentSelectedImage(); ok {
			m.pendingDelImg = im.ID
			m.pendingAct = actDeleteImage
			m.confirmReturn = stateImages
			m.confirmText = fmt.Sprintf("Delete image %q (id=%d)?\nThis cannot be undone.", im.Name, im.ID)
			m.st = stateConfirm
		}

	case k.String() == "R":
		if !isUserImageMode(m.imagesMode) {
			m.errText = "rename only available for user images"
			return m, cmds
		}
		if im, ok := m.currentSelectedImage(); ok {
			m.pendingImgID = im.ID
			m.imgRenameIn = newInput("New name", im.Name)
			m.imgRenameIn.SetValue(im.Name)
			m.imgRenameIn.Focus()
			m.st = stateImageRename
		}

	case k.String() == "T":
		if !isUserImageMode(m.imagesMode) {
			m.errText = "transfer only available for user images"
			return m, cmds
		}
		if im, ok := m.currentSelectedImage(); ok {
			m.pendingImgID = im.ID
			m.imgTransferIn = newInput("Target region", "nyc3")
			m.imgTransferIn.Focus()
			m.st = stateImageTransfer
		}

	case k.String() == "V":
		if !isUserImageMode(m.imagesMode) {
			m.errText = "convert only available for user/backup images"
			return m, cmds
		}
		if im, ok := m.currentSelectedImage(); ok {
			m.pendingImgID = im.ID
			m.pendingAct = actConvertImage
			m.confirmReturn = stateImages
			m.confirmText = fmt.Sprintf("Convert image %q (id=%d) into a snapshot?\nOnly works on backup-type images.", im.Name, im.ID)
			m.st = stateConfirm
		}

	case k.String() == "c":
		// Launch droplet from selected image (snapshot or any user image).
		if im, ok := m.currentSelectedImage(); ok {
			m.launchDropletFromImage(im.ID, im.Name, firstRegionOf(im.Regions))
		}
	}
	return m, cmds
}

func (m Model) viewImages() string {
	hint := fmt.Sprintf("[%s images]", m.imagesMode)
	legend := lipgloss.NewStyle().Faint(true).Render(
		"Keys: r refresh | t cycle user/distro/app | f tag filter | " +
			"c create droplet | R rename | T transfer | V convert | d delete  " + hint)
	body := m.imageTable.View()
	if len(m.imageRows) == 0 {
		body += "\n" + lipgloss.NewStyle().Faint(true).Render("No images. Press 't' to cycle, 'f' to filter by tag.")
	}
	return body + "\n" + legend + "\n" + m.footer()
}

func nextImageMode(mode string) string {
	switch mode {
	case "user":
		return "distribution"
	case "distribution":
		return "application"
	default:
		return "user"
	}
}

func isUserImageMode(mode string) bool {
	// User snapshots/backups/custom uploads come back from ListUserImages or
	// can be selected via tag filter. We trust tag-filter results as user
	// images for destructive ops only if the user explicitly opted in via 'R'
	// on a user-managed entry — but we can't easily tell apart user vs
	// distribution from a tag filter, so be permissive there too.
	return mode == "user" || strings.HasPrefix(mode, "tag:")
}

func firstRegionOf(csv string) string {
	csv = strings.TrimSpace(csv)
	if csv == "" {
		return ""
	}
	if i := strings.IndexByte(csv, ','); i >= 0 {
		return csv[:i]
	}
	return csv
}

func (m Model) refreshImagesCmd(mode string) tea.Cmd {
	m.busy = true
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(m.ctx, 60*time.Second)
		defer cancel()
		var (
			rows []do.ImageRow
			err  error
		)
		switch {
		case mode == "user":
			rows, err = m.api.ListUserImages(ctx)
		case mode == "distribution":
			rows, err = m.api.ListDistributionImages(ctx)
		case mode == "application":
			rows, err = m.api.ListApplicationImages(ctx)
		case strings.HasPrefix(mode, "tag:"):
			rows, err = m.api.ListImagesByTag(ctx, strings.TrimPrefix(mode, "tag:"))
		default:
			rows, err = m.api.ListUserImages(ctx)
			mode = "user"
		}
		if err != nil {
			return apiErrMsg{err: err}
		}
		return imagesLoadedMsg{rows: rows, mode: mode}
	}
}

func (m Model) deleteImageCmd(id int) tea.Cmd {
	m.busy = true
	target := "image:" + strconv.Itoa(id)
	m.logOp("image.delete", target, "requested")
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(m.ctx, 30*time.Second)
		defer cancel()
		if err := m.api.DeleteImage(ctx, id); err != nil {
			return apiErrMsg{err: err}
		}
		return apiDoneMsg{act: actDeleteImage, status: fmt.Sprintf("Deleted image %d", id), target: target}
	}
}

func (m Model) currentSelectedImage() (do.ImageRow, bool) {
	i := m.imageTable.Cursor()
	if i < 0 || i >= len(m.imageRows) {
		return do.ImageRow{}, false
	}
	return m.imageRows[i], true
}

func toImageRows(rows []do.ImageRow) []table.Row {
	out := make([]table.Row, 0, len(rows))
	for _, r := range rows {
		out = append(out, table.Row{
			strconv.Itoa(r.ID),
			r.Name,
			r.Distribution,
			r.Slug,
			strconv.Itoa(r.MinDiskGB),
			r.Regions,
		})
	}
	return out
}

func (m Model) updateAlerts(k tea.KeyMsg, cmds []tea.Cmd) (Model, []tea.Cmd) {
	if m.busy {
		return m, cmds
	}
	switch {
	case key.Matches(k, m.keys.Refresh):
		cmds = append(cmds, m.refreshAlertsCmd())

	case key.Matches(k, m.keys.Create):
		m.initAlertForm()
		m.st = stateCreateAlert

	case key.Matches(k, m.keys.Delete):
		if a, ok := m.currentSelectedAlert(); ok {
			m.pendingDelAlrt = a.UUID
			m.pendingAct = actDeleteAlertPolicy
			m.confirmReturn = stateAlerts
			desc := a.Description
			if desc == "" {
				desc = a.UUID
			}
			m.confirmText = fmt.Sprintf("Delete alert policy %q?\nThis cannot be undone.", desc)
			m.st = stateConfirm
		}
	}
	return m, cmds
}

func (m Model) viewAlerts() string {
	legend := lipgloss.NewStyle().Faint(true).Render("Keys: r refresh | c create | d delete | q quit")
	body := m.alertTable.View()
	if len(m.alertRows) == 0 {
		body += "\n" + lipgloss.NewStyle().Faint(true).Render("No alert policies.")
	}
	return body + "\n" + legend + "\n" + m.footer()
}

func (m Model) refreshAlertsCmd() tea.Cmd {
	m.busy = true
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(m.ctx, 30*time.Second)
		defer cancel()
		rows, err := m.api.ListAlertPolicies(ctx)
		if err != nil {
			return apiErrMsg{err: err}
		}
		return alertsLoadedMsg{rows: rows}
	}
}

func (m Model) deleteAlertPolicyCmd(uuid string) tea.Cmd {
	m.busy = true
	target := "alert:" + uuid
	m.logOp("alert.delete", target, "requested")
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(m.ctx, 30*time.Second)
		defer cancel()
		if err := m.api.DeleteAlertPolicy(ctx, uuid); err != nil {
			return apiErrMsg{err: err}
		}
		return apiDoneMsg{act: actDeleteAlertPolicy, status: "Deleted alert policy " + uuid, target: target}
	}
}

func (m Model) currentSelectedAlert() (do.AlertPolicyRow, bool) {
	i := m.alertTable.Cursor()
	if i < 0 || i >= len(m.alertRows) {
		return do.AlertPolicyRow{}, false
	}
	return m.alertRows[i], true
}

func toAlertRows(rows []do.AlertPolicyRow) []table.Row {
	out := make([]table.Row, 0, len(rows))
	for _, r := range rows {
		on := ""
		if r.Enabled {
			on = "✓"
		}
		out = append(out, table.Row{
			shortAlertType(r.Type),
			r.Compare,
			strconv.FormatFloat(float64(r.Value), 'f', -1, 32),
			r.Window,
			on,
			r.Description,
		})
	}
	return out
}

// shortAlertType strips the long "v1/insights/droplet/" prefix that DO returns.
func shortAlertType(s string) string {
	const p = "v1/insights/"
	if len(s) > len(p) && s[:len(p)] == p {
		return s[len(p):]
	}
	return s
}

func trim(s string) string { return strings.TrimSpace(s) }
