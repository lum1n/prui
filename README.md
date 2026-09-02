# prui

Terminal UI for reviewing pull requests on **GitHub** (cloud + Enterprise) and **Bitbucket** (cloud + Data Center/Server).

## Features

- List and open PRs from the current git remote or an explicit target
- Syntax-highlighted unified diffs (Chroma; pierre-inspired line annotations)
- Draft inline / threaded comments, yank plain code, submit reviews
- Multi-host config (on-prem URLs, optional custom CA)
- Optional **AI summarize** (`S`) via Claude, GitHub Copilot (cloud/GHE), Codex, or OpenCode
- Device login: `prui auth login` stores a token so you don’t need to export secrets

## Install

```bash
curl -fsSL https://raw.githubusercontent.com/lum1n/prui/master/install.sh | sh
```

Installs to `~/.local/bin/prui` (override with `BINDIR=/usr/local/bin`). Requires a published GitHub Release.

```bash
go install github.com/lum1n/prui/cmd/prui@latest   # compile from source (Go 1.26+)
make install                                       # from a clone
prui version
```

### Cutting a release

Push a semver tag; GitHub Actions runs GoReleaser (linux/darwin × amd64/arm64):

```bash
git tag v0.1.0
git push origin v0.1.0
```

Update `internal/version/version.go`’s default `Version` when you want untagged `go build` to report the same number.

## Quick start

```bash
# 1. Config — copy and edit
mkdir -p ~/.config/prui
cp config.example.yaml ~/.config/prui/config.yaml

# 2. Log in to each GitHub/GHE host you use (device/browser via gh)
prui auth login --hostname github.com
prui auth login --hostname ghe.example.com

prui auth status
prui                          # list open PRs for current git remote
```

## Config

Path: `~/.config/prui/config.yaml` (see [`config.example.yaml`](config.example.yaml)).

### Forge hosts

```yaml
hosts:
  - name: github
    kind: github
    base_url: https://github.com
    api_url: https://api.github.com/

  - name: work-ghe
    kind: github
    base_url: https://ghe.example.com
    api_url: https://ghe.example.com/api/v3
    # match_hosts:           # if SSH hostname differs from base_url
    #   - git.ghe.example.com
    # ca_cert: /path/to/corp-ca.pem

  - name: work-bb
    kind: bitbucket_dc
    base_url: https://bitbucket.example.com
    api_url: https://bitbucket.example.com/rest/api/1.0
    cookie_env: BB_COOKIE    # Bitbucket DC often needs session cookies
    match_hosts:
      - git-ssh.example.com

defaults:
  host: work-bb

ui:
  diff: unified      # unified | split
  files: selected    # selected | all
  theme: dark
```

For GitHub/GHE you usually **omit** `token_env` / `cookie_env` and use `prui auth login` instead.

### Authentication

Secrets are never stored in the YAML file.

| Method | When to use |
|--------|-------------|
| `prui auth login --hostname HOST` | Preferred for GitHub.com / GHE. Saves token to `~/.config/prui/credentials.json` (0600). |
| `prui auth login --host NAME` | Same, using a `hosts[].name` from config. |
| `prui auth login --hostname HOST --client-id ID` | Native device flow (no `gh`); needs an OAuth App on that GHE. |
| Env `token_env` / `cookie_env` | Override or Bitbucket cookies; a **set** cookie wins over token/store. |

Resolution order for GitHub/GHE: **env cookie → env token → stored login → `gh auth token`**.

```bash
prui auth login --hostname ghe.example.com
prui auth status
prui auth logout --hostname ghe.example.com
```

Bitbucket cookie example:

```bash
export BB_COOKIE='JSESSIONID=...; BITBUCKETSESSIONID=...'
```

### AI summarize

Press **`S`** in review or overview. Provider is chosen only from config (`ai.default`) — no in-app picker.

```yaml
ai:
  default: copilot-ghe          # name under providers
  max_context_bytes: 120000
  timeout_sec: 120
  providers:
    claude:
      kind: claude
      model: claude-sonnet-4-5
      token_env: ANTHROPIC_API_KEY

    # GitHub.com Copilot (uses github.com login / GITHUB_TOKEN / gh)
    copilot:
      kind: copilot
      model: gpt-4o             # or omit to auto-pick; gpt-4.1 may be unavailable on some SKUs

    # GHE Copilot while reviewing Bitbucket (no GHE hosts entry required)
    copilot-ghe:
      kind: copilot
      model: gpt-4o
      api_url: https://ghe.example.com/api/v3
      # Or reuse a forge host:
      # github_host: work-ghe

    codex:
      kind: codex
      # model: ""               # optional --model for `codex exec`
      # binary: codex

    opencode:
      kind: opencode
      # model: ""
      # binary: opencode
```

**Copilot + GHE (typical with Bitbucket as the forge):**

1. Set `api_url` (or `github_host`) on the provider as above.
2. `prui auth login --hostname ghe.example.com`
3. Open a PR → `S`

If you see `The requested model is not supported`, change `model` to one your org allows (often `gpt-4o` or `gpt-4o-mini`), or leave `model` empty once auto-pick is available in your build.

## Usage

```bash
prui                          # list open PRs for current git remote
prui owner/repo#42            # open a specific PR
prui https://ghe.example.com/org/repo/pull/42
prui --host work-bb PROJECT/repo#7

prui auth login --hostname ghe.example.com
prui auth status
prui pr list owner/repo
prui version
```

### Review keys

| Key | Action |
|-----|--------|
| `j`/`k` | Move |
| `Ctrl+d`/`Ctrl+u` | Page down / up (half screen) |
| `Tab` | Files ↔ diff |
| `Enter` | Open PR / load file |
| `c` | New comment on line |
| `p` | PR overview (status, tasks, description, summary, conversation) |
| `C` | Overview focused on conversation |
| `S` | AI summarize (`ai.default`) |
| `R` | Reply to selected comment (diff target / overview conversation) |
| `,` / `.` | Prev/next reply target on the cursor line |
| `1`–`9` | Jump to reply target `#N` on the cursor line |
| `e` | Edit draft on line |
| `x` | Delete draft on line |
| `v` | Range select |
| `y` | Yank plain code (cursor line or visual range; no gutters) |
| `r` | Submit review |
| `[`/`]` | Prev/next hunk |
| `o` | Show PR URL |
| `O` | Open PR in browser |
| `?` | Help |
| `q` | Back / quit |

Comment editor: `enter`/`ctrl+s` save, `esc` cancel. On a threaded line, targets are numbered when focused (`▸`); use `,`/`.` or `1`–`9`, then `R`. Overview (`p`): `tab` switches Tasks / Description / Summary / Conversation; on Tasks, `space`/`enter` toggles; press `S` to summarize. Yank (`y`): source text only for the cursor line or `v` range.

## Architecture

Forge differences are normalized behind `internal/provider.Host`. Diff parsing and highlighting live in `internal/diff`. AI completers live in `internal/ai`. Draft reviews persist under `~/.config/prui/drafts/`; login tokens under `~/.config/prui/credentials.json`.
