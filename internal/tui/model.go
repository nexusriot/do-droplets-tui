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

	stateVolumeDetails
	stateAttachVolume // droplet picker for attach
	stateResizeVolume // single-field resize input
	stateSnapshotName // single-field snapshot name input

	stateSnapshots // all-snapshots tab

	stateSpaces
	stateSpaceObjects
	stateCreateBucket

	stateReservedIPs
	stateCreateReservedIP
	stateAssignReservedIP // droplet picker for assign

	stateFirewalls
	stateFirewallDetails

	stateDomains
	stateCreateDomain
	stateDomainRecords
	stateCreateRecord

	stateAI

	stateAccount

	stateVPCs
	stateCreateVPC

	stateImages

	stateAlerts

	// extended droplet action forms
	stateDropletSnapName
	stateDropletResize
	stateDropletRename
	stateDropletRebuild

	// extended firewall flows
	stateCreateFirewall
	stateFirewallAddDroplets    // droplet picker for "add to firewall"
	stateFirewallRemoveDroplets // picker for "remove from firewall"
	stateFirewallAddRule        // form to add a single rule

	// images extensions
	stateImageTagFilter // single input: tag → ListByTag
	stateImageRename    // single input: new name
	stateImageTransfer  // single input: target region

	// monitoring
	stateCreateAlert      // create alert policy form
	stateDropletMetrics   // CPU sparkline overlay on droplet details

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
	actAttachVolume
	actDetachVolume
	actResizeVolume
	actCreateSnapshot
	actDeleteSnapshot

	actCreateReservedIP
	actDeleteReservedIP
	actAssignReservedIP
	actUnassignReservedIP

	actDeleteFirewall

	actCreateBucket
	actDeleteBucket
	actDeleteObject

	actCreateDomain
	actDeleteDomain
	actCreateDNSRecord
	actDeleteDNSRecord

	actCreateVPC
	actDeleteVPC

	actDeleteImage

	actDeleteAlertPolicy

	// extended droplet actions
	actPowerCycle
	actPasswordReset
	actEnableIPv6
	actEnablePrivateNet
	actEnableBackups
	actDisableBackups
	actSnapshotDroplet
	actResizeDroplet
	actRenameDroplet
	actRebuildDroplet

	// extended firewall ops
	actCreateFirewall
	actAddFirewallDroplets
	actRemoveFirewallDroplets
	actAddFirewallRules
	actRemoveFirewallRules

	// images extensions
	actRenameImage
	actTransferImage
	actConvertImage

	// monitoring
	actCreateAlertPolicy
)

type Options struct {
	DefaultRegion string
	DefaultSize   string
	DefaultImage  string
	DefaultTags   string // CSV
	DefaultIPv6   bool
}

// InferenceAPI is implemented by *inference.Client. Defined here to avoid import cycles.
type InferenceAPI interface {
	ListModels(context.Context) ([]InferenceModel, error)
	ChatCompletion(context.Context, InferenceChatReq) (string, int, int, error)
	Embed(context.Context, string, string) (int, error)
}

type InferenceModel struct {
	ID      string
	OwnedBy string
}

type InferenceChatReq struct {
	Model   string
	System  string
	User    string
	MaxToks int
}

// SpacesAPI is implemented by *spaces.Client. Defined here to avoid import cycles.
type SpacesAPI interface {
	ListBuckets(context.Context) ([]SpacesBucketRow, error)
	CreateBucket(context.Context, string) error
	DeleteBucket(context.Context, string) error
	ListObjects(context.Context, string, string) ([]SpacesObjectRow, error)
	DeleteObject(context.Context, string, string) error
}

type SpacesBucketRow struct {
	Name    string
	Created string
}

type SpacesObjectRow struct {
	Key          string
	SizeBytes    int64
	LastModified string
	StorageClass string
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
	GetVolume(context.Context, string) (*godo.Volume, error)
	AttachVolume(context.Context, string, int) error
	DetachVolume(context.Context, string, int) error
	ResizeVolume(context.Context, string, string, int64) error
	ListVolumeSnapshots(context.Context, string) ([]do.SnapshotRow, error)
	CreateVolumeSnapshot(context.Context, string, string) (*godo.Snapshot, error)
	DeleteSnapshot(context.Context, string) error

	// snapshots (all)
	ListAllSnapshots(context.Context) ([]do.SnapshotRow, error)

	// reserved IPs
	ListReservedIPs(context.Context) ([]do.ReservedIPRow, error)
	CreateReservedIP(context.Context, do.CreateReservedIPReq) (*godo.ReservedIP, error)
	DeleteReservedIP(context.Context, string) error
	AssignReservedIP(context.Context, string, int) error
	UnassignReservedIP(context.Context, string) error

	// firewalls
	ListFirewalls(context.Context) ([]do.FirewallRow, error)
	GetFirewall(context.Context, string) (*do.FirewallDetails, error)
	DeleteFirewall(context.Context, string) error

	// domains
	ListDomains(context.Context) ([]do.DomainRow, error)
	CreateDomain(context.Context, do.CreateDomainReq) (*godo.Domain, error)
	DeleteDomain(context.Context, string) error
	ListDomainRecords(context.Context, string) ([]do.DomainRecordRow, error)
	CreateDomainRecord(context.Context, do.CreateRecordReq) (*godo.DomainRecord, error)
	DeleteDomainRecord(context.Context, string, int) error

	// extended droplet actions
	PowerCycle(context.Context, int) error
	PasswordReset(context.Context, int) error
	EnableIPv6(context.Context, int) error
	EnablePrivateNetworking(context.Context, int) error
	EnableBackups(context.Context, int) error
	DisableBackups(context.Context, int) error
	SnapshotDroplet(context.Context, int, string) error
	ResizeDroplet(context.Context, int, string, bool) error
	RenameDroplet(context.Context, int, string) error
	RebuildDroplet(context.Context, int, string) error

	// extended firewall ops
	CreateFirewall(context.Context, do.CreateFirewallReq) (*godo.Firewall, error)
	AddFirewallDroplets(context.Context, string, ...int) error
	RemoveFirewallDroplets(context.Context, string, ...int) error
	AddFirewallTags(context.Context, string, ...string) error
	RemoveFirewallTags(context.Context, string, ...string) error
	AddFirewallRules(context.Context, string, []do.FirewallRuleSpec, []do.FirewallRuleSpec) error
	RemoveFirewallRules(context.Context, string, []do.FirewallRuleSpec, []do.FirewallRuleSpec) error

	// account & balance
	GetAccount(context.Context) (*do.AccountInfo, error)
	GetBalance(context.Context) (*do.BalanceInfo, error)

	// vpcs
	ListVPCs(context.Context) ([]do.VPCRow, error)
	CreateVPC(context.Context, do.CreateVPCReq) (*godo.VPC, error)
	DeleteVPC(context.Context, string) error

	// images
	ListUserImages(context.Context) ([]do.ImageRow, error)
	ListDistributionImages(context.Context) ([]do.ImageRow, error)
	ListApplicationImages(context.Context) ([]do.ImageRow, error)
	ListImagesByTag(context.Context, string) ([]do.ImageRow, error)
	UpdateImage(context.Context, int, string) error
	TransferImage(context.Context, int, string) error
	ConvertImage(context.Context, int) error
	DeleteImage(context.Context, int) error

	// alert policies
	ListAlertPolicies(context.Context) ([]do.AlertPolicyRow, error)
	CreateAlertPolicy(context.Context, do.CreateAlertPolicyReq) (*godo.AlertPolicy, error)
	DeleteAlertPolicy(context.Context, string) error

	// metrics
	GetDropletCPULastHour(context.Context, int) ([]do.MetricSample, error)
}

