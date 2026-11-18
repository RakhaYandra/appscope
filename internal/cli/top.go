package cli

import (
	"fmt"
	"os"
	"strings"
	"syscall"
	"time"

	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/spf13/cobra"
)

type topModel struct {
	socketPath string
	apps       []appView
	cursor     int
	sortKey    string
	filter     textinput.Model
	filtering  bool
	confirming bool
	source     string
	status     string
	width      int
	height     int
}

type refreshMsg struct{ view view }
type tickMsg struct{}

func newTopCmd(socketPath *string) *cobra.Command {
	var interval int
	cmd := &cobra.Command{
		Use:   "top",
		Short: "Interactive TUI of the heaviest applications",
		RunE: func(cmd *cobra.Command, args []string) error {
			m := newTopModel(*socketPath)
			p := tea.NewProgram(m, tea.WithAltScreen())
			_, err := p.Run()
			return err
		},
	}
	cmd.Flags().IntVar(&interval, "limit", 50, "max applications to show")
	return cmd
}

func newTopModel(socketPath string) topModel {
	fi := textinput.New()
	fi.Placeholder = "filter app name..."
	fi.CharLimit = 64
	return topModel{
		socketPath: socketPath,
		sortKey:    "cpu",
		filter:     fi,
		height:     24,
		width:      100,
	}
}

func (m topModel) Init() tea.Cmd {
	return tea.Batch(refreshCmd(m.socketPath, 50), tickCmd())
}

func refreshCmd(socketPath string, limit int) tea.Cmd {
	return func() tea.Msg {
		return refreshMsg{fetchView(socketPath, limit)}
	}
}

func tickCmd() tea.Cmd {
	return tea.Tick(time.Second, func(time.Time) tea.Msg { return tickMsg{} })
}

func (m topModel) visibleApps() []appView {
	apps := make([]appView, len(m.apps))
	copy(apps, m.apps)
	sortApps(apps, m.sortKey)
	return filterApps(apps, m.filter.Value())
}

func (m topModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		return m, nil
	case refreshMsg:
		m.apps = msg.view.Apps
		m.source = msg.view.Source
		return m, nil
	case tickMsg:
		return m, tea.Batch(refreshCmd(m.socketPath, 50), tickCmd())
	case tea.KeyMsg:
		if m.confirming {
			switch msg.String() {
			case "t", "T":
				m.killSelected(syscall.SIGTERM)
				m.confirming = false
			case "k", "K":
				m.killSelected(syscall.SIGKILL)
				m.confirming = false
			default:
				m.confirming = false
				m.status = "kill cancelled"
			}
			return m, nil
		}
		if m.filtering {
			switch msg.String() {
			case "esc", "enter":
				m.filtering = false
				m.filter.Blur()
				return m, nil
			}
			var cmd tea.Cmd
			m.filter, cmd = m.filter.Update(msg)
			return m, cmd
		}
		switch msg.String() {
		case "q", "esc", "ctrl+c":
			return m, tea.Quit
		case "up", "k":
			if m.cursor > 0 {
				m.cursor--
			}
		case "down", "j":
			if m.cursor < len(m.visibleApps())-1 {
				m.cursor++
			}
		case "c":
			m.sortKey = "cpu"
		case "m":
			m.sortKey = "memory"
		case "d":
			m.sortKey = "disk"
		case "/":
			m.filtering = true
			return m, m.filter.Focus()
		case "K":
			if len(m.visibleApps()) > 0 {
				m.confirming = true
			}
		}
	}
	return m, nil
}

func (m *topModel) killSelected(sig syscall.Signal) {
	apps := m.visibleApps()
	if m.cursor >= len(apps) {
		return
	}
	target := apps[m.cursor]
	killed := 0
	for _, pid := range target.PIDs {
		p, err := os.FindProcess(pid)
		if err != nil {
			continue
		}
		if err := p.Signal(sig); err == nil {
			killed++
		}
	}
	m.status = fmt.Sprintf("sent %s to %d process(es) of %s", signalName(sig), killed, target.Name)
}

func signalName(sig syscall.Signal) string {
	if sig == syscall.SIGKILL {
		return "SIGKILL"
	}
	return "SIGTERM"
}

func (m topModel) View() string {
	var b strings.Builder
	b.WriteString("AppScope — Top Applications\n")
	fmt.Fprintf(&b, "   sort: %s   source: %s\n\n", m.sortKey, m.source)
	fmt.Fprintf(&b, "  %-22s %-6s %-8s %-9s %-14s\n", "NAME", "PROCS", "CPU%", "RSS", "DISK R/W")
	apps := m.visibleApps()
	maxRows := m.height - 6
	if maxRows < 1 {
		maxRows = 1
	}
	shown := apps
	if len(shown) > maxRows {
		shown = shown[:maxRows]
	}
	for i, a := range shown {
		cursor := "  "
		if i == m.cursor {
			cursor = "> "
		}
		fmt.Fprintf(&b, "%s%-22s %-6d %-8.1f %-9s %.1f/%.1f MB/s\n",
			cursor, truncate(a.Name, 22), a.ProcessCount, a.CPUPercent,
			humanBytes(a.MemoryRSSBytes),
			float64(a.DiskReadBps)/(1024*1024), float64(a.DiskWriteBps)/(1024*1024))
	}
	if len(apps) == 0 {
		b.WriteString("  (no applications)\n")
	}
	if m.filtering {
		fmt.Fprintf(&b, "\nfilter: %s\n", m.filter.View())
	}
	if m.confirming {
		apps := m.visibleApps()
		name := ""
		if m.cursor < len(apps) {
			name = apps[m.cursor].Name
		}
		fmt.Fprintf(&b, "\nKill %s: [t] SIGTERM / [k] SIGKILL / any other key cancels ", name)
	} else if m.status != "" {
		fmt.Fprintf(&b, "\n%s\n", m.status)
	}
	b.WriteString("\nj/k move  c/m/d sort  / filter  K kill  q quit\n")
	return b.String()
}

func truncate(s string, max int) string {
	if len(s) <= max {
		return s
	}
	return s[:max-1] + "…"
}
