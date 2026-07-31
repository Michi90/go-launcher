package main

import "time"

type screen int

const (
	screenMain screen = iota
	screenTools
	screenAgents
	screenTimezone
	screenForm
	screenConfirm
	screenInstall
	screenDone
)

type itemID string

const (
	idUpdate     itemID = "update"
	idTimezone   itemID = "timezone"
	idHostname   itemID = "hostname"
	idLocale     itemID = "locale"
	idNTP        itemID = "ntp"
	idUser       itemID = "user"
	idSudo       itemID = "sudo_nopasswd"
	idSSH        itemID = "ssh"
	idAliases    itemID = "aliases"
	idBasePkgs   itemID = "base_packages"
	idDevTools   itemID = "development_tools"
	idMonitoring itemID = "monitoring"
	idAgents     itemID = "agents"
	idDocker     itemID = "docker"
	idSamba      itemID = "samba"
	idGitConfig  itemID = "git_config"
)

type menuItem struct {
	ID          itemID
	Title       string
	Description string
	Selected    bool
	Status      statusInfo
}

type selectableTool struct {
	ID          string
	Title       string
	Description string
	Selected    bool
	Status      statusInfo
}

type statusInfo struct {
	Checked bool
	Present bool
	Detail  string
}

type config struct {
	Timezone         string
	Hostname         string
	Locale           string
	Username         string
	Password         string
	PasswordConfirm  string
	SSHPublicKey     string
	SSHPort          int
	PermitRootLogin  bool
	SSHPasswordAuth  bool
	SSHPubkeyOnly    bool
	SambaShareName   string
	SambaPath        string
	GitName          string
	GitEmail         string
	GitDefaultBranch string
}

type installStep struct {
	Name string
	Run  func(*installContext) error
}

type installContext struct {
	Config config
	Log    func(string)
}

type installResult struct {
	StartedAt  time.Time
	FinishedAt time.Time
	Completed  []string
	Failed     []string
	IP         string
	Versions   map[string]string
	ReportPath string
}

type logMsg string
type statusMsg struct {
	Items  map[itemID]statusInfo
	Tools  map[string]statusInfo
	Agents map[string]statusInfo
}
type stepMsg struct {
	Index int
	Name  string
}
type doneMsg installResult
