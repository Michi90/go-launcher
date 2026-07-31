package main

import (
	"fmt"
	"strings"
	"sync"
	"time"

	tea "github.com/charmbracelet/bubbletea"
)

func buildInstallSteps(m model) []installStep {
	var steps []installStep
	add := func(name string, run func(*installContext) error) {
		steps = append(steps, installStep{Name: name, Run: run})
	}

	if m.selected(idUpdate) {
		add("Update & Upgrade", opUpdateSystem)
	}
	if m.selected(idTimezone) {
		add("Zeitzone", opTimezone)
	}
	if m.selected(idHostname) {
		add("Hostname", opHostname)
	}
	if m.selected(idLocale) {
		add("Locale", opLocale)
	}
	if m.selected(idNTP) {
		add("NTP", opNTP)
	}
	if m.selected(idUser) {
		add("Benutzer", opUserCreate)
	}
	if m.selected(idSudo) {
		add("sudo ohne Passwort", opSudoNoPassword)
	}
	if m.selected(idSSH) {
		add("SSH", opSSHSetup)
	}
	if m.selected(idAliases) {
		add(".bashrc Aliases", opAliases)
	}
	if m.selected(idBasePkgs) {
		add("Basispakete", opBasePackages)
	}

	if m.selected(idDevTools) || m.selected(idMonitoring) {
		if m.toolSelected("build-essential") {
			add("Build-Tools", opBuildTools)
		}
		if m.toolSelected("python") {
			add("Python", opPython)
		}
		if m.toolSelected("go") {
			add("Go", opGo)
		}
		if m.toolSelected("node") {
			add("Node.js 22", opNode)
		}
		if m.toolSelected("uv") {
			add("uv", opUV)
		}
		if m.selected(idMonitoring) {
			add("Monitoring", opMonitoring)
		}
	}
	if m.selected(idAgents) {
		if m.agentSelected("pi") {
			add("Pi Coding Agent", opAgentPi)
		}
		if m.agentSelected("claude") {
			add("Claude Code", opAgentClaude)
		}
		if m.agentSelected("gemini") {
			add("Gemini CLI", opAgentGemini)
		}
		if m.agentSelected("codex") {
			add("OpenAI Codex CLI", opAgentCodex)
		}
	}
	if m.selected(idDocker) {
		add("Docker", opDockerInstall)
	}
	if m.selected(idSamba) {
		add("Samba", opSambaSetup)
	}
	if m.selected(idGitConfig) {
		add("Git konfigurieren", opGitConfig)
	}
	add("Setup-README erzeugen", opWriteReport)
	return steps
}

func startInstall(cfg config, steps []installStep, ch chan tea.Msg) tea.Cmd {
	return func() tea.Msg {
		result := installResult{
			StartedAt: time.Now(),
			IP:        primaryIP(),
			Versions:  map[string]string{},
		}
		writer := &channelLogger{ch: ch}
		ctx := &installContext{Config: cfg, Log: writer.Log}

		for i, step := range steps {
			ch <- stepMsg{Index: i, Name: step.Name}
			writer.Log("\n=== " + step.Name + " ===")
			if err := step.Run(ctx); err != nil {
				result.Failed = append(result.Failed, step.Name+": "+err.Error())
				writer.Log("FEHLER: " + err.Error())
			} else {
				result.Completed = append(result.Completed, step.Name)
				writer.Log("OK: " + step.Name)
			}
		}
		result.FinishedAt = time.Now()
		result.IP = primaryIP()
		result.Versions = collectVersions()
		result.ReportPath = reportPath(cfg.Username)
		return doneMsg(result)
	}
}

func waitMsg(ch chan tea.Msg) tea.Cmd { return func() tea.Msg { return <-ch } }

type channelLogger struct {
	ch chan tea.Msg
	mu sync.Mutex
}

func (l *channelLogger) Log(s string) {
	l.mu.Lock()
	defer l.mu.Unlock()
	for _, line := range strings.Split(strings.TrimRight(s, "\r\n"), "\n") {
		if line != "" {
			l.ch <- logMsg(line)
		}
	}
}

func collectVersions() map[string]string {
	out := map[string]string{}
	for name, cmd := range map[string][]string{
		"Node":    {"node", "--version"},
		"npm":     {"npm", "--version"},
		"Go":      {"go", "version"},
		"Python":  {"python3", "--version"},
		"uv":      {"uv", "--version"},
		"Docker":  {"docker", "--version"},
		"Compose": {"docker", "compose", "version"},
		"Pi":      {"pi", "--version"},
		"Claude":  {"claude", "--version"},
		"Gemini":  {"gemini", "--version"},
		"Codex":   {"codex", "--version"},
	} {
		if v := commandOutput(cmd[0], cmd[1:]...); v != "" {
			out[name] = v
		}
	}
	return out
}

func reportPath(username string) string {
	if username == "" {
		return "/root/setup-report.md"
	}
	return fmt.Sprintf("/home/%s/setup-report.md", username)
}
