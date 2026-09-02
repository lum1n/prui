# prui

Terminal UI for reviewing pull requests on **GitHub** (cloud + Enterprise) and **Bitbucket** (cloud + Data Center/Server).

## Features

- List and open PRs from the current git remote or an explicit target
- Syntax-highlighted unified diffs (Chroma; pierre-inspired line annotations)
- Draft inline comments on selected lines (range select where the host supports it)
- Submit reviews: comment, approve, request changes (GitHub); comment + approve (Bitbucket)
- Multi-host config with on-prem base URLs and optional custom CA

## Install

```bash
go install github.com/vegard/prui/cmd/prui@latest
# or from this repo:
go build -o prui ./cmd/prui
```

## Config

`~/.config/prui/config.yaml`:

```yaml
hosts:
  - name: github
    kind: github
    base_url: https://github.com
    api_url: https://api.github.com/
    token_env: GITHUB_TOKEN

  - name: work-ghe
    kind: github
    base_url: https://ghe.example.com
    api_url: https://ghe.example.com/api/v3
    cookie_env: GHE_COOKIE
    # token_env: GHE_TOKEN   # optional fallback if cookies are not used
    ca_cert: /path/to/corp-ca.pem

  - name: bitbucket
    kind: bitbucket_cloud
    base_url: https://bitbucket.org
    api_url: https://api.bitbucket.org/2.0
    token_env: BITBUCKET_TOKEN
    username: yourname   # required for app passwords

  - name: work-bb
    kind: bitbucket_dc
    base_url: https://bitbucket.example.com
    api_url: https://bitbucket.example.com/rest/api/1.0
    cookie_env: BB_COOKIE
    # token_env: BB_TOKEN
    # username: yourname

defaults:
  host: work-ghe

ui:
  diff: unified
  theme: dark
```

Auth is never stored in the config file. For on-prem hosts without API tokens, set `cookie_env` to an env var that holds a browser session `Cookie` header value (DevTools → Network → copy request Cookie). A leading `Cookie:` prefix is stripped if present.

```bash
# Example: paste session cookies from the browser
export GHE_COOKIE='user_session=...; _gh_sess=...; logged_in=yes'
export BB_COOKIE='JSESSIONID=...; BITBUCKETSESSIONID=...'
prui auth status
```

When both `cookie_env` and `token_env` are configured, a set cookie wins. For `github.com` without `cookie_env`, prui still falls back to `gh auth token`.

## Usage

```bash
export GHE_TOKEN=...
prui                          # list open PRs for current git remote
prui owner/repo#42            # open a specific PR
prui https://ghe.example.com/org/repo/pull/42
prui --host work-bb PROJECT/repo#7

prui auth status
prui pr list owner/repo
```

### Review keys

| Key | Action |
|-----|--------|
| `j`/`k` | Move |
| `Tab` | Files ↔ diff |
| `Enter` | Open PR / load file |
| `c` | Comment on line |
| `v` | Range select |
| `r` | Submit review |
| `[`/`]` | Prev/next hunk |
| `o` | Show PR URL |
| `?` | Help |
| `q` | Back / quit |

Comment editor: `ctrl+s` to save draft, `esc` to cancel.

## Architecture

Forge differences are normalized behind `internal/provider.Host`. Diff parsing and highlighting live in `internal/diff`. Draft reviews persist under `~/.config/prui/drafts/`.
