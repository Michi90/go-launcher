package main

import (
	"bufio"
	"bytes"
	"errors"
	"fmt"
	"net"
	"os"
	"os/exec"
	"os/user"
	"path/filepath"
	"regexp"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/charmbracelet/bubbles/spinner"
	"github.com/charmbracelet/bubbles/textinput"
	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

type operationID int

const (
	opUpdate operationID = iota
	opPackages
	opUser
	opSudoNoPassword
	opSSH
	opNode
	opDocker
	opPi
	opSamba
)

type operation struct {
	ID          operationID
	Title       string
	Description string
	Selected    bool
}

type config struct {
	Username        string
	Password        string
	SSHPublicKey    string
	SSHPort         int
	SSHPasswordAuth bool
    NodeMajor       int
    Timezone        string
	Packages        string
}

type screen int

const (
	screenOperations screen = iota
	screenForm
	screenConfirm
	screenInstall
	screenDone
)

type fieldID int

const (
	fieldUsername fieldID = iota
	fieldPassword
	fieldPasswordConfirm
	fieldSSHKey
	fieldSSHPort
	fieldSSHPassword
	fieldNodeMajor
	fieldPackages
)

type field struct {
	ID          fieldID
	Label       string
	Description string
	Input       textinput.Model
	Required    bool
}

type logMsg string
type stepMsg struct {
	Index int
	Name  string
}
type installDoneMsg struct {
	Result installResult
}
type installResult struct {
	Completed []string
	Failed    []string
	IP        string
}

type model struct {
	screen       screen
	operations   []operation
	opCursor     int
	fields       []field
	fieldCursor  int
	cfg          config
	width        int
	height       int
	errText      string
	viewport     viewport.Model
	spinner      spinner.Model
	currentStep  string
	totalSteps   int
	currentIndex int
	logs         []string
	result       installResult
	logCh        chan tea.Msg
	started      bool
}

var (
	titleStyle = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("205"))
	boxStyle   = lipgloss.NewStyle().Border(lipgloss.RoundedBorder()).Padding(1, 2)
	active     = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("212"))
	muted      = lipgloss.NewStyle().Foreground(lipgloss.Color("241"))
	success    = lipgloss.NewStyle().Foreground(lipgloss.Color("42"))
	danger     = lipgloss.NewStyle().Foreground(lipgloss.Color("196"))
	warning    = lipgloss.NewStyle().Foreground(lipgloss.Color("214"))
)

func initialModel() model {
	s := spinner.New()
	s.Spinner = spinner.Dot

	ops := []operation{
		{opUpdate, "System aktualisieren", "apt update und apt upgrade", true},
		{opPackages, "Standardpakete", "git, curl, nano, htop, jq, Build-Tools usw.", true},
		{opUser, "Benutzer anlegen", "Benutzer mit Home-Verzeichnis und sudo-Gruppe", true},
		{opSudoNoPassword, "sudo ohne Passwort", "NOPASSWD-Regel unter /etc/sudoers.d", true},
		{opSSH, "SSH-Server", "OpenSSH installieren und absichern", true},
		{opNode, "Node.js", "Node.js 22 oder neuer installieren", true},
		{opDocker, "Docker", "Docker Engine, Buildx und Docker Compose", true},
		{opPi, "Pi Coding Agent", "Installation über npm", true},
		{opSamba, "Samba Home-Freigabe", "Komplettes Home-Verzeichnis für Windows freigeben", true},
	}

	return model{
		screen:     screenOperations,
		operations: ops,
		spinner:    s,
		cfg: config{
			Username:        suggestedUser(),
			SSHPort:         22,
			SSHPasswordAuth: true,
			NodeMajor:       22,
            Timezone:        "Europe/Berlin",
			Packages:        "git curl wget nano vim htop tree unzip zip jq ca-certificates gnupg lsb-release software-properties-common build-essential",
		},
		logCh: make(chan tea.Msg, 256),
	}
}

