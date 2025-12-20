package tui

import (
	"context"
	"fmt"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/charmbracelet/bubbles/help"
	"github.com/charmbracelet/bubbles/key"
	"github.com/charmbracelet/bubbles/spinner"
	"github.com/charmbracelet/bubbles/table"
	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/digitalocean/godo"
	"github.com/gdamore/tcell/v2"

	"github.com/nexusriot/do-droplets-tui/internal/do"
)

type state int

const (
	stateDroplets state = iota
	stateVolumes
	stateOpsLog

	stateDetails
	stateCreateDroplet
	statePickSSHKeys
	stateCreateVolume

	stateConfirm // generic confirm dialog
)

type actionKind int

const (
	actPowerOn actionKind = iota
	actPowerOff
	actShutdown
	actReboot
	actDeleteDroplet
	actCreateDroplet
	actCreateVolume
	actDeleteVolume
)

type Options struct {
	DefaultRegion string
	DefaultSize   string
	DefaultImage  string
	DefaultTags   string // CSV
	DefaultIPv6   bool
}

type api interface {
	// droplets
	ListDroplets(context.Context) ([]do.DropletRow, error)
	GetDroplet(context.Context, int) (*godo.Droplet, error)
	PowerOn(context.Context, int) error
	PowerOff(context.Context, int) error
	Shutdown(context.Context, int) error
	Reboot(context.Context, int) error
	DeleteDroplet(context.Context, int) error
	CreateDroplet(context.Context, do.CreateDropletReq) (*godo.Droplet, error)

	// ssh
	ListSSHKeys(context.Context) ([]do.SSHKeyRow, error)

	// volumes
	ListVolumes(context.Context) ([]do.VolumeRow, error)
	CreateVolume(context.Context, do.CreateVolumeReq) (*godo.Volume, error)
	DeleteVolume(context.Context, string) error
}

type keyMap struct {
	Up, Down, Enter, Back key.Binding
	Refresh               key.Binding

	TabDroplets key.Binding
	TabVolumes  key.Binding
	TabOps      key.Binding

	Create key.Binding
	Delete key.Binding

	Details key.Binding

	PowerOn, PowerOff, Shutdown, Reboot key.Binding

	PickSSH key.Binding

	Yes, No key.Binding
	Quit    key.Binding
}

func (k keyMap) ShortHelp() []key.Binding {
	return []key.Binding{k.TabDroplets, k.TabVolumes, k.TabOps, k.Refresh, k.Create, k.Delete, k.Quit}
}
func (k keyMap) FullHelp() [][]key.Binding {
	return [][]key.Binding{
		{k.TabDroplets, k.TabVolumes, k.TabOps},
		{k.Up, k.Down, k.Enter, k.Back},
		{k.Refresh, k.Create, k.Delete},
		{k.PowerOn, k.PowerOff, k.Shutdown, k.Reboot},
		{k.PickSSH},
		{k.Quit},
	}
}

func defaultKeys() keyMap {
	_ = tcell.KeyEnter

	return keyMap{
		Up:    key.NewBinding(key.WithKeys("up", "k"), key.WithHelp("↑/k", "up")),
		Down:  key.NewBinding(key.WithKeys("down", "j"), key.WithHelp("↓/j", "down")),
		Enter: key.NewBinding(key.WithKeys("enter"), key.WithHelp("enter", "select")),
		Back:  key.NewBinding(key.WithKeys("esc"), key.WithHelp("esc", "back")),
		Quit:  key.NewBinding(key.WithKeys("q", "ctrl+c"), key.WithHelp("q", "quit")),

		Refresh: key.NewBinding(key.WithKeys("r"), key.WithHelp("r", "refresh")),

		TabDroplets: key.NewBinding(key.WithKeys("1"), key.WithHelp("1", "droplets")),
		TabVolumes:  key.NewBinding(key.WithKeys("2"), key.WithHelp("2", "volumes")),
		TabOps:      key.NewBinding(key.WithKeys("l"), key.WithHelp("l", "ops log")),

		Create: key.NewBinding(key.WithKeys("c"), key.WithHelp("c", "create")),
		Delete: key.NewBinding(key.WithKeys("d"), key.WithHelp("d", "delete")),

		Details: key.NewBinding(key.WithKeys("enter"), key.WithHelp("enter", "details")),

		PowerOn:  key.NewBinding(key.WithKeys("o"), key.WithHelp("o", "power on")),
		PowerOff: key.NewBinding(key.WithKeys("p"), key.WithHelp("p", "power off")),
		Shutdown: key.NewBinding(key.WithKeys("s"), key.WithHelp("s", "shutdown")),
		Reboot:   key.NewBinding(key.WithKeys("b"), key.WithHelp("b", "reboot")),

		// NOTE: capital K avoids conflict with vim-style up (k)
		PickSSH: key.NewBinding(key.WithKeys("K"), key.WithHelp("K", "pick ssh keys")),

		Yes: key.NewBinding(key.WithKeys("y"), key.WithHelp("y", "yes")),
		No:  key.NewBinding(key.WithKeys("n"), key.WithHelp("n", "no")),
	}
}

type OpEntry struct {
	When   time.Time
	Kind   string
	Target string
	Result string
}

