package ui

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/lipgloss"
)

// Table renders a simple aligned table to stdout.
type Table struct {
	headers []string
	rows    [][]string
	// minWidth per column; computed from content.
	widths []int
}

// NewTable creates a Table with the given column headers.
func NewTable(headers []string) *Table {
	widths := make([]int, len(headers))
	for i, h := range headers {
		widths[i] = len(h)
	}
	return &Table{
		headers: headers,
		widths:  widths,
	}
}

// AddRow appends a row. Extra columns are silently ignored; missing columns
// are treated as empty strings.
func (t *Table) AddRow(row []string) {
	// Normalise row length to match headers.
	normalised := make([]string, len(t.headers))
	for i := range normalised {
		if i < len(row) {
			normalised[i] = row[i]
		}
	}
	t.rows = append(t.rows, normalised)
	for i, cell := range normalised {
		if len(cell) > t.widths[i] {
			t.widths[i] = len(cell)
		}
	}
}

// headerStyle is the lipgloss style applied to column headers.
var headerStyle = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("6")).Underline(true)

// Render returns the formatted table as a string.
func (t *Table) Render() string {
	if len(t.headers) == 0 {
		return ""
	}

	var b strings.Builder
	padding := "  " // column separator

	// Header row.
	for i, h := range t.headers {
		cell := padRight(h, t.widths[i])
		if ColorEnabled {
			cell = headerStyle.Render(padRight(h, t.widths[i]))
		}
		if i > 0 {
			b.WriteString(padding)
		}
		b.WriteString(cell)
	}
	b.WriteByte('\n')

	// Data rows.
	for _, row := range t.rows {
		for i, cell := range row {
			padded := padRight(cell, t.widths[i])
			if i > 0 {
				b.WriteString(padding)
			}
			b.WriteString(padded)
		}
		b.WriteByte('\n')
	}

	return b.String()
}

// String is an alias for Render so Table satisfies fmt.Stringer.
func (t *Table) String() string { return t.Render() }

// PrintTable is a convenience function that creates a table, adds the given
// rows, and prints the result to stdout.
func PrintTable(headers []string, rows [][]string) {
	t := NewTable(headers)
	for _, r := range rows {
		t.AddRow(r)
	}
	fmt.Print(t.Render())
}

// padRight pads s with spaces on the right so that its visible length is at
// least width.
func padRight(s string, width int) string {
	n := len(s)
	if n >= width {
		return s
	}
	return s + strings.Repeat(" ", width-n)
}