func main() {
	if os.Geteuid() != 0 {
		fmt.Fprintln(os.Stderr, "Bitte als root starten: sudo ./ubuntu-setup")
		os.Exit(1)
	}
	if err := ensureUbuntu(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}

	p := tea.NewProgram(initialModel(), tea.WithAltScreen())
	if _, err := p.Run(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func (m model) Init() tea.Cmd { return nil }

func (m model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width, m.height = msg.Width, msg.Height
		m.resizeViewport()
	case spinner.TickMsg:
		var cmd tea.Cmd
		m.spinner, cmd = m.spinner.Update(msg)
		return m, cmd
	case logMsg:
		m.logs = append(m.logs, string(msg))
		m.refreshLogs()
		return m, waitForInstallMsg(m.logCh)
	case stepMsg:
		m.currentIndex = msg.Index + 1
		m.currentStep = msg.Name
		return m, waitForInstallMsg(m.logCh)
	case installDoneMsg:
		m.result = msg.Result
		m.screen = screenDone
		return m, nil
	case tea.KeyMsg:
		if msg.String() == "ctrl+c" {
			return m, tea.Quit
		}
		switch m.screen {
		case screenOperations:
			return m.updateOperations(msg)
		case screenForm:
			return m.updateForm(msg)
		case screenConfirm:
			return m.updateConfirm(msg)
		case screenInstall:
			if msg.String() == "up" || msg.String() == "down" || msg.String() == "pgup" || msg.String() == "pgdown" {
				var cmd tea.Cmd
				m.viewport, cmd = m.viewport.Update(msg)
				return m, cmd
			}
		case screenDone:
			if msg.String() == "enter" || msg.String() == "q" || msg.String() == "esc" {
				return m, tea.Quit
			}
		}
	}

	return m, nil
}

func (m model) updateOperations(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "up", "k":
		if m.opCursor > 0 {
			m.opCursor--
		}
	case "down", "j":
		if m.opCursor < len(m.operations)-1 {
			m.opCursor++
		}
	case " ":
		m.operations[m.opCursor].Selected = !m.operations[m.opCursor].Selected
	case "enter":
		if !m.anySelected() {
			m.errText = "Mindestens eine Operation auswählen."
			return m, nil
		}
		m.errText = ""
		m.buildFields()
		if len(m.fields) == 0 {
			m.screen = screenConfirm
			return m, nil
		}
		m.screen = screenForm
		m.fields[0].Input.Focus()
		return m, textinput.Blink
	case "q", "esc":
		return m, tea.Quit
	}
	return m, nil
}

func (m model) updateForm(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "esc":
		m.screen = screenOperations
		m.errText = ""
		return m, nil
	case "tab", "down":
		m.saveCurrentField()
		if m.fieldCursor < len(m.fields)-1 {
			m.fields[m.fieldCursor].Input.Blur()
			m.fieldCursor++
			m.fields[m.fieldCursor].Input.Focus()
		}
		return m, textinput.Blink
	case "shift+tab", "up":
		m.saveCurrentField()
		if m.fieldCursor > 0 {
			m.fields[m.fieldCursor].Input.Blur()
			m.fieldCursor--
			m.fields[m.fieldCursor].Input.Focus()
		}
		return m, textinput.Blink
	case "enter":
		m.saveCurrentField()
		if err := m.validateField(m.fieldCursor); err != nil {
			m.errText = err.Error()
			return m, nil
		}
		m.errText = ""
		if m.fieldCursor < len(m.fields)-1 {
			m.fields[m.fieldCursor].Input.Blur()
			m.fieldCursor++
			m.fields[m.fieldCursor].Input.Focus()
			return m, textinput.Blink
		}
		if err := m.validateAll(); err != nil {
			m.errText = err.Error()
			return m, nil
		}
		m.screen = screenConfirm
		return m, nil
	}

	var cmd tea.Cmd
	m.fields[m.fieldCursor].Input, cmd = m.fields[m.fieldCursor].Input.Update(msg)
	return m, cmd
}

func (m model) updateConfirm(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "esc", "b":
		if len(m.fields) > 0 {
			m.screen = screenForm
			m.fields[m.fieldCursor].Input.Focus()
			return m, textinput.Blink
		}
		m.screen = screenOperations
	case "enter", "y", "j":
		if !m.started {
			m.started = true
			m.screen = screenInstall
			m.totalSteps = len(m.selectedOperations())
			return m, tea.Batch(m.spinner.Tick, startInstallCmd(m.cfg, m.selectedOperations(), m.logCh), waitForInstallMsg(m.logCh))
		}
	case "n", "q":
		return m, tea.Quit
	}
	return m, nil
}

