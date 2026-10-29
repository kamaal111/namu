// Package ui renders the optional branch picker to stderr.
package ui

import (
	"context"
	"errors"
	"fmt"
	"io"
	"strings"
	"unicode"

	"charm.land/bubbles/v2/textinput"
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	git "github.com/kamaal111/namu/internal/git"
)

type model struct {
	branches              []git.Branch
	input                 textinput.Model
	cursor, width, height int
	selected              string
	canceled              bool
}

func newModel(branches []git.Branch) model {
	input := textinput.New()
	input.Placeholder = "Search branches"
	input.Focus()
	return model{branches: branches, input: input, width: 80, height: 24}
}
func matches(label, query string) bool {
	remaining := []rune(strings.ToLower(query))
	for _, r := range label {
		if len(remaining) > 0 && unicode.ToLower(r) == remaining[0] {
			remaining = remaining[1:]
		}
	}
	return len(remaining) == 0
}
func (m model) filtered() []git.Branch {
	var result []git.Branch
	for _, b := range m.branches {
		if matches(b.Label(), m.input.Value()) {
			result = append(result, b)
		}
	}
	return result
}
func (m model) Init() tea.Cmd { return textinput.Blink }
func (m model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch v := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = v.Width
		m.height = v.Height
		m.input.SetWidth(max(1, v.Width-4))
	case tea.KeyPressMsg:
		switch v.String() {
		case "esc":
			m.canceled = true
			return m, tea.Quit
		case "ctrl+c":
			return m, tea.Interrupt
		case "up", "ctrl+p":
			m.cursor = max(0, m.cursor-1)
			return m, nil
		case "down", "ctrl+n":
			m.cursor = min(max(0, len(m.filtered())-1), m.cursor+1)
			return m, nil
		case "enter":
			branches := m.filtered()
			if len(branches) > 0 {
				m.selected = branches[min(m.cursor, len(branches)-1)].Ref
				return m, tea.Quit
			}
			return m, nil
		}
	}
	var cmd tea.Cmd
	m.input, cmd = m.input.Update(msg)
	m.cursor = min(m.cursor, max(0, len(m.filtered())-1))
	return m, cmd
}
func (m model) View() tea.View {
	var b strings.Builder
	b.WriteString("namu · Open branch\n" + m.input.View() + "\n\n")
	branches := m.filtered()
	rows := max(1, m.height-7)
	start := max(0, m.cursor-rows+1)
	for i := start; i < min(len(branches), start+rows); i++ {
		branch := branches[i]
		marker := "  "
		if i == m.cursor {
			marker = "> "
		}
		kind := "local"
		if branch.Remote != "" {
			kind = "remote"
		}
		fmt.Fprintf(&b, "%s%s · %s\n", marker, branch.Label(), kind)
	}
	if len(branches) == 0 {
		b.WriteString("No matching branches\n")
	}
	b.WriteString("\n↑/↓ move · Enter open · Esc back · Ctrl+C cancel")
	view := tea.NewView(lipgloss.NewStyle().MaxWidth(max(1, m.width)).MaxHeight(max(1, m.height)).Render(b.String()))
	view.AltScreen = true
	return view
}
func Pick(ctx context.Context, branches []git.Branch, input io.Reader, output io.Writer) (string, error) {
	if len(branches) == 0 {
		return "", fmt.Errorf("no branches available")
	}
	result, err := tea.NewProgram(newModel(branches), tea.WithContext(ctx), tea.WithInput(input), tea.WithOutput(output)).Run()
	if err != nil {
		if ctx.Err() != nil {
			return "", ctx.Err()
		}
		if errors.Is(err, tea.ErrInterrupted) {
			return "", context.Canceled
		}
		return "", err
	}
	picked, ok := result.(model)
	if !ok {
		return "", fmt.Errorf("unexpected branch picker state")
	}
	return picked.selected, nil
}