type keyMap struct {
	Up, Down, Enter, Back key.Binding
	Refresh               key.Binding

	TabDroplets  key.Binding
	TabVolumes   key.Binding
	TabOps       key.Binding
	TabSnapshots key.Binding
	TabIPs       key.Binding
	TabFirewalls key.Binding
	TabDomains   key.Binding
	TabSpaces    key.Binding
	TabAI        key.Binding
	TabAccount   key.Binding
	TabVPCs      key.Binding
	TabImages    key.Binding
	TabAlerts    key.Binding

	ToggleImages key.Binding

	Create key.Binding
	Delete key.Binding

	Details key.Binding

	Attach   key.Binding
	Detach   key.Binding
	Resize   key.Binding
	Snapshot key.Binding

	PowerOn, PowerOff, Shutdown, Reboot key.Binding

	PickSSH key.Binding

	Yes, No   key.Binding
	Quit      key.Binding
	ForceQuit key.Binding
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
		// Quit only fires when no text input has focus; ctrl+c always fires.
		Quit:      key.NewBinding(key.WithKeys("q"), key.WithHelp("q", "quit")),
		ForceQuit: key.NewBinding(key.WithKeys("ctrl+c"), key.WithHelp("ctrl+c", "force quit")),

		Refresh: key.NewBinding(key.WithKeys("r"), key.WithHelp("r", "refresh")),

		TabDroplets:  key.NewBinding(key.WithKeys("1"), key.WithHelp("1", "droplets")),
		TabVolumes:   key.NewBinding(key.WithKeys("2"), key.WithHelp("2", "volumes")),
		TabOps:       key.NewBinding(key.WithKeys("l"), key.WithHelp("l", "ops log")),
		TabSnapshots: key.NewBinding(key.WithKeys("3"), key.WithHelp("3", "snapshots")),
		TabIPs:       key.NewBinding(key.WithKeys("4"), key.WithHelp("4", "reserved IPs")),
		TabFirewalls: key.NewBinding(key.WithKeys("5"), key.WithHelp("5", "firewalls")),
		TabDomains:   key.NewBinding(key.WithKeys("6"), key.WithHelp("6", "domains")),
		TabSpaces:    key.NewBinding(key.WithKeys("7"), key.WithHelp("7", "spaces")),
		TabAI:        key.NewBinding(key.WithKeys("8"), key.WithHelp("8", "AI")),
		TabAccount:   key.NewBinding(key.WithKeys("9"), key.WithHelp("9", "account")),
		TabVPCs:      key.NewBinding(key.WithKeys("0"), key.WithHelp("0", "vpcs")),
		TabImages:    key.NewBinding(key.WithKeys("i"), key.WithHelp("i", "images")),
		TabAlerts:    key.NewBinding(key.WithKeys("m"), key.WithHelp("m", "alert policies")),

		ToggleImages: key.NewBinding(key.WithKeys("t"), key.WithHelp("t", "toggle user/distro")),

		Create: key.NewBinding(key.WithKeys("c"), key.WithHelp("c", "create")),
		Delete: key.NewBinding(key.WithKeys("d"), key.WithHelp("d", "delete")),

		Attach:   key.NewBinding(key.WithKeys("a"), key.WithHelp("a", "attach")),
		Detach:   key.NewBinding(key.WithKeys("x"), key.WithHelp("x", "detach")),
		Resize:   key.NewBinding(key.WithKeys("e"), key.WithHelp("e", "resize")),
		Snapshot: key.NewBinding(key.WithKeys("n"), key.WithHelp("n", "snapshot")),

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

	// volume details + snapshots
	volumeDetails *godo.Volume
	volSnapshots  []do.SnapshotRow
	volSnapTable  table.Model

	// attach droplet picker
	attachTable table.Model

	// single-field inputs
	resizeIn   textinput.Model
	snapNameIn textinput.Model

	// snapshots tab
	allSnapshots  []do.SnapshotRow
	snapshotTable table.Model

	// reserved IPs tab
	reservedIPRows    []do.ReservedIPRow
	reservedIPTable   table.Model
	assignTable       table.Model // droplet picker for assign
	createIPIn        textinput.Model
	pendingAssignIP   string
	pendingAssignDrop int
	pendingDeleteIP   string
	pendingUnassignIP string

	// firewalls tab
	firewallRows    []do.FirewallRow
	firewallTable   table.Model
	firewallDetails *do.FirewallDetails
	selectedFWID    string
	pendingDeleteFW string

	// inference / AI tab
	inferenceClient InferenceAPI // nil = not configured
	aiModels        []InferenceModel
	aiModelIdx      int
	aiPromptIn      textinput.Model
	aiSystemIn      textinput.Model
	aiResponse      string
	aiUsageInfo     string
	aiFocusField    int // 0=model selector, 1=system, 2=prompt
	aiPending       bool

	// spaces tab
	spacesClient        SpacesAPI // nil = not configured
	bucketRows          []SpacesBucketRow
	bucketTable         table.Model
	selectedBucket      string
	objectRows          []SpacesObjectRow
	objectTable         table.Model
	bucketNameIn        textinput.Model
	pendingDeleteBucket string
	pendingDeleteObjKey string

	// domains tab
	domainRows            []do.DomainRow
	domainTable           table.Model
	selectedDomain        string
	domainRecordRows      []do.DomainRecordRow
	domainRecordTable     table.Model
	pendingDeleteDomain   string
	pendingDeleteRecordID int
	// create domain form
	domainNameIn    textinput.Model
	domainIPIn      textinput.Model
	focusDomainForm int
	// create record form
	recTypeIn    textinput.Model
	recNameIn    textinput.Model
	recDataIn    textinput.Model
	recTTLIn     textinput.Model
	recPrioIn    textinput.Model
	focusRecForm int

	pendingAttachVolID  string
	pendingAttachDropID int
	pendingDetachVolID  string
	pendingDetachDropID int
	pendingResizeVolID  string
	pendingResizeRegion string
	pendingResizeGB     int64
	pendingSnapVolID    string
	pendingSnapName     string
	pendingDeleteSnapID string

	// generic confirm
	confirmText   string
	confirmReturn state
	pendingAct    actionKind

	pendingCreateDroplet *do.CreateDropletReq
	pendingCreateVolume  *do.CreateVolumeReq
	pendingDeleteVolID   string

	// account & balance
	accountInfo *do.AccountInfo
	balanceInfo *do.BalanceInfo

	// vpcs
	vpcRows         []do.VPCRow
	vpcTable        table.Model
	pendingDeleteVP string
	// create vpc form
	vpcNameIn    textinput.Model
	vpcRegionIn  textinput.Model
	vpcIPRangeIn textinput.Model
	vpcDescIn    textinput.Model
	focusVPCForm int

	// images
	imageRows      []do.ImageRow
	imageTable     table.Model
	imagesMode     string // "user" | "distribution" | "application" | "tag:<name>"
	imagesTag      string // remembered last filter tag
	pendingDelImg  int
	// extension: rename / transfer inputs + pending values
	imgRenameIn       textinput.Model
	imgTransferIn     textinput.Model
	imgTagFilterIn    textinput.Model
	pendingImgID      int
	pendingImgNewName string
	pendingImgRegion  string

	// alert policies
	alertRows      []do.AlertPolicyRow
	alertTable     table.Model
	pendingDelAlrt string
	alertForm      alertForm

	// droplet metrics overlay
	dropletMetrics []do.MetricSample

	// extended droplet action inputs (single-field forms)
	dropSnapNameIn textinput.Model
	dropResizeIn   textinput.Model
	dropResizeDisk bool // pending checkbox (toggled with space)
	dropRenameIn   textinput.Model
	dropRebuildIn  textinput.Model
	pendingDropletNewName string
	pendingDropletNewSize string
	pendingDropletRebuild string
	pendingDropletSnap    string

	// extended firewall flows
	fwNameIn         textinput.Model
	fwInRulesIn      textinput.Model // raw "tcp:22,tcp:80-90,udp:53" CSV
	fwOutRulesIn     textinput.Model // raw "tcp:all,udp:all"
	fwDropletIDsIn   textinput.Model // CSV ints
	fwTagsIn         textinput.Model // CSV
	focusFWForm      int
	fwPickerTable    table.Model
	fwPickerSelected map[int]bool
	fwAddRuleProto   textinput.Model
	fwAddRulePorts   textinput.Model
	fwAddRuleSrc     textinput.Model // CSV of CIDRs
	fwAddRuleDir     string          // "in" | "out"
	focusFWRule      int
	pendingFwInRules  []do.FirewallRuleSpec
	pendingFwOutRules []do.FirewallRuleSpec

	// misc
	errText string
	status  string
}

