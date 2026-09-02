# Building and releasing prui

Maintainer notes — not required for installing or using prui. Consumers should follow [`README.md`](README.md).

## Local build

Requires Go 1.26+.

```bash
make build      # ./prui with version ldflags from git
make install    # install to $(go env GOPATH)/bin
make test
make vet
```

`internal/version/version.go` ships a default `Version` for untagged `go build` / `go install .`. Release binaries get version/commit/date from GoReleaser ldflags. Prefer keeping the source default in sync with the latest tag.

## Release

Pushing a semver tag `v*` runs [`.github/workflows/release.yml`](.github/workflows/release.yml) (GoReleaser: linux/darwin × amd64/arm64).

```bash
# Working tree clean and pushed to master
git tag -a v0.1.1 -m "v0.1.1"
git push origin v0.1.1
```

After the workflow succeeds, the release appears at https://github.com/lum1n/prui/releases and `install.sh` picks up the new assets.

Optional local dry-run:

```bash
goreleaser release --snapshot --clean
```

## Install script

[`install.sh`](install.sh) downloads the latest GitHub Release tarball for the current OS/arch into `BINDIR` (default `~/.local/bin`).
