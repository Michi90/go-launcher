package main

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/charmbracelet/bubbles/spinner"
	"github.com/charmbracelet/bubbles/textinput"
	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"
)

type formField struct {
	Key         string
	Label       string
	Description string
	Input       textinput.Model
	Password    bool
}

type model struct {
	screen       screen
	items        []menuItem
	cursor       int
	tools        []selectableTool
	toolCursor   int
	agents       []selectableTool
	agentCursor  int
	timezones    []string
	tzCursor     int
	fields       []formField
	fieldCursor  int
	cfg          config
	width        int
	height       int
	errText      string
	spinner      spinner.Model
	viewport     viewport.Model
	logs         []string
	currentStep  string
	currentIndex int
	totalSteps   int
	result       installResult
	msgCh        chan tea.Msg
	started      bool
}

func newModel() model {
	s := spinner.New()
	s.Spinner = spinner.Dot

	m := model{
		screen:  screenMain,
		spinner: s,
		msgCh:   make(chan tea.Msg, 256),
		cfg: config{
			Timezone:         "Europe/Berlin",
			Hostname:         currentHostname(),
			Locale:           "de_DE.UTF-8",
			Username:         suggestedUser(),
			SSHPort:          22,
			PermitRootLogin:  false,
			SSHPasswordAuth:  true,
			SSHPubkeyOnly:    false,
			SambaShareName:   "home",
			GitDefaultBranch: "main",
		},
		timezones: []string{
			"Europe/Berlin", "Europe/Vienna", "Europe/Zurich",
			"Europe/London", "UTC", "America/New_York",
			"America/Chicago", "America/Denver", "America/Los_Angeles",
			"Asia/Tokyo", "Asia/Shanghai", "Australia/Sydney",
		},
	}

	m.items = []menuItem{
		{idUpdate, "Update & Upgrade", "Paketlisten und installierte Pakete aktualisieren", true, statusInfo{}},
		{idTimezone, "Zeitzone", "Zeitzone aus einer Liste auswählen", true, statusInfo{}},
		{idHostname, "Hostname", "Systemnamen ändern", false, statusInfo{}},
		{idLocale, "Locale", "de_DE.UTF-8 oder en_US.UTF-8 konfigurieren", true, statusInfo{}},
		{idNTP, "NTP", "systemd-timesyncd aktivieren", true, statusInfo{}},
		{idUser, "Benutzer", "Benutzer anlegen oder aktualisieren", true, statusInfo{}},
		{idSudo, "sudo ohne Passwort", "NOPASSWD-Regel einrichten", true, statusInfo{}},
		{idSSH, "SSH", "OpenSSH installieren und konfigurieren", true, statusInfo{}},
		{idAliases, ".bashrc Aliases", "Sinnvolle Shell-Aliases ergänzen", true, statusInfo{}},
		{idBasePkgs, "Basispakete", "git, curl, wget, nano, jq, unzip, htop", true, statusInfo{}},
		{idDevTools, "Entwicklungswerkzeuge", "Python, Go, Node, uv und Build-Tools auswählen", true, statusInfo{}},
		{idMonitoring, "Monitoring", "btop und htop auswählen", true, statusInfo{}},
		{idAgents, "Coding Agents", "Pi, Claude Code, Gemini CLI und Codex auswählen", true, statusInfo{}},
		{idDocker, "Docker", "Engine, Compose und Benutzergruppe", true, statusInfo{}},
		{idSamba, "Samba", "Frei wählbares Verzeichnis freigeben", true, statusInfo{}},
		{idGitConfig, "Git konfigurieren", "Name, E-Mail und Default Branch setzen", false, statusInfo{}},
	}

	m.tools = []selectableTool{
		{"build-essential", "Build-Tools", "build-essential, gcc, make, pkg-config", true, statusInfo{}},
		{"python", "Python 3", "python3, python3-pip, python3-venv", true, statusInfo{}},
		{"go", "Go", "Aktuelle Ubuntu-Paketversion", false, statusInfo{}},
		{"node", "Node.js 22", "NodeSource 22.x und npm", true, statusInfo{}},
		{"uv", "uv", "Python-Paket- und Projektmanager", true, statusInfo{}},
	}
	m.agents = []selectableTool{
		{"pi", "Pi Coding Agent", "@earendil-works/pi-coding-agent", true, statusInfo{}},
		{"claude", "Claude Code", "@anthropic-ai/claude-code", false, statusInfo{}},
		{"gemini", "Gemini CLI", "@google/gemini-cli", false, statusInfo{}},
		{"codex", "OpenAI Codex CLI", "@openai/codex", false, statusInfo{}},
	}
	return m
}

func (m model) Init() tea.Cmd {
	return tea.Batch(checkStatusCmd(), m.spinner.Tick)
}

