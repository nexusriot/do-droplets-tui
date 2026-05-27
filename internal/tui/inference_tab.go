package tui

import (
	"context"
	"fmt"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

func (m Model) updateAI(k tea.KeyMsg, cmds []tea.Cmd) (Model, []tea.Cmd) {
	if m.inferenceClient == nil {
		return m, cmds
	}

	// lazy-init inputs
	if m.aiPromptIn.Prompt == "" {
		m.aiPromptIn = newInput("Prompt", "Ask something...")
		m.aiSystemIn = newInput("System (optional)", "You are a helpful assistant.")
		m.aiSystemIn.SetValue("You are a helpful assistant.")
		m.aiPromptIn.Focus()
	}

	if m.busy || m.aiPending {
		// Even when busy, allow leaving the tab so the user isn't stuck.
		if k.String() == "esc" {
			m.aiPromptIn.Blur()
			m.aiSystemIn.Blur()
			m.st = stateDroplets
			return m, cmds
		}
		return m, cmds
	}

	inputFocused := m.aiPromptIn.Focused() || m.aiSystemIn.Focused()

	if inputFocused {
		switch k.String() {
		case "esc":
			m.aiPromptIn.Blur()
			m.aiSystemIn.Blur()
			m.status = "AI inputs unfocused — press tab to re-focus, esc again to leave"
			return m, cmds
		case "tab":
			m.aiFocusField = (m.aiFocusField + 1) % 2
			if m.aiFocusField == 0 {
				m.aiPromptIn.Focus()
				m.aiSystemIn.Blur()
			} else {
				m.aiSystemIn.Focus()
				m.aiPromptIn.Blur()
			}
			return m, cmds
		case "ctrl+j", "enter":
			if m.aiFocusField == 1 {
				// system field — let enter pass through to the input
				break
			}
			prompt := strings.TrimSpace(m.aiPromptIn.Value())
			if prompt == "" {
				m.errText = "prompt is required"
				return m, cmds
			}
			if len(m.aiModels) == 0 {
				m.errText = "no models loaded (press r)"
				return m, cmds
			}
			modelID := m.aiModels[m.aiModelIdx].ID
			system := strings.TrimSpace(m.aiSystemIn.Value())
			m.aiPending = true
			m.aiResponse = "Waiting for response..."
			m.status = "Sending to " + modelID
			cmds = append(cmds, m.chatCompletionCmd(modelID, system, prompt))
			return m, cmds
		case "ctrl+c":
			return m, append(cmds, tea.Quit)
		}
		// All other keys go straight to the focused input.
		var cmd tea.Cmd
		if m.aiFocusField == 0 {
			m.aiPromptIn, cmd = m.aiPromptIn.Update(k)
		} else {
			m.aiSystemIn, cmd = m.aiSystemIn.Update(k)
		}
		if cmd != nil {
			cmds = append(cmds, cmd)
		}
		return m, cmds
	}

	// No input focused — navigation and action keys.
	switch k.String() {
	case "esc":
		m.st = stateDroplets
		return m, cmds
	case "tab":
		m.aiFocusField = (m.aiFocusField + 1) % 2
		if m.aiFocusField == 0 {
			m.aiPromptIn.Focus()
			m.aiSystemIn.Blur()
		} else {
			m.aiSystemIn.Focus()
			m.aiPromptIn.Blur()
		}
	case "r":
		m.aiResponse = ""
		m.aiUsageInfo = ""
		cmds = append(cmds, m.loadAIModelsCmd())
	case "up", "k":
		if m.aiModelIdx > 0 {
			m.aiModelIdx--
		}
	case "down", "j":
		if m.aiModelIdx < len(m.aiModels)-1 {
			m.aiModelIdx++
		}
	case "ctrl+c":
		return m, append(cmds, tea.Quit)
	}
	return m, cmds
}

func (m Model) viewAI() string {
	if m.inferenceClient == nil {
		return lipgloss.NewStyle().Faint(true).Render(
			"AI Inference not configured.\nAdd inference.model_access_key to config.json.",
		) + "\n\n" + m.footer()
	}

	// lazy-init placeholder
	promptView := m.aiPromptIn.View()
	if m.aiPromptIn.Prompt == "" {
		promptView = "[Prompt input — press 8 then type]"
	}
	systemView := m.aiSystemIn.View()
	if m.aiSystemIn.Prompt == "" {
		systemView = "[System prompt input]"
	}

	var b strings.Builder
	b.WriteString(lipgloss.NewStyle().Bold(true).Render("DO AI Inference") + "\n\n")

	// Model selector
	b.WriteString(lipgloss.NewStyle().Bold(true).Render("Model:") + " ")
	if len(m.aiModels) == 0 {
		b.WriteString(lipgloss.NewStyle().Faint(true).Render("(press r to load models)"))
	} else {
		b.WriteString(lipgloss.NewStyle().Foreground(lipgloss.Color("10")).Render(m.aiModels[m.aiModelIdx].ID))
		b.WriteString(lipgloss.NewStyle().Faint(true).Render(fmt.Sprintf("  ↑/↓ to pick (%d/%d)", m.aiModelIdx+1, len(m.aiModels))))
	}
	b.WriteString("\n\n")

	// System + prompt inputs
	b.WriteString("System prompt: " + systemView + "\n")
	b.WriteString("Prompt:        " + promptView + "\n")
	b.WriteString(lipgloss.NewStyle().Faint(true).Render(
		"Tab switch field | Enter send | r reload models | "+
			"Esc unfocus (then 1/2/3/… to switch tabs, or Esc again → Droplets) | Ctrl+C quit") + "\n\n")

	// Response area
	if m.aiResponse != "" {
		b.WriteString(lipgloss.NewStyle().Bold(true).Render("Response:") + "\n")
		b.WriteString(lipgloss.NewStyle().
			Border(lipgloss.RoundedBorder()).
			Padding(0, 1).
			Width(80).
			Render(m.aiResponse) + "\n")
		if m.aiUsageInfo != "" {
			b.WriteString(lipgloss.NewStyle().Faint(true).Render(m.aiUsageInfo) + "\n")
		}
	}

	return b.String() + m.footer()
}

func (m Model) loadAIModelsCmd() tea.Cmd {
	m.busy = true
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(m.ctx, 15*time.Second)
		defer cancel()
		models, err := m.inferenceClient.ListModels(ctx)
		if err != nil {
			return apiErrMsg{err: err}
		}
		return aiModelsLoadedMsg{models: models}
	}
}

func (m Model) chatCompletionCmd(modelID, system, prompt string) tea.Cmd {
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(m.ctx, 120*time.Second)
		defer cancel()
		text, promptToks, completionToks, err := m.inferenceClient.ChatCompletion(ctx, InferenceChatReq{
			Model:   modelID,
			System:  system,
			User:    prompt,
			MaxToks: 2048,
		})
		if err != nil {
			return apiErrMsg{err: err}
		}
		usage := fmt.Sprintf("Tokens: %d prompt + %d completion", promptToks, completionToks)
		return aiResponseMsg{text: text, usageInfo: usage}
	}
}