func NewModel(api api, opts Options, spacesClient SpacesAPI, inferenceClient InferenceAPI) Model {
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

	snapT := table.New(
		table.WithColumns([]table.Column{
			{Title: "Name", Width: 28},
			{Title: "GB", Width: 8},
			{Title: "Created", Width: 22},
			{Title: "ID", Width: 24},
		}),
		table.WithFocused(true),
		table.WithHeight(10),
	)

	attachT := table.New(table.WithColumns(dCols), table.WithFocused(true), table.WithHeight(14))

	snapAllT := table.New(
		table.WithColumns([]table.Column{
			{Title: "Name", Width: 28},
			{Title: "Type", Width: 8},
			{Title: "Region", Width: 8},
			{Title: "GB", Width: 8},
			{Title: "Created", Width: 22},
		}),
		table.WithFocused(true), table.WithHeight(14),
	)

	rIPT := table.New(
		table.WithColumns([]table.Column{
			{Title: "IP", Width: 18},
			{Title: "Region", Width: 8},
			{Title: "Droplet", Width: 24},
		}),
		table.WithFocused(true), table.WithHeight(14),
	)

	assignT := table.New(table.WithColumns(dCols), table.WithFocused(true), table.WithHeight(14))

	fwT := table.New(
		table.WithColumns([]table.Column{
			{Title: "Name", Width: 26},
			{Title: "Status", Width: 10},
			{Title: "Droplets", Width: 9},
			{Title: "In", Width: 5},
			{Title: "Out", Width: 5},
			{Title: "ID", Width: 24},
		}),
		table.WithFocused(true), table.WithHeight(14),
	)

	domT := table.New(
		table.WithColumns([]table.Column{
			{Title: "Domain", Width: 36},
			{Title: "TTL", Width: 8},
		}),
		table.WithFocused(true), table.WithHeight(14),
	)

	recT := table.New(
		table.WithColumns([]table.Column{
			{Title: "Type", Width: 7},
			{Title: "Name", Width: 28},
			{Title: "Data", Width: 32},
			{Title: "TTL", Width: 7},
		}),
		table.WithFocused(true), table.WithHeight(12),
	)

	bucketT := table.New(
		table.WithColumns([]table.Column{
			{Title: "Bucket", Width: 40},
			{Title: "Created", Width: 22},
		}),
		table.WithFocused(true), table.WithHeight(14),
	)

	vpcT := table.New(
		table.WithColumns([]table.Column{
			{Title: "Name", Width: 24},
			{Title: "Region", Width: 8},
			{Title: "IP Range", Width: 20},
			{Title: "Default", Width: 8},
			{Title: "ID", Width: 36},
		}),
		table.WithFocused(true), table.WithHeight(14),
	)

	imgT := table.New(
		table.WithColumns([]table.Column{
			{Title: "ID", Width: 10},
			{Title: "Name", Width: 30},
			{Title: "Distribution", Width: 14},
			{Title: "Slug", Width: 22},
			{Title: "Min GB", Width: 7},
			{Title: "Regions", Width: 18},
		}),
		table.WithFocused(true), table.WithHeight(14),
	)

	alertT := table.New(
		table.WithColumns([]table.Column{
			{Title: "Type", Width: 26},
			{Title: "Cmp", Width: 11},
			{Title: "Value", Width: 8},
			{Title: "Win", Width: 6},
			{Title: "On", Width: 3},
			{Title: "Description", Width: 30},
		}),
		table.WithFocused(true), table.WithHeight(14),
	)

	objectT := table.New(
		table.WithColumns([]table.Column{
			{Title: "Key", Width: 42},
			{Title: "Size", Width: 12},
			{Title: "Modified", Width: 22},
		}),
		table.WithFocused(true), table.WithHeight(14),
	)

	m := Model{
		api:               api,
		ctx:               context.Background(),
		keys:              defaultKeys(),
		help:              help.New(),
		opts:              opts,
		spinner:           sp,
		st:                stateDroplets,
		dropletTable:      dt,
		volumeTable:       vt,
		opsTable:          ot,
		sshTable:          sshT,
		volSnapTable:      snapT,
		attachTable:       attachT,
		snapshotTable:     snapAllT,
		reservedIPTable:   rIPT,
		assignTable:       assignT,
		firewallTable:     fwT,
		domainTable:       domT,
		domainRecordTable: recT,
		spacesClient:      spacesClient,
		inferenceClient:   inferenceClient,
		bucketTable:       bucketT,
		objectTable:       objectT,
		vpcTable:          vpcT,
		imageTable:        imgT,
		alertTable:        alertT,
		imagesMode:        "user",
		sshSelected:       map[int]bool{},
		fwPickerTable:     table.New(table.WithColumns(dCols), table.WithFocused(true), table.WithHeight(14)),
		fwPickerSelected:  map[int]bool{},
		status:            "Press r to refresh (droplets tab)",
	}

	m.initDropletCreateForm()
	m.initVolumeCreateForm()

	return m
}