func (m model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width, m.height = msg.Width, msg.Height
		m.resizeViewport()
	case spinner.TickMsg:
		var cmd tea.Cmd
		m.spinner, cmd = m.spinner.Update(msg)
		return m, cmd
	case statusMsg:
		for i := range m.items {
			if v, ok := msg.Items[m.items[i].ID]; ok {
				m.items[i].Status = v
			}
		}
		for i := range m.tools {
			if v, ok := msg.Tools[m.tools[i].ID]; ok {
				m.tools[i].Status = v
			}
		}
		for i := range m.agents {
			if v, ok := msg.Agents[m.agents[i].ID]; ok {
				m.agents[i].Status = v
			}
		}
	case logMsg:
		m.logs = append(m.logs, string(msg))
		m.refreshLogs()
		return m, waitMsg(m.msgCh)
	case stepMsg:
		m.currentIndex = msg.Index + 1
		m.currentStep = msg.Name
		return m, waitMsg(m.msgCh)
	case doneMsg:
		m.result = installResult(msg)
		m.screen = screenDone
		return m, nil
	case tea.KeyMsg:
		if msg.String() == "ctrl+c" {
			return m, tea.Quit
		}
		switch m.screen {
		case screenMain:
			return m.updateMain(msg)
		case screenTools:
			return m.updateTools(msg)
		case screenAgents:
			return m.updateAgents(msg)
		case screenTimezone:
			return m.updateTimezone(msg)
		case screenForm:
			return m.updateForm(msg)
		case screenConfirm:
			return m.updateConfirm(msg)
		case screenInstall:
			var cmd tea.Cmd
			m.viewport, cmd = m.viewport.Update(msg)
			return m, cmd
		case screenDone:
			if msg.String() == "enter" || msg.String() == "q" || msg.String() == "esc" {
				return m, tea.Quit
			}
		}
	}
	return m, nil
}

func (m model) updateMain(k tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch k.String() {
	case "up", "k":
		if m.cursor > 0 {
			m.cursor--
		}
	case "down", "j":
		if m.cursor < len(m.items)-1 {
			m.cursor++
		}
	case " ":
		m.items[m.cursor].Selected = !m.items[m.cursor].Selected
	case "enter":
		if !m.anySelected() {
			m.errText = "Mindestens eine Operation auswählen."
			return m, nil
		}
		if m.selected(idDevTools) || m.selected(idMonitoring) {
			m.screen = screenTools
			return m, nil
		}
		if m.selected(idAgents) {
			m.screen = screenAgents
			return m, nil
		}
		if m.selected(idTimezone) {
			m.screen = screenTimezone
			return m, nil
		}
		m.buildFields()
		m.screen = screenForm
		return m, m.focusFirst()
	case "q", "esc":
		return m, tea.Quit
	}
	return m, nil
}

func (m model) updateTools(k tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch k.String() {
	case "up", "k":
		if m.toolCursor > 0 {
			m.toolCursor--
		}
	case "down", "j":
		if m.toolCursor < len(m.tools)-1 {
			m.toolCursor++
		}
	case " ":
		m.tools[m.toolCursor].Selected = !m.tools[m.toolCursor].Selected
	case "enter":
		if m.selected(idAgents) {
			m.screen = screenAgents
		} else if m.selected(idTimezone) {
			m.screen = screenTimezone
		} else {
			m.buildFields()
			m.screen = screenForm
			return m, m.focusFirst()
		}
	case "esc":
		m.screen = screenMain
	}
	return m, nil
}

func (m model) updateAgents(k tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch k.String() {
	case "up", "k":
		if m.agentCursor > 0 {
			m.agentCursor--
		}
	case "down", "j":
		if m.agentCursor < len(m.agents)-1 {
			m.agentCursor++
		}
	case " ":
		m.agents[m.agentCursor].Selected = !m.agents[m.agentCursor].Selected
	case "enter":
		if m.selected(idTimezone) {
			m.screen = screenTimezone
		} else {
			m.buildFields()
			m.screen = screenForm
			return m, m.focusFirst()
		}
	case "esc":
		if m.selected(idDevTools) || m.selected(idMonitoring) {
			m.screen = screenTools
		} else {
			m.screen = screenMain
		}
	}
	return m, nil
}

func (m model) updateTimezone(k tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch k.String() {
	case "up", "k":
		if m.tzCursor > 0 {
			m.tzCursor--
		}
	case "down", "j":
		if m.tzCursor < len(m.timezones)-1 {
			m.tzCursor++
		}
	case "enter":
		m.cfg.Timezone = m.timezones[m.tzCursor]
		m.buildFields()
		m.screen = screenForm
		return m, m.focusFirst()
	case "esc":
		if m.selected(idAgents) {
			m.screen = screenAgents
		} else if m.selected(idDevTools) {
			m.screen = screenTools
		} else {
			m.screen = screenMain
		}
	}
	return m, nil
}

