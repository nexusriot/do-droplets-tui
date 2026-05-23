# AI Features Design

Scope: design the missing AI surface in `do-droplets-tui` against the actual
DigitalOcean public API. Today the app exposes only the **OpenAI-compatible
serverless inference endpoint** (`https://inference.do-ai.run/v1`) — chat
completions + embeddings + model list. The DigitalOcean **GenAI Platform**
exposes a much wider tree under `https://api.digitalocean.com/v2/gen-ai/...`
that we currently touch zero of.

The numbers in §6 are the recommended shipping order.

---

## 1. What we have today

`internal/inference/client.go` is a hand-rolled HTTP client targeting the
serverless inference endpoint:

| Endpoint              | Method            | Status |
|-----------------------|-------------------|--------|
| `/v1/models`          | `ListModels`      | ✓ |
| `/v1/chat/completions`| `ChatCompletion`  | ✓ (non-streaming) |
| `/v1/embeddings`      | `Embed`           | ✓ in client, **not wired to TUI** |

UI: one screen `stateAI` (`inference_tab.go`) with model picker, system field,
prompt field, response area. No streaming, no chat history, no embeddings UI,
no parameter controls (temperature/top_p/max_tokens), no save/load.

Two clients coexist:
- `internal/do/Client` — `godo.Client` wrapper for the control-plane API.
- `internal/inference/Client` — bespoke client for serverless inference.

Adding GenAI Platform means **a third client** rooted at
`https://api.digitalocean.com/v2/gen-ai/`, authenticated with the same
**personal access token** used by `godo`. It is separate from the inference
client because the auth model and base URL differ.

---

## 2. The DO API surface we don't expose

Authoritative reference: https://docs.digitalocean.com/reference/api/api-reference/#tag/GenAI-Platform

All paths are under `https://api.digitalocean.com/v2/gen-ai/`.

### 2.1 Agents (custom RAG/tool-using assistants)
```
GET    /agents
POST   /agents
GET    /agents/{uuid}
PUT    /agents/{uuid}
DELETE /agents/{uuid}
PUT    /agents/{uuid}/deployment_visibility   # public / private endpoint
```

An Agent bundles: a foundation model, system instructions, attached knowledge
bases, attached function routes, child agents (for routing), an API key, and a
deployment endpoint (chat completions-compatible).

### 2.2 Child agents (routing / handoff)
```
POST   /agents/{uuid}/child_agents/{child_uuid}
DELETE /agents/{uuid}/child_agents/{child_uuid}
```

### 2.3 Agent API keys (per-agent access tokens)
```
GET    /agents/{uuid}/api_keys
POST   /agents/{uuid}/api_keys
DELETE /agents/{uuid}/api_keys/{key_uuid}
PUT    /agents/{uuid}/api_keys/{key_uuid}/regenerate
```

### 2.4 Agent versions (rollback)
```
GET /agents/{uuid}/versions
PUT /agents/{uuid}/versions      # rollback to a version
```

### 2.5 Function routes (agent tools)
```
GET    /agents/{uuid}/functions
POST   /agents/{uuid}/functions
PUT    /agents/{uuid}/functions/{route_uuid}
DELETE /agents/{uuid}/functions/{route_uuid}
```

### 2.6 Knowledge bases (RAG corpora)
```
GET    /knowledge_bases
POST   /knowledge_bases
GET    /knowledge_bases/{uuid}
PUT    /knowledge_bases/{uuid}
DELETE /knowledge_bases/{uuid}

GET    /knowledge_bases/{uuid}/data_sources
POST   /knowledge_bases/{uuid}/data_sources
DELETE /knowledge_bases/{uuid}/data_sources/{ds_uuid}
```

Data-source kinds: Spaces bucket (we already browse those!), web crawl,
uploaded file, file URL.

### 2.7 Attach knowledge base to an agent
```
POST   /agents/{uuid}/knowledge_bases/{kb_uuid}
DELETE /agents/{uuid}/knowledge_bases/{kb_uuid}
```