func (m model) View() string {
	header := titleStyle.Render("Ubuntu Setup TUI")
	subtitle := muted.Render("Interaktive Grundeinrichtung für Ubuntu")
	content := ""

	switch m.screen {
	case screenOperations:
		content = m.viewOperations()
	case screenForm:
		content = m.viewForm()
	case screenConfirm:
		content = m.viewConfirm()
	case screenInstall:
		content = m.viewInstall()
	case screenDone:
		content = m.viewDone()
	}

	width := m.width - 4
	if width < 60 {
		width = 60
	}
	return boxStyle.Width(width).Render(header + "\n" + subtitle + "\n\n" + content)
}

func (m model) viewOperations() string {
	var b strings.Builder
	b.WriteString("Operationen auswählen\n\n")
	for i, op := range m.operations {
		cursor := "  "
		if i == m.opCursor {
			cursor = "› "
		}
		check := "[ ]"
		if op.Selected {
			check = "[x]"
		}
		line := fmt.Sprintf("%s%s %-25s %s", cursor, check, op.Title, muted.Render(op.Description))
		if i == m.opCursor {
			line = active.Render(line)
		}
		b.WriteString(line + "\n")
	}
	b.WriteString("\n" + muted.Render("↑/↓ navigieren   Leertaste auswählen   Enter weiter   q abbrechen"))
	if m.errText != "" {
		b.WriteString("\n\n" + danger.Render(m.errText))
	}
	return b.String()
}

func (m model) viewForm() string {
	var b strings.Builder
	b.WriteString(fmt.Sprintf("Parameter %d/%d\n\n", m.fieldCursor+1, len(m.fields)))

	f := m.fields[m.fieldCursor]
	b.WriteString(active.Render(f.Label) + "\n")
	b.WriteString(muted.Render(f.Description) + "\n\n")
	b.WriteString(f.Input.View())

	b.WriteString("\n\n" + muted.Render("Enter weiter   ↑/↓ oder Tab wechseln   Esc zurück"))
	if m.errText != "" {
		b.WriteString("\n\n" + danger.Render(m.errText))
	}
	return b.String()
}

func (m model) viewConfirm() string {
	var b strings.Builder
	b.WriteString("Konfiguration bestätigen\n\n")

	for _, op := range m.selectedOperations() {
		b.WriteString(success.Render("✓") + " " + op.Title + "\n")
	}

	if m.needsUser() {
		b.WriteString(fmt.Sprintf("\nBenutzer:       %s", m.cfg.Username))
		b.WriteString("\nPasswort:       " + strings.Repeat("•", 10))
	}
	if m.isSelected(opSSH) {
		b.WriteString(fmt.Sprintf("\nSSH-Port:       %d", m.cfg.SSHPort))
		b.WriteString(fmt.Sprintf("\nSSH-Passwort:   %s", boolText(m.cfg.SSHPasswordAuth)))
		b.WriteString(fmt.Sprintf("\nSSH-Key:        %s", boolText(strings.TrimSpace(m.cfg.SSHPublicKey) != "")))
	}
	if m.isSelected(opNode) {
		b.WriteString(fmt.Sprintf("\nNode.js:        %d.x", m.cfg.NodeMajor))
	}
	if m.isSelected(opSamba) {
		b.WriteString(fmt.Sprintf("\nSamba-Freigabe: \\\\SERVER\\%s-home", m.cfg.Username))
	}
	if m.isSelected(opPi) {
		b.WriteString("\nPi-Paket:       @earendil-works/pi-coding-agent")
	}

	b.WriteString("\n\n" + warning.Render("Nach dem Start sind keine weiteren Eingaben notwendig."))
	b.WriteString("\n\n" + muted.Render("Enter starten   Esc zurück   q abbrechen"))
	return b.String()
}

func (m model) viewInstall() string {
	status := fmt.Sprintf("%s Schritt %d/%d", m.spinner.View(), m.currentIndex, m.totalSteps)
	if m.currentStep != "" {
		status += ": " + m.currentStep
	}
	return status + "\n\n" + m.viewport.View() + "\n\n" + muted.Render("↑/↓ scrollen   Strg+C abbrechen")
}

