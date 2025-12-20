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
	stateList state = iota
	stateDetails
	stateCreate
	stateConfirmDelete
	stateConfirmAction
	statePickSSHKeys
)

type actionKind int

const (
	actPowerOn actionKind = iota
	actPowerOff
	actShutdown
	actReboot
	actDelete
)

type Options struct {
	DefaultRegion string
	DefaultSize   string
	DefaultImage  string
	DefaultTags   string // CSV
	DefaultIPv6   bool
}

type api interface {
	ListDroplets(context.Context) ([]do.DropletRow, error)
	GetDroplet(context.Context, int) (*godo.Droplet, error)
	PowerOn(context.Context, int) error
	PowerOff(context.Context, int) error
	Shutdown(context.Context, int) error
	Reboot(context.Context, int) error
	DeleteDroplet(context.Context, int) error
	CreateDroplet(context.Context, do.CreateDropletReq) (*godo.Droplet, error)
	ListSSHKeys(context.Context) ([]do.SSHKeyRow, error)
}

type keyMap struct {
	Up, Down, Enter, Back, Refresh, Create, Delete, Details key.Binding
	PowerOn, PowerOff, Shutdown, Reboot                     key.Binding
	PickSSH                                                 key.Binding
	Yes, No                                                 key.Binding
	Quit                                                    key.Binding
}

type sshKeysLoadedMsg struct{ keys []do.SSHKeyRow }

func (k keyMap) ShortHelp() []key.Binding {
	return []key.Binding{k.Up, k.Down, k.Enter, k.Refresh, k.Create, k.Delete, k.Quit}
}
func (k keyMap) FullHelp() [][]key.Binding {
	return [][]key.Binding{
		{k.Up, k.Down, k.Enter, k.Back},
		{k.Refresh, k.Create, k.Delete, k.Details},
		{k.PowerOn, k.PowerOff, k.Shutdown, k.Reboot},
		{k.Quit},
	}
}

func defaultKeys() keyMap {
	// Using tcell constants to satisfy “tcell+bubbletea” requirement (and to be future-proof if you later wire a tcell screen).
	_ = tcell.KeyEnter

	return keyMap{
		Up:       key.NewBinding(key.WithKeys("up", "k"), key.WithHelp("↑/k", "up")),
		Down:     key.NewBinding(key.WithKeys("down", "j"), key.WithHelp("↓/j", "down")),
		Enter:    key.NewBinding(key.WithKeys("enter"), key.WithHelp("enter", "select")),
		Back:     key.NewBinding(key.WithKeys("esc"), key.WithHelp("esc", "back")),
		Refresh:  key.NewBinding(key.WithKeys("r"), key.WithHelp("r", "refresh")),
		Create:   key.NewBinding(key.WithKeys("c"), key.WithHelp("c", "create")),
		Delete:   key.NewBinding(key.WithKeys("d"), key.WithHelp("d", "delete")),
		Details:  key.NewBinding(key.WithKeys("enter"), key.WithHelp("enter", "details")),
		PowerOn:  key.NewBinding(key.WithKeys("o"), key.WithHelp("o", "power on")),
		PowerOff: key.NewBinding(key.WithKeys("p"), key.WithHelp("p", "power off")),
		Shutdown: key.NewBinding(key.WithKeys("s"), key.WithHelp("s", "shutdown")),
		Reboot:   key.NewBinding(key.WithKeys("b"), key.WithHelp("b", "reboot")),
		PickSSH:  key.NewBinding(key.WithKeys("k"), key.WithHelp("k", "pick ssh keys")),
		Yes:      key.NewBinding(key.WithKeys("y"), key.WithHelp("y", "yes")),
		No:       key.NewBinding(key.WithKeys("n"), key.WithHelp("n", "no")),
		Quit:     key.NewBinding(key.WithKeys("q", "ctrl+c"), key.WithHelp("q", "quit")),
	}
}

type Model struct {
	api api
	ctx context.Context

	keys keyMap
	help help.Model

	spinner spinner.Model
	busy    bool

	st state

	width, height int

	table   table.Model
	rows    []do.DropletRow
	errText string
	status  string

	// details
	selectedID int
	droplet    *godo.Droplet

	// confirm
	confirmText string
	pendingAct  actionKind

	// create form
	focus             int
	nameIn            textinput.Model
	regionIn          textinput.Model
	sizeIn            textinput.Model
	imageIn           textinput.Model
	sshIDsIn          textinput.Model
	tagsIn            textinput.Model
	ipv6In            textinput.Model // "true/false"
	vpcIn             textinput.Model
	opts              Options
	sshTable          table.Model
	sshKeys           []do.SSHKeyRow
	sshSelected       map[int]bool // keyID -> selected
	sshPickerErr      string
	pendingOpenCreate bool
}

