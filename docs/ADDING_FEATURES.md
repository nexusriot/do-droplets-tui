# Adding Functionality to do-droplets-tui

A practical guide to extending this DigitalOcean TUI. Read this before adding
a new action, screen, form, or tab.

## 1. Architecture in one minute

```
cmd/do-droplets-tui/main.go   entrypoint: load config, build do.Client, run Bubble Tea program
internal/config/config.go     JSON config file + env override (DO_TOKEN)
internal/do/client.go         thin wrapper over godo (DigitalOcean SDK); the only place godo is touched
internal/tui/model.go         the whole UI: Bubble Tea Model (state, Update, View)
```

The app follows the **Elm Architecture** (Bubble Tea):

- `Model` holds all state.
- `Update(msg)` receives messages (`tea.Msg`) and returns a new `Model` + `tea.Cmd`.
- `View()` renders the current `Model` to a string.
- A `tea.Cmd` is a function that runs asynchronously (off the UI goroutine) and
  returns a `tea.Msg`. All network I/O happens inside `Cmd`s so the UI never blocks.

The `api` interface (model.go ~line 60) decouples the TUI from `do.Client`. The
TUI only ever calls methods on `api`; `do.Client` is the concrete implementation.
This is also the seam for testing with a fake client.

## 2. The standard data flow for any action

Every user action follows the same loop:

```
key press
  -> updateXxx() handler sets state / builds a tea.Cmd
  -> Cmd runs network call in background (with context timeout)
  -> Cmd returns a *Msg (apiDoneMsg / apiErrMsg / xxxLoadedMsg)
  -> Update() handles the Msg: clears busy, updates state, logs op, refreshes
  -> View() re-renders
```

Mutating actions also go through a **confirm dialog** (`stateConfirm`) before
the `Cmd` runs.

## 3. Recipe A — Add a new droplet action (e.g. "Power Cycle", "Rename")

This is the most common change. Example: add a **Power Cycle** action.

1. **Client method** — `internal/do/client.go`:
   ```go
   func (c *Client) PowerCycle(ctx context.Context, id int) error {
       _, _, err := c.godo.DropletActions.PowerCycle(ctx, id)
       return err
   }
   ```

2. **Extend the `api` interface** — `model.go`, the `api` interface block:
   ```go
   PowerCycle(context.Context, int) error
   ```

3. **Add an `actionKind`** — in the `const ( actPowerOn ... )` block:
   ```go
   actPowerCycle
   ```

4. **Map it to a label** — `actToString()`:
   ```go
   case actPowerCycle:
       return "droplet.powercycle"
   ```

5. **Add a key binding** — add a field to `keyMap`, set it in `defaultKeys()`,
   and (optionally) add it to `ShortHelp`/`FullHelp`:
   ```go
   PowerCycle key.Binding
   // in defaultKeys():
   PowerCycle: key.NewBinding(key.WithKeys("y"), key.WithHelp("y", "power cycle")),
   ```
   Pick an unused key. Currently used: `1 2 l r c d enter esc o p s b K q y n`
   and `↑↓ j k tab`. Note `y`/`n` are reserved for the confirm dialog — choosing
   them for a list action is fine because the confirm screen handles keys
   separately, but avoid ambiguity.

6. **Wire the key in the handler** — `updateDroplets()` (and `updateDetails()`
   if it should work from the details screen too):
   ```go
   case key.Matches(k, m.keys.PowerCycle):
       m = m.confirmDropletAction(actPowerCycle, "Power Cycle", stateDroplets)
   ```

7. **Run the action when confirmed** — `updateConfirm()`, extend the
   `actPowerOn, actPowerOff, ...` case so it also matches `actPowerCycle`
   (it already dispatches via `runDropletActionCmd`).

8. **Execute the call** — `runDropletActionCmd()`, add a case to its `switch`:
   ```go
   case actPowerCycle:
       err = m.api.PowerCycle(ctx, id)
   ```

9. **Post-action behavior** — `apiDoneMsg` handler in `Update()`. Add
   `actPowerCycle` to the `actPowerOn, actPowerOff, ...` case so it returns to
   the list, and to the refresh `switch` so the droplet list reloads.

That is the full surface for a mutating droplet action: **9 spots**, all
mechanical. If you add many actions, consider whether the `apiDoneMsg`
switches should group by category instead of listing each constant.

## 4. Recipe B — Add a new column / field to an existing table

Example: show "Memory" in the droplet table.

