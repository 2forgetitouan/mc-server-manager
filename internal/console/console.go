package console

import (
	"bufio"
	"context"
	"fmt"
	"os/exec"
	"strings"
	"time"

	"mc-server-manager/internal/rcon"
	"mc-server-manager/internal/ui"

	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
)

type Options struct {
	RCONAddress  string
	RCONPassword string
	SystemdUnit  string
	LogLines     int
}

type Console struct {
	opts Options
}

func New(opts Options) *Console {
	return &Console{opts: opts}
}

func (c *Console) Run(_ context.Context) error {
	logCh := make(chan string, 100)

	ctx, cancel := context.WithCancel(context.Background())

	go streamLogs(ctx, c.opts.SystemdUnit, c.opts.LogLines, logCh)

	p := tea.NewProgram(
		newModel(c.opts, logCh, cancel),
		tea.WithAltScreen(),
	)
	_, err := p.Run()
	cancel()
	return err
}

func streamLogs(ctx context.Context, unit string, lines int, ch chan<- string) {
	defer close(ch)

	cmd := exec.CommandContext(ctx, "journalctl",
		"-u", unit,
		"-n", fmt.Sprintf("%d", lines),
		"--follow",
		"--no-pager",
		"--output", "cat",
	)

	stdout, err := cmd.StdoutPipe()
	if err != nil {
		ch <- ui.Error("Failed to start log stream: " + err.Error())
		return
	}

	if err := cmd.Start(); err != nil {
		ch <- ui.Error("Failed to start journalctl: " + err.Error())
		return
	}

	scanner := bufio.NewScanner(stdout)
	scanner.Buffer(make([]byte, 0, 64*1024), 1024*1024)
	for scanner.Scan() {
		select {
		case ch <- scanner.Text():
		case <-ctx.Done():
			return
		}
	}

	cmd.Wait()
}

type logMsg string
type rconResponseMsg struct {
	response string
}
type rconErrorMsg struct {
	err error
}

type model struct {
	opts       Options
	textInput  textinput.Model
	logCh      <-chan string
	cancelFunc context.CancelFunc
	logs       []string
	maxLogs    int
	history    []string
	historyIdx int
	quitting   bool
	width      int
	height     int
}

func newModel(opts Options, logCh <-chan string, cancel context.CancelFunc) model {
	ti := textinput.New()
	ti.Placeholder = "Type a command..."
	ti.Prompt = "> "
	ti.Focus()
	ti.CharLimit = 256
	ti.Width = 80

	return model{
		opts:       opts,
		textInput:  ti,
		logCh:      logCh,
		cancelFunc: cancel,
		logs:       make([]string, 0, 500),
		maxLogs:    500,
		history:    make([]string, 0),
		historyIdx: -1,
		width:      80,
		height:     24,
	}
}

func (m model) Init() tea.Cmd {
	return tea.Batch(
		textinput.Blink,
		waitForLog(m.logCh),
	)
}

func waitForLog(ch <-chan string) tea.Cmd {
	return func() tea.Msg {
		line, ok := <-ch
		if !ok {
			return nil
		}
		return logMsg(line)
	}
}

func (m model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch msg.Type {
		case tea.KeyCtrlC, tea.KeyCtrlD:
			m.quitting = true
			m.cancelFunc()
			return m, tea.Quit

		case tea.KeyEnter:
			input := strings.TrimSpace(m.textInput.Value())
			if input == "" {
				return m, nil
			}
			m.textInput.SetValue("")
			m.history = append(m.history, input)
			m.historyIdx = len(m.history)

			if input == "exit" || input == "quit" {
				m.quitting = true
				m.cancelFunc()
				return m, tea.Quit
			}

			m.addLog(ui.Highlight("> " + input))
			return m, m.sendRCONCommand(input)

		case tea.KeyUp:
			if len(m.history) > 0 && m.historyIdx > 0 {
				m.historyIdx--
				m.textInput.SetValue(m.history[m.historyIdx])
				m.textInput.CursorEnd()
			}
			return m, nil

		case tea.KeyDown:
			if m.historyIdx < len(m.history)-1 {
				m.historyIdx++
				m.textInput.SetValue(m.history[m.historyIdx])
				m.textInput.CursorEnd()
			} else {
				m.historyIdx = len(m.history)
				m.textInput.SetValue("")
			}
			return m, nil
		}

	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		m.textInput.Width = msg.Width - 4

	case logMsg:
		line := string(msg)
		colored := colorizeLogLine(line)
		m.addLog(colored)
		return m, waitForLog(m.logCh)

	case rconResponseMsg:
		if msg.response != "" {
			for _, line := range strings.Split(msg.response, "\n") {
				line = strings.TrimSpace(line)
				if line != "" {
					m.addLog(ui.Info("  " + line))
				}
			}
		}
		return m, nil

	case rconErrorMsg:
		m.addLog(ui.Error(fmt.Sprintf("  Error: %v", msg.err)))
		return m, nil
	}

	var cmd tea.Cmd
	m.textInput, cmd = m.textInput.Update(msg)
	return m, cmd
}

func (m *model) addLog(line string) {
	m.logs = append(m.logs, line)
	if len(m.logs) > m.maxLogs {
		copy(m.logs, m.logs[len(m.logs)-m.maxLogs:])
		m.logs = m.logs[:m.maxLogs]
	}
}

func (m model) View() string {
	if m.quitting {
		return ""
	}

	logHeight := m.height - 3
	if logHeight < 1 {
		logHeight = 1
	}

	var b strings.Builder

	start := 0
	if len(m.logs) > logHeight {
		start = len(m.logs) - logHeight
	}
	displayed := 0
	for i := start; i < len(m.logs); i++ {
		line := m.logs[i]
		if m.width > 0 && len(line) > m.width {
			line = line[:m.width]
		}
		b.WriteString(line)
		b.WriteString("\n")
		displayed++
	}
	for i := displayed; i < logHeight; i++ {
		b.WriteString("\n")
	}

	sep := strings.Repeat("─", m.width)
	if ui.ColorEnabled {
		sep = ui.Dim(sep)
	}
	b.WriteString(sep)
	b.WriteString("\n")
	b.WriteString(m.textInput.View())

	return b.String()
}

func (m model) sendRCONCommand(command string) tea.Cmd {
	opts := m.opts
	return func() tea.Msg {
		client, err := rcon.Dial(opts.RCONAddress, opts.RCONPassword, 5*time.Second)
		if err != nil {
			return rconErrorMsg{err: fmt.Errorf("RCON connection failed: %w", err)}
		}
		defer client.Close()

		resp, err := client.Execute(command)
		if err != nil {
			return rconErrorMsg{err: err}
		}
		return rconResponseMsg{response: resp}
	}
}

func colorizeLogLine(line string) string {
	if !ui.ColorEnabled {
		return line
	}
	lower := strings.ToLower(line)
	switch {
	case strings.Contains(lower, "error") || strings.Contains(lower, "severe"):
		return ui.Error(line)
	case strings.Contains(lower, "warn"):
		return ui.Warning(line)
	default:
		return line
	}
}