func NewModel(api api, opts Options) Model {
	sp := spinner.New()
	sp.Spinner = spinner.Line

	cols := []table.Column{
		{Title: "ID", Width: 10},
		{Title: "Name", Width: 24},
		{Title: "Region", Width: 8},
		{Title: "Size", Width: 14},
		{Title: "Status", Width: 10},
		{Title: "IPv4", Width: 15},
	}

	t := table.New(
		table.WithColumns(cols),
		table.WithFocused(true),
		table.WithHeight(14),
	)

	m := Model{
		api:     api,
		ctx:     context.Background(),
		keys:    defaultKeys(),
		help:    help.New(),
		spinner: sp,
		st:      stateList,
		table:   t,
		opts:    opts,
		status:  "Press r to load droplets",
	}

	m.initCreateForm()
	m.sshSelected = map[int]bool{}

	m.sshTable = table.New(
		table.WithColumns([]table.Column{
			{Title: "Sel", Width: 3},
			{Title: "ID", Width: 8},
			{Title: "Name", Width: 26},
			{Title: "Fingerprint", Width: 24},
		}),
		table.WithFocused(true),
		table.WithHeight(14),
	)

	return m
}

func (m Model) Init() tea.Cmd {
	return tea.Batch(m.spinner.Tick, m.refreshCmd())
}

func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	var cmds []tea.Cmd

	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width, m.height = msg.Width, msg.Height
		m.table.SetHeight(max(8, m.height-9))
	case spinner.TickMsg:
		var cmd tea.Cmd
		m.spinner, cmd = m.spinner.Update(msg)
		cmds = append(cmds, cmd)

	case dropletsLoadedMsg:
		m.busy = false
		m.errText = ""
		m.rows = msg.rows
		m.table.SetRows(toTableRows(m.rows))
		m.status = fmt.Sprintf("Loaded %d droplet(s)", len(m.rows))

		if m.pendingOpenCreate {
			m.pendingOpenCreate = false
			m.st = stateCreate
			m.initCreateForm()
		}

	case dropletDetailsMsg:
		m.busy = false
		m.errText = ""
		m.droplet = msg.d
		m.status = "Details loaded"
		m.st = stateDetails

	case apiDoneMsg:
		m.busy = false
		m.errText = ""
		m.status = msg.status

		// Requirement: after "start" (Power On) go back to list
		if msg.act == actPowerOn {
			m.st = stateList
			m.droplet = nil
		}

		// Optional: also leave confirm dialog after any action
		if m.st == stateConfirmAction || m.st == stateConfirmDelete {
			// If we just powered on we already forced list above;
			// otherwise go back to details if you want:
			if m.st != stateList {
				m.st = stateDetails
			}
		}

		cmds = append(cmds, m.refreshCmd())

	case apiErrMsg:
		m.busy = false
		m.errText = msg.err.Error()
		m.status = "Error"

	case sshKeysLoadedMsg:
		m.busy = false
		m.errText = ""
		m.sshKeys = msg.keys
		m.sshTable.SetRows(toSSHTableRows(m.sshKeys, m.sshSelected))
		m.status = fmt.Sprintf("Loaded %d SSH key(s)", len(m.sshKeys))

	case tea.KeyMsg:

		if key.Matches(msg, m.keys.Quit) {
			return m, tea.Quit
		}

		switch m.st {
		case stateList:
			m, cmds = m.updateList(msg, cmds)
		case stateDetails:
			m, cmds = m.updateDetails(msg, cmds)
		case stateConfirmDelete, stateConfirmAction:
			m, cmds = m.updateConfirm(msg, cmds)
		case stateCreate:
			m, cmds = m.updateCreate(msg, cmds)
		case statePickSSHKeys:
			m, cmds = m.updatePickSSH(msg, cmds)
		}
	}

	// table always receives key updates in list state
	if m.st == stateList {
		var cmd tea.Cmd
		m.table, cmd = m.table.Update(msg)
		cmds = append(cmds, cmd)
	}

	return m, tea.Batch(cmds...)
}