func (m model) updateForm(k tea.KeyMsg) (tea.Model, tea.Cmd) {
	if len(m.fields) == 0 {
		m.screen = screenConfirm
		return m, nil
	}
	switch k.String() {
	case "esc":
		m.screen = screenMain
		return m, nil
	case "tab", "down":
		m.saveField()
		if m.fieldCursor < len(m.fields)-1 {
			m.fields[m.fieldCursor].Input.Blur()
			m.fieldCursor++
			m.fields[m.fieldCursor].Input.Focus()
		}
		return m, textinput.Blink
	case "shift+tab", "up":
		m.saveField()
		if m.fieldCursor > 0 {
			m.fields[m.fieldCursor].Input.Blur()
			m.fieldCursor--
			m.fields[m.fieldCursor].Input.Focus()
		}
		return m, textinput.Blink
	case "enter":
		m.saveField()
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
	m.fields[m.fieldCursor].Input, cmd = m.fields[m.fieldCursor].Input.Update(k)
	return m, cmd
}

func (m model) updateConfirm(k tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch k.String() {
	case "esc", "b":
		m.screen = screenForm
		return m, m.focusFirst()
	case "enter", "j", "y":
		if !m.started {
			m.started = true
			m.screen = screenInstall
			steps := buildInstallSteps(m)
			m.totalSteps = len(steps)
			return m, tea.Batch(m.spinner.Tick, startInstall(m.cfg, steps, m.msgCh), waitMsg(m.msgCh))
		}
	case "q", "n":
		return m, tea.Quit
	}
	return m, nil
}

func (m *model) buildFields() {
	m.fields = nil
	m.fieldCursor = 0

	if m.selected(idHostname) {
		m.addField("hostname", "Hostname", "Neuer Systemname", m.cfg.Hostname, false)
	}
	if m.selected(idLocale) {
		m.addField("locale", "Locale", "de_DE.UTF-8 oder en_US.UTF-8", m.cfg.Locale, false)
	}
	if m.needsUser() {
		m.addField("username", "Benutzername", "Linux-Benutzer für sudo, SSH, Docker, Samba und Git", m.cfg.Username, false)
		m.addField("password", "Benutzerpasswort", "Wird vorab für Linux und Samba gesetzt", "", true)
		m.addField("password_confirm", "Passwort wiederholen", "Keine Eingabe während der Installation notwendig", "", true)
	}
	if m.selected(idSSH) {
		m.addField("ssh_key", "SSH Public Key", "Optionaler vollständiger Public Key", m.cfg.SSHPublicKey, false)
		m.addField("ssh_port", "SSH-Port", "Standard: 22", strconv.Itoa(m.cfg.SSHPort), false)
		m.addField("ssh_root", "Root-Login erlauben", "ja oder nein", yesNo(m.cfg.PermitRootLogin), false)
		m.addField("ssh_password", "Passwort-Login erlauben", "ja oder nein", yesNo(m.cfg.SSHPasswordAuth), false)
		m.addField("ssh_pubkey_only", "Nur Public Key", "ja oder nein", yesNo(m.cfg.SSHPubkeyOnly), false)
	}
	if m.selected(idSamba) {
		defaultPath := "/home/" + m.cfg.Username
		if m.cfg.SambaPath == "" {
			m.cfg.SambaPath = defaultPath
		}
		if m.cfg.SambaShareName == "home" {
			m.cfg.SambaShareName = m.cfg.Username + "-home"
		}
		m.addField("samba_name", "Samba-Freigabename", "Name unter Windows", m.cfg.SambaShareName, false)
		m.addField("samba_path", "Samba-Verzeichnis", "Absoluter Pfad", m.cfg.SambaPath, false)
	}
	if m.selected(idGitConfig) {
		m.addField("git_name", "Git-Name", "git config --global user.name", m.cfg.GitName, false)
		m.addField("git_email", "Git-E-Mail", "git config --global user.email", m.cfg.GitEmail, false)
		m.addField("git_branch", "Git Default Branch", "Normalerweise main", m.cfg.GitDefaultBranch, false)
	}
}

func (m *model) addField(key, label, desc, value string, password bool) {
	in := textinput.New()
	in.SetValue(value)
	in.CharLimit = 4096
	in.Width = 72
	if password {
		in.EchoMode = textinput.EchoPassword
		in.EchoCharacter = '•'
	}
	m.fields = append(m.fields, formField{Key: key, Label: label, Description: desc, Input: in, Password: password})
}

func (m *model) focusFirst() tea.Cmd {
	if len(m.fields) == 0 {
		return nil
	}
	m.fields[0].Input.Focus()
	return textinput.Blink
}

func (m *model) saveField() {
	if len(m.fields) == 0 {
		return
	}
	f := m.fields[m.fieldCursor]
	v := strings.TrimSpace(f.Input.Value())
	switch f.Key {
	case "hostname":
		m.cfg.Hostname = v
	case "locale":
		m.cfg.Locale = v
	case "username":
		m.cfg.Username = v
	case "password":
		m.cfg.Password = f.Input.Value()
	case "password_confirm":
		m.cfg.PasswordConfirm = f.Input.Value()
	case "ssh_key":
		m.cfg.SSHPublicKey = v
	case "ssh_port":
		if n, e := strconv.Atoi(v); e == nil {
			m.cfg.SSHPort = n
		}
	case "ssh_root":
		m.cfg.PermitRootLogin = parseYes(v)
	case "ssh_password":
		m.cfg.SSHPasswordAuth = parseYes(v)
	case "ssh_pubkey_only":
		m.cfg.SSHPubkeyOnly = parseYes(v)
	case "samba_name":
		m.cfg.SambaShareName = v
	case "samba_path":
		m.cfg.SambaPath = v
	case "git_name":
		m.cfg.GitName = v
	case "git_email":
		m.cfg.GitEmail = v
	case "git_branch":
		m.cfg.GitDefaultBranch = v
	}
}

func (m model) validateField(i int) error {
	f := m.fields[i]
	v := strings.TrimSpace(f.Input.Value())
	switch f.Key {
	case "hostname":
		if v == "" || strings.ContainsAny(v, " /\\") {
			return fmt.Errorf("Ungültiger Hostname")
		}
	case "locale":
		if v != "de_DE.UTF-8" && v != "en_US.UTF-8" {
			return fmt.Errorf("Locale muss de_DE.UTF-8 oder en_US.UTF-8 sein")
		}
	case "username":
		if !validUsername(v) {
			return fmt.Errorf("Ungültiger Benutzername")
		}
	case "password":
		if len(f.Input.Value()) < 8 {
			return fmt.Errorf("Passwort muss mindestens 8 Zeichen haben")
		}
	case "password_confirm":
		var p string
		for _, x := range m.fields {
			if x.Key == "password" {
				p = x.Input.Value()
			}
		}
		if f.Input.Value() != p {
			return fmt.Errorf("Passwörter stimmen nicht überein")
		}
	case "ssh_port":
		n, e := strconv.Atoi(v)
		if e != nil || n < 1 || n > 65535 {
			return fmt.Errorf("Ungültiger SSH-Port")
		}
	case "ssh_root", "ssh_password", "ssh_pubkey_only":
		if !isYesNo(v) {
			return fmt.Errorf("Bitte ja oder nein eingeben")
		}
	case "samba_name":
		if v == "" || strings.ContainsAny(v, `/\[]`) {
			return fmt.Errorf("Ungültiger Freigabename")
		}
	case "samba_path":
		if !strings.HasPrefix(v, "/") {
			return fmt.Errorf("Samba-Pfad muss absolut sein")
		}
	case "git_email":
		if v == "" || !strings.Contains(v, "@") {
			return fmt.Errorf("Ungültige Git-E-Mail")
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
	m.saveField()
	if m.cfg.SSHPubkeyOnly && strings.TrimSpace(m.cfg.SSHPublicKey) == "" {
		return fmt.Errorf("Pubkey-only benötigt einen SSH Public Key")
	}
	return nil
}

func (m model) selected(id itemID) bool {
	for _, x := range m.items {
		if x.ID == id {
			return x.Selected
		}
	}
	return false
}
func (m model) anySelected() bool {
	for _, x := range m.items {
		if x.Selected {
			return true
		}
	}
	return false
}
func (m model) needsUser() bool {
	return m.selected(idUser) || m.selected(idSudo) || m.selected(idSSH) ||
		m.selected(idAliases) || m.selected(idDocker) || m.selected(idSamba) || m.selected(idGitConfig)
}
func (m model) toolSelected(id string) bool {
	for _, x := range m.tools {
		if x.ID == id {
			return x.Selected
		}
	}
	return false
}
func (m model) agentSelected(id string) bool {
	for _, x := range m.agents {
		if x.ID == id {
			return x.Selected
		}
	}
	return false
}
func (m *model) resizeViewport() {
	w, h := m.width-10, m.height-13
	if w < 50 {
		w = 50
	}
	if h < 8 {
		h = 8
	}
	m.viewport = viewport.New(w, h)
	m.refreshLogs()
}
func (m *model) refreshLogs() {
	m.viewport.SetContent(strings.Join(m.logs, "\n"))
	m.viewport.GotoBottom()
}
