# prui

Terminal UI for reviewing pull requests on **GitHub** (cloud + Enterprise) and **Bitbucket** (cloud + Data Center/Server).

## Features

- List and open PRs from the current git remote or an explicit target
- PR list tabs: **Open**, **Drafts**, **Merged** (merged is view-only)
- Syntax-highlighted unified diffs
- Draft inline / threaded comments, yank plain code, submit reviews
- Multi-host config (on-prem URLs, optional custom CA)
- Review status: approvals, change requests, and whether you approved
- Optional **AI summarize** (`S`) via Claude, GitHub Copilot, Codex, or OpenCode
- Device login: `prui auth login` stores a token so you don’t need to export secrets

## Install

```bash
curl -fsSL https://raw.githubusercontent.com/lum1n/prui/master/install.sh | sh
```

Installs to `~/.local/bin/prui` (override with `BINDIR=/usr/local/bin`). Requires a published [GitHub Release](https://github.com/lum1n/prui/releases).

```bash
go install github.com/lum1n/prui/cmd/prui@latest   # Go 1.26+
prui version
```

Maintainer build/release notes: [`BUILD.md`](BUILD.md).

## Quick start

```bash
mkdir -p ~/.config/prui
cp config.example.yaml ~/.config/prui/config.yaml   # edit hosts as needed

prui auth login --hostname github.com               # GitHub / GHE
prui auth status
prui                                                # list PRs for current git remote
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

  - name: ghe
    kind: github
    base_url: https://ghe.example.com
    api_url: https://ghe.example.com/api/v3
    # match_hosts:           # if SSH hostname differs from base_url
    #   - git.ghe.example.com
    # ca_cert: /path/to/corp-ca.pem

  - name: bitbucket
    kind: bitbucket_cloud
    base_url: https://bitbucket.org
    api_url: https://api.bitbucket.org/2.0
    token_env: BITBUCKET_TOKEN
    username: yourname

  - name: bitbucket-dc
    kind: bitbucket_dc
    base_url: https://bitbucket.example.com
    api_url: https://bitbucket.example.com/rest/api/1.0
    cookie_env: BB_COOKIE
    match_hosts:
      - git.bitbucket.example.com

defaults:
  host: github

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
| `prui auth login --hostname HOST --client-id ID` | Native device flow (no `gh`); needs an OAuth App on that host. |
| Env `token_env` / `cookie_env` | Override or Bitbucket cookies; a **set** cookie wins over token/store. |

Resolution order for GitHub/GHE: **env cookie → env token → stored login → `gh auth token`**.

```bash
prui auth login --hostname github.com
prui auth status
prui auth logout --hostname github.com
```

Bitbucket cookie example:

```bash
export BB_COOKIE='JSESSIONID=...; BITBUCKETSESSIONID=...'
```

### AI summarize

Press **`S`** in review or overview. The provider is chosen from config (`ai.default`) — no in-app picker.

Detail levels (config `ai.summary_detail`, or cycle with **`s`**):

| Level | Behavior |
|-------|----------|
| `short` | One paragraph only |
| `medium` | At most three paragraphs (default) |
| `full` | No length restriction |

```yaml
ai:
  default: claude
  summary_detail: medium   # short | medium | full
  max_context_bytes: 120000
  timeout_sec: 120
  providers:
    claude:
      kind: claude
      model: claude-sonnet-4-5
      token_env: ANTHROPIC_API_KEY

    copilot:
      kind: copilot
      model: gpt-4o
      # For GitHub Enterprise Copilot, set api_url or github_host:
      # api_url: https://ghe.example.com/api/v3
      # github_host: ghe

    codex:
      kind: codex
      # model: ""
      # binary: codex

    opencode:
      kind: opencode
      # model: ""
      # binary: opencode
```

If Copilot returns `The requested model is not supported`, set `model` to one your org allows (for example `gpt-4o`), or omit `model` to let prui pick.

## Usage

```bash
prui                          # list PRs for current git remote
prui owner/repo#42            # open a specific PR
prui https://github.com/owner/repo/pull/42
prui --host bitbucket-dc PROJECT/repo#7

prui auth status
prui pr list owner/repo
prui version
```

### Review keys

| Key | Action |
|-----|--------|
| `j`/`k` | Move |
| `Ctrl+d`/`Ctrl+u` | Page down / up (half screen) |
| `Tab` | PR list: next tab · Files ↔ diff |
| `←`/`→` | PR list: Open / Drafts / Merged |
| `1`/`2`/`3` | PR list: jump to Open / Drafts / Merged |
| `Enter` | Open PR / load file |
| `c` | New comment on line |
| `p` | PR overview (status, reviews, tasks, description, summary, conversation) |
| `C` | Overview focused on conversation |
| `S` | AI summarize (`ai.default`, current detail level) |
| `s` | Cycle summary detail: short → medium → full |
| `R` | Reply to selected comment (diff target / overview conversation) |
| `,` / `.` | Prev/next reply target on the cursor line |
| `1`–`9` | Jump to reply target `#N` on the cursor line |
| `e` | Edit draft on line |
| `x` | Delete draft on line |
| `v` | Range select |
| `y` | Yank plain code (cursor line or visual range; no gutters) |
| `r` | Submit review (comment / approve / request changes) |
| `[`/`]` | Prev/next hunk |
| `o` | Show PR URL |
| `O` | Open PR in browser |
| `?` | Help |
| `q` | Back / quit |

Comment editor: `enter`/`ctrl+s` save, `esc` cancel. On a threaded line, targets are numbered when focused (`▸`); use `,`/`.` or `1`–`9`, then `R`. Overview (`p`): tab through Tasks / Description / Summary / Conversation. Submit (`r`) offers Comment, Approve, and Request changes. **Merged** PRs are view-only (no comments, task toggles, or submit).

## Architecture

Forge differences are normalized behind `internal/provider.Host`. Diff parsing and highlighting live in `internal/diff`. AI completers live in `internal/ai`. Draft reviews persist under `~/.config/prui/drafts/`; login tokens under `~/.config/prui/credentials.json`.