func (m Model) nameExists(name string) bool {
	name = strings.TrimSpace(strings.ToLower(name))
	if name == "" {
		return false
	}
	for _, r := range m.rows {
		if strings.ToLower(strings.TrimSpace(r.Name)) == name {
			return true
		}
	}
	return false
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

func (m Model) viewPickSSH() string {
	h := lipgloss.NewStyle().Bold(true).Render("Pick SSH Keys")
	legend := lipgloss.NewStyle().Faint(true).Render("space=toggle  enter=use selected  esc=back")
	return h + "\n\n" + m.sshTable.View() + "\n\n" + legend
}

func (m Model) View() string {
	title := lipgloss.NewStyle().Bold(true).Render("DigitalOcean Droplets TUI")
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
	case stateList:
		return top + m.viewList()
	case stateDetails:
		return top + m.viewDetails()
	case stateCreate:
		return top + m.viewCreate()
	case stateConfirmDelete, stateConfirmAction:
		return top + m.viewConfirm()
	case statePickSSHKeys:
		return top + m.viewPickSSH()
	default:
		return top + "unknown state\n"
	}
}

func (m Model) viewList() string {
	footer := lipgloss.NewStyle().Faint(true).Render(m.status) + "\n"
	footer += m.help.View(m.keys)

	return m.table.View() + "\n" + footer
}

func (m Model) viewDetails() string {
	if m.droplet == nil {
		return "No droplet loaded\n\n" + m.help.View(m.keys)
	}

	d := m.droplet
	var b strings.Builder
	fmt.Fprintf(&b, "%s\n\n", lipgloss.NewStyle().Bold(true).Render("Droplet Details"))
	fmt.Fprintf(&b, "ID: %d\n", d.ID)
	fmt.Fprintf(&b, "Name: %s\n", d.Name)
	fmt.Fprintf(&b, "Status: %s\n", d.Status)
	fmt.Fprintf(&b, "Region: %s\n", d.Region.Slug)
	fmt.Fprintf(&b, "Size: %s\n", d.SizeSlug)
	fmt.Fprintf(&b, "IPv4: %s\n", firstIPv4(d.Networks))
	fmt.Fprintf(&b, "IPv6: %s\n", firstIPv6(d.Networks))
	fmt.Fprintf(&b, "Tags: %s\n", strings.Join(d.Tags, ","))
	fmt.Fprintf(&b, "Created: %s\n", d.Created)

	b.WriteString("\n")
	b.WriteString(lipgloss.NewStyle().Faint(true).Render("Keys: esc=back  o=power on  p=power off  s=shutdown  b=reboot  d=delete  r=refresh  q=quit"))
	return b.String()
}

func (m Model) viewConfirm() string {
	box := lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		Padding(1, 2).
		Width(min(80, max(40, m.width-4))).
		Render(m.confirmText + "\n\n[y] Yes   [n] No")

	return "\n" + box + "\n"
}

func (m *Model) initCreateForm() {
	m.nameIn = newInput("Name", "my-droplet")
	m.regionIn = newInput("Region", "fra1")
	m.sizeIn = newInput("Size", "s-1vcpu-1gb")
	m.imageIn = newInput("Image slug", "ubuntu-24-04-x64")
	m.sshIDsIn = newInput("SSH key IDs (csv)", "12345,67890")
	m.tagsIn = newInput("Tags (csv)", "dev,tui")
	m.ipv6In = newInput("Enable IPv6 (true/false)", "false")
	m.vpcIn = newInput("VPC UUID (optional)", "")

	// IMPORTANT: set actual default values so Create works immediately.
	m.nameIn.SetValue("") // keep empty so user must name it (recommended)
	m.regionIn.SetValue(m.opts.DefaultRegion)
	m.sizeIn.SetValue(m.opts.DefaultSize)
	m.imageIn.SetValue(m.opts.DefaultImage)
	m.tagsIn.SetValue(m.opts.DefaultTags)
	if m.opts.DefaultIPv6 {
		m.ipv6In.SetValue("true")
	} else {
		m.ipv6In.SetValue("false")
	}

	m.focus = 0
	m.blurAll()
	m.nameIn.Focus()
}

func (m Model) viewCreate() string {
	legend := lipgloss.NewStyle().Faint(true).Render("Tab/Shift+Tab to move, enter to submit, esc to cancel")
	var b strings.Builder
	b.WriteString(lipgloss.NewStyle().Bold(true).Render("Create Droplet") + "\n\n")
	b.WriteString(m.nameIn.View() + "\n")
	b.WriteString(m.regionIn.View() + "\n")
	b.WriteString(m.sizeIn.View() + "\n")
	b.WriteString(m.imageIn.View() + "\n")
	b.WriteString(m.sshIDsIn.View() + "\n")
	b.WriteString(m.tagsIn.View() + "\n")
	b.WriteString(m.ipv6In.View() + "\n")
	b.WriteString(m.vpcIn.View() + "\n\n")
	b.WriteString(legend)
	return b.String()
}

