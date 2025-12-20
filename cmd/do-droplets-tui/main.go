package main

import (
	"flag"
	"fmt"
	"os"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/nexusriot/do-droplets-tui/internal/config"
	"github.com/nexusriot/do-droplets-tui/internal/do"
	"github.com/nexusriot/do-droplets-tui/internal/tui"
)

func main() {
	cfgPath := flag.String("config", config.DefaultPath, "Path to config file")
	flag.Parse()

	cfg, err := config.Load(*cfgPath)
	if err != nil {
		fmt.Fprintln(os.Stderr, "ERROR:", err)
		os.Exit(1)
	}

	// Optional env override (handy for CI)
	if v := os.Getenv("DO_TOKEN"); v != "" {
		cfg.DigitalOcean.Token = v
	}

	client, err := do.New(cfg.DigitalOcean.Token)
	if err != nil {
		fmt.Fprintln(os.Stderr, "ERROR:", err)
		os.Exit(1)
	}

	m := tui.NewModel(client, tui.Options{
		DefaultRegion: cfg.UI.DefaultRegion,
		DefaultSize:   cfg.UI.DefaultSize,
		DefaultImage:  cfg.UI.DefaultImage,
		DefaultTags:   cfg.UI.DefaultTags,
		DefaultIPv6:   cfg.UI.DefaultIPv6,
	})

	p := tea.NewProgram(m, tea.WithAltScreen())
	if _, err := p.Run(); err != nil {
		fmt.Fprintln(os.Stderr, "run error:", err)
		os.Exit(1)
	}
}
