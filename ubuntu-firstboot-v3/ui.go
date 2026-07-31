package main

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/lipgloss"
)

var (
	titleStyle  = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("205"))
	boxStyle    = lipgloss.NewStyle().Border(lipgloss.RoundedBorder()).Padding(1, 2)
	activeStyle = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("212"))
	mutedStyle  = lipgloss.NewStyle().Foreground(lipgloss.Color("241"))
	okStyle     = lipgloss.NewStyle().Foreground(lipgloss.Color("42"))
	warnStyle   = lipgloss.NewStyle().Foreground(lipgloss.Color("214"))
	errStyle    = lipgloss.NewStyle().Foreground(lipgloss.Color("196"))
)

func (m model) View() string {
	var body string
	switch m.screen {
	case screenMain:
		body = m.viewMain()
	case screenTools:
		body = m.viewTools()
	case screenAgents:
		body = m.viewAgents()
	case screenTimezone:
		body = m.viewTimezone()
	case screenForm:
		body = m.viewForm()
	case screenConfirm:
		body = m.viewConfirm()
	case screenInstall:
		body = m.viewInstall()
	case screenDone:
		body = m.viewDone()
	}
	w := m.width - 4
	if w < 70 {
		w = 70
	}
	return boxStyle.Width(w).Render(
		titleStyle.Render("Ubuntu First Boot V3") + "\n" +
			mutedStyle.Render("Bestehende Installationen werden erkannt; Änderungen werden erst nach Bestätigung ausgeführt.") +
			"\n\n" + body,
	)
}

func (m model) viewMain() string {
	var b strings.Builder
	b.WriteString("Operationen\n\n")
	for i, x := range m.items {
		cursor := "  "
		if i == m.cursor {
			cursor = "› "
		}
		check := "[ ]"
		if x.Selected {
			check = "[x]"
		}
		status := statusLabel(x.Status)
		line := fmt.Sprintf("%s%s %-25s %-13s %s", cursor, check, x.Title, status, mutedStyle.Render(x.Description))
		if i == m.cursor {
			line = activeStyle.Render(line)
		}
		b.WriteString(line + "\n")
	}
	b.WriteString("\n" + mutedStyle.Render("↑/↓ navigieren   Leertaste auswählen   Enter weiter   q abbrechen"))
	if m.errText != "" {
		b.WriteString("\n\n" + errStyle.Render(m.errText))
	}
	return b.String()
}

func (m model) viewTools() string {
	var b strings.Builder
	b.WriteString("Entwicklungs- und Monitoring-Werkzeuge\n\n")
	for i, x := range m.tools {
		cursor := "  "
		if i == m.toolCursor {
			cursor = "› "
		}
		check := "[ ]"
		if x.Selected {
			check = "[x]"
		}
		line := fmt.Sprintf("%s%s %-20s %-13s %s", cursor, check, x.Title, statusLabel(x.Status), mutedStyle.Render(x.Description))
		if i == m.toolCursor {
			line = activeStyle.Render(line)
		}
		b.WriteString(line + "\n")
	}
	b.WriteString("\n" + mutedStyle.Render("↑/↓ navigieren   Leertaste auswählen   Enter weiter   Esc zurück"))
	return b.String()
}

func (m model) viewAgents() string {
	var b strings.Builder
	b.WriteString("Coding Agents\n\n")
	for i, x := range m.agents {
		cursor := "  "
		if i == m.agentCursor {
			cursor = "› "
		}
		check := "[ ]"
		if x.Selected {
			check = "[x]"
		}
		line := fmt.Sprintf("%s%s %-22s %-13s %s", cursor, check, x.Title, statusLabel(x.Status), mutedStyle.Render(x.Description))
		if i == m.agentCursor {
			line = activeStyle.Render(line)
		}
		b.WriteString(line + "\n")
	}
	b.WriteString("\n" + mutedStyle.Render("↑/↓ navigieren   Leertaste auswählen   Enter weiter   Esc zurück"))
	return b.String()
}

func (m model) viewTimezone() string {
	var b strings.Builder
	b.WriteString("Zeitzone auswählen\n\n")
	for i, tz := range m.timezones {
		cursor := "  "
		if i == m.tzCursor {
			cursor = "› "
		}
		mark := " "
		if tz == m.cfg.Timezone {
			mark = "✓"
		}
		line := fmt.Sprintf("%s[%s] %s", cursor, mark, tz)
		if i == m.tzCursor {
			line = activeStyle.Render(line)
		}
		b.WriteString(line + "\n")
	}
	b.WriteString("\n" + mutedStyle.Render("↑/↓ navigieren   Enter übernehmen   Esc zurück"))
	return b.String()
}