type Model struct {
	api  api
	ctx  context.Context
	keys keyMap
	help help.Model

	opts Options

	spinner spinner.Model
	busy    bool

	width, height int
	st            state

	// droplets
	dropletTable  table.Model
	dropletRows   []do.DropletRow
	restoreDropID int // keep cursor after refresh

	selectedDropletID int
	dropletDetails    *godo.Droplet

	// volumes
	volumeTable  table.Model
	volumeRows   []do.VolumeRow
	restoreVolID string

	selectedVolumeID string

	// ops
	ops      []OpEntry
	opsTable table.Model

	// create droplet form
	focusDropletForm int
	nameIn           textinput.Model
	regionIn         textinput.Model
	sizeIn           textinput.Model
	imageIn          textinput.Model
	sshIDsIn         textinput.Model
	tagsIn           textinput.Model
	ipv6In           textinput.Model
	vpcIn            textinput.Model

	// ssh picker
	sshTable    table.Model
	sshKeys     []do.SSHKeyRow
	sshSelected map[int]bool

	// create volume form
	focusVolForm int
	volNameIn    textinput.Model
	volRegionIn  textinput.Model
	volSizeIn    textinput.Model
	volDescIn    textinput.Model

	// generic confirm
	confirmText   string
	confirmReturn state
	pendingAct    actionKind

	pendingCreateDroplet *do.CreateDropletReq
	pendingCreateVolume  *do.CreateVolumeReq
	pendingDeleteVolID   string

	// misc
	errText string
	status  string
}

func NewModel(api api, opts Options) Model {
	sp := spinner.New()
	sp.Spinner = spinner.Line

	dCols := []table.Column{
		{Title: "ID", Width: 10},
		{Title: "Name", Width: 24},
		{Title: "Region", Width: 8},
		{Title: "Size", Width: 14},
		{Title: "Status", Width: 10},
		{Title: "IPv4", Width: 15},
	}
	dt := table.New(table.WithColumns(dCols), table.WithFocused(true), table.WithHeight(14))

	vCols := []table.Column{
		{Title: "ID", Width: 24},
		{Title: "Name", Width: 22},
		{Title: "Region", Width: 8},
		{Title: "GB", Width: 6},
		{Title: "Description", Width: 26},
	}
	vt := table.New(table.WithColumns(vCols), table.WithFocused(true), table.WithHeight(14))

	oCols := []table.Column{
		{Title: "Time", Width: 19},
		{Title: "Kind", Width: 18},
		{Title: "Target", Width: 20},
		{Title: "Result", Width: 30},
	}
	ot := table.New(table.WithColumns(oCols), table.WithFocused(true), table.WithHeight(14))

	sshT := table.New(
		table.WithColumns([]table.Column{
			{Title: "Sel", Width: 3},
			{Title: "ID", Width: 8},
			{Title: "Name", Width: 28},
			{Title: "Fingerprint", Width: 24},
		}),
		table.WithFocused(true),
		table.WithHeight(14),
	)

	m := Model{
		api:          api,
		ctx:          context.Background(),
		keys:         defaultKeys(),
		help:         help.New(),
		opts:         opts,
		spinner:      sp,
		st:           stateDroplets,
		dropletTable: dt,
		volumeTable:  vt,
		opsTable:     ot,
		sshTable:     sshT,
		sshSelected:  map[int]bool{},
		status:       "Press r to refresh (droplets tab)",
	}

	m.initDropletCreateForm()
	m.initVolumeCreateForm()

	return m
}

func (m Model) Init() tea.Cmd {
	return tea.Batch(m.spinner.Tick, m.refreshDropletsCmd())
}

/* ---------------- messages ---------------- */

type apiErrMsg struct{ err error }
type dropletsLoadedMsg struct{ rows []do.DropletRow }
type dropletDetailsMsg struct{ d *godo.Droplet }
type volumesLoadedMsg struct{ rows []do.VolumeRow }
type sshKeysLoadedMsg struct{ keys []do.SSHKeyRow }

type apiDoneMsg struct {
	act    actionKind
	status string
	target string
}

/* ---------------- update ---------------- */

func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	var cmds []tea.Cmd

	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width, m.height = msg.Width, msg.Height
		h := max(8, m.height-9)
		m.dropletTable.SetHeight(h)
		m.volumeTable.SetHeight(h)
		m.opsTable.SetHeight(h)
		m.sshTable.SetHeight(h)

	case spinner.TickMsg:
		var cmd tea.Cmd
		m.spinner, cmd = m.spinner.Update(msg)
		cmds = append(cmds, cmd)

	case apiErrMsg:
		m.busy = false
		m.errText = msg.err.Error()
		m.status = "Error"
		m.logOp("error", "-", msg.err.Error())

	case dropletsLoadedMsg:
		m.busy = false
		m.errText = ""
		m.dropletRows = msg.rows
		m.dropletTable.SetRows(toDropletTableRows(m.dropletRows))
		m.status = fmt.Sprintf("Loaded %d droplet(s)", len(m.dropletRows))
		m.restoreDropletCursor()

	case dropletDetailsMsg:
		m.busy = false
		m.errText = ""
		m.dropletDetails = msg.d
		m.status = "Details loaded"
		m.st = stateDetails

	case volumesLoadedMsg:
		m.busy = false
		m.errText = ""
		m.volumeRows = msg.rows
		m.volumeTable.SetRows(toVolumeTableRows(m.volumeRows))
		m.status = fmt.Sprintf("Loaded %d volume(s)", len(m.volumeRows))
		m.restoreVolumeCursor()

	case sshKeysLoadedMsg:
		m.busy = false
		m.errText = ""
		m.sshKeys = msg.keys
		m.sshTable.SetRows(toSSHTableRows(m.sshKeys, m.sshSelected))
		m.status = fmt.Sprintf("Loaded %d SSH key(s)", len(m.sshKeys))

	case apiDoneMsg:
		m.busy = false
		m.errText = ""
		m.status = msg.status
		m.logOp(actToString(msg.act), msg.target, msg.status)

		// behavior requirements:
		// - after power on: go back to droplet list
		// Close confirm/details/create screens after actions
		switch msg.act {
		case actPowerOn, actPowerOff, actShutdown, actReboot:
			// After actions on droplet, go back to droplets list (your earlier requirement included PowerOn;
			// doing it for all is usually nicer UX)
			m.st = stateDroplets
			m.dropletDetails = nil

		case actDeleteDroplet:
			// FIX: after delete, close dialog and return to list
			m.st = stateDroplets
			m.dropletDetails = nil
			m.selectedDropletID = 0

		case actCreateDroplet:
			// Auto close create window after creating
			m.st = stateDroplets
			m.dropletDetails = nil

		case actCreateVolume, actDeleteVolume:
			m.st = stateVolumes
			m.selectedVolumeID = ""
		}

		// refresh the active tab after changes
		switch {
		case msg.act == actCreateDroplet || msg.act == actDeleteDroplet || msg.act == actPowerOn || msg.act == actPowerOff || msg.act == actShutdown || msg.act == actReboot:
			cmds = append(cmds, m.refreshDropletsCmd())
		case msg.act == actCreateVolume || msg.act == actDeleteVolume:
			cmds = append(cmds, m.refreshVolumesCmd())
		}

	case tea.KeyMsg:
		if key.Matches(msg, m.keys.Quit) {
			return m, tea.Quit
		}

		// global tab switching (except confirm screens)
		if m.st != stateConfirm && m.st != statePickSSHKeys && m.st != stateCreateDroplet && m.st != stateCreateVolume && m.st != stateDetails {
			switch {
			case key.Matches(msg, m.keys.TabDroplets):
				m.st = stateDroplets
				m.status = "Droplets"
			case key.Matches(msg, m.keys.TabVolumes):
				m.st = stateVolumes
				m.status = "Volumes"
				if len(m.volumeRows) == 0 {
					cmds = append(cmds, m.refreshVolumesCmd())
				}
			case key.Matches(msg, m.keys.TabOps):
				m.st = stateOpsLog
				m.opsTable.SetRows(toOpsRows(m.ops))
				m.status = "Ops log"
			}
		}

		switch m.st {
		case stateDroplets:
			m, cmds = m.updateDroplets(msg, cmds)
		case stateDetails:
			m, cmds = m.updateDetails(msg, cmds)
		case stateCreateDroplet:
			m, cmds = m.updateCreateDroplet(msg, cmds)
		case statePickSSHKeys:
			m, cmds = m.updatePickSSH(msg, cmds)
		case stateVolumes:
			m, cmds = m.updateVolumes(msg, cmds)
		case stateCreateVolume:
			m, cmds = m.updateCreateVolume(msg, cmds)
		case stateOpsLog:
			m, cmds = m.updateOps(msg, cmds)
		case stateConfirm:
			m, cmds = m.updateConfirm(msg, cmds)
		}
	}

	// pass through table navigation
	switch m.st {
	case stateDroplets:
		var cmd tea.Cmd
		m.dropletTable, cmd = m.dropletTable.Update(msg)
		cmds = append(cmds, cmd)
	case stateVolumes:
		var cmd tea.Cmd
		m.volumeTable, cmd = m.volumeTable.Update(msg)
		cmds = append(cmds, cmd)
	case stateOpsLog:
		var cmd tea.Cmd
		m.opsTable, cmd = m.opsTable.Update(msg)
		cmds = append(cmds, cmd)
	case statePickSSHKeys:
		var cmd tea.Cmd
		m.sshTable, cmd = m.sshTable.Update(msg)
		cmds = append(cmds, cmd)
	}

	return m, tea.Batch(cmds...)
}

/* ---------------- views ---------------- */

func (m Model) View() string {
	title := lipgloss.NewStyle().Bold(true).Render("DigitalOcean TUI  |  1=Droplets  2=Volumes  l=Ops")
	top := title + "\n"

	if m.busy {
		top += m.spinner.View() + " " + lipgloss.NewStyle().Faint(true).Render("Working...") + "\n"
	} else {
		top += "\n"
	}

	if m.errText != "" {
		top += lipgloss.NewStyle().Foreground(lipgloss.Color("9")).Render("Error: "+m.errText) + "\n\n"
	}

	switch m.st {
	case stateDroplets:
		return top + m.viewDroplets()
	case stateDetails:
		return top + m.viewDetails()
	case stateCreateDroplet:
		return top + m.viewCreateDroplet()
	case statePickSSHKeys:
		return top + m.viewPickSSH()
	case stateVolumes:
		return top + m.viewVolumes()
	case stateCreateVolume:
		return top + m.viewCreateVolume()
	case stateOpsLog:
		return top + m.viewOps()
	case stateConfirm:
		return top + m.viewConfirm()
	default:
		return top + "unknown state\n"
	}
}

func (m Model) footer() string {
	return lipgloss.NewStyle().Faint(true).Render(m.status) + "\n" + m.help.View(m.keys)
}

func (m Model) viewDroplets() string {
	legend := lipgloss.NewStyle().Faint(true).Render("Keys: r refresh | enter details | c create | d delete | o/p/s/b power | q quit")
	body := m.dropletTable.View()
	if len(m.dropletRows) == 0 {
		body = body + "\n" + lipgloss.NewStyle().Faint(true).Render("No droplets found. Press 'c' to create one.")
	}
	return body + "\n" + legend + "\n" + m.footer()
}

func (m Model) viewVolumes() string {
	legend := lipgloss.NewStyle().Faint(true).Render("Keys: r refresh | c create | d delete | q quit")
	return m.volumeTable.View() + "\n" + legend + "\n" + m.footer()
}

func (m Model) viewOps() string {
	legend := lipgloss.NewStyle().Faint(true).Render("Ops log (most recent first). Use arrows/j/k. q quit")
	return m.opsTable.View() + "\n" + legend + "\n" + m.footer()
}

