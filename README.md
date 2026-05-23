# do-droplets-tui

A terminal UI for managing a DigitalOcean account. Lists, creates,
mutates, and deletes the resources you reach in the web console — droplets,
volumes, snapshots, reserved IPs, firewalls, domains, Spaces buckets, AI
inference, account/billing, VPCs, images, monitoring alert policies — plus a
local ops log of every action it has run.

Single binary. Written in Go on top of [Bubble Tea](https://github.com/charmbracelet/bubbletea),
[bubbles](https://github.com/charmbracelet/bubbles), and
[lipgloss](https://github.com/charmbracelet/lipgloss).
Uses [`digitalocean/godo`](https://github.com/digitalocean/godo) for the
control plane, a hand-rolled S3 client for Spaces, and a small HTTP client
for the OpenAI-compatible serverless inference endpoint.

---

## Features

### Tabs

| Key | Tab | What's there |
|---|---|---|
| `1` | **Droplets** | list, create, delete, power on/off, shutdown, reboot, details (with extended actions — see below) |
| `2` | **Volumes** | list, create, delete, attach/detach to droplets, resize, per-volume snapshots |
| `3` | **Snapshots** | all snapshots across the account; delete; **`c` to launch a new droplet from a snapshot** |
| `4` | **Reserved IPs** | list, create, delete, assign/unassign to droplets |
| `5` | **Firewalls** | list, view rules, create, delete, add/remove droplets, add rules |
| `6` | **Domains** | list, create, delete, list/create/delete DNS records |
| `7` | **Spaces** | bucket browser (list, create, delete) and object browser (list, delete) |
| `8` | **AI Inference** | chat with the serverless inference endpoint (OpenAI-compatible) |
| `9` | **Account** | account info, droplet/volume/IP limits, balance with prepayment sign-correction |
| `0` | **VPCs** | list, create, delete (default VPC protected) |
| `i` | **Images** | user / distribution / application / tag-filter; rename / transfer / convert / launch-droplet-from |
| `m` | **Alert Policies** | list, create, delete monitoring alerts |
| `l` | **Ops log** | every mutating action this session has performed |

### Extended droplet actions (on **Droplets → details**)

Capital-letter keys so they never collide with text-input states:

| Key | Action |
|---|---|
| `S` | Snapshot droplet (prompts for name) |
| `E` | rEsize (form has `space` toggle for irreversible disk resize) |
| `B` | reBuild from image slug or ID |
| `M` | Modify (rename) |
| `Y` | power cYcle (hard reset) |
| `U` | toggle backUps on/off (auto-detected from current state) |
| `I` | enable IPv6 |
| `V` | enable priVate networking |
| `W` | passWord reset (email to account) |
| `G` | Graph CPU last hour (Unicode block sparkline + min/avg/max) |

### Extended firewall management (on **Firewalls** tab)

- `c` on the list → create-firewall form with CSV rule syntax (`tcp:22,tcp:80-90,udp:53,icmp:`); sources default to `0.0.0.0/0` and `::/0`
- `a` / `x` on details → droplet picker (space-to-toggle, enter-to-confirm) to add/remove droplets
- `c` on details → add a single inbound/outbound rule (`Ctrl+D` toggles direction)

### Create a droplet from an image or snapshot

- From **Snapshots** tab: `c` → form is pre-filled with image=`<snapshot.ID>` and region=`<snapshot.Region>`
- From **Images** tab: `c` → same flow for any selectable image (user, distribution, application, or tag-filter)
- Name is auto-suggested from the image name (`My App v2.0` → `my-app-v2-0-droplet`)

### Input safety

- `q` quits *only* in non-input states; `Ctrl+C` is the universal kill.  Typing a name like `qa-staging` no longer terminates the program.
- Form/input states are explicitly classified (`inTextInputState`) — global single-letter keys won't fire while you're typing.
- The AI tab uses a runtime-aware gate (`canSwitchTabNow`): press `Esc` once to unfocus inputs, then any tab key jumps where you want.

### Ops log

Every mutating action records `kind`, `target`, and `result` in a session-local ring (200 entries). Press `l` to view. Errors land there too.

---

## Install

### From source

```sh
go install github.com/nexusriot/do-droplets-tui/cmd/do-droplets-tui@latest
```

### Build

```sh
git clone https://github.com/nexusriot/do-droplets-tui
cd do-droplets-tui
make x86_64-static        # linux/amd64, fully static (CGO off)
# or:
make all                  # linux amd64+arm64+armv7, darwin amd64+arm64, windows amd64
make debs                 # .deb packages for amd64 / arm64 / armhf
make run                  # go run ./cmd/do-droplets-tui
```

`make help` lists every target.

---

## Configuration

The binary reads a JSON config (default `/etc/do-droplets-tui/config.json`,
override with `--config`). The `DO_TOKEN` environment variable overrides
`digitalocean.token` (handy for one-off runs).

```jsonc
{
  "digitalocean": {
    "token": "dop_v1_..."
  },
  "ui": {
    "default_region": "fra1",
    "default_size":   "s-1vcpu-1gb",
    "default_image":  "ubuntu-24-04-x64",
    "default_tags":   "dev,tui",
    "default_ipv6":   false
  },
  "spaces": {
    "access_key": "",
    "secret_key": "",
    "region":     "fra1"
  },
  "inference": {
    "model_access_key": "",
    "base_url":         ""
  }
}
```

| Field | Required? | Notes |
|---|---|---|
| `digitalocean.token` | yes | DigitalOcean PAT. May also be supplied via `DO_TOKEN`. |
| `ui.*` | optional | Defaults pre-fill the Create Droplet form. |
| `spaces.*` | optional | Without these, the Spaces tab shows "not configured." |
| `inference.*` | optional | Without `model_access_key`, the AI tab shows "not configured." `base_url` defaults to `https://inference.do-ai.run/v1`. |

Tokens are read once at startup and never written to disk by this tool.

---

## Architecture

```
cmd/do-droplets-tui/main.go
  loads config, builds clients, runs Bubble Tea program

internal/config/config.go      JSON config + DO_TOKEN env override
internal/do/                   godo wrapper — the ONLY place godo is imported
   client.go                       droplets / volumes / SSH keys / volume snapshots
   resources.go                    snapshots, reserved IPs, firewalls (basic), domains
   account_vpcs_images_alerts.go   account, balance, VPCs, images (user/distro), alert policies
   droplet_ext.go                  power-cycle, password-reset, ipv6, priv-net,
                                   backups, snapshot, resize, rename, rebuild,
                                   firewall create + rule/droplet mutations
   images_ext.go                   ListApplication, ListByTag, Update, Transfer,
                                   Convert, CreateAlertPolicy, GetDropletCPU
internal/inference/client.go   hand-rolled HTTP client for inference.do-ai.run/v1
internal/spaces/client.go      hand-rolled S3 client for DO Spaces (no AWS SDK)
internal/tui/                  the UI; a single Bubble Tea Model
   model.go                        state enum, key map, msg routing, View dispatch
   <resource>_tab.go               one file per tab
   <resource>_ext.go               extensions layered on top via indirection helpers
```

### Bubble Tea recap

- **`Model`** holds *all* state (resource rows, tables, form inputs, pending action targets, etc.).
- **`Update(msg)`** receives messages (`tea.Msg`) and returns a new `Model` + `tea.Cmd`.
- **`View()`** renders the current `Model` to a string.
- **`tea.Cmd`s** run asynchronously off the UI goroutine. Every network call is wrapped in one so the UI never blocks.

### Key seams

- `api` interface in [model.go](internal/tui/model.go) — the TUI never touches `godo` directly. `do.Client` is the concrete implementation; tests can stub.
- `stateConfirm` — every mutating action routes through a generic yes/no dialog before the `Cmd` fires.
- `inTextInputState(state)` — single-letter globals (`q` quit, single-letter tab switches) consult this so they don't fire mid-typing.
- `canSwitchTabNow(Model)` — runtime-aware tab gate; allows the AI tab to be exited via number keys once its text inputs are blurred.

See [`docs/ADDING_FEATURES.md`](docs/ADDING_FEATURES.md) for the recipe to add a new action / column / form field / tab.

---

## Development

```sh
make fmt          # gofmt
make vet          # go vet ./...
make test         # go test ./...
make test-race    # go test -race -count=1 ./...
make tidy         # go mod tidy
make run          # go run ./cmd/do-droplets-tui
```

Tests live next to the code they cover; the suite focuses on pure helpers
(`ParseCSVInts`, `parseRuleCSV`, `signedBalance`, `sparkline`,
`flattenCPU` …) and validators (`buildCreateDropletReq`). The `api`
interface makes the `Model` testable with a fake — see
[`internal/tui/build_droplet_req_test.go`](internal/tui/build_droplet_req_test.go)
for the pattern.

### Adding features

1. Read `docs/ADDING_FEATURES.md` for the standard recipe.
2. Read `docs/AI_FEATURES_DESIGN.md` if you're touching the AI tab — there is a 13-slice rollout plan for the DigitalOcean GenAI Platform that isn't shipped yet.

---

## Status & scope

Built for one person (the author) who wants a fast terminal alternative to
clicking through the DO control panel.  It covers everything the author
uses regularly and a lot more besides, but **does not** currently expose:

- Load Balancers
- Kubernetes (DOKS)
- App Platform
- Managed Databases
- Container Registry
- CDN endpoints
- Certificates
- Functions / Serverless
- Uptime Checks
- BillingHistory / Invoices
- GenAI Platform (agents, knowledge bases, function routes — see [`docs/AI_FEATURES_DESIGN.md`](docs/AI_FEATURES_DESIGN.md))
- Anything the DO API itself doesn't expose (web console only)

Patches welcome. The architecture is intentionally additive — every new
resource lands as one new `<resource>_tab.go` file plus a handful of
mechanical wirings in `model.go`.

---

## Safety

- **Every** mutating action goes through `stateConfirm` first.
- The default firewall ("VPC default") refuses delete.
- Disk-resize warns about irreversibility.
- Rebuild warns that all data on the droplet will be lost.
- Tokens are never logged, never persisted by this tool, never sent anywhere except DigitalOcean's API.
- `Ctrl+C` always exits; `q` only exits when not in a text-input state.

---

## License

MIT.