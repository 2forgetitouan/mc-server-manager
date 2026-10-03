package ui

import (
	"fmt"
	"os"
	"strings"

	"github.com/charmbracelet/lipgloss"
	"golang.org/x/term"
)

// ColorEnabled controls whether styled output is produced. When false, all
// formatting functions return plain text.
var ColorEnabled = true

// Predefined lipgloss styles. They are initialised once colors are resolved but
// are safe to call even before InitColors (they just check ColorEnabled).
var (
	boldStyle      = lipgloss.NewStyle().Bold(true)
	dimStyle       = lipgloss.NewStyle().Faint(true)
	successStyle   = lipgloss.NewStyle().Foreground(lipgloss.Color("2")) // green
	warningStyle   = lipgloss.NewStyle().Foreground(lipgloss.Color("3")) // yellow
	errorStyle     = lipgloss.NewStyle().Foreground(lipgloss.Color("1")) // red
	infoStyle      = lipgloss.NewStyle().Foreground(lipgloss.Color("6")) // cyan
	highlightStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("5")) // magenta
	keyStyle       = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("6"))
)

// InitColors decides whether colors should be used based on the NO_COLOR
// environment variable, whether stdout is a terminal, and the explicit color
// flag ("auto", "always", or "never").
func InitColors(colorFlag string) {
	switch strings.ToLower(colorFlag) {
	case "always":
		ColorEnabled = true
	case "never":
		ColorEnabled = false
	default: // "auto" or empty
		// Respect the NO_COLOR convention (https://no-color.org).
		if _, ok := os.LookupEnv("NO_COLOR"); ok {
			ColorEnabled = false
			return
		}
		ColorEnabled = term.IsTerminal(int(os.Stdout.Fd()))
	}
}

// render applies a lipgloss style only when colors are enabled.
func render(style lipgloss.Style, s string) string {
	if !ColorEnabled {
		return s
	}
	return style.Render(s)
}

// Bold returns s in bold.
func Bold(s string) string { return render(boldStyle, s) }

// Dim returns s in a dimmed / faint style.
func Dim(s string) string { return render(dimStyle, s) }

// Success returns s in green (indicating success).
func Success(s string) string { return render(successStyle, s) }

// Warning returns s in yellow (indicating a warning).
func Warning(s string) string { return render(warningStyle, s) }

// Error returns s in red (indicating an error).
func Error(s string) string { return render(errorStyle, s) }

// Info returns s in cyan (informational).
func Info(s string) string { return render(infoStyle, s) }

// Highlight returns s in magenta (for emphasis).
func Highlight(s string) string { return render(highlightStyle, s) }

// StatusIcon returns a coloured status indicator for common service states.
func StatusIcon(state string) string {
	switch strings.ToLower(state) {
	case "running", "active":
		return Success("●")
	case "stopped", "inactive", "dead":
		return Error("○")
	case "starting", "activating":
		return Warning("◐")
	case "stopping", "deactivating":
		return Warning("◑")
	case "failed":
		return Error("✗")
	default:
		return Dim("?")
	}
}

// FormatKV formats a key-value pair for terminal display.
func FormatKV(key, value string) string {
	if ColorEnabled {
		return fmt.Sprintf("  %s  %s", keyStyle.Render(key+":"), value)
	}
	return fmt.Sprintf("  %s: %s", key, value)
}
