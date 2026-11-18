# AppScope — instruksi proyek

AppScope: lightweight Application Resource Intelligence Platform untuk Linux/Omarchy. Daemon Go membaca `/proc` + `/sys`, mengagregasi metrik per aplikasi logis (bukan per-PID), menyajikannya via CLI (Cobra, TUI Bubbletea); Omarchy QML plugin menyusul setelah P0 hijau. "One Engine, Two Interfaces."

Spesifikasi: `AppScope PRD v2.0` (dokumen di luar repo; ringkasan arsitektur di README).

## Perintah

- Build: `go build ./...`
- Test: `go test ./...` (wajib hijau sebelum klaim selesai; target coverage ≥80% di internal/engine, internal/group, internal/store)
- Lint/format: `gofmt -l . && go vet ./... && golangci-lint run ./...`
- Jalankan lokal: `go run ./cmd/appscope`

## Struktur & konvensi

- Entry point: `cmd/appscope/main.go` (CLI + `daemon` subcommand)
- Modul utama: `internal/engine` (collector procfs), `internal/group` (3-tier grouping), `internal/store` (SQLite WAL), `internal/ipc` (JSON-RPC over Unix socket), `internal/models`, `internal/cli`, `internal/tui`
- Konvensi kode: logika bisnis di paket `internal/*`, `cmd/` tipis; komentar hanya untuk invarian non-obvious; error selalu dibungkus `fmt.Errorf("...: %w", err)`
- Hal yang dilarang: dependensi baru tanpa izin user; cgo selain driver `mattn/go-sqlite3`; membaca `/proc` di luar `internal/engine`; root/sudo untuk operasi normal (read-only monitoring)

## Knowledge graph

- Default global (`docs/specs/[id-fitur]/knowledge-graph.json`) dipakai, kecuali:
- Override proyek: sama dengan default

## Plan & spec

- Lokasi plan: `docs/plans/` (format skill `writing-plans`)
- Lokasi spec: `docs/specs/`

## Skill khusus proyek

- Tambahan di luar global: tidak ada
- Skill yang JANGAN dipakai di sini: tidak ada
- Skill butuh tracker (`to-spec`, `implement`, `code-review`, `to-tickets`): aktif hanya setelah `/setup-matt-pocock-skills` per-repo