func (m Model) Init() tea.Cmd {
	return tea.Batch(m.spinner.Tick, m.refreshDropletsCmd())
}

type apiErrMsg struct{ err error }
type dropletsLoadedMsg struct{ rows []do.DropletRow }
type dropletDetailsMsg struct{ d *godo.Droplet }
type volumesLoadedMsg struct{ rows []do.VolumeRow }
type sshKeysLoadedMsg struct{ keys []do.SSHKeyRow }
type volumeDetailsMsg struct {
	v     *godo.Volume
	snaps []do.SnapshotRow
}
type allSnapshotsLoadedMsg struct{ rows []do.SnapshotRow }
type reservedIPsLoadedMsg struct{ rows []do.ReservedIPRow }
type firewallsLoadedMsg struct{ rows []do.FirewallRow }
type firewallDetailsMsg struct{ details *do.FirewallDetails }
type domainsLoadedMsg struct{ rows []do.DomainRow }
type domainRecordsLoadedMsg struct {
	domain string
	rows   []do.DomainRecordRow
}
type aiModelsLoadedMsg struct{ models []InferenceModel }
type aiResponseMsg struct {
	text      string
	usageInfo string
}
type accountLoadedMsg struct {
	acc *do.AccountInfo
	bal *do.BalanceInfo
}
type vpcsLoadedMsg struct{ rows []do.VPCRow }
type imagesLoadedMsg struct {
	rows []do.ImageRow
	mode string // "user" | "distribution" | "application" | "tag:<name>"
}
type alertsLoadedMsg struct{ rows []do.AlertPolicyRow }
type dropletMetricsLoadedMsg struct{ samples []do.MetricSample }
type bucketsLoadedMsg struct{ rows []SpacesBucketRow }
type objectsLoadedMsg struct {
	bucket string
	rows   []SpacesObjectRow
}