1. `do.DropletRow` — add the field (`Memory int`).
2. `ListDroplets()` in client.go — populate it from `godo.Droplet`.
3. `NewModel()` — add a `table.Column` to `dCols`.
4. `toDropletTableRows()` — append the value to each `table.Row` (order must
   match the columns).

Column widths are fixed; keep the total under typical terminal width.

## 5. Recipe C — Add a whole new tab / screen

Example: a "Snapshots" tab.

1. **State** — add `stateSnapshots` to the `state` const block.
2. **Data type + client** — add `SnapshotRow` and `ListSnapshots()` in client.go,
   add `ListSnapshots` to the `api` interface.
3. **Message** — add `type snapshotsLoadedMsg struct{ rows []do.SnapshotRow }`.
4. **Model fields** — add `snapshotTable table.Model` and `snapshotRows []do.SnapshotRow`.
5. **Table init** — build the `table.Model` in `NewModel()`.
6. **Loader Cmd** — copy `refreshDropletsCmd()` as `refreshSnapshotsCmd()`.
7. **Key binding** — add a `TabSnapshots` binding (e.g. key `3`) and handle it
   in the "global tab switching" block of `Update()`.
8. **Update handler** — write `updateSnapshots()` and call it from the
   `switch m.st` block in `Update()`.
9. **Table passthrough** — add a `case stateSnapshots:` to the second
   `switch m.st` (table navigation) near the end of `Update()`.
10. **WindowSizeMsg** — add `m.snapshotTable.SetHeight(h)`.
11. **Message handler** — handle `snapshotsLoadedMsg` in `Update()`.
12. **View** — write `viewSnapshots()` and add a `case stateSnapshots:` to `View()`.
13. **Title** — update the title string in `View()` to advertise the new tab.

## 6. Recipe D — Add a field to the Create Droplet form

The form uses N `textinput.Model` fields plus an integer `focusDropletForm`
cursor cycled modulo N. To add a field:

1. Add the `textinput.Model` to the Model struct.
2. `initDropletCreateForm()` — create it with `newInput(...)`, set default.
3. **Bump the modulus** — `updateCreateDroplet()` uses `% 8`; change every
   `8` to `9` (the field count). This is the easy thing to forget.
4. `blurDropletForm()` — add `m.xxxIn.Blur()`.
5. `focusDropletOne()` — add a `case` for the new index.
6. The input-update `switch m.focusDropletForm` in `updateCreateDroplet()` —
   add a `case`.
7. `viewCreateDroplet()` — render `m.xxxIn.View()`.
8. `buildCreateDropletReq()` — read and validate the value, put it in the req.
9. `do.CreateDropletReq` + `CreateDroplet()` — carry the field into the godo call.

The hard-coded `% 8` is a known sharp edge — the field count lives in three
literals. If the form grows, replace the literal with a `const dropletFormFields = 9`.

## 7. Conventions to keep

- **No blocking I/O in `Update()` or `View()`.** Network calls go in a `tea.Cmd`
  with `context.WithTimeout`.
- **Set `m.busy = true`** when starting a Cmd; the Msg handler clears it.
  Note: `refresh*Cmd` etc. set `busy` on a value receiver — the visible effect
  comes from handlers also toggling it; keep that pattern consistent.
- **Every mutating action goes through `stateConfirm`** — never fire a
  delete/create/power Cmd directly from a list key handler.
- **Log every op** via `m.logOp(...)` so it shows in the Ops tab.
- **Errors** become `apiErrMsg`; the handler shows `m.errText` and logs.
- **godo is isolated** in `internal/do`. The TUI must not import `godo` for
  anything except the few exposed types (`*godo.Droplet`, `*godo.Networks`).
  Prefer adding a flat `do.XxxRow` struct over leaking godo types.

## 8. Build & test

```
go build ./...
go vet ./...
go run ./cmd/do-droplets-tui --config ./config.json
```

There are currently no tests. The `api` interface makes the `Model` testable
with a fake: implement `api` with a stub, call `NewModel(fake, opts)`, feed
`tea.KeyMsg` / message values into `Update()`, and assert on returned state.
Good first target: `buildCreateDropletReq()` validation and the `updateConfirm`
dispatch.

## 9. Quick checklist for a new mutating action

- [ ] `do.Client` method
- [ ] `api` interface entry
- [ ] `actionKind` constant
- [ ] `actToString()` case
- [ ] `keyMap` field + `defaultKeys()` binding
- [ ] key wired in `updateDroplets()` / `updateDetails()`
- [ ] `updateConfirm()` dispatch
- [ ] `runDropletActionCmd()` (or a new Cmd) case
- [ ] `apiDoneMsg` handler: post-action state + refresh
- [ ] help text / legend updated