func (m Model) viewDetails() string {
	if m.dropletDetails == nil {
		return "No droplet loaded\n\n" + m.footer()
	}
	d := m.dropletDetails
	var b strings.Builder
	b.WriteString(lipgloss.NewStyle().Bold(true).Render("Droplet Details") + "\n\n")
	fmt.Fprintf(&b, "ID: %d\n", d.ID)
	fmt.Fprintf(&b, "Name: %s\n", d.Name)
	fmt.Fprintf(&b, "Status: %s\n", d.Status)
	fmt.Fprintf(&b, "Region: %s\n", d.Region.Slug)
	fmt.Fprintf(&b, "Size: %s\n", d.SizeSlug)
	fmt.Fprintf(&b, "IPv4: %s\n", firstIPv4(d.Networks))
	fmt.Fprintf(&b, "Tags: %s\n", strings.Join(d.Tags, ","))
	fmt.Fprintf(&b, "Created: %s\n", d.Created)

	b.WriteString("\n")
	b.WriteString(lipgloss.NewStyle().Faint(true).Render("Keys: esc back | r refresh | o/p/s/b power | d delete | q quit"))
	return b.String()
}

func (m Model) viewConfirm() string {
	box := lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		Padding(1, 2).
		Width(min(90, max(40, m.width-4))).
		Render(m.confirmText + "\n\n[y] Yes   [n] No")
	return "\n" + box + "\n"
}

func (m Model) viewCreateDroplet() string {
	var b strings.Builder
	b.WriteString(lipgloss.NewStyle().Bold(true).Render("Create Droplet") + "\n\n")
	b.WriteString(lipgloss.NewStyle().Faint(true).Render("Tab/Shift+Tab move | Enter submit (confirm) | Esc cancel | K pick SSH keys") + "\n\n")

	b.WriteString(m.nameIn.View() + "\n")
	b.WriteString(m.regionIn.View() + "\n")
	b.WriteString(m.sizeIn.View() + "\n")
	b.WriteString(m.imageIn.View() + "\n")
	b.WriteString(m.sshIDsIn.View() + "\n")
	b.WriteString(lipgloss.NewStyle().Faint(true).Render("Selected SSH keys: "+m.selectedSSHNames()) + "\n")
	b.WriteString(m.tagsIn.View() + "\n")
	b.WriteString(m.ipv6In.View() + "\n")
	b.WriteString(m.vpcIn.View() + "\n")

	return b.String()
}

func (m Model) viewPickSSH() string {
	h := lipgloss.NewStyle().Bold(true).Render("Pick SSH Keys")
	legend := lipgloss.NewStyle().Faint(true).Render("space=toggle  enter=use selected  esc=back")
	return h + "\n\n" + m.sshTable.View() + "\n\n" + legend
}

func (m Model) viewCreateVolume() string {
	var b strings.Builder
	b.WriteString(lipgloss.NewStyle().Bold(true).Render("Create Volume") + "\n\n")
	b.WriteString(lipgloss.NewStyle().Faint(true).Render("Tab/Shift+Tab move | Enter submit (confirm) | Esc cancel") + "\n\n")
	b.WriteString(m.volNameIn.View() + "\n")
	b.WriteString(m.volRegionIn.View() + "\n")
	b.WriteString(m.volSizeIn.View() + "\n")
	b.WriteString(m.volDescIn.View() + "\n")
	return b.String()
}

/* ---------------- droplets handlers ---------------- */

func (m Model) updateDroplets(k tea.KeyMsg, cmds []tea.Cmd) (Model, []tea.Cmd) {
	if m.busy {
		return m, cmds
	}

	switch {
	case key.Matches(k, m.keys.Refresh):
		m.captureDropletCursor()
		cmds = append(cmds, m.refreshDropletsCmd())

	case key.Matches(k, m.keys.Create):
		m.st = stateCreateDroplet
		m.initDropletCreateForm()

	case key.Matches(k, m.keys.Enter):
		id, ok := m.currentSelectedDropletID()
		if ok {
			m.selectedDropletID = id
			cmds = append(cmds, m.loadDetailsCmd(id))
		}

	case key.Matches(k, m.keys.Delete):
		id, ok := m.currentSelectedDropletID()
		if ok {
			m.selectedDropletID = id
			m.pendingAct = actDeleteDroplet
			m.confirmReturn = stateDroplets
			m.confirmText = fmt.Sprintf("Delete droplet %d?\nThis cannot be undone.", id)
			m.st = stateConfirm
		}

	case key.Matches(k, m.keys.PowerOn):
		m = m.confirmDropletAction(actPowerOn, "Power ON", stateDroplets)
	case key.Matches(k, m.keys.PowerOff):
		m = m.confirmDropletAction(actPowerOff, "Power OFF", stateDroplets)
	case key.Matches(k, m.keys.Shutdown):
		m = m.confirmDropletAction(actShutdown, "Shutdown", stateDroplets)
	case key.Matches(k, m.keys.Reboot):
		m = m.confirmDropletAction(actReboot, "Reboot", stateDroplets)
	}

	return m, cmds
}

func (m Model) updateDetails(k tea.KeyMsg, cmds []tea.Cmd) (Model, []tea.Cmd) {
	if key.Matches(k, m.keys.Back) {
		m.st = stateDroplets
		return m, cmds
	}
	if m.busy {
		return m, cmds
	}
	id := m.selectedDropletID

	switch {
	case key.Matches(k, m.keys.Refresh):
		cmds = append(cmds, m.loadDetailsCmd(id))
	case key.Matches(k, m.keys.Delete):
		m.pendingAct = actDeleteDroplet
		m.confirmReturn = stateDetails
		m.confirmText = fmt.Sprintf("Delete droplet %d?\nThis cannot be undone.", id)
		m.st = stateConfirm
	case key.Matches(k, m.keys.PowerOn):
		m.pendingAct = actPowerOn
		m.confirmReturn = stateDetails
		m.confirmText = fmt.Sprintf("Power ON droplet %d?", id)
		m.st = stateConfirm
	case key.Matches(k, m.keys.PowerOff):
		m.pendingAct = actPowerOff
		m.confirmReturn = stateDetails
		m.confirmText = fmt.Sprintf("Power OFF droplet %d?", id)
		m.st = stateConfirm
	case key.Matches(k, m.keys.Shutdown):
		m.pendingAct = actShutdown
		m.confirmReturn = stateDetails
		m.confirmText = fmt.Sprintf("Shutdown droplet %d?", id)
		m.st = stateConfirm
	case key.Matches(k, m.keys.Reboot):
		m.pendingAct = actReboot
		m.confirmReturn = stateDetails
		m.confirmText = fmt.Sprintf("Reboot droplet %d?", id)
		m.st = stateConfirm
	}

	return m, cmds
}