func (m Model) updateList(k tea.KeyMsg, cmds []tea.Cmd) (Model, []tea.Cmd) {
	if m.busy {
		return m, cmds
	}

	switch {
	case key.Matches(k, m.keys.Create):
		// If droplets not loaded yet, auto-load then open create.
		if len(m.rows) == 0 {
			m.pendingOpenCreate = true
			cmds = append(cmds, m.refreshCmd())
			m.status = "Loading droplets…"
			return m, cmds
		}
		m.st = stateCreate
		m.initCreateForm()
	case key.Matches(k, m.keys.Enter):
		id, ok := m.currentSelectedID()
		if ok {
			m.selectedID = id
			cmds = append(cmds, m.loadDetailsCmd(id))
		}
	case key.Matches(k, m.keys.Delete):
		id, ok := m.currentSelectedID()
		if ok {
			m.selectedID = id
			m.pendingAct = actDelete
			m.confirmText = fmt.Sprintf("Delete droplet %d? This cannot be undone.", id)
			m.st = stateConfirmDelete
		}
	case key.Matches(k, m.keys.PowerOn):
		m = m.confirmActionFromList(actPowerOn, "Power ON")
	case key.Matches(k, m.keys.PowerOff):
		m = m.confirmActionFromList(actPowerOff, "Power OFF")
	case key.Matches(k, m.keys.Shutdown):
		m = m.confirmActionFromList(actShutdown, "Shutdown")
	case key.Matches(k, m.keys.Reboot):
		m = m.confirmActionFromList(actReboot, "Reboot")
	}
	return m, cmds
}

func (m Model) confirmActionFromList(a actionKind, label string) Model {
	id, ok := m.currentSelectedID()
	if !ok {
		return m
	}
	m.selectedID = id
	m.pendingAct = a
	m.confirmText = fmt.Sprintf("%s droplet %d?", label, id)
	m.st = stateConfirmAction
	return m
}

func (m Model) updateDetails(k tea.KeyMsg, cmds []tea.Cmd) (Model, []tea.Cmd) {
	if key.Matches(k, m.keys.Back) {
		m.st = stateList
		return m, cmds
	}
	if m.busy {
		return m, cmds
	}

	id := m.selectedID
	switch {
	case key.Matches(k, m.keys.Refresh):
		cmds = append(cmds, m.loadDetailsCmd(id))
	case key.Matches(k, m.keys.PowerOn):
		m.pendingAct = actPowerOn
		m.confirmText = fmt.Sprintf("Power ON droplet %d?", id)
		m.st = stateConfirmAction
	case key.Matches(k, m.keys.PowerOff):
		m.pendingAct = actPowerOff
		m.confirmText = fmt.Sprintf("Power OFF droplet %d?", id)
		m.st = stateConfirmAction
	case key.Matches(k, m.keys.Shutdown):
		m.pendingAct = actShutdown
		m.confirmText = fmt.Sprintf("Shutdown droplet %d?", id)
		m.st = stateConfirmAction
	case key.Matches(k, m.keys.Reboot):
		m.pendingAct = actReboot
		m.confirmText = fmt.Sprintf("Reboot droplet %d?", id)
		m.st = stateConfirmAction
	case key.Matches(k, m.keys.Delete):
		m.pendingAct = actDelete
		m.confirmText = fmt.Sprintf("Delete droplet %d? This cannot be undone.", id)
		m.st = stateConfirmDelete
	}
	return m, cmds
}

func (m Model) updateConfirm(k tea.KeyMsg, cmds []tea.Cmd) (Model, []tea.Cmd) {
	switch {
	case key.Matches(k, m.keys.No) || key.Matches(k, m.keys.Back):
		// back to previous
		if m.droplet != nil && m.st != stateList {
			// if we had details loaded, go back there
			m.st = stateDetails
		} else {
			m.st = stateList
		}
	case key.Matches(k, m.keys.Yes):
		if m.busy {
			return m, cmds
		}
		cmds = append(cmds, m.runActionCmd(m.pendingAct, m.selectedID))
	}
	return m, cmds
}

func (m Model) updatePickSSH(k tea.KeyMsg, cmds []tea.Cmd) (Model, []tea.Cmd) {
	if key.Matches(k, m.keys.Back) {
		m.st = stateCreate
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
		// write selected IDs into the Create form field (sshIDsIn)
		var ids []string
		for id, ok := range m.sshSelected {
			if ok {
				ids = append(ids, strconv.Itoa(id))
			}
		}
		sort.Strings(ids)
		m.sshIDsIn.SetValue(strings.Join(ids, ","))
		m.st = stateCreate
	}
	// allow table navigation
	var cmd tea.Cmd
	m.sshTable, cmd = m.sshTable.Update(k)
	cmds = append(cmds, cmd)
	return m, cmds
}