### 2.8 Indexing jobs (rebuild KB embeddings)
```
GET    /indexing_jobs
POST   /indexing_jobs
GET    /indexing_jobs/{uuid}
PUT    /indexing_jobs/{uuid}/cancel
GET    /indexing_jobs/{uuid}/data_sources
```

### 2.9 Models catalog (foundation models we can pick for agents)
```
GET /models
```

Different from `/v1/models` on the inference endpoint — this lists the
**catalog** of models the GenAI platform can host (Llama variants, Mistral,
Anthropic via BYOK, etc.) plus their metadata (context length, RAG support,
agreement requirements).

### 2.10 Model access keys (serverless inference keys)
```
GET    /models/api_keys
POST   /models/api_keys
DELETE /models/api_keys/{key_uuid}
PUT    /models/api_keys/{key_uuid}/regenerate
```

These are exactly the kind of key the user has to paste into our config today.
Letting the TUI mint and rotate them is a quality-of-life win.

### 2.11 BYOK provider keys (Anthropic / OpenAI passthrough)
```
GET    /anthropic/keys
POST   /anthropic/keys
GET    /anthropic/keys/{uuid}
DELETE /anthropic/keys/{uuid}
GET    /anthropic/keys/{uuid}/agents
GET    /anthropic/keys/{uuid}/models

GET    /openai/keys
POST   /openai/keys
GET    /openai/keys/{uuid}
DELETE /openai/keys/{uuid}
GET    /openai/keys/{uuid}/agents
GET    /openai/keys/{uuid}/models
```

### 2.12 GenAI regions
```
GET /regions
```

### 2.13 Evaluation (preview)
```
GET    /evaluation_runs
POST   /evaluation_runs
GET    /evaluation_runs/{uuid}
GET    /evaluation_test_cases
POST   /evaluation_test_cases
```
Smallest priority — feature is still maturing.

### 2.14 Missing on the inference endpoint itself
The current `inference.Client` doesn't expose features the endpoint already
supports:

| Feature                  | How                                                                  |
|--------------------------|----------------------------------------------------------------------|
| Streaming responses      | `stream: true` body + SSE parser (`data: {...}\n\n` chunks)          |
| Temperature / top_p      | Already-present JSON fields, just no UI                              |
| Stop sequences           | `stop: ["..."]`                                                      |
| `max_tokens` knob        | Hard-coded to 2048 in `chatCompletionCmd` — make it a field          |
| Chat history             | Build `Messages` list instead of single user-turn                    |
| Embeddings               | `Embed` already exists, no UI                                        |
| Function calling / tools | `tools: [...]` + `tool_choice` — useful with Agents but also direct  |

---

## 3. Proposed architecture

### 3.1 New package layout

```
internal/genai/
  client.go        # *Client, New(token), do(), get/post/put/delete helpers
  agents.go        # Agent struct + List/Get/Create/Update/Delete/SetVisibility
  agent_keys.go    # AgentAPIKey + List/Create/Delete/Regenerate
  agent_versions.go
  child_agents.go
  functions.go     # function routes
  knowledge.go     # KnowledgeBase + DataSource + Attach/Detach
  indexing.go      # IndexingJob + Start/Get/Cancel/ListSources
  models.go        # catalog (List)
  model_keys.go    # /models/api_keys
  byok.go          # /anthropic/keys + /openai/keys
  regions.go
```

Same shape as `internal/do`: thin wrapper, flat row structs returned to TUI,
nothing internal leaks. Authenticated with the **DO_TOKEN** the rest of the
app already loads — no new config required for the *control-plane* GenAI work.

The bespoke inference client stays in `internal/inference` and gains:

```
internal/inference/
  client.go        # existing
  stream.go        # SSE decoder for streaming chat
```

A new `chat.go` in the TUI layer will own the conversation state (messages
slice), independent of which backend services it.

### 3.2 New TUI states

Existing AI tab becomes a **menu hub** rather than a single chat box:

```
stateAI              -> top-level: "1 Inference  2 Agents  3 KBs  4 Keys"
stateAIInference     -> the existing chat box (renamed from current)
stateAIAgents        -> list of agents
stateAIAgentDetail   -> agent metadata + chat-with-agent
stateAIAgentCreate   -> form
stateAIKBs           -> list of knowledge bases
stateAIKBDetail      -> KB metadata + data sources + indexing jobs
stateAIKBCreate
stateAIDataSourceCreate
stateAIIndexingJobs  -> recent jobs across all KBs
stateAIModels        -> foundation-model catalog (read-only browser)
stateAIModelKeys     -> serverless inference keys CRUD
stateAIBYOK          -> Anthropic / OpenAI key CRUD (tab between providers)
stateAIFunctionRoutes (under agent detail)
```

Add a sub-tab strip rendered when `m.st` is in the AI cluster. Keep the global
`8` to enter AI; once inside, use letters (`a` agents, `k` knowledge bases,
`y` keys, `c` chat, `m` models) to switch sub-state. The number keys stay
reserved for the **top-level tabs** so we don't lose 1–9.

### 3.3 New action kinds

```
actCreateAgent, actDeleteAgent, actUpdateAgent
actCreateAgentKey, actDeleteAgentKey, actRegenAgentKey
actCreateKB, actDeleteKB, actUpdateKB
actCreateDataSource, actDeleteDataSource
actStartIndexing, actCancelIndexing
actAttachKBToAgent, actDetachKBFromAgent
actAttachChildAgent, actDetachChildAgent
actCreateFunctionRoute, actDeleteFunctionRoute, actUpdateFunctionRoute
actSetAgentVisibility
actCreateModelKey, actDeleteModelKey, actRegenModelKey
actAddAnthropicKey, actDeleteAnthropicKey
actAddOpenAIKey, actDeleteOpenAIKey
```

Each follows the existing `stateConfirm → run Cmd → apiDoneMsg → refresh`
recipe. Roughly 30 mechanical wirings.

### 3.4 Streaming chat

SSE decoder in `internal/inference/stream.go`:

```go
type StreamChunk struct {
    Delta        string
    FinishReason string
    Done         bool
}

func (c *Client) ChatCompletionStream(
    ctx context.Context, req CompletionRequest,
    onChunk func(StreamChunk),
) error
```

In the TUI:
- Each chunk fires a `aiChunkMsg{text string}` `tea.Msg` (via `tea.Cmd`
  that runs a goroutine + a channel — match what Bubble Tea recommends:
  return a `tea.Cmd` that calls `onChunk` from inside the program loop).
- Append to `m.aiResponse` and re-render. Existing layout already shows the
  response in a bordered box, so visible streaming "just works."
- Final chunk emits `aiResponseMsg{...}` to clear `m.aiPending` and record
  usage.

Cancellation: store the chunk-pump cancel func on the model, expose `Ctrl+G`
to stop a streaming response mid-flight.

### 3.5 Chat history

Replace the single-turn model with:

```go
type ChatTurn struct {
    Role    string // "user" | "assistant" | "system"
    Content string
    Tokens  int
}

type ChatSession struct {
    Title   string
    Model   string // model id or agent uuid (prefix "agent:" disambiguates)
    System  string
    Turns   []ChatTurn
    Created time.Time
}
```

State in `Model`:
```go
chatSession ChatSession
chatHist    []ChatSession   // ring of recent sessions
```

UI: a left pane (sessions, `Ctrl+P`/`Ctrl+N` to switch) + right pane (turns).
`Ctrl+L` clears, `Ctrl+S` saves to `~/.config/do-droplets-tui/chats/<ts>.json`.
Loading sessions is a `j`/`k` table over the saved files.

This works for both inference-endpoint chats and agent chats (agent endpoint
is OpenAI-compatible — same `/chat/completions` payload, different base URL +
agent API key).

### 3.6 Agent chat

Agents expose their own OpenAI-compatible endpoint at
`https://<deployment-domain>/api/v1/chat/completions` authenticated with an
agent API key. We get the endpoint URL + key from `GET /agents/{uuid}` after
ensuring an API key exists.