func (m model) viewForm() string {
	if len(m.fields) == 0 {
		return "Keine zusätzlichen Angaben nötig.\n\nEnter weiter"
	}
	f := m.fields[m.fieldCursor]
	s := fmt.Sprintf("Parameter %d/%d\n\n%s\n%s\n\n%s",
		m.fieldCursor+1, len(m.fields),
		activeStyle.Render(f.Label),
		mutedStyle.Render(f.Description),
		f.Input.View(),
	)
	s += "\n\n" + mutedStyle.Render("Enter weiter   Tab/↑/↓ wechseln   Esc zurück")
	if m.errText != "" {
		s += "\n\n" + errStyle.Render(m.errText)
	}
	return s
}

func (m model) viewConfirm() string {
	var b strings.Builder
	b.WriteString("Einmalige Bestätigung\n\n")
	for _, x := range m.items {
		if x.Selected {
			b.WriteString(okStyle.Render("✓") + " " + x.Title + "\n")
		}
	}
	if m.selected(idTimezone) {
		b.WriteString("\nZeitzone: " + m.cfg.Timezone)
	}
	if m.selected(idHostname) {
		b.WriteString("\nHostname: " + m.cfg.Hostname)
	}
	if m.needsUser() {
		b.WriteString("\nBenutzer: " + m.cfg.Username + "\nPasswort: ••••••••••")
	}
	if m.selected(idSSH) {
		b.WriteString(fmt.Sprintf("\nSSH: Port %d, Root=%s, Passwort=%s, Pubkey-only=%s",
			m.cfg.SSHPort, yesNo(m.cfg.PermitRootLogin), yesNo(m.cfg.SSHPasswordAuth), yesNo(m.cfg.SSHPubkeyOnly)))
	}
	if m.selected(idSamba) {
		b.WriteString("\nSamba: " + m.cfg.SambaShareName + " → " + m.cfg.SambaPath)
	}
	if m.selected(idGitConfig) {
		b.WriteString("\nGit: " + m.cfg.GitName + " <" + m.cfg.GitEmail + ">, Branch " + m.cfg.GitDefaultBranch)
	}
	b.WriteString("\n\n" + warnStyle.Render("Nach dem Start sind keine weiteren Eingaben notwendig."))
	b.WriteString("\n\n" + mutedStyle.Render("Enter starten   Esc zurück   q abbrechen"))
	return b.String()
}

func (m model) viewInstall() string {
	head := fmt.Sprintf("%s Schritt %d/%d", m.spinner.View(), m.currentIndex, m.totalSteps)
	if m.currentStep != "" {
		head += ": " + m.currentStep
	}
	return head + "\n\n" + m.viewport.View() + "\n\n" + mutedStyle.Render("↑/↓ scrollen   Strg+C abbrechen")
}

func (m model) viewDone() string {
	var b strings.Builder
	if len(m.result.Failed) == 0 {
		b.WriteString(okStyle.Render("Setup erfolgreich abgeschlossen") + "\n\n")
	} else {
		b.WriteString(warnStyle.Render("Setup mit Fehlern abgeschlossen") + "\n\n")
	}
	for _, x := range m.result.Completed {
		b.WriteString(okStyle.Render("✓") + " " + x + "\n")
	}
	for _, x := range m.result.Failed {
		b.WriteString(errStyle.Render("✗") + " " + x + "\n")
	}

	b.WriteString("\nHostname: " + currentHostname())
	b.WriteString("\nServer-IP: " + m.result.IP)
	if m.needsUser() {
		b.WriteString("\nBenutzer: " + m.cfg.Username)
	}
	if m.selected(idSSH) && m.result.IP != "" {
		b.WriteString("\n\nSSH:\n" + activeStyle.Render(fmt.Sprintf("ssh -p %d %s@%s", m.cfg.SSHPort, m.cfg.Username, m.result.IP)))
	}
	if m.selected(idSamba) && m.result.IP != "" {
		b.WriteString("\n\nSamba:\n" + activeStyle.Render(fmt.Sprintf(`\\%s\%s`, m.result.IP, m.cfg.SambaShareName)))
	}
	if m.result.ReportPath != "" {
		b.WriteString("\n\nSetup-README:\n" + activeStyle.Render(m.result.ReportPath))
	}
	if len(m.result.Versions) > 0 {
		b.WriteString("\n\nVersionen:\n")
		for k, v := range m.result.Versions {
			b.WriteString(fmt.Sprintf("  %-12s %s\n", k, v))
		}
	}
	b.WriteString("\n" + mutedStyle.Render("Enter oder q zum Beenden"))
	return b.String()
}

func statusLabel(s statusInfo) string {
	if !s.Checked {
		return mutedStyle.Render("prüfe …")
	}
	if s.Present {
		if s.Detail != "" {
			return okStyle.Render("vorhanden")
		}
		return okStyle.Render("vorhanden")
	}
	return warnStyle.Render("fehlt")
}