type apiDoneMsg struct {
	act    actionKind
	status string
	target string
}

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
		m.attachTable.SetHeight(h)
		m.assignTable.SetHeight(h)
		m.volSnapTable.SetHeight(max(6, h-8))
		m.snapshotTable.SetHeight(h)
		m.reservedIPTable.SetHeight(h)
		m.firewallTable.SetHeight(h)
		m.domainTable.SetHeight(h)
		m.domainRecordTable.SetHeight(max(6, h-10))
		m.bucketTable.SetHeight(h)
		m.objectTable.SetHeight(h)
		m.vpcTable.SetHeight(h)
		m.imageTable.SetHeight(h)
		m.alertTable.SetHeight(h)
		m.fwPickerTable.SetHeight(h)

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

	case volumeDetailsMsg:
		m.busy = false
		m.errText = ""
		m.volumeDetails = msg.v
		m.volSnapshots = msg.snaps
		m.volSnapTable.SetRows(toSnapshotRows(m.volSnapshots))
		m.status = fmt.Sprintf("Volume details loaded (%d snapshot(s))", len(m.volSnapshots))
		m.st = stateVolumeDetails

	case sshKeysLoadedMsg:
		m.busy = false
		m.errText = ""
		m.sshKeys = msg.keys
		m.sshTable.SetRows(toSSHTableRows(m.sshKeys, m.sshSelected))
		m.status = fmt.Sprintf("Loaded %d SSH key(s)", len(m.sshKeys))

	case allSnapshotsLoadedMsg:
		m.busy = false
		m.errText = ""
		m.allSnapshots = msg.rows
		m.snapshotTable.SetRows(toAllSnapshotRows(m.allSnapshots))
		m.status = fmt.Sprintf("Loaded %d snapshot(s)", len(m.allSnapshots))

	case reservedIPsLoadedMsg:
		m.busy = false
		m.errText = ""
		m.reservedIPRows = msg.rows
		m.reservedIPTable.SetRows(toReservedIPRows(m.reservedIPRows))
		m.status = fmt.Sprintf("Loaded %d reserved IP(s)", len(m.reservedIPRows))

	case firewallsLoadedMsg:
		m.busy = false
		m.errText = ""
		m.firewallRows = msg.rows
		m.firewallTable.SetRows(toFirewallRows(m.firewallRows))
		m.status = fmt.Sprintf("Loaded %d firewall(s)", len(m.firewallRows))

	case firewallDetailsMsg:
		m.busy = false
		m.errText = ""
		m.firewallDetails = msg.details
		m.status = "Firewall details loaded"
		m.st = stateFirewallDetails

	case domainsLoadedMsg:
		m.busy = false
		m.errText = ""
		m.domainRows = msg.rows
		m.domainTable.SetRows(toDomainRows(m.domainRows))
		m.status = fmt.Sprintf("Loaded %d domain(s)", len(m.domainRows))

	case domainRecordsLoadedMsg:
		m.busy = false
		m.errText = ""
		m.selectedDomain = msg.domain
		m.domainRecordRows = msg.rows
		m.domainRecordTable.SetRows(toDomainRecordRows(m.domainRecordRows))
		m.status = fmt.Sprintf("Loaded %d record(s) for %s", len(m.domainRecordRows), msg.domain)
		m.st = stateDomainRecords

	case aiModelsLoadedMsg:
		m.busy = false
		m.errText = ""
		m.aiModels = msg.models
		m.status = fmt.Sprintf("Loaded %d model(s)", len(m.aiModels))

	case aiResponseMsg:
		m.busy = false
		m.errText = ""
		m.aiPending = false
		m.aiResponse = msg.text
		m.aiUsageInfo = msg.usageInfo
		m.status = "Response received"

	case bucketsLoadedMsg:
		m.busy = false
		m.errText = ""
		m.bucketRows = msg.rows
		m.bucketTable.SetRows(toBucketRows(m.bucketRows))
		m.status = fmt.Sprintf("Loaded %d bucket(s)", len(m.bucketRows))

	case objectsLoadedMsg:
		m.busy = false
		m.errText = ""
		m.selectedBucket = msg.bucket
		m.objectRows = msg.rows
		m.objectTable.SetRows(toObjectRows(m.objectRows))
		m.status = fmt.Sprintf("Loaded %d object(s) in %s", len(m.objectRows), msg.bucket)
		m.st = stateSpaceObjects

	case accountLoadedMsg:
		m.busy = false
		m.errText = ""
		m.accountInfo = msg.acc
		m.balanceInfo = msg.bal
		m.status = "Account loaded"

	case vpcsLoadedMsg:
		m.busy = false
		m.errText = ""
		m.vpcRows = msg.rows
		m.vpcTable.SetRows(toVPCRows(m.vpcRows))
		m.status = fmt.Sprintf("Loaded %d VPC(s)", len(m.vpcRows))

	case imagesLoadedMsg:
		m.busy = false
		m.errText = ""
		m.imageRows = msg.rows
		m.imagesMode = msg.mode
		m.imageTable.SetRows(toImageRows(m.imageRows))
		m.status = fmt.Sprintf("Loaded %d %s image(s)", len(m.imageRows), msg.mode)

	case alertsLoadedMsg:
		m.busy = false
		m.errText = ""
		m.alertRows = msg.rows
		m.alertTable.SetRows(toAlertRows(m.alertRows))
		m.status = fmt.Sprintf("Loaded %d alert policy(ies)", len(m.alertRows))

	case dropletMetricsLoadedMsg:
		m.busy = false
		m.errText = ""
		m.dropletMetrics = msg.samples
		m.status = fmt.Sprintf("Loaded %d CPU sample(s)", len(m.dropletMetrics))
		m.st = stateDropletMetrics

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

		case actCreateVolume, actDeleteVolume, actAttachVolume, actDetachVolume, actResizeVolume:
			m.st = stateVolumes
			m.selectedVolumeID = ""

		case actCreateSnapshot, actDeleteSnapshot:
			m.st = stateVolumeDetails

		case actCreateReservedIP, actDeleteReservedIP, actAssignReservedIP, actUnassignReservedIP:
			m.st = stateReservedIPs

		case actDeleteFirewall:
			m.st = stateFirewalls

		case actCreateBucket, actDeleteBucket:
			m.st = stateSpaces

		case actDeleteObject:
			m.st = stateSpaceObjects

		case actCreateDomain, actDeleteDomain:
			m.st = stateDomains

		case actCreateDNSRecord, actDeleteDNSRecord:
			m.st = stateDomainRecords

		case actCreateVPC, actDeleteVPC:
			m.st = stateVPCs

		case actDeleteImage:
			m.st = stateImages

		case actDeleteAlertPolicy:
			m.st = stateAlerts

		case actPowerCycle, actPasswordReset, actEnableIPv6, actEnablePrivateNet,
			actEnableBackups, actDisableBackups,
			actSnapshotDroplet, actResizeDroplet, actRenameDroplet, actRebuildDroplet:
			// Return to droplet details so the user sees the updated state.
			m.st = stateDetails

		case actCreateFirewall, actAddFirewallDroplets, actRemoveFirewallDroplets,
			actAddFirewallRules, actRemoveFirewallRules:
			m.st = stateFirewalls

		case actRenameImage, actTransferImage, actConvertImage:
			m.st = stateImages

		case actCreateAlertPolicy:
			m.st = stateAlerts
		}

		// refresh the active tab after changes
		switch {
		case msg.act == actCreateDroplet || msg.act == actDeleteDroplet || msg.act == actPowerOn || msg.act == actPowerOff || msg.act == actShutdown || msg.act == actReboot:
			cmds = append(cmds, m.refreshDropletsCmd())
		case msg.act == actCreateVolume || msg.act == actDeleteVolume || msg.act == actAttachVolume || msg.act == actDetachVolume || msg.act == actResizeVolume:
			cmds = append(cmds, m.refreshVolumesCmd())
		case msg.act == actCreateReservedIP || msg.act == actDeleteReservedIP || msg.act == actAssignReservedIP || msg.act == actUnassignReservedIP:
			cmds = append(cmds, m.refreshReservedIPsCmd())
		case msg.act == actDeleteFirewall:
			cmds = append(cmds, m.refreshFirewallsCmd())
		case msg.act == actCreateBucket || msg.act == actDeleteBucket:
			cmds = append(cmds, m.refreshBucketsCmd())
		case msg.act == actDeleteObject:
			if m.selectedBucket != "" {
				cmds = append(cmds, m.loadObjectsCmd(m.selectedBucket))
			}
		case msg.act == actCreateDomain || msg.act == actDeleteDomain:
			cmds = append(cmds, m.refreshDomainsCmd())
		case msg.act == actCreateDNSRecord || msg.act == actDeleteDNSRecord:
			if m.selectedDomain != "" {
				cmds = append(cmds, m.loadDomainRecordsCmd(m.selectedDomain))
			}
		case msg.act == actCreateSnapshot || msg.act == actDeleteSnapshot:
			if m.volumeDetails != nil {
				cmds = append(cmds, m.loadVolumeDetailsCmd(m.volumeDetails.ID))
			}
		case msg.act == actCreateVPC || msg.act == actDeleteVPC:
			cmds = append(cmds, m.refreshVPCsCmd())
		case msg.act == actDeleteImage:
			cmds = append(cmds, m.refreshImagesCmd(m.imagesMode))
		case msg.act == actDeleteAlertPolicy:
			cmds = append(cmds, m.refreshAlertsCmd())
		case msg.act == actPowerCycle || msg.act == actPasswordReset ||
			msg.act == actEnableIPv6 || msg.act == actEnablePrivateNet ||
			msg.act == actEnableBackups || msg.act == actDisableBackups ||
			msg.act == actSnapshotDroplet || msg.act == actResizeDroplet ||
			msg.act == actRenameDroplet || msg.act == actRebuildDroplet:
			// Refresh both list (rename/resize changes columns) and details.
			cmds = append(cmds, m.refreshDropletsCmd())
			if m.selectedDropletID != 0 {
				cmds = append(cmds, m.loadDetailsCmd(m.selectedDropletID))
			}
		case msg.act == actCreateFirewall || msg.act == actAddFirewallDroplets ||
			msg.act == actRemoveFirewallDroplets || msg.act == actAddFirewallRules ||
			msg.act == actRemoveFirewallRules:
			cmds = append(cmds, m.refreshFirewallsCmd())
		case msg.act == actRenameImage || msg.act == actTransferImage || msg.act == actConvertImage:
			cmds = append(cmds, m.refreshImagesCmd(m.imagesMode))
		case msg.act == actCreateAlertPolicy:
			cmds = append(cmds, m.refreshAlertsCmd())
		}

	case tea.KeyMsg:
		// ctrl+c always exits. Plain `q` only exits when no text-input
		// state is active — otherwise typing a name like "queue" or "qa-1"
		// would terminate the program.
		if key.Matches(msg, m.keys.ForceQuit) {
			return m, tea.Quit
		}
		if !inTextInputState(m.st) && key.Matches(msg, m.keys.Quit) {
			return m, tea.Quit
		}

		// global tab switching (except confirm/modal screens)
		tabSwitched := false
		if m.canSwitchTabNow() {
			switch {
			case key.Matches(msg, m.keys.TabDroplets):
				m.st = stateDroplets
				m.status = "Droplets"
				tabSwitched = true
			case key.Matches(msg, m.keys.TabVolumes):
				m.st = stateVolumes
				m.status = "Volumes"
				if len(m.volumeRows) == 0 {
					cmds = append(cmds, m.refreshVolumesCmd())
				}
				tabSwitched = true
			case key.Matches(msg, m.keys.TabOps):
				m.st = stateOpsLog
				m.opsTable.SetRows(toOpsRows(m.ops))
				m.status = "Ops log"
				tabSwitched = true
			case key.Matches(msg, m.keys.TabSnapshots):
				m.st = stateSnapshots
				m.status = "Snapshots"
				if len(m.allSnapshots) == 0 {
					cmds = append(cmds, m.refreshAllSnapshotsCmd())
				}
				tabSwitched = true
			case key.Matches(msg, m.keys.TabIPs):
				m.st = stateReservedIPs
				m.status = "Reserved IPs"
				if len(m.reservedIPRows) == 0 {
					cmds = append(cmds, m.refreshReservedIPsCmd())
				}
				tabSwitched = true
			case key.Matches(msg, m.keys.TabFirewalls):
				m.st = stateFirewalls
				m.status = "Firewalls"
				if len(m.firewallRows) == 0 {
					cmds = append(cmds, m.refreshFirewallsCmd())
				}
				tabSwitched = true
			case key.Matches(msg, m.keys.TabDomains):
				m.st = stateDomains
				m.status = "Domains"
				if len(m.domainRows) == 0 {
					cmds = append(cmds, m.refreshDomainsCmd())
				}
				tabSwitched = true
			case key.Matches(msg, m.keys.TabSpaces):
				m.st = stateSpaces
				m.status = "Spaces"
				if len(m.bucketRows) == 0 && m.spacesClient != nil {
					cmds = append(cmds, m.refreshBucketsCmd())
				}
				tabSwitched = true
			case key.Matches(msg, m.keys.TabAI):
				m.st = stateAI
				m.status = "AI Inference"
				if len(m.aiModels) == 0 && m.inferenceClient != nil {
					cmds = append(cmds, m.loadAIModelsCmd())
				}
				tabSwitched = true
			case key.Matches(msg, m.keys.TabAccount):
				m.st = stateAccount
				m.status = "Account"
				if m.accountInfo == nil {
					cmds = append(cmds, m.refreshAccountCmd())
				}
				tabSwitched = true
			case key.Matches(msg, m.keys.TabVPCs):
				m.st = stateVPCs
				m.status = "VPCs"
				if len(m.vpcRows) == 0 {
					cmds = append(cmds, m.refreshVPCsCmd())
				}
				tabSwitched = true
			case key.Matches(msg, m.keys.TabImages):
				m.st = stateImages
				m.status = "Images"
				if len(m.imageRows) == 0 {
					cmds = append(cmds, m.refreshImagesCmd(m.imagesMode))
				}
				tabSwitched = true
			case key.Matches(msg, m.keys.TabAlerts):
				m.st = stateAlerts
				m.status = "Alert Policies"
				if len(m.alertRows) == 0 {
					cmds = append(cmds, m.refreshAlertsCmd())
				}
				tabSwitched = true
			}
		}
		if tabSwitched {
			return m, tea.Batch(cmds...)
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
		case stateVolumeDetails:
			m, cmds = m.updateVolumeDetails(msg, cmds)
		case stateAttachVolume:
			m, cmds = m.updateAttachVolume(msg, cmds)
		case stateResizeVolume:
			m, cmds = m.updateResizeVolume(msg, cmds)
		case stateSnapshotName:
			m, cmds = m.updateSnapshotName(msg, cmds)
		case stateSnapshots:
			m, cmds = m.updateSnapshots(msg, cmds)
		case stateReservedIPs:
			m, cmds = m.updateReservedIPs(msg, cmds)
		case stateCreateReservedIP:
			m, cmds = m.updateCreateReservedIP(msg, cmds)
		case stateAssignReservedIP:
			m, cmds = m.updateAssignReservedIP(msg, cmds)
		case stateFirewalls:
			m, cmds = m.updateFirewalls(msg, cmds)
		case stateFirewallDetails:
			m, cmds = m.updateFirewallDetails(msg, cmds)
		case stateDomains:
			m, cmds = m.updateDomains(msg, cmds)
		case stateCreateDomain:
			m, cmds = m.updateCreateDomain(msg, cmds)
		case stateDomainRecords:
			m, cmds = m.updateDomainRecords(msg, cmds)
		case stateCreateRecord:
			m, cmds = m.updateCreateRecord(msg, cmds)
		case stateSpaces:
			m, cmds = m.updateSpaces(msg, cmds)
		case stateSpaceObjects:
			m, cmds = m.updateSpaceObjects(msg, cmds)
		case stateCreateBucket:
			m, cmds = m.updateCreateBucket(msg, cmds)
		case stateAI:
			m, cmds = m.updateAI(msg, cmds)
		case stateAccount:
			m, cmds = m.updateAccount(msg, cmds)
		case stateVPCs:
			m, cmds = m.updateVPCs(msg, cmds)
		case stateCreateVPC:
			m, cmds = m.updateCreateVPC(msg, cmds)
		case stateImages:
			m, cmds = m.updateImages(msg, cmds)
		case stateAlerts:
			m, cmds = m.updateAlerts(msg, cmds)
		case stateDropletSnapName:
			m, cmds = m.updateDropletSnapName(msg, cmds)
		case stateDropletResize:
			m, cmds = m.updateDropletResize(msg, cmds)
		case stateDropletRename:
			m, cmds = m.updateDropletRename(msg, cmds)
		case stateDropletRebuild:
			m, cmds = m.updateDropletRebuild(msg, cmds)
		case stateCreateFirewall:
			m, cmds = m.updateCreateFirewall(msg, cmds)
		case stateFirewallAddDroplets:
			m, cmds = m.updateFirewallPicker(msg, cmds, true)
		case stateFirewallRemoveDroplets:
			m, cmds = m.updateFirewallPicker(msg, cmds, false)
		case stateFirewallAddRule:
			m, cmds = m.updateFirewallAddRule(msg, cmds)
		case stateImageTagFilter:
			m, cmds = m.updateImageTagFilter(msg, cmds)
		case stateImageRename:
			m, cmds = m.updateImageRename(msg, cmds)
		case stateImageTransfer:
			m, cmds = m.updateImageTransfer(msg, cmds)
		case stateCreateAlert:
			m, cmds = m.updateCreateAlert(msg, cmds)
		case stateDropletMetrics:
			m, cmds = m.updateDropletMetrics(msg, cmds)
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
	case stateAttachVolume:
		var cmd tea.Cmd
		m.attachTable, cmd = m.attachTable.Update(msg)
		cmds = append(cmds, cmd)
	case stateAssignReservedIP:
		var cmd tea.Cmd
		m.assignTable, cmd = m.assignTable.Update(msg)
		cmds = append(cmds, cmd)
	case stateSnapshots:
		var cmd tea.Cmd
		m.snapshotTable, cmd = m.snapshotTable.Update(msg)
		cmds = append(cmds, cmd)
	case stateReservedIPs:
		var cmd tea.Cmd
		m.reservedIPTable, cmd = m.reservedIPTable.Update(msg)
		cmds = append(cmds, cmd)
	case stateFirewalls:
		var cmd tea.Cmd
		m.firewallTable, cmd = m.firewallTable.Update(msg)
		cmds = append(cmds, cmd)
	case stateDomains:
		var cmd tea.Cmd
		m.domainTable, cmd = m.domainTable.Update(msg)
		cmds = append(cmds, cmd)
	case stateDomainRecords:
		var cmd tea.Cmd
		m.domainRecordTable, cmd = m.domainRecordTable.Update(msg)
		cmds = append(cmds, cmd)
	case stateSpaces:
		var cmd tea.Cmd
		m.bucketTable, cmd = m.bucketTable.Update(msg)
		cmds = append(cmds, cmd)
	case stateSpaceObjects:
		var cmd tea.Cmd
		m.objectTable, cmd = m.objectTable.Update(msg)
		cmds = append(cmds, cmd)
	case stateVolumeDetails:
		var cmd tea.Cmd
		m.volSnapTable, cmd = m.volSnapTable.Update(msg)
		cmds = append(cmds, cmd)
	case stateVPCs:
		var cmd tea.Cmd
		m.vpcTable, cmd = m.vpcTable.Update(msg)
		cmds = append(cmds, cmd)
	case stateImages:
		var cmd tea.Cmd
		m.imageTable, cmd = m.imageTable.Update(msg)
		cmds = append(cmds, cmd)
	case stateAlerts:
		var cmd tea.Cmd
		m.alertTable, cmd = m.alertTable.Update(msg)
		cmds = append(cmds, cmd)
	case stateFirewallAddDroplets, stateFirewallRemoveDroplets:
		var cmd tea.Cmd
		m.fwPickerTable, cmd = m.fwPickerTable.Update(msg)
		cmds = append(cmds, cmd)
	}

	return m, tea.Batch(cmds...)
}