We give the existing `inference.Client` a constructor variant:

```go
func NewWithEndpoint(baseURL, apiKey string) *Client
```

so the same chat code reuses it for agent conversations. Sessions tagged
`Model: "agent:<uuid>"` route to that client; sessions tagged with a foundation
model id route to the default serverless client.

### 3.7 Embeddings panel

Sub-state `stateAIEmbed` with:
- Model picker (filters models with `embedding` capability)
- Multi-line text input
- Output: vector dimension, first/last 5 floats, L2 norm, prompt-token count
- "Copy as JSON" key (writes the full vector to clipboard via `osc52` or
  to `/tmp/embedding-<ts>.json` if no clipboard is available — keep TUI
  dependencies clean)

### 3.8 Knowledge base detail screen

```
KB: my-docs        Status: indexed
Region: nyc3       Embedding model: text-embedding-3-large
Foundation model: llama-3.3-70b-instruct

Data sources (3):
   spaces:my-bucket/docs/         (2025-12-01)
   web: https://docs.example.com/ (2025-11-30)
   file: handbook.pdf             (2025-11-28)

Indexing jobs (last 5):
   job-abc  RUNNING   started 2 min ago    42% (sources 1/3)
   job-xyz  COMPLETE  2025-12-01 03:00     ok
   ...

Keys: r refresh | c add data source | d delete source | i start indexing |
      a attach to agent | x cancel job
```

Spaces data sources double-dip on the existing Spaces tab — we already have
a bucket picker we can reuse for the add-source flow.

### 3.9 Function routes (agent tools)

These let an agent call your own HTTP endpoints as tools. CRUD form fields:

```
name         my-weather-tool
description  Get weather for a city
url          https://example.com/weather
method       GET | POST
auth_header  X-API-Key: …
input_schema (JSON)
```

`input_schema` is the largest field — instead of a single-line `textinput`,
use `bubbles/textarea` (already a transitive dep) for that one input.

### 3.10 Model catalog browser

Read-only table:

```
Name                       Owner       Context  Embeds  RAG  Agreement
llama-3.3-70b-instruct     Meta        128k     no      yes  required
mistral-large-2407         Mistral     128k     no      yes  no
text-embedding-3-large     OpenAI      8k       yes     n/a  byok
...
```

Useful as a reference when filling out the agent-create form, where the
"Model UUID" today would otherwise have to be hand-copied from the web UI.
Enter on a row → "Use as agent model" → returns to the create form pre-filled.

### 3.11 Model access keys CRUD

The key that today lives in `config.json` as
`inference.model_access_key`. New screen lets you:

- List existing keys (name, prefix, created, last-used)
- Create a new key (name)
- Rotate / regenerate
- Delete
- "Use in this session" — replace the live `inferenceClient` token without
  restarting the TUI (a `m.inferenceClient` swap; existing client is
  garbage-collected)

### 3.12 BYOK key management

Two-column screen (Anthropic | OpenAI), same CRUD. Selecting a key shows:
- Which models it grants the GenAI platform access to (`/keys/{uuid}/models`)
- Which agents currently use it (`/keys/{uuid}/agents`)

### 3.13 Indexing-job watcher

A long-running concern: indexing of a large Spaces bucket can take many
minutes. Pattern:

- After `actStartIndexing`, schedule a `tea.Cmd` that polls
  `GET /indexing_jobs/{uuid}` every 5 s until status is terminal.
- Surface progress in the KB detail screen + the ops log.
- Keep a top-level "Indexing jobs" sub-state to view across all KBs.

Cancel via `PUT /indexing_jobs/{uuid}/cancel`.

---

## 4. Risks / edge cases

1. **GenAI not in godo v1.133.0.** A hand-rolled HTTP client is fine — we
   already do this for inference and Spaces (S3). Keep the JSON struct tags
   minimal and tolerant; the GenAI API is still evolving.
2. **Endpoint base differs per agent.** Agent deployments have their own
   domains. We must read the deployment URL from the agent record, not
   hard-code it.
