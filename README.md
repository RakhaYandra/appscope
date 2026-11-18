# AppScope

Lightweight Application Resource Intelligence Platform for Linux & Omarchy Desktop.

Standard tools (`top`, `btop`) show resources per PID. Modern apps (Firefox, VS Code, Electron) spawn dozens of processes. AppScope groups them into logical applications and reports real CPU, RSS/VMS, and disk I/O per app — via a terminal CLI first, an Omarchy desktop plugin next.

## Architecture

```
Linux procfs/sysfs → Core Engine daemon (Go) → Unix socket IPC (JSON-RPC) → CLI + Omarchy Plugin
```

SQLite (WAL) stores historical snapshots with multi-tier rollup (1 Hz raw ≤24 h → per-minute avg/max/min ≤7 d → purge).

See `AGENTS.md` for build/test commands and project conventions. PRD v2.0 is the single source of truth for requirements; implementation starts with P0 (core engine + IPC contract), then the QML plugin.

## Status

Early development — P0 MVP in progress.

## Usage

```sh
go build -o ~/.local/bin/appscope ./cmd/appscope
appscope                 # system overview + top apps
appscope top             # interactive TUI (j/k, c/m/d sort, / filter, K kill, q quit)
appscope inspect <app>   # deep inspection + process tree
appscope daemon run      # foreground collector (used by systemd)
appscope daemon status   # via systemctl --user
```

Systemd user service: `cp packaging/appscoped.service ~/.config/systemd/user/ && systemctl --user enable --now appscoped`. The service deliberately avoids mount-namespace sandboxing — it makes `/proc/<pid>/io` unreadable and kills per-app disk I/O. AppScope is read-only and runs unprivileged.

CLI uses the daemon's Unix socket when it is running, and falls back to a one-shot local sample otherwise.