func (m Model) View() string {
	title := lipgloss.NewStyle().Bold(true).Render("DO TUI  |  1=Drop 2=Vol 3=Snap 4=IPs 5=FW 6=Dom 7=Spaces 8=AI 9=Acct 0=VPCs i=Img m=Alerts l=Ops")
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
	case stateVolumeDetails:
		return top + m.viewVolumeDetails()
	case stateAttachVolume:
		return top + m.viewAttachVolume()
	case stateResizeVolume:
		return top + m.viewResizeVolume()
	case stateSnapshotName:
		return top + m.viewSnapshotName()
	case stateSnapshots:
		return top + m.viewSnapshots()
	case stateReservedIPs:
		return top + m.viewReservedIPs()
	case stateCreateReservedIP:
		return top + m.viewCreateReservedIP()
	case stateAssignReservedIP:
		return top + m.viewAssignReservedIP()
	case stateFirewalls:
		return top + m.viewFirewalls()
	case stateFirewallDetails:
		return top + m.viewFirewallDetails()
	case stateDomains:
		return top + m.viewDomains()
	case stateCreateDomain:
		return top + m.viewCreateDomain()
	case stateDomainRecords:
		return top + m.viewDomainRecords()
	case stateCreateRecord:
		return top + m.viewCreateRecord()
	case stateSpaces:
		return top + m.viewSpaces()
	case stateSpaceObjects:
		return top + m.viewSpaceObjects()
	case stateCreateBucket:
		return top + m.viewCreateBucket()
	case stateAI:
		return top + m.viewAI()
	case stateAccount:
		return top + m.viewAccount()
	case stateVPCs:
		return top + m.viewVPCs()
	case stateCreateVPC:
		return top + m.viewCreateVPC()
	case stateImages:
		return top + m.viewImages()
	case stateAlerts:
		return top + m.viewAlerts()
	case stateDropletSnapName:
		return top + m.viewDropletSnapName()
	case stateDropletResize:
		return top + m.viewDropletResize()
	case stateDropletRename:
		return top + m.viewDropletRename()
	case stateDropletRebuild:
		return top + m.viewDropletRebuild()
	case stateCreateFirewall:
		return top + m.viewCreateFirewall()
	case stateFirewallAddDroplets:
		return top + m.viewFirewallPicker(true)
	case stateFirewallRemoveDroplets:
		return top + m.viewFirewallPicker(false)
	case stateFirewallAddRule:
		return top + m.viewFirewallAddRule()
	case stateImageTagFilter:
		return top + m.viewImageTagFilter()
	case stateImageRename:
		return top + m.viewImageRename()
	case stateImageTransfer:
		return top + m.viewImageTransfer()
	case stateCreateAlert:
		return top + m.viewCreateAlert()
	case stateDropletMetrics:
		return top + m.viewDropletMetrics()
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
	legend := lipgloss.NewStyle().Faint(true).Render("Keys: r refresh | c create | d delete | enter details | a attach | x detach | e resize | q quit")
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
	b.WriteString(lipgloss.NewStyle().Faint(true).Render(
		"Keys: esc back | r refresh | o/p/s/b power | d delete | "+
			"S snap | E resize | B rebuild | M rename | Y power-cycle | "+
			"U backups | I ipv6 | V priv-net | W pw-reset | G graph CPU"))
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
	default:
		// Extended actions (capital-letter keys) on the details screen.
		if mm, cc, handled := m.updateDetailsExt(k, cmds); handled {
			return mm, cc
		}
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

	case key.Matches(k, m.keys.Enter):
		if v, ok := m.currentSelectedVolume(); ok {
			m.selectedVolumeID = v.ID
			cmds = append(cmds, m.loadVolumeDetailsCmd(v.ID))
		}

	case key.Matches(k, m.keys.Attach):
		if v, ok := m.currentSelectedVolume(); ok {
			m.pendingAttachVolID = v.ID
			m.st = stateAttachVolume
			if len(m.dropletRows) == 0 {
				cmds = append(cmds, m.refreshDropletsCmd())
			} else {
				m.attachTable.SetRows(toDropletTableRows(m.dropletRows))
			}
		}

	case key.Matches(k, m.keys.Detach):
		if v, ok := m.currentSelectedVolume(); ok {
			if len(v.DropletIDs) == 0 {
				m.errText = "volume is not attached to any droplet"
				return m, cmds
			}
			m.pendingDetachVolID = v.ID
			m.pendingDetachDropID = v.DropletIDs[0]
			m.pendingAct = actDetachVolume
			m.confirmReturn = stateVolumes
			m.confirmText = fmt.Sprintf("Detach volume %s from droplet %d?", v.ID, v.DropletIDs[0])
			m.st = stateConfirm
		}

	case key.Matches(k, m.keys.Resize):
		if v, ok := m.currentSelectedVolume(); ok {
			m.pendingResizeVolID = v.ID
			m.pendingResizeRegion = v.Region
			m.resizeIn = newInput("New size GB", strconv.FormatInt(v.SizeGB, 10))
			m.resizeIn.SetValue(strconv.FormatInt(v.SizeGB, 10))
			m.resizeIn.Focus()
			m.st = stateResizeVolume
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

		case actAttachVolume:
			cmds = append(cmds, m.attachVolumeCmd(m.pendingAttachVolID, m.pendingAttachDropID))
			return m, cmds

		case actDetachVolume:
			cmds = append(cmds, m.detachVolumeCmd(m.pendingDetachVolID, m.pendingDetachDropID))
			return m, cmds

		case actResizeVolume:
			cmds = append(cmds, m.resizeVolumeCmd(m.pendingResizeVolID, m.pendingResizeRegion, m.pendingResizeGB))
			return m, cmds

		case actCreateSnapshot:
			cmds = append(cmds, m.createSnapshotCmd(m.pendingSnapVolID, m.pendingSnapName))
			return m, cmds

		case actDeleteSnapshot:
			id := m.pendingDeleteSnapID
			m.pendingDeleteSnapID = ""
			cmds = append(cmds, m.deleteSnapshotCmd(id))
			return m, cmds

		case actCreateReservedIP:
			cmds = append(cmds, m.createReservedIPCmd())
			return m, cmds
		case actDeleteReservedIP:
			ip := m.pendingDeleteIP
			m.pendingDeleteIP = ""
			cmds = append(cmds, m.deleteReservedIPCmd(ip))
			return m, cmds
		case actAssignReservedIP:
			cmds = append(cmds, m.assignReservedIPCmd(m.pendingAssignIP, m.pendingAssignDrop))
			return m, cmds
		case actUnassignReservedIP:
			ip := m.pendingUnassignIP
			m.pendingUnassignIP = ""
			cmds = append(cmds, m.unassignReservedIPCmd(ip))
			return m, cmds

		case actDeleteFirewall:
			id := m.pendingDeleteFW
			m.pendingDeleteFW = ""
			cmds = append(cmds, m.deleteFirewallCmd(id))
			return m, cmds

		case actCreateDomain:
			cmds = append(cmds, m.createDomainCmd())
			return m, cmds
		case actDeleteDomain:
			name := m.pendingDeleteDomain
			m.pendingDeleteDomain = ""
			cmds = append(cmds, m.deleteDomainCmd(name))
			return m, cmds
		case actCreateDNSRecord:
			cmds = append(cmds, m.createDNSRecordCmd())
			return m, cmds
		case actDeleteDNSRecord:
			id := m.pendingDeleteRecordID
			m.pendingDeleteRecordID = 0
			cmds = append(cmds, m.deleteDNSRecordCmd(m.selectedDomain, id))
			return m, cmds

		case actCreateBucket:
			cmds = append(cmds, m.createBucketCmd())
			return m, cmds
		case actDeleteBucket:
			name := m.pendingDeleteBucket
			m.pendingDeleteBucket = ""
			cmds = append(cmds, m.deleteBucketCmd(name))
			return m, cmds
		case actDeleteObject:
			k := m.pendingDeleteObjKey
			m.pendingDeleteObjKey = ""
			cmds = append(cmds, m.deleteObjectCmd(m.selectedBucket, k))
			return m, cmds

		case actCreateVPC:
			cmds = append(cmds, m.createVPCCmd())
			return m, cmds
		case actDeleteVPC:
			id := m.pendingDeleteVP
			m.pendingDeleteVP = ""
			cmds = append(cmds, m.deleteVPCCmd(id))
			return m, cmds

		case actDeleteImage:
			id := m.pendingDelImg
			m.pendingDelImg = 0
			cmds = append(cmds, m.deleteImageCmd(id))
			return m, cmds

		case actDeleteAlertPolicy:
			id := m.pendingDelAlrt
			m.pendingDelAlrt = ""
			cmds = append(cmds, m.deleteAlertPolicyCmd(id))
			return m, cmds

		case actPowerCycle, actPasswordReset, actEnableIPv6, actEnablePrivateNet,
			actEnableBackups, actDisableBackups,
			actSnapshotDroplet, actResizeDroplet, actRenameDroplet, actRebuildDroplet:
			cmds = append(cmds, m.runExtDropletActionCmd(m.pendingAct))
			return m, cmds

		case actCreateFirewall:
			cmds = append(cmds, m.createFirewallCmd())
			return m, cmds
		case actAddFirewallDroplets:
			cmds = append(cmds, m.modifyFirewallDropletsCmd(true))
			return m, cmds
		case actRemoveFirewallDroplets:
			cmds = append(cmds, m.modifyFirewallDropletsCmd(false))
			return m, cmds
		case actAddFirewallRules:
			cmds = append(cmds, m.modifyFirewallRulesCmd(true))
			return m, cmds
		case actRemoveFirewallRules:
			cmds = append(cmds, m.modifyFirewallRulesCmd(false))
			return m, cmds

		case actRenameImage:
			cmds = append(cmds, m.renameImageCmd())
			return m, cmds
		case actTransferImage:
			cmds = append(cmds, m.transferImageCmd())
			return m, cmds
		case actConvertImage:
			cmds = append(cmds, m.convertImageCmd())
			return m, cmds

		case actCreateAlertPolicy:
			cmds = append(cmds, m.createAlertCmd())
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
	case actAttachVolume:
		return "volume.attach"
	case actDetachVolume:
		return "volume.detach"
	case actResizeVolume:
		return "volume.resize"
	case actCreateSnapshot:
		return "snapshot.create"
	case actDeleteSnapshot:
		return "snapshot.delete"
	case actCreateReservedIP:
		return "ip.create"
	case actDeleteReservedIP:
		return "ip.delete"
	case actAssignReservedIP:
		return "ip.assign"
	case actUnassignReservedIP:
		return "ip.unassign"
	case actDeleteFirewall:
		return "firewall.delete"
	case actCreateDomain:
		return "domain.create"
	case actDeleteDomain:
		return "domain.delete"
	case actCreateDNSRecord:
		return "dns.create"
	case actDeleteDNSRecord:
		return "dns.delete"
	case actCreateBucket:
		return "spaces.bucket.create"
	case actDeleteBucket:
		return "spaces.bucket.delete"
	case actDeleteObject:
		return "spaces.object.delete"
	case actCreateVPC:
		return "vpc.create"
	case actDeleteVPC:
		return "vpc.delete"
	case actDeleteImage:
		return "image.delete"
	case actDeleteAlertPolicy:
		return "alert.delete"
	case actPowerCycle:
		return "droplet.powercycle"
	case actPasswordReset:
		return "droplet.passwordreset"
	case actEnableIPv6:
		return "droplet.enableipv6"
	case actEnablePrivateNet:
		return "droplet.enableprivnet"
	case actEnableBackups:
		return "droplet.enablebackups"
	case actDisableBackups:
		return "droplet.disablebackups"
	case actSnapshotDroplet:
		return "droplet.snapshot"
	case actResizeDroplet:
		return "droplet.resize"
	case actRenameDroplet:
		return "droplet.rename"
	case actRebuildDroplet:
		return "droplet.rebuild"
	case actCreateFirewall:
		return "firewall.create"
	case actAddFirewallDroplets:
		return "firewall.add_droplets"
	case actRemoveFirewallDroplets:
		return "firewall.remove_droplets"
	case actAddFirewallRules:
		return "firewall.add_rules"
	case actRemoveFirewallRules:
		return "firewall.remove_rules"
	case actRenameImage:
		return "image.rename"
	case actTransferImage:
		return "image.transfer"
	case actConvertImage:
		return "image.convert"
	case actCreateAlertPolicy:
		return "alert.create"
	default:
		return "unknown"
	}
}

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