3. **Public-endpoint agents need no API key, private ones do.** Visibility
   toggle changes auth requirements; UI must surface this clearly.
4. **Streaming + Bubble Tea.** The clean pattern is a channel-backed `tea.Cmd`
   that returns one chunk per call. Don't try to push to the program from
   inside an HTTP read goroutine — race with the model.
5. **Embeddings can be large.** A 3072-dim float64 vector prints ridiculously.
   Always summarise; provide an explicit "dump full vector" action.
6. **Pagination.** `GET /agents`, `/knowledge_bases`, `/indexing_jobs`
   page at 200 items. Loop until `links.pages.next` is empty (same pattern
   as `ListAllSnapshots` in `internal/do/resources.go`).
7. **Token scope.** A fine-grained PAT without GenAI scope will return 403
   on all new endpoints. Detect once on tab entry and show a clear message
   instead of letting every action fail.
8. **Rate limits.** Inference endpoint has its own limits (separate from
   control-plane). Catch 429, show backoff hint, don't retry-storm.
9. **Two model lists.** `/v1/models` (serverless inference) and
   `/v2/gen-ai/models` (platform catalog) are distinct. Label both tabs
   clearly so users don't think one is broken.

---

## 5. Config additions

```jsonc
{
  "ai": {
    "model_access_key": "...",              // existing
    "inference_base_url": "...",            // existing, optional
    "stream":             true,             // new: default streaming on
    "default_temperature": 0.7,             // new
    "default_max_tokens":  2048,            // new
    "chat_save_dir":       "~/.config/do-droplets-tui/chats"  // new
  }
}
```

The GenAI Platform tabs use the same `DO_TOKEN` already configured, so no
extra credential lives in the config.

---

## 6. Recommended shipping order

The slices are sized so each is one PR / one merge.

| # | Slice                                          | Effort | Notes                                                                 |
|---|------------------------------------------------|--------|------------------------------------------------------------------------|
| 1 | **Streaming inference + chat history**          | S      | No new endpoints; pure UX upgrade on what we already have.            |
| 2 | **Embeddings panel**                            | S      | Wire the existing `Embed` client method to a sub-state.               |
| 3 | **`internal/genai` skeleton + models catalog**  | S      | Get the third HTTP client landed; one read-only screen to exercise it.|
| 4 | **Model access keys CRUD**                      | S      | Immediately useful: rotate the key the TUI itself uses.               |
| 5 | **Agents list + detail + chat**                 | M      | The headline feature. Reuses the streaming chat from slice 1.         |
| 6 | **Agent create / update / delete + visibility** | M      | Multi-field form; mostly mechanical.                                  |
| 7 | **Knowledge bases list + create + delete**      | M      |                                                                       |
| 8 | **Data sources + indexing jobs + watcher**      | M      | Polling Cmd is the only new pattern.                                  |
| 9 | **Attach KB ↔ agent  +  child-agent routing**   | S      | Cross-resource pickers; reuse existing droplet/volume picker patterns.|
| 10| **Function routes (agent tools)**                | M      | Needs `bubbles/textarea` for JSON schema field.                       |
| 11| **BYOK keys (Anthropic / OpenAI)**              | S      | CRUD + linked-resources read.                                         |
| 12| **Agent API keys + versions + rollback**        | S      |                                                                       |
| 13| **Evaluation runs (preview)**                   | M      | Park until the API leaves preview.                                    |

Slices 1–4 deliver a sharp upgrade on the *existing* AI tab with no new
backend code; slices 5–9 are the actual GenAI Platform integration; 10–13
fill out the long tail.

---

## 7. Out of scope

- **App Platform AI runtimes** — covered by Apps API, not GenAI.
- **Vector databases** — DO doesn't expose its own; users bring their own
  pgvector / Pinecone outside the TUI.
- **Fine-tuning** — not in the GenAI public API today.
- **Training data uploads beyond data sources** — covered by Spaces/HTTPS
  data-source kinds.