func (m model) viewDone() string {
	var b strings.Builder
	if len(m.result.Failed) == 0 {
		b.WriteString(success.Render("Setup erfolgreich abgeschlossen") + "\n\n")
	} else {
		b.WriteString(warning.Render("Setup mit einzelnen Fehlern abgeschlossen") + "\n\n")
	}

	for _, item := range m.result.Completed {
		b.WriteString(success.Render("✓") + " " + item + "\n")
	}
	for _, item := range m.result.Failed {
		b.WriteString(danger.Render("✗") + " " + item + "\n")
	}

	if m.needsUser() {
		b.WriteString(fmt.Sprintf("\nBenutzer: %s\n", m.cfg.Username))
	}
	if m.result.IP != "" {
		b.WriteString(fmt.Sprintf("Server-IP: %s\n", m.result.IP))
	}
	if m.isSelected(opSSH) && m.result.IP != "" {
		b.WriteString("\nSSH:\n")
		b.WriteString(active.Render(fmt.Sprintf("ssh -p %d %s@%s", m.cfg.SSHPort, m.cfg.Username, m.result.IP)) + "\n")
	}
	if m.isSelected(opSamba) && m.result.IP != "" {
		b.WriteString("\nWindows-Samba:\n")
		b.WriteString(active.Render(fmt.Sprintf(`\\%s\%s-home`, m.result.IP, m.cfg.Username)) + "\n")
	}
	if m.isSelected(opPi) {
		b.WriteString("\nPi starten:\n" + active.Render("pi") + "\n")
	}

	b.WriteString("\n" + muted.Render("Enter oder q zum Beenden"))
	return b.String()
}

func (m *model) resizeViewport() {
	h := m.height - 12
	if h < 8 {
		h = 8
	}
	w := m.width - 10
	if w < 50 {
		w = 50
	}
	m.viewport = viewport.New(w, h)
	m.refreshLogs()
}

func (m *model) refreshLogs() {
	m.viewport.SetContent(strings.Join(m.logs, "\n"))
	m.viewport.GotoBottom()
}

func (m *model) buildFields() {
	m.fields = nil
	m.fieldCursor = 0

	if m.needsUser() {
		m.addField(fieldUsername, "Benutzername", "Linux-Benutzer für sudo, SSH, Docker und Samba.", m.cfg.Username, false)
		m.addField(fieldPassword, "Benutzerpasswort", "Wird einmalig für Linux und Samba verwendet und nicht protokolliert.", "", true)
		m.addField(fieldPasswordConfirm, "Passwort wiederholen", "Dasselbe Passwort erneut eingeben.", "", true)
	}
	if m.isSelected(opSSH) {
		m.addField(fieldSSHKey, "SSH Public Key", "Optional. Vollständiger Key, z. B. ssh-ed25519 AAAA... pc@name", m.cfg.SSHPublicKey, false)
		m.addField(fieldSSHPort, "SSH-Port", "Standard ist 22.", strconv.Itoa(m.cfg.SSHPort), false)
		m.addField(fieldSSHPassword, "SSH-Passwortanmeldung", "ja oder nein", yesNo(m.cfg.SSHPasswordAuth), false)
	}
	if m.isSelected(opNode) {
		m.addField(fieldNodeMajor, "Node.js Hauptversion", "Mindestens Version 22.", strconv.Itoa(m.cfg.NodeMajor), false)
	}
	if m.isSelected(opPackages) {
		m.addField(fieldPackages, "Standardpakete", "Leerzeichengetrennte APT-Paketnamen.", m.cfg.Packages, false)
	}
}

func (m *model) addField(id fieldID, label, description, value string, password bool) {
	in := textinput.New()
	in.SetValue(value)
	in.CharLimit = 4096
	in.Width = 70
	if password {
		in.EchoMode = textinput.EchoPassword
		in.EchoCharacter = '•'
	}
	m.fields = append(m.fields, field{ID: id, Label: label, Description: description, Input: in, Required: password})
}

