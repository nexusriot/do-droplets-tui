package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"time"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/nexusriot/do-droplets-tui/internal/config"
	"github.com/nexusriot/do-droplets-tui/internal/do"
	"github.com/nexusriot/do-droplets-tui/internal/inference"
	"github.com/nexusriot/do-droplets-tui/internal/spaces"
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

	// Spaces client is optional; if not configured the tab shows a message.
	var spacesClient tui.SpacesAPI
	if cfg.Spaces.AccessKey != "" {
		sc, err := spaces.New(cfg.Spaces.AccessKey, cfg.Spaces.SecretKey, cfg.Spaces.Region)
		if err != nil {
			fmt.Fprintln(os.Stderr, "WARN spaces client:", err)
		} else {
			spacesClient = &spacesAdapter{c: sc}
		}
	}

	// Inference client is optional; if not configured the tab shows a message.
	var inferenceClient tui.InferenceAPI
	if cfg.Inference.ModelAccessKey != "" {
		ic, err := inference.New(cfg.Inference.ModelAccessKey, cfg.Inference.BaseURL)
		if err != nil {
			fmt.Fprintln(os.Stderr, "WARN inference client:", err)
		} else {
			inferenceClient = &inferenceAdapter{c: ic}
		}
	}

	m := tui.NewModel(client, tui.Options{
		DefaultRegion: cfg.UI.DefaultRegion,
		DefaultSize:   cfg.UI.DefaultSize,
		DefaultImage:  cfg.UI.DefaultImage,
		DefaultTags:   cfg.UI.DefaultTags,
		DefaultIPv6:   cfg.UI.DefaultIPv6,
	}, spacesClient, inferenceClient)

	p := tea.NewProgram(m, tea.WithAltScreen())
	if _, err := p.Run(); err != nil {
		fmt.Fprintln(os.Stderr, "run error:", err)
		os.Exit(1)
	}
}

type spacesAdapter struct{ c *spaces.Client }

func (a *spacesAdapter) ListBuckets(ctx context.Context) ([]tui.SpacesBucketRow, error) {
	rows, err := a.c.ListBuckets(ctx)
	if err != nil {
		return nil, err
	}
	out := make([]tui.SpacesBucketRow, 0, len(rows))
	for _, r := range rows {
		out = append(out, tui.SpacesBucketRow{
			Name:    r.Name,
			Created: r.Created.Format(time.DateTime),
		})
	}
	return out, nil
}

func (a *spacesAdapter) CreateBucket(ctx context.Context, name string) error {
	return a.c.CreateBucket(ctx, name)
}

func (a *spacesAdapter) DeleteBucket(ctx context.Context, name string) error {
	return a.c.DeleteBucket(ctx, name)
}

func (a *spacesAdapter) ListObjects(ctx context.Context, bucket, prefix string) ([]tui.SpacesObjectRow, error) {
	rows, err := a.c.ListObjects(ctx, bucket, prefix)
	if err != nil {
		return nil, err
	}
	out := make([]tui.SpacesObjectRow, 0, len(rows))
	for _, r := range rows {
		out = append(out, tui.SpacesObjectRow{
			Key:          r.Key,
			SizeBytes:    r.Size,
			LastModified: r.LastModified.Format(time.DateTime),
			StorageClass: r.StorageClass,
		})
	}
	return out, nil
}

func (a *spacesAdapter) DeleteObject(ctx context.Context, bucket, key string) error {
	return a.c.DeleteObject(ctx, bucket, key)
}

type inferenceAdapter struct{ c *inference.Client }

func (a *inferenceAdapter) ListModels(ctx context.Context) ([]tui.InferenceModel, error) {
	models, err := a.c.ListModels(ctx)
	if err != nil {
		return nil, err
	}
	out := make([]tui.InferenceModel, 0, len(models))
	for _, m := range models {
		out = append(out, tui.InferenceModel{ID: m.ID, OwnedBy: m.OwnedBy})
	}
	return out, nil
}

func (a *inferenceAdapter) ChatCompletion(ctx context.Context, req tui.InferenceChatReq) (string, int, int, error) {
	msgs := []inference.ChatMessage{
		{Role: "user", Content: req.User},
	}
	if req.System != "" {
		msgs = []inference.ChatMessage{
			{Role: "system", Content: req.System},
			{Role: "user", Content: req.User},
		}
	}
	maxToks := req.MaxToks
	if maxToks == 0 {
		maxToks = 2048
	}
	resp, err := a.c.ChatCompletion(ctx, inference.CompletionRequest{
		Model:     req.Model,
		Messages:  msgs,
		MaxTokens: maxToks,
	})
	if err != nil {
		return "", 0, 0, err
	}
	text := ""
	if len(resp.Choices) > 0 {
		text = resp.Choices[0].Message.Content
	}
	return text, resp.Usage.PromptTokens, resp.Usage.CompletionTokens, nil
}

func (a *inferenceAdapter) Embed(ctx context.Context, model, input string) (int, error) {
	resp, err := a.c.Embed(ctx, inference.EmbeddingRequest{Model: model, Input: input})
	if err != nil {
		return 0, err
	}
	if len(resp.Data) > 0 {
		return len(resp.Data[0].Embedding), nil
	}
	return 0, nil
}
