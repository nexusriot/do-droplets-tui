package main

import (
	"fmt"
	"os"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/nexusriot/do-droplets-tui/internal/do"
	"github.com/nexusriot/do-droplets-tui/internal/tui"
)

func main() {
	token := os.Getenv("DO_TOKEN")
	if token == "" {
		fmt.Fprintln(os.Stderr, "ERROR: DO_TOKEN is not set")
		fmt.Fprintln(os.Stderr, "Export it, e.g.: export DO_TOKEN=...")
		os.Exit(1)
	}

	client := do.New(token)
	m := tui.NewModel(client)

	p := tea.NewProgram(m, tea.WithAltScreen())
	if _, err := p.Run(); err != nil {
		fmt.Fprintln(os.Stderr, "run error:", err)
		os.Exit(1)
	}
}