func (m *model) saveCurrentField() {
	if len(m.fields) == 0 {
		return
	}
	f := m.fields[m.fieldCursor]
	v := strings.TrimSpace(f.Input.Value())
	switch f.ID {
	case fieldUsername:
		m.cfg.Username = v
	case fieldPassword:
		m.cfg.Password = f.Input.Value()
	case fieldSSHKey:
		m.cfg.SSHPublicKey = v
	case fieldSSHPort:
		if n, err := strconv.Atoi(v); err == nil {
			m.cfg.SSHPort = n
		}
	case fieldSSHPassword:
		m.cfg.SSHPasswordAuth = parseYes(v)
	case fieldNodeMajor:
		if n, err := strconv.Atoi(v); err == nil {
			m.cfg.NodeMajor = n
		}
	case fieldPackages:
		m.cfg.Packages = v
	}
}

func (m model) validateField(i int) error {
	f := m.fields[i]
	v := strings.TrimSpace(f.Input.Value())

	switch f.ID {
	case fieldUsername:
		if !regexp.MustCompile(`^[a-z_][a-z0-9_-]{0,31}$`).MatchString(v) {
			return errors.New("Ungültiger Linux-Benutzername.")
		}
	case fieldPassword:
		if len(f.Input.Value()) < 8 {
			return errors.New("Das Passwort muss mindestens 8 Zeichen haben.")
		}
	case fieldPasswordConfirm:
		password := ""
		for _, x := range m.fields {
			if x.ID == fieldPassword {
				password = x.Input.Value()
			}
		}
		if f.Input.Value() != password {
			return errors.New("Die Passwörter stimmen nicht überein.")
		}
	case fieldSSHPort:
		n, err := strconv.Atoi(v)
		if err != nil || n < 1 || n > 65535 {
			return errors.New("SSH-Port muss zwischen 1 und 65535 liegen.")
		}
	case fieldSSHPassword:
		if !isYesNo(v) {
			return errors.New("Bitte ja oder nein eingeben.")
		}
	case fieldNodeMajor:
		n, err := strconv.Atoi(v)
		if err != nil || n < 22 {
			return errors.New("Node.js muss Version 22 oder neuer sein.")
		}
	}
	return nil
}

func (m *model) validateAll() error {
	for i := range m.fields {
		if err := m.validateField(i); err != nil {
			m.fieldCursor = i
			return err
		}
	}
	m.saveCurrentField()
	return nil
}

func (m model) anySelected() bool {
	for _, op := range m.operations {
		if op.Selected {
			return true
		}
	}
	return false
}

func (m model) isSelected(id operationID) bool {
	for _, op := range m.operations {
		if op.ID == id {
			return op.Selected
		}
	}
	return false
}

func (m model) selectedOperations() []operation {
	var out []operation
	for _, op := range m.operations {
		if op.Selected {
			out = append(out, op)
		}
	}
	return out
}

func (m model) needsUser() bool {
	return m.isSelected(opUser) || m.isSelected(opSudoNoPassword) || m.isSelected(opSSH) ||
		m.isSelected(opDocker) || m.isSelected(opSamba)
}

func startInstallCmd(cfg config, ops []operation, ch chan tea.Msg) tea.Cmd {
	return func() tea.Msg {
		result := runInstallation(cfg, ops, ch)
		return installDoneMsg{Result: result}
	}
}

func waitForInstallMsg(ch chan tea.Msg) tea.Cmd {
	return func() tea.Msg { return <-ch }
}

func runInstallation(cfg config, ops []operation, ch chan tea.Msg) installResult {
	result := installResult{IP: primaryIP()}
	logger := &channelWriter{ch: ch}

	for i, op := range ops {
		ch <- stepMsg{Index: i, Name: op.Title}
		ch <- logMsg(fmt.Sprintf("\n=== %s ===", op.Title))

		var err error
		switch op.ID {
		case opUpdate:
			err = updateSystem(logger)
		case opPackages:
			err = installPackages(cfg, logger)
		case opUser:
			err = createUser(cfg, logger)
		case opSudoNoPassword:
			err = passwordlessSudo(cfg, logger)
		case opSSH:
			err = setupSSH(cfg, logger)
		case opNode:
			err = installNode(cfg, logger)
		case opDocker:
			err = installDocker(cfg, logger)
		case opPi:
			err = installPi(logger)
		case opSamba:
			err = setupSamba(cfg, logger)
		}

		if err != nil {
			result.Failed = append(result.Failed, op.Title+": "+err.Error())
			ch <- logMsg("FEHLER: " + err.Error())
		} else {
			result.Completed = append(result.Completed, op.Title)
			ch <- logMsg("OK: " + op.Title)
		}
	}
	return result
}