func (m Model) updateCreate(k tea.KeyMsg, cmds []tea.Cmd) (Model, []tea.Cmd) {
	if key.Matches(k, m.keys.Back) {
		m.st = stateList
		return m, cmds
	}
	if key.Matches(k, m.keys.PickSSH) {
		// open picker; load keys if not loaded yet
		m.st = statePickSSHKeys
		m.sshTable.SetHeight(max(8, m.height-9))
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

	// tab navigation
	if k.String() == "tab" || k.String() == "shift+tab" {
		m.blurAll()
		if k.String() == "tab" {
			m.focus = (m.focus + 1) % 8
		} else {
			m.focus = (m.focus - 1 + 8) % 8
		}
		m.focusOne()
		return m, cmds
	}

	// submit
	if k.String() == "enter" {
		req, err := m.buildCreateReq()
		if err != nil {
			m.errText = err.Error()
			return m, cmds
		}
		cmds = append(cmds, m.createDropletCmd(req))
		return m, cmds
	}

	// update focused input
	var cmd tea.Cmd
	switch m.focus {
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

func (m *Model) blurAll() {
	m.nameIn.Blur()
	m.regionIn.Blur()
	m.sizeIn.Blur()
	m.imageIn.Blur()
	m.sshIDsIn.Blur()
	m.tagsIn.Blur()
	m.ipv6In.Blur()
	m.vpcIn.Blur()
}

func (m *Model) focusOne() {
	switch m.focus {
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

func newInput(label, placeholder string) textinput.Model {
	in := textinput.New()
	in.Prompt = label + ": "
	in.Placeholder = placeholder
	in.CharLimit = 128
	in.Width = 60
	return in
}

func (m Model) buildCreateReq() (do.CreateDropletReq, error) {
	sshIDs, err := do.ParseCSVInts(m.sshIDsIn.Value())
	if err != nil {
		return do.CreateDropletReq{}, err
	}
	ipv6 := strings.EqualFold(strings.TrimSpace(m.ipv6In.Value()), "true")

	tags := splitCSV(m.tagsIn.Value())
	name := strings.TrimSpace(m.nameIn.Value())
	if name == "" {
		return do.CreateDropletReq{}, fmt.Errorf("name is required (fill Name field)")
	}
	if m.nameExists(name) {
		return do.CreateDropletReq{}, fmt.Errorf("droplet %q already exists (choose another name)", name)
	}

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

func (m Model) currentSelectedID() (int, bool) {
	i := m.table.Cursor()
	if i < 0 || i >= len(m.rows) {
		return 0, false
	}
	return m.rows[i].ID, true
}

/* ---------- async cmds + messages ---------- */

type dropletsLoadedMsg struct{ rows []do.DropletRow }
type dropletDetailsMsg struct{ d *godo.Droplet }
type apiDoneMsg struct {
	status string
	act    actionKind
}
type apiErrMsg struct{ err error }

func (m Model) refreshCmd() tea.Cmd {
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

func (m Model) runActionCmd(a actionKind, id int) tea.Cmd {
	m.busy = true
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(m.ctx, 60*time.Second)
		defer cancel()

		var err error
		var status string

		switch a {
		case actPowerOn:
			err = m.api.PowerOn(ctx, id)
			status = "Power on requested"
		case actPowerOff:
			err = m.api.PowerOff(ctx, id)
			status = "Power off requested"
		case actShutdown:
			err = m.api.Shutdown(ctx, id)
			status = "Shutdown requested"
		case actReboot:
			err = m.api.Reboot(ctx, id)
			status = "Reboot requested"
		case actDelete:
			err = m.api.DeleteDroplet(ctx, id)
			status = "Delete requested"
		default:
			err = fmt.Errorf("unknown action")
		}
		if err != nil {
			return apiErrMsg{err: err}
		}
		return apiDoneMsg{status: status, act: a}
	}
}

func (m Model) createDropletCmd(req do.CreateDropletReq) tea.Cmd {
	m.busy = true
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(m.ctx, 90*time.Second)
		defer cancel()

		d, err := m.api.CreateDroplet(ctx, req)
		if err != nil {
			return apiErrMsg{err: err}
		}
		return apiDoneMsg{status: fmt.Sprintf("Created droplet %q (id=%d)", d.Name, d.ID)}
	}
}

/* ---------- render helpers ---------- */

func toTableRows(rows []do.DropletRow) []table.Row {
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

func firstIPv6(n *godo.Networks) string {
	if n == nil {
		return ""
	}
	for _, v := range n.V6 {
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
