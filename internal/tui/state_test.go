package tui

import (
	"testing"

	"github.com/charmbracelet/bubbles/textinput"
)

func TestTabSwitchAllowed(t *testing.T) {
	// Top-level list/log screens accept tab switching.
	allowed := []state{
		stateDroplets, stateVolumes, stateOpsLog,
		stateSnapshots, stateReservedIPs, stateFirewalls,
		stateDomains, stateSpaces,
		stateAccount, stateVPCs, stateImages, stateAlerts,
	}
	for _, s := range allowed {
		if !tabSwitchAllowed(s) {
			t.Errorf("state %d should allow tab switching", s)
		}
	}

	// Modal / form / detail states must NOT — typing in inputs would otherwise
	// trigger unintended tab switches.
	denied := []state{
		stateConfirm,
		stateDetails,
		stateCreateDroplet, stateCreateVolume, stateCreateReservedIP,
		stateCreateDomain, stateCreateRecord, stateCreateBucket,
		stateCreateVPC,
		stateResizeVolume, stateSnapshotName,
		statePickSSHKeys, stateAttachVolume, stateAssignReservedIP,
		stateFirewallDetails, stateVolumeDetails,
		stateDomainRecords, stateSpaceObjects,
		// stateAI has text inputs and was deliberately removed from the
		// allow-list — covered explicitly so a future "re-add to allowed"
		// regression breaks this test.
		stateAI,
		stateDropletSnapName, stateDropletResize,
		stateDropletRename, stateDropletRebuild,
		stateCreateFirewall, stateFirewallAddRule,
		stateFirewallAddDroplets, stateFirewallRemoveDroplets,
	}
	for _, s := range denied {
		if tabSwitchAllowed(s) {
			t.Errorf("state %d should NOT allow tab switching", s)
		}
	}
}

func TestCanSwitchTabNow(t *testing.T) {
	// Non-AI state: same as tabSwitchAllowed.
	m := Model{st: stateDroplets}
	if !m.canSwitchTabNow() {
		t.Error("stateDroplets should allow tab switching")
	}
	m.st = stateConfirm
	if m.canSwitchTabNow() {
		t.Error("stateConfirm should NOT allow tab switching")
	}

	// stateAI with both inputs blurred → allowed.
	m = Model{st: stateAI, aiPromptIn: textinput.New(), aiSystemIn: textinput.New()}
	if !m.canSwitchTabNow() {
		t.Error("stateAI with no focused input should allow tab switching")
	}

	// stateAI with prompt focused → blocked.
	m = Model{st: stateAI, aiPromptIn: textinput.New(), aiSystemIn: textinput.New()}
	m.aiPromptIn.Focus()
	if m.canSwitchTabNow() {
		t.Error("stateAI with aiPromptIn focused should NOT allow tab switching")
	}

	// stateAI with system input focused → blocked.
	m = Model{st: stateAI, aiPromptIn: textinput.New(), aiSystemIn: textinput.New()}
	m.aiSystemIn.Focus()
	if m.canSwitchTabNow() {
		t.Error("stateAI with aiSystemIn focused should NOT allow tab switching")
	}
}

func TestInTextInputState(t *testing.T) {
	// All states that own at least one textinput.Model the user actively
	// types into. Plain `q` must not quit while we're here.
	inputs := []state{
		stateCreateDroplet, stateCreateVolume, stateCreateReservedIP,
		stateCreateDomain, stateCreateRecord, stateCreateBucket,
		stateCreateVPC,
		stateResizeVolume, stateSnapshotName,
		stateAI,
		stateDropletSnapName, stateDropletResize,
		stateDropletRename, stateDropletRebuild,
		stateCreateFirewall, stateFirewallAddRule,
	}
	for _, s := range inputs {
		if !inTextInputState(s) {
			t.Errorf("state %d should be classified as text-input", s)
		}
	}

	// States with no text input: q quitting them is fine.
	nonInputs := []state{
		stateDroplets, stateVolumes, stateOpsLog,
		stateDetails, stateVolumeDetails, stateFirewallDetails,
		stateSnapshots, stateReservedIPs, stateFirewalls,
		stateDomains, stateSpaces,
		stateAccount, stateVPCs, stateImages, stateAlerts,
		stateConfirm,
		statePickSSHKeys, stateAttachVolume, stateAssignReservedIP,
		stateFirewallAddDroplets, stateFirewallRemoveDroplets,
	}
	for _, s := range nonInputs {
		if inTextInputState(s) {
			t.Errorf("state %d should NOT be classified as text-input", s)
		}
	}
}