type channelWriter struct {
	ch chan tea.Msg
	mu sync.Mutex
}

func (w *channelWriter) Write(p []byte) (int, error) {
	w.mu.Lock()
	defer w.mu.Unlock()
	s := strings.TrimRight(string(p), "\r\n")
	if s != "" {
		for _, line := range strings.Split(s, "\n") {
			w.ch <- logMsg(line)
		}
	}
	return len(p), nil
}

func updateSystem(w *channelWriter) error {
	if err := repairAPT(w); err != nil {
		return err
	}
	if err := apt(w, "update"); err != nil {
		return err
	}
	return apt(w, "-y", "upgrade")
}

func installPackages(cfg config, w *channelWriter) error {
	if err := repairAPT(w); err != nil {
		return err
	}
	packages := uniqueFields(cfg.Packages)
	if len(packages) == 0 {
		return nil
	}
	args := append([]string{"install", "-y"}, packages...)
	return apt(w, args...)
}

func createUser(cfg config, w *channelWriter) error {
	if !commandSuccess("id", "-u", cfg.Username) {
		if err := runLogged(w, "useradd", "-m", "-s", "/bin/bash", cfg.Username); err != nil {
			return err
		}
	}
	if err := runLogged(w, "usermod", "-aG", "sudo", cfg.Username); err != nil {
		return err
	}
	return runWithInput(w, cfg.Username+":"+cfg.Password+"\n", "chpasswd")
}

func passwordlessSudo(cfg config, w *channelWriter) error {
	if !commandSuccess("id", "-u", cfg.Username) {
		return fmt.Errorf("Benutzer %s existiert nicht", cfg.Username)
	}
	path := "/etc/sudoers.d/90-" + cfg.Username + "-nopasswd"
	content := fmt.Sprintf("%s ALL=(ALL:ALL) NOPASSWD: ALL\n", cfg.Username)
	if err := os.WriteFile(path, []byte(content), 0440); err != nil {
		return err
	}
	return runLogged(w, "visudo", "-cf", path)
}

func setupSSH(cfg config, w *channelWriter) error {
	if err := apt(w, "install", "-y", "openssh-server"); err != nil {
		return err
	}
	if strings.TrimSpace(cfg.SSHPublicKey) != "" {
		if err := installAuthorizedKey(cfg.Username, cfg.SSHPublicKey); err != nil {
			return err
		}
	}

	passwordAuth := "no"
	if cfg.SSHPasswordAuth {
		passwordAuth = "yes"
	}
	conf := fmt.Sprintf(`# Managed by ubuntu-setup-tui
Port %d
PermitRootLogin no
PubkeyAuthentication yes
PasswordAuthentication %s
KbdInteractiveAuthentication no
UsePAM yes
`, cfg.SSHPort, passwordAuth)
	if err := os.MkdirAll("/etc/ssh/sshd_config.d", 0755); err != nil {
		return err
	}
	if err := os.WriteFile("/etc/ssh/sshd_config.d/99-ubuntu-setup.conf", []byte(conf), 0644); err != nil {
		return err
	}
	if err := runLogged(w, "sshd", "-t"); err != nil {
		return err
	}
	if err := runLogged(w, "systemctl", "enable", "--now", "ssh"); err != nil {
		return err
	}
	return runLogged(w, "systemctl", "reload", "ssh")
}

func installNode(cfg config, w *channelWriter) error {
	script := fmt.Sprintf(`set -e
curl -fsSL https://deb.nodesource.com/setup_%d.x | bash -
apt-get install -y nodejs
timedatectl set-timezone "%s"
node --version
npm --version
timedatectl
`, cfg.NodeMajor, cfg.Timezone)

	return runShellLogged(w, script)
}

