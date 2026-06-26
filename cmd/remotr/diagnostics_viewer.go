package main

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"fmt"
	"io"
	"sort"
	"strings"

	"github.com/charmbracelet/bubbles/list"
	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/sahilm/fuzzy"
)

type diagFileItem struct {
	name    string
	content string
}

func (i diagFileItem) Title() string       { return i.name }
func (i diagFileItem) Description() string { return fmt.Sprintf("%d bytes", len(i.content)) }
func (i diagFileItem) FilterValue() string { return i.name }

type diagnosticsViewerModel struct {
	files            map[string]string
	names            []string
	list             list.Model
	viewport         viewport.Model
	mode             string // list | view
	filter           string
	filtering        bool
	viewName         string
	viewFullContent  string
	contentQuery     string
	contentSearching bool
	width            int
	height           int
	err              error
}

func runDiagnosticsViewer(bundle []byte) error {
	files, err := extractTarGz(bundle)
	if err != nil {
		return fmt.Errorf("open diagnostics bundle: %w", err)
	}
	names := make([]string, 0, len(files))
	for name := range files {
		names = append(names, name)
	}
	sort.Strings(names)

	items := make([]list.Item, len(names))
	for i, name := range names {
		items[i] = diagFileItem{name: name, content: files[name]}
	}

	delegate := list.NewDefaultDelegate()
	l := list.New(items, delegate, 0, 0)
	l.Title = "Diagnostics"
	l.SetShowStatusBar(true)
	l.SetFilteringEnabled(true)
	l.Styles.Title = lipgloss.NewStyle().Bold(true)
	l.SetStatusBarItemName("file", "files")

	vp := viewport.New(0, 0)

	m := diagnosticsViewerModel{
		files: files,
		names: names,
		list:  l,
		viewport: vp,
		mode:  "list",
	}
	p := tea.NewProgram(m, tea.WithAltScreen())
	final, err := p.Run()
	if err != nil {
		return err
	}
	if fm, ok := final.(diagnosticsViewerModel); ok && fm.err != nil {
		return fm.err
	}
	return nil
}

func (m diagnosticsViewerModel) Init() tea.Cmd { return nil }

func (m diagnosticsViewerModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		m.list.SetWidth(msg.Width)
		m.list.SetHeight(msg.Height - 2)
		m.viewport.Width = msg.Width
		m.viewport.Height = msg.Height - 2
	case tea.KeyMsg:
		switch msg.String() {
		case "ctrl+c":
			return m, tea.Quit
		case "q":
			if m.mode == "view" {
				if m.contentSearching {
					m.clearContentSearch()
					return m, nil
				}
				m.mode = "list"
				return m, nil
			}
			return m, tea.Quit
		case "esc":
			if m.mode == "view" {
				if m.contentSearching {
					m.clearContentSearch()
					return m, nil
				}
				m.mode = "list"
				return m, nil
			}
		case "enter":
			if m.mode == "list" {
				if item, ok := m.list.SelectedItem().(diagFileItem); ok {
					m.openView(item.name, item.content)
				}
				return m, nil
			}
		case "/":
			if m.mode == "list" {
				m.filtering = true
				m.filter = ""
				return m, nil
			}
			if m.mode == "view" {
				m.contentSearching = true
				m.contentQuery = ""
				return m, nil
			}
		case "backspace":
			if m.mode == "list" && m.filtering && len(m.filter) > 0 {
				m.filter = m.filter[:len(m.filter)-1]
				m.applyListFilter()
				return m, nil
			}
			if m.mode == "view" && m.contentSearching && len(m.contentQuery) > 0 {
				m.contentQuery = m.contentQuery[:len(m.contentQuery)-1]
				m.applyContentFilter()
				return m, nil
			}
		default:
			if m.mode == "list" && m.filtering && len(msg.Runes) > 0 && msg.Type == tea.KeyRunes {
				m.filter += string(msg.Runes)
				m.applyListFilter()
				return m, nil
			}
			if m.mode == "view" && m.contentSearching && len(msg.Runes) > 0 && msg.Type == tea.KeyRunes {
				m.contentQuery += string(msg.Runes)
				m.applyContentFilter()
				return m, nil
			}
		}
	}

	var cmd tea.Cmd
	if m.mode == "list" {
		m.list, cmd = m.list.Update(msg)
	} else if !m.contentSearching {
		m.viewport, cmd = m.viewport.Update(msg)
	}
	return m, cmd
}

func (m *diagnosticsViewerModel) openView(name, content string) {
	m.viewName = name
	m.viewFullContent = content
	m.contentQuery = ""
	m.contentSearching = false
	m.viewport.SetContent(content)
	m.viewport.GotoTop()
	m.mode = "view"
}

func (m *diagnosticsViewerModel) clearContentSearch() {
	m.contentSearching = false
	m.contentQuery = ""
	m.viewport.SetContent(m.viewFullContent)
	m.viewport.GotoTop()
}

func (m *diagnosticsViewerModel) applyListFilter() {
	if strings.TrimSpace(m.filter) == "" {
		items := make([]list.Item, len(m.names))
		for i, name := range m.names {
			items[i] = diagFileItem{name: name, content: m.files[name]}
		}
		m.list.SetItems(items)
		return
	}
	matches := fuzzy.Find(m.filter, m.names)
	items := make([]list.Item, 0, len(matches))
	for _, match := range matches {
		name := m.names[match.Index]
		items = append(items, diagFileItem{name: name, content: m.files[name]})
	}
	m.list.SetItems(items)
}

func (m *diagnosticsViewerModel) applyContentFilter() {
	m.viewport.SetContent(filterContentLines(m.contentQuery, m.viewFullContent))
	m.viewport.GotoTop()
}

func filterContentLines(query, content string) string {
	if strings.TrimSpace(query) == "" {
		return content
	}
	lines := strings.Split(content, "\n")
	matches := fuzzy.Find(query, lines)
	if len(matches) == 0 {
		return "(no matches)"
	}
	var b strings.Builder
	for _, match := range matches {
		fmt.Fprintf(&b, "%5d │ %s\n", match.Index+1, lines[match.Index])
	}
	return strings.TrimRight(b.String(), "\n")
}

func (m diagnosticsViewerModel) View() string {
	if m.mode == "view" {
		header := lipgloss.NewStyle().Bold(true).Render(m.viewName + " · / search · esc clear · q back")
		if m.contentSearching {
			header += "\nsearch: " + m.contentQuery + "_"
		}
		return header + "\n" + m.viewport.View()
	}
	help := "↑/↓ navigate · / filter · enter view · q quit"
	if m.filtering {
		help = "filter: " + m.filter + "_"
	}
	return m.list.View() + "\n" + help
}

func extractTarGz(data []byte) (map[string]string, error) {
	gz, err := gzip.NewReader(bytes.NewReader(data))
	if err != nil {
		return nil, err
	}
	defer gz.Close()
	tr := tar.NewReader(gz)
	out := make(map[string]string)
	for {
		hdr, err := tr.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			return nil, err
		}
		if hdr.Typeflag != tar.TypeReg {
			continue
		}
		raw, err := io.ReadAll(io.LimitReader(tr, 10<<20))
		if err != nil {
			return nil, err
		}
		out[hdr.Name] = string(raw)
	}
	return out, nil
}