func (m Model) confirmDropletAction(a actionKind, label string, ret state) Model {
	id, ok := m.currentSelectedDropletID()
	if !ok {
		return m
	}
	m.selectedDropletID = id
	m.pendingAct = a
	m.confirmReturn = ret
	m.confirmText = fmt.Sprintf("%s droplet %d?", label, id)
	m.st = stateConfirm
	return m
}

/* ---------------- create droplet ---------------- */

func (m Model) updateCreateDroplet(k tea.KeyMsg, cmds []tea.Cmd) (Model, []tea.Cmd) {
	if key.Matches(k, m.keys.Back) {
		m.st = stateDroplets
		return m, cmds
	}
	if key.Matches(k, m.keys.PickSSH) {
		// sync selection map from field
		m.sshSelected = map[int]bool{}
		ids, _ := do.ParseCSVInts(m.sshIDsIn.Value())
		for _, id := range ids {
			m.sshSelected[id] = true
		}

		m.st = statePickSSHKeys
		if len(m.sshKeys) == 0 {
			cmds = append(cmds, m.loadSSHKeysCmd())
		} else {
			m.sshTable.SetRows(toSSHTableRows(m.sshKeys, m.sshSelected))
		}
		return m, cmds
	}
	if m.busy {
		return m, cmds
	}

	// navigation
	if k.String() == "tab" || k.String() == "shift+tab" {
		m.blurDropletForm()
		if k.String() == "tab" {
			m.focusDropletForm = (m.focusDropletForm + 1) % 8
		} else {
			m.focusDropletForm = (m.focusDropletForm - 1 + 8) % 8
		}
		m.focusDropletOne()
		return m, cmds
	}

	if k.String() == "enter" {
		req, err := m.buildCreateDropletReq()
		if err != nil {
			m.errText = err.Error()
			return m, cmds
		}

		m.pendingCreateDroplet = &req
		m.pendingAct = actCreateDroplet
		m.confirmReturn = stateCreateDroplet
		m.confirmText = fmt.Sprintf(
			"Create droplet?\n\nName: %s\nRegion: %s\nSize: %s\nImage: %s\nIPv6: %v\nTags: %s\nSSH IDs: %s",
			req.Name, req.Region, req.Size, req.ImageSlug, req.EnableIPv6,
			strings.Join(req.Tags, ","), m.sshIDsIn.Value(),
		)
		m.st = stateConfirm
		return m, cmds
	}

	// update focused input
	var cmd tea.Cmd
	switch m.focusDropletForm {
	case 0:
		m.nameIn, cmd = m.nameIn.Update(k)
	case 1:
		m.regionIn, cmd = m.regionIn.Update(k)
	case 2:
		m.sizeIn, cmd = m.sizeIn.Update(k)
	case 3:
		m.imageIn, cmd = m.imageIn.Update(k)
	case 4:
		m.sshIDsIn, cmd = m.sshIDsIn.Update(k)
	case 5:
		m.tagsIn, cmd = m.tagsIn.Update(k)
	case 6:
		m.ipv6In, cmd = m.ipv6In.Update(k)
	case 7:
		m.vpcIn, cmd = m.vpcIn.Update(k)
	}
	cmds = append(cmds, cmd)
	return m, cmds
}

func (m Model) updatePickSSH(k tea.KeyMsg, cmds []tea.Cmd) (Model, []tea.Cmd) {
	if key.Matches(k, m.keys.Back) {
		m.st = stateCreateDroplet
		return m, cmds
	}
	if m.busy {
		return m, cmds
	}

	switch k.String() {
	case " ":
		i := m.sshTable.Cursor()
		if i >= 0 && i < len(m.sshKeys) {
			id := m.sshKeys[i].ID
			m.sshSelected[id] = !m.sshSelected[id]
			m.sshTable.SetRows(toSSHTableRows(m.sshKeys, m.sshSelected))
		}
	case "enter":
		var ids []string
		for id, ok := range m.sshSelected {
			if ok {
				ids = append(ids, strconv.Itoa(id))
			}
		}
		sort.Strings(ids)
		m.sshIDsIn.SetValue(strings.Join(ids, ","))
		m.st = stateCreateDroplet
	}
	return m, cmds
}

/* ---------------- volumes handlers ---------------- */

func (m Model) updateVolumes(k tea.KeyMsg, cmds []tea.Cmd) (Model, []tea.Cmd) {
	if m.busy {
		return m, cmds
	}

	switch {
	case key.Matches(k, m.keys.Refresh):
		m.captureVolumeCursor()
		cmds = append(cmds, m.refreshVolumesCmd())

	case key.Matches(k, m.keys.Create):
		m.st = stateCreateVolume
		m.initVolumeCreateForm()

	case key.Matches(k, m.keys.Delete):
		id, ok := m.currentSelectedVolumeID()
		if ok {
			m.selectedVolumeID = id
			m.pendingDeleteVolID = id
			m.pendingAct = actDeleteVolume
			m.confirmReturn = stateVolumes
			m.confirmText = fmt.Sprintf("Delete volume %s?\nThis cannot be undone.", id)
			m.st = stateConfirm
		}
	}

	return m, cmds
}

