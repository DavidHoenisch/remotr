package main

import (
	"fmt"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

type queryViewerModel struct {
	queryName string
	rows      []flattenedQueryRow
	cols      []string
	grouped   bool
	sortDesc  bool
	filter    string
	filtering bool
	cursor    int
	width     int
	height    int
}

func newQueryViewerModel(queryName string, rows []flattenedQueryRow, cols []string) queryViewerModel {
	return queryViewerModel{
		queryName: queryName,
		rows:      rows,
		cols:      cols,
		width:     120,
		height:    30,
	}
}

func runQueryResultsViewer(queryName string, rows []flattenedQueryRow, cols []string) error {
	m := newQueryViewerModel(queryName, rows, cols)
	p := tea.NewProgram(m, tea.WithAltScreen())
	_, err := p.Run()
	return err
}

func (m queryViewerModel) Init() tea.Cmd { return nil }

func (m queryViewerModel) filteredRows() []flattenedQueryRow {
	if m.filter == "" {
		return m.rows
	}
	needle := strings.ToLower(m.filter)
	out := make([]flattenedQueryRow, 0, len(m.rows))
	for _, r := range m.rows {
		if strings.Contains(strings.ToLower(r.EndpointID), needle) {
			out = append(out, r)
			continue
		}
		for _, v := range r.Values {
			if strings.Contains(strings.ToLower(v), needle) {
				out = append(out, r)
				break
			}
		}
	}
	return out
}

func (m queryViewerModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width, m.height = msg.Width, msg.Height
	case tea.KeyMsg:
		if m.filtering {
			switch msg.String() {
			case "esc", "enter":
				m.filtering = false
			case "backspace":
				if len(m.filter) > 0 {
					m.filter = m.filter[:len(m.filter)-1]
				}
			default:
				if len(msg.Runes) > 0 {
					m.filter += string(msg.Runes)
					m.cursor = 0
				}
			}
			return m, nil
		}
		switch msg.String() {
		case "q", "ctrl+c":
			return m, tea.Quit
		case "/":
			m.filtering = true
		case "g":
			m.grouped = !m.grouped
			m.cursor = 0
		case "s":
			m.sortDesc = !m.sortDesc
		case "j", "down":
			m.cursor++
		case "k", "up":
			if m.cursor > 0 {
				m.cursor--
			}
		}
	}
	return m, nil
}

func (m queryViewerModel) View() string {
	titleStyle := lipgloss.NewStyle().Bold(true)
	helpStyle := lipgloss.NewStyle().Faint(true)

	modeLabel := "raw"
	if m.grouped {
		modeLabel = "grouped"
	}
	filtered := m.filteredRows()

	var b strings.Builder
	b.WriteString(titleStyle.Render(fmt.Sprintf("remotr query results · %s [%s · %d rows]", m.queryName, modeLabel, len(filtered))))
	b.WriteByte('\n')
	if m.filtering {
		b.WriteString(fmt.Sprintf("filter: %s_\n", m.filter))
	} else if m.filter != "" {
		b.WriteString(fmt.Sprintf("filter: %s (press / to edit)\n", m.filter))
	}
	b.WriteString(helpStyle.Render("keys: / filter · g toggle raw/grouped · s toggle sort · j/k scroll · q quit"))
	b.WriteString("\n\n")

	maxLines := m.height - 6
	if maxLines < 5 {
		maxLines = 5
	}

	if m.grouped {
		order := "asc"
		if m.sortDesc {
			order = "desc"
		}
		groups := aggregateQueryRows(filtered, order)
		if m.cursor >= len(groups) && len(groups) > 0 {
			m.cursor = len(groups) - 1
		}
		for i := m.cursor; i < len(groups) && i < m.cursor+maxLines; i++ {
			g := groups[i]
			b.WriteString(fmt.Sprintf("[%3d] %-24s %s\n", g.Count, truncateCell(g.SampleEndpoints, 24), formatKVLine(g.Values)))
		}
	} else {
		if m.cursor >= len(filtered) && len(filtered) > 0 {
			m.cursor = len(filtered) - 1
		}
		for i := m.cursor; i < len(filtered) && i < m.cursor+maxLines; i++ {
			r := filtered[i]
			b.WriteString(fmt.Sprintf("%-24s %s\n", truncateCell(r.EndpointID, 24), formatKVLine(r.Values)))
		}
	}
	return b.String()
}