func installDocker(cfg config, w *channelWriter) error {
	script := `set -e
apt-get update
apt-get install -y ca-certificates curl
install -m 0755 -d /etc/apt/keyrings
curl -fsSL https://download.docker.com/linux/ubuntu/gpg -o /etc/apt/keyrings/docker.asc
chmod a+r /etc/apt/keyrings/docker.asc
. /etc/os-release
ARCH="$(dpkg --print-architecture)"
cat >/etc/apt/sources.list.d/docker.sources <<EOF
Types: deb
URIs: https://download.docker.com/linux/ubuntu
Suites: ${UBUNTU_CODENAME:-$VERSION_CODENAME}
Components: stable
Architectures: ${ARCH}
Signed-By: /etc/apt/keyrings/docker.asc
EOF
apt-get update
apt-get install -y docker-ce docker-ce-cli containerd.io docker-buildx-plugin docker-compose-plugin
systemctl enable --now docker
`
	if err := runShellLogged(w, script); err != nil {
		return err
	}
	if commandSuccess("id", "-u", cfg.Username) {
		if err := runLogged(w, "usermod", "-aG", "docker", cfg.Username); err != nil {
			return err
		}
	}
	return runLogged(w, "docker", "compose", "version")
}

func installPi(w *channelWriter) error {
	if !commandSuccess("npm", "--version") {
		return errors.New("npm ist nicht installiert; Node.js muss vorher installiert werden")
	}
	if err := runLogged(w, "npm", "install", "-g", "--ignore-scripts", "@earendil-works/pi-coding-agent"); err != nil {
		return err
	}
	return runLogged(w, "pi", "--version")
}

func setupSamba(cfg config, w *channelWriter) error {
	if err := apt(w, "install", "-y", "samba"); err != nil {
		return err
	}
	if !commandSuccess("id", "-u", cfg.Username) {
		return fmt.Errorf("Benutzer %s existiert nicht", cfg.Username)
	}

	u, err := user.Lookup(cfg.Username)
	if err != nil {
		return err
	}
	shareName := cfg.Username + "-home"
	block := fmt.Sprintf(`
# BEGIN ubuntu-setup-tui %s
[%s]
   path = %s
   browseable = yes
   read only = no
   valid users = %s
   force user = %s
   create mask = 0660
   directory mask = 0770
# END ubuntu-setup-tui %s
`, cfg.Username, shareName, u.HomeDir, cfg.Username, cfg.Username, cfg.Username)

	path := "/etc/samba/smb.conf"
	data, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	re := regexp.MustCompile(`(?s)\n?# BEGIN ubuntu-setup-tui ` + regexp.QuoteMeta(cfg.Username) + `.*?# END ubuntu-setup-tui ` + regexp.QuoteMeta(cfg.Username) + `\n?`)
	cleaned := re.ReplaceAllString(string(data), "\n")
	if err := os.WriteFile(path, []byte(strings.TrimRight(cleaned, "\n")+block), 0644); err != nil {
		return err
	}

	if err := runWithInput(w, cfg.Password+"\n"+cfg.Password+"\n", "smbpasswd", "-s", "-a", cfg.Username); err != nil {
		return err
	}
	if err := runLogged(w, "testparm", "-s"); err != nil {
		return err
	}
	if err := runLogged(w, "systemctl", "enable", "--now", "smbd"); err != nil {
		return err
	}
	return runLogged(w, "systemctl", "restart", "smbd")
}

func repairAPT(w *channelWriter) error {
	_ = runLogged(w, "dpkg", "--configure", "-a")
	_ = apt(w, "-f", "install", "-y")
	return nil
}

func apt(w *channelWriter, args ...string) error {
	var last error
	for attempt := 1; attempt <= 3; attempt++ {
		full := append([]string{
			"-o", "DPkg::Lock::Timeout=120",
			"-o", "Acquire::Retries=3",
		}, args...)
		last = runEnvLogged(w, append(os.Environ(), "DEBIAN_FRONTEND=noninteractive"), "apt-get", full...)
		if last == nil {
			return nil
		}
		w.Write([]byte(fmt.Sprintf("APT-Versuch %d fehlgeschlagen, erneuter Versuch …\n", attempt)))
		time.Sleep(time.Duration(attempt*3) * time.Second)
	}
	return last
}