func (m Model) updateCreateVolume(k tea.KeyMsg, cmds []tea.Cmd) (Model, []tea.Cmd) {
	if key.Matches(k, m.keys.Back) {
		m.st = stateVolumes
		return m, cmds
	}
	if m.busy {
		return m, cmds
	}

	if k.String() == "tab" || k.String() == "shift+tab" {
		m.blurVolForm()
		if k.String() == "tab" {
			m.focusVolForm = (m.focusVolForm + 1) % 4
		} else {
			m.focusVolForm = (m.focusVolForm - 1 + 4) % 4
		}
		m.focusVolOne()
		return m, cmds
	}

	if k.String() == "enter" {
		req, err := m.buildCreateVolumeReq()
		if err != nil {
			m.errText = err.Error()
			return m, cmds
		}
		m.pendingCreateVolume = &req
		m.pendingAct = actCreateVolume
		m.confirmReturn = stateCreateVolume
		m.confirmText = fmt.Sprintf("Create volume?\n\nName: %s\nRegion: %s\nSizeGB: %d\nDesc: %s",
			req.Name, req.Region, req.SizeGB, req.Description,
		)
		m.st = stateConfirm
		return m, cmds
	}

	var cmd tea.Cmd
	switch m.focusVolForm {
	case 0:
		m.volNameIn, cmd = m.volNameIn.Update(k)
	case 1:
		m.volRegionIn, cmd = m.volRegionIn.Update(k)
	case 2:
		m.volSizeIn, cmd = m.volSizeIn.Update(k)
	case 3:
		m.volDescIn, cmd = m.volDescIn.Update(k)
	}
	cmds = append(cmds, cmd)
	return m, cmds
}

func (m Model) updateOps(k tea.KeyMsg, cmds []tea.Cmd) (Model, []tea.Cmd) {
	_ = k
	_ = cmds
	m.opsTable.SetRows(toOpsRows(m.ops))
	return m, cmds
}

func (m Model) updateConfirm(k tea.KeyMsg, cmds []tea.Cmd) (Model, []tea.Cmd) {
	switch {
	case key.Matches(k, m.keys.No) || key.Matches(k, m.keys.Back):
		m.st = m.confirmReturn

	case key.Matches(k, m.keys.Yes):
		if m.busy {
			return m, cmds
		}

		switch m.pendingAct {
		case actCreateDroplet:
			if m.pendingCreateDroplet == nil {
				m.errText = "internal: pending droplet req is nil"
				m.st = m.confirmReturn
				return m, cmds
			}
			req := *m.pendingCreateDroplet
			m.pendingCreateDroplet = nil
			cmds = append(cmds, m.createDropletCmd(req))
			return m, cmds

		case actDeleteDroplet:
			cmds = append(cmds, m.runDropletActionCmd(actDeleteDroplet, m.selectedDropletID))
			return m, cmds

		case actPowerOn, actPowerOff, actShutdown, actReboot:
			cmds = append(cmds, m.runDropletActionCmd(m.pendingAct, m.selectedDropletID))
			return m, cmds

		case actCreateVolume:
			if m.pendingCreateVolume == nil {
				m.errText = "internal: pending volume req is nil"
				m.st = m.confirmReturn
				return m, cmds
			}
			req := *m.pendingCreateVolume
			m.pendingCreateVolume = nil
			cmds = append(cmds, m.createVolumeCmd(req))
			return m, cmds

		case actDeleteVolume:
			id := m.pendingDeleteVolID
			m.pendingDeleteVolID = ""
			cmds = append(cmds, m.deleteVolumeCmd(id))
			return m, cmds
		}

		m.st = m.confirmReturn
	}
	return m, cmds
}

func (m Model) refreshDropletsCmd() tea.Cmd {
	m.busy = true
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(m.ctx, 30*time.Second)
		defer cancel()
		rows, err := m.api.ListDroplets(ctx)
		if err != nil {
			return apiErrMsg{err: err}
		}
		return dropletsLoadedMsg{rows: rows}
	}
}

func (m Model) loadDetailsCmd(id int) tea.Cmd {
	m.busy = true
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(m.ctx, 30*time.Second)
		defer cancel()
		d, err := m.api.GetDroplet(ctx, id)
		if err != nil {
			return apiErrMsg{err: err}
		}
		return dropletDetailsMsg{d: d}
	}
}

func (m Model) loadSSHKeysCmd() tea.Cmd {
	m.busy = true
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(m.ctx, 30*time.Second)
		defer cancel()
		keys, err := m.api.ListSSHKeys(ctx)
		if err != nil {
			return apiErrMsg{err: err}
		}
		return sshKeysLoadedMsg{keys: keys}
	}
}

func (m Model) refreshVolumesCmd() tea.Cmd {
	m.busy = true
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(m.ctx, 30*time.Second)
		defer cancel()
		rows, err := m.api.ListVolumes(ctx)
		if err != nil {
			return apiErrMsg{err: err}
		}
		return volumesLoadedMsg{rows: rows}
	}
}

func (m Model) runDropletActionCmd(a actionKind, id int) tea.Cmd {
	m.busy = true
	target := fmt.Sprintf("droplet:%d", id)
	m.logOp(actToString(a), target, "requested")

	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(m.ctx, 60*time.Second)
		defer cancel()

		var err error
		switch a {
		case actPowerOn:
			err = m.api.PowerOn(ctx, id)
		case actPowerOff:
			err = m.api.PowerOff(ctx, id)
		case actShutdown:
			err = m.api.Shutdown(ctx, id)
		case actReboot:
			err = m.api.Reboot(ctx, id)
		case actDeleteDroplet:
			err = m.api.DeleteDroplet(ctx, id)
		default:
			err = fmt.Errorf("unknown droplet action")
		}

		if err != nil {
			return apiErrMsg{err: err}
		}
		return apiDoneMsg{act: a, status: "ok", target: target}
	}
}

func (m Model) createDropletCmd(req do.CreateDropletReq) tea.Cmd {
	m.busy = true
	target := "droplet:" + req.Name
	m.logOp("droplet.create", target, "requested")

	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(m.ctx, 90*time.Second)
		defer cancel()

		d, err := m.api.CreateDroplet(ctx, req)
		if err != nil {
			return apiErrMsg{err: err}
		}
		return apiDoneMsg{
			act:    actCreateDroplet,
			status: fmt.Sprintf("Created droplet %q (id=%d)", d.Name, d.ID),
			target: target,
		}
	}
}

