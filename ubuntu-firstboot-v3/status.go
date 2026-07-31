package main

import (
	"os"
	"os/exec"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
)

func checkStatusCmd() tea.Cmd {
	return func() tea.Msg {
		items := map[itemID]statusInfo{}
		tools := map[string]statusInfo{}
		agents := map[string]statusInfo{}

		items[idUpdate] = statusInfo{Checked: true, Present: false, Detail: "wird bei Ausführung geprüft"}
		items[idTimezone] = statusInfo{Checked: true, Present: true, Detail: commandOutput("timedatectl", "show", "-p", "Timezone", "--value")}
		items[idHostname] = statusInfo{Checked: true, Present: true, Detail: currentHostname()}
		items[idLocale] = statusInfo{Checked: true, Present: commandSuccess("locale"), Detail: commandOutput("locale")}
		items[idNTP] = statusInfo{Checked: true, Present: commandSuccess("systemctl", "is-enabled", "systemd-timesyncd")}
		items[idUser] = statusInfo{Checked: true, Present: commandSuccess("id", "-u", suggestedUser())}
		items[idSudo] = statusInfo{Checked: true, Present: fileExists("/etc/sudoers.d/90-" + suggestedUser() + "-nopasswd")}
		items[idSSH] = statusInfo{Checked: true, Present: commandSuccess("sshd", "-V")}
		items[idAliases] = statusInfo{Checked: true, Present: bashrcHasAliases(suggestedUser())}
		items[idBasePkgs] = statusInfo{Checked: true, Present: commandsExist("git", "curl", "wget", "nano", "jq", "unzip", "htop")}
		items[idDevTools] = statusInfo{Checked: true, Present: false}
		items[idMonitoring] = statusInfo{Checked: true, Present: commandsExist("btop", "htop")}
		items[idAgents] = statusInfo{Checked: true, Present: false}
		items[idDocker] = statusInfo{Checked: true, Present: commandSuccess("docker", "--version")}
		items[idSamba] = statusInfo{Checked: true, Present: commandSuccess("smbd", "--version")}
		items[idGitConfig] = statusInfo{Checked: true, Present: commandSuccess("git", "config", "--global", "user.name")}

		tools["build-essential"] = statusInfo{Checked: true, Present: commandSuccess("dpkg-query", "-W", "build-essential")}
		tools["python"] = statusInfo{Checked: true, Present: commandSuccess("python3", "--version")}
		tools["go"] = statusInfo{Checked: true, Present: commandSuccess("go", "version"), Detail: commandOutput("go", "version")}
		tools["node"] = statusInfo{Checked: true, Present: commandSuccess("node", "--version"), Detail: commandOutput("node", "--version")}
		tools["uv"] = statusInfo{Checked: true, Present: commandSuccess("uv", "--version"), Detail: commandOutput("uv", "--version")}

		agents["pi"] = statusInfo{Checked: true, Present: commandSuccess("pi", "--version")}
		agents["claude"] = statusInfo{Checked: true, Present: commandSuccess("claude", "--version")}
		agents["gemini"] = statusInfo{Checked: true, Present: commandSuccess("gemini", "--version")}
		agents["codex"] = statusInfo{Checked: true, Present: commandSuccess("codex", "--version")}

		return statusMsg{Items: items, Tools: tools, Agents: agents}
	}
}

func fileExists(path string) bool { _, err := os.Stat(path); return err == nil }
func commandsExist(names ...string) bool {
	for _, n := range names {
		if _, err := exec.LookPath(n); err != nil {
			return false
		}
	}
	return true
}
func bashrcHasAliases(username string) bool {
	home := "/home/" + username
	data, _ := os.ReadFile(home + "/.bashrc")
	return strings.Contains(string(data), "# BEGIN ubuntu-firstboot aliases")
}