func runLogged(w *channelWriter, name string, args ...string) error {
	return runEnvLogged(w, os.Environ(), name, args...)
}

func runEnvLogged(w *channelWriter, env []string, name string, args ...string) error {
	w.Write([]byte("$ " + name + " " + strings.Join(args, " ") + "\n"))
	cmd := exec.Command(name, args...)
	cmd.Env = env
	cmd.Stdout = w
	cmd.Stderr = w
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("%s: %w", name, err)
	}
	return nil
}

func runWithInput(w *channelWriter, input, name string, args ...string) error {
	w.Write([]byte("$ " + name + " " + strings.Join(args, " ") + " <verdeckte Eingabe>\n"))
	cmd := exec.Command(name, args...)
	cmd.Stdin = strings.NewReader(input)
	cmd.Stdout = w
	cmd.Stderr = w
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("%s: %w", name, err)
	}
	return nil
}

func runShellLogged(w *channelWriter, script string) error {
	cmd := exec.Command("/bin/bash", "-c", "set -euo pipefail\n"+script)
	cmd.Stdout = w
	cmd.Stderr = w
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("Shell-Schritt: %w", err)
	}
	return nil
}

func installAuthorizedKey(username, key string) error {
	u, err := user.Lookup(username)
	if err != nil {
		return err
	}
	uid, _ := strconv.Atoi(u.Uid)
	gid, _ := strconv.Atoi(u.Gid)
	dir := filepath.Join(u.HomeDir, ".ssh")
	file := filepath.Join(dir, "authorized_keys")
	if err := os.MkdirAll(dir, 0700); err != nil {
		return err
	}
	existing, _ := os.ReadFile(file)
	if !strings.Contains(string(existing), strings.TrimSpace(key)) {
		f, err := os.OpenFile(file, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0600)
		if err != nil {
			return err
		}
		_, err = fmt.Fprintln(f, strings.TrimSpace(key))
		_ = f.Close()
		if err != nil {
			return err
		}
	}
	_ = os.Chmod(file, 0600)
	_ = os.Chown(dir, uid, gid)
	return os.Chown(file, uid, gid)
}

func ensureUbuntu() error {
	data, err := os.ReadFile("/etc/os-release")
	if err != nil {
		return err
	}
	if !regexp.MustCompile(`(?m)^ID=ubuntu$`).Match(data) {
		return errors.New("dieses Programm unterstützt ausschließlich Ubuntu")
	}
	return nil
}

func commandSuccess(name string, args ...string) bool {
	return exec.Command(name, args...).Run() == nil
}

func primaryIP() string {
	conn, err := net.Dial("udp", "1.1.1.1:80")
	if err == nil {
		defer conn.Close()
		if addr, ok := conn.LocalAddr().(*net.UDPAddr); ok {
			return addr.IP.String()
		}
	}
	addrs, _ := net.InterfaceAddrs()
	for _, addr := range addrs {
		ip, _, err := net.ParseCIDR(addr.String())
		if err == nil && ip.IsPrivate() && !ip.IsLoopback() {
			return ip.String()
		}
	}
	return ""
}

func suggestedUser() string {
	if u := os.Getenv("SUDO_USER"); u != "" && u != "root" {
		return u
	}
	return "michi"
}

func uniqueFields(v string) []string {
	seen := map[string]bool{}
	var out []string
	for _, x := range strings.Fields(v) {
		if !seen[x] {
			seen[x] = true
			out = append(out, x)
		}
	}
	return out
}

func yesNo(v bool) string {
	if v {
		return "ja"
	}
	return "nein"
}

func boolText(v bool) string {
	if v {
		return "Ja"
	}
	return "Nein"
}

func parseYes(v string) bool {
	switch strings.ToLower(strings.TrimSpace(v)) {
	case "j", "ja", "y", "yes", "1", "true":
		return true
	default:
		return false
	}
}

func isYesNo(v string) bool {
	switch strings.ToLower(strings.TrimSpace(v)) {
	case "j", "ja", "y", "yes", "1", "true", "n", "nein", "no", "0", "false":
		return true
	default:
		return false
	}
}

// Keep imports stable when building on multiple Go versions.
var _ = bufio.NewReader
var _ = bytes.NewBuffer
var _ = runtime.GOARCH