func (m Model) createVolumeCmd(req do.CreateVolumeReq) tea.Cmd {
	m.busy = true
	target := "volume:" + req.Name
	m.logOp("volume.create", target, "requested")

	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(m.ctx, 60*time.Second)
		defer cancel()

		v, err := m.api.CreateVolume(ctx, req)
		if err != nil {
			return apiErrMsg{err: err}
		}
		return apiDoneMsg{
			act:    actCreateVolume,
			status: fmt.Sprintf("Created volume %q (id=%s)", v.Name, v.ID),
			target: target,
		}
	}
}

func (m Model) deleteVolumeCmd(id string) tea.Cmd {
	m.busy = true
	target := "volume:" + id
	m.logOp("volume.delete", target, "requested")

	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(m.ctx, 60*time.Second)
		defer cancel()

		if err := m.api.DeleteVolume(ctx, id); err != nil {
			return apiErrMsg{err: err}
		}
		return apiDoneMsg{
			act:    actDeleteVolume,
			status: "Deleted volume " + id,
			target: target,
		}
	}
}

/* ---------------- form init + helpers ---------------- */

func newInput(label, placeholder string) textinput.Model {
	in := textinput.New()
	in.Prompt = label + ": "
	in.Placeholder = placeholder
	in.CharLimit = 128
	in.Width = 64
	return in
}

func (m *Model) initDropletCreateForm() {
	m.nameIn = newInput("Name", "my-droplet")
	m.regionIn = newInput("Region", "fra1")
	m.sizeIn = newInput("Size", "s-1vcpu-1gb")
	m.imageIn = newInput("Image slug", "ubuntu-24-04-x64")
	m.sshIDsIn = newInput("SSH key IDs (csv)", "")
	m.tagsIn = newInput("Tags (csv)", "")
	m.ipv6In = newInput("Enable IPv6 (true/false)", "false")
	m.vpcIn = newInput("VPC UUID (optional)", "")

	m.nameIn.SetValue("")
	m.regionIn.SetValue(m.opts.DefaultRegion)
	m.sizeIn.SetValue(m.opts.DefaultSize)
	m.imageIn.SetValue(m.opts.DefaultImage)
	m.tagsIn.SetValue(m.opts.DefaultTags)
	if m.opts.DefaultIPv6 {
		m.ipv6In.SetValue("true")
	} else {
		m.ipv6In.SetValue("false")
	}

	m.focusDropletForm = 0
	m.blurDropletForm()
	m.nameIn.Focus()
}

func (m *Model) blurDropletForm() {
	m.nameIn.Blur()
	m.regionIn.Blur()
	m.sizeIn.Blur()
	m.imageIn.Blur()
	m.sshIDsIn.Blur()
	m.tagsIn.Blur()
	m.ipv6In.Blur()
	m.vpcIn.Blur()
}

func (m *Model) focusDropletOne() {
	switch m.focusDropletForm {
	case 0:
		m.nameIn.Focus()
	case 1:
		m.regionIn.Focus()
	case 2:
		m.sizeIn.Focus()
	case 3:
		m.imageIn.Focus()
	case 4:
		m.sshIDsIn.Focus()
	case 5:
		m.tagsIn.Focus()
	case 6:
		m.ipv6In.Focus()
	case 7:
		m.vpcIn.Focus()
	}
}

func (m Model) buildCreateDropletReq() (do.CreateDropletReq, error) {
	name := strings.TrimSpace(m.nameIn.Value())
	if name == "" {
		return do.CreateDropletReq{}, fmt.Errorf("name is required (fill Name field)")
	}
	if m.dropletNameExists(name) {
		return do.CreateDropletReq{}, fmt.Errorf("droplet %q already exists (choose another name)", name)
	}

	sshIDs, err := do.ParseCSVInts(m.sshIDsIn.Value())
	if err != nil {
		return do.CreateDropletReq{}, err
	}

	ipv6 := strings.EqualFold(strings.TrimSpace(m.ipv6In.Value()), "true")
	tags := splitCSV(m.tagsIn.Value())

	return do.CreateDropletReq{
		Name:       name,
		Region:     strings.TrimSpace(m.regionIn.Value()),
		Size:       strings.TrimSpace(m.sizeIn.Value()),
		ImageSlug:  strings.TrimSpace(m.imageIn.Value()),
		SSHKeyIDs:  sshIDs,
		Tags:       tags,
		EnableIPv6: ipv6,
		VPCUUID:    strings.TrimSpace(m.vpcIn.Value()),
	}, nil
}

func (m *Model) initVolumeCreateForm() {
	m.volNameIn = newInput("Name", "my-volume")
	m.volRegionIn = newInput("Region", m.opts.DefaultRegion)
	m.volSizeIn = newInput("SizeGB", "10")
	m.volDescIn = newInput("Description", "")

	m.volNameIn.SetValue("")
	m.volRegionIn.SetValue(m.opts.DefaultRegion)
	m.volSizeIn.SetValue("10")
	m.volDescIn.SetValue("")

	m.focusVolForm = 0
	m.blurVolForm()
	m.volNameIn.Focus()
}

func (m *Model) blurVolForm() {
	m.volNameIn.Blur()
	m.volRegionIn.Blur()
	m.volSizeIn.Blur()
	m.volDescIn.Blur()
}

func (m *Model) focusVolOne() {
	switch m.focusVolForm {
	case 0:
		m.volNameIn.Focus()
	case 1:
		m.volRegionIn.Focus()
	case 2:
		m.volSizeIn.Focus()
	case 3:
		m.volDescIn.Focus()
	}
}

