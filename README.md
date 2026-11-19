# AppScope

Lightweight Application Resource Intelligence Platform for Linux & Omarchy Desktop.

Standard tools (`top`, `btop`) show resources per PID. Modern apps (Firefox, VS Code, Electron) spawn dozens of processes. AppScope groups them into logical applications and reports real CPU, RSS/VMS, and disk I/O per app — via a terminal CLI first, an Omarchy desktop plugin next.

## Architecture

```
Linux procfs/sysfs → Core Engine daemon (Go) → Unix socket IPC (JSON-RPC) → CLI + Omarchy Plugin
```

SQLite (WAL) stores historical snapshots; `appscope history` aggregates them into time-bucketed averages on the fly (multi-tier rollup per PRD is not implemented yet).

## Development

```sh
go build ./...          # build everything
go test ./...           # test suite (green before merging)
golangci-lint run ./... # lint (zero issues tolerated)
go run ./cmd/appscope   # run the CLI
```

## Status

v0.1.0 — CLI P0 MVP + P1 (overview, top, inspect, history, report, alert) done; released via GitHub Release. Omarchy QML plugin planned next.

## Install

**From GitHub Release (x86_64):**

```sh
curl -L https://github.com/RakhaYandra/appscope/releases/download/v0.1.0/appscope-linux-amd64 -o ~/.local/bin/appscope
chmod +x ~/.local/bin/appscope
mkdir -p ~/.config/systemd/user ~/.local/share/appscope
sed 's|/usr/bin/appscope|'"$HOME"'/.local/bin/appscope|' packaging/appscoped.service > ~/.config/systemd/user/appscoped.service
systemctl --user daemon-reload
systemctl --user enable --now appscoped.service
```

**From source:** `go build -o ~/.local/bin/appscope ./cmd/appscope`

**Arch Linux:** `packaging/PKGBUILD` builds `appscope-cli` (binary + systemd user unit at `/usr/lib/systemd/user/appscoped.service`). AUR submission pending.

## Usage

```sh
go build -o ~/.local/bin/appscope ./cmd/appscope
appscope                 # system overview + top apps
appscope top             # interactive TUI (j/k, c/m/d sort, / filter, K kill, q quit)
appscope inspect <app>   # deep inspection + process tree
appscope daemon run      # foreground collector (used by systemd)
appscope daemon status   # via systemctl --user
```

Systemd user service: see Install. The service deliberately avoids mount-namespace sandboxing — it makes `/proc/<pid>/io` unreadable and kills per-app disk I/O. AppScope is read-only and runs unprivileged.

CLI uses the daemon's Unix socket when it is running, and falls back to a one-shot local sample otherwise.