func (m Model) buildCreateVolumeReq() (do.CreateVolumeReq, error) {
	name := strings.TrimSpace(m.volNameIn.Value())
	if name == "" {
		return do.CreateVolumeReq{}, fmt.Errorf("volume name is required")
	}
	region := strings.TrimSpace(m.volRegionIn.Value())
	if region == "" {
		region = m.opts.DefaultRegion
	}
	sizeStr := strings.TrimSpace(m.volSizeIn.Value())
	gb, err := strconv.ParseInt(sizeStr, 10, 64)
	if err != nil || gb <= 0 {
		return do.CreateVolumeReq{}, fmt.Errorf("SizeGB must be a positive integer")
	}
	desc := strings.TrimSpace(m.volDescIn.Value())

	return do.CreateVolumeReq{
		Name:        name,
		Region:      region,
		SizeGB:      gb,
		Description: desc,
	}, nil
}

/* ---------------- selection + restore cursor ---------------- */

func (m Model) currentSelectedDropletID() (int, bool) {
	i := m.dropletTable.Cursor()
	if i < 0 || i >= len(m.dropletRows) {
		return 0, false
	}
	return m.dropletRows[i].ID, true
}

func (m *Model) captureDropletCursor() {
	if id, ok := m.currentSelectedDropletID(); ok {
		m.restoreDropID = id
	}
}

func (m *Model) restoreDropletCursor() {
	if m.restoreDropID == 0 {
		return
	}
	for i := range m.dropletRows {
		if m.dropletRows[i].ID == m.restoreDropID {
			m.dropletTable.SetCursor(i)
			break
		}
	}
	m.restoreDropID = 0
}

func (m Model) currentSelectedVolumeID() (string, bool) {
	i := m.volumeTable.Cursor()
	if i < 0 || i >= len(m.volumeRows) {
		return "", false
	}
	return m.volumeRows[i].ID, true
}

func (m *Model) captureVolumeCursor() {
	if id, ok := m.currentSelectedVolumeID(); ok {
		m.restoreVolID = id
	}
}

func (m *Model) restoreVolumeCursor() {
	if m.restoreVolID == "" {
		return
	}
	for i := range m.volumeRows {
		if m.volumeRows[i].ID == m.restoreVolID {
			m.volumeTable.SetCursor(i)
			break
		}
	}
	m.restoreVolID = ""
}

/* ---------------- ssh helpers ---------------- */

func (m Model) selectedSSHNames() string {
	if len(m.sshSelected) == 0 || len(m.sshKeys) == 0 {
		return "-"
	}
	var names []string
	for _, k := range m.sshKeys {
		if m.sshSelected[k.ID] {
			names = append(names, k.Name)
		}
	}
	if len(names) == 0 {
		return "-"
	}
	return strings.Join(names, ", ")
}

/* ---------------- ops log ---------------- */

func (m *Model) logOp(kind, target, result string) {
	e := OpEntry{When: time.Now(), Kind: kind, Target: target, Result: result}
	m.ops = append([]OpEntry{e}, m.ops...)
	if len(m.ops) > 200 {
		m.ops = m.ops[:200]
	}
}

func actToString(a actionKind) string {
	switch a {
	case actPowerOn:
		return "droplet.poweron"
	case actPowerOff:
		return "droplet.poweroff"
	case actShutdown:
		return "droplet.shutdown"
	case actReboot:
		return "droplet.reboot"
	case actDeleteDroplet:
		return "droplet.delete"
	case actCreateDroplet:
		return "droplet.create"
	case actCreateVolume:
		return "volume.create"
	case actDeleteVolume:
		return "volume.delete"
	default:
		return "unknown"
	}
}

/* ---------------- table row builders ---------------- */

func toDropletTableRows(rows []do.DropletRow) []table.Row {
	out := make([]table.Row, 0, len(rows))
	for _, r := range rows {
		out = append(out, table.Row{
			strconv.Itoa(r.ID),
			r.Name,
			r.Region,
			r.Size,
			r.Status,
			r.IPv4,
		})
	}
	return out
}

func toVolumeTableRows(rows []do.VolumeRow) []table.Row {
	out := make([]table.Row, 0, len(rows))
	for _, r := range rows {
		out = append(out, table.Row{
			r.ID,
			r.Name,
			r.Region,
			strconv.FormatInt(r.SizeGB, 10),
			r.Description,
		})
	}
	return out
}

func toOpsRows(rows []OpEntry) []table.Row {
	out := make([]table.Row, 0, len(rows))
	for _, r := range rows {
		out = append(out, table.Row{
			r.When.Format("2006-01-02 15:04:05"),
			r.Kind,
			r.Target,
			r.Result,
		})
	}
	return out
}

func toSSHTableRows(keys []do.SSHKeyRow, sel map[int]bool) []table.Row {
	out := make([]table.Row, 0, len(keys))
	for _, k := range keys {
		mark := " "
		if sel[k.ID] {
			mark = "✓"
		}
		out = append(out, table.Row{
			mark,
			strconv.Itoa(k.ID),
			k.Name,
			k.Fingerprint,
		})
	}
	return out
}

/* ---------------- misc helpers ---------------- */

func (m Model) dropletNameExists(name string) bool {
	name = strings.TrimSpace(strings.ToLower(name))
	if name == "" {
		return false
	}
	for _, r := range m.dropletRows {
		if strings.ToLower(strings.TrimSpace(r.Name)) == name {
			return true
		}
	}
	return false
}

func splitCSV(s string) []string {
	var out []string
	for _, p := range strings.Split(s, ",") {
		p = strings.TrimSpace(p)
		if p != "" {
			out = append(out, p)
		}
	}
	return out
}

func firstIPv4(n *godo.Networks) string {
	if n == nil {
		return ""
	}
	for _, v := range n.V4 {
		if v.IPAddress != "" {
			return v.IPAddress
		}
	}
	return ""
}

func max(a, b int) int {
	if a > b {
		return a
	}
	return b
}
func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}
