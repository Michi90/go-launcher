package main

import (
	"bufio"
	"crypto/sha256"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"os/user"
	"path/filepath"
	"regexp"
	"runtime"
	"strconv"
	"strings"
	"time"
)

const appName = "Ubuntu Setup TUI"

type Config struct {
	UpdateSystem     bool
	InstallPackages  bool
	CreateUser       bool
	PasswordlessSudo bool
	SetupSSH         bool
	InstallNode      bool
	InstallDocker    bool
	Username         string
	SetUserPassword  bool
	SSHPublicKey     string
	SSHPort          int
	SSHPasswordAuth  bool
	NodeMajor        int
	Packages         []string
}

type Step struct {
	Name string
	Run  func(Config) error
}

var reader = bufio.NewReader(os.Stdin)

func main() {
	if os.Geteuid() != 0 {
		fatal("Dieses Programm muss als root gestartet werden, z. B.:\n  sudo ./ubuntu-setup")
	}

	if err := requireUbuntu(); err != nil {
		fatal(err.Error())
	}

	cfg, err := collectConfig()
	if err != nil {
		fatal(err.Error())
	}

	printSummary(cfg)
	if !confirm("\nÄnderungen jetzt ausführen?", false) {
		fmt.Println("Abgebrochen.")
		return
	}

	steps := buildSteps(cfg)
	if len(steps) == 0 {
		fmt.Println("Keine Operation ausgewählt.")
		return
	}

	fmt.Printf("\n%s\n", strings.Repeat("=", 72))
	fmt.Println("Installation wird gestartet")
	fmt.Println(strings.Repeat("=", 72))

	for i, step := range steps {
		fmt.Printf("\n[%d/%d] %s\n%s\n", i+1, len(steps), step.Name, strings.Repeat("-", 72))
		if err := step.Run(cfg); err != nil {
			fmt.Printf("\nFEHLER bei „%s“:\n%v\n", step.Name, err)
			if !confirm("Mit den übrigen Schritten fortfahren?", false) {
				os.Exit(1)
			}
		} else {
			fmt.Printf("✓ %s abgeschlossen\n", step.Name)
		}
	}

	fmt.Printf("\n%s\n", strings.Repeat("=", 72))
	fmt.Println("Setup abgeschlossen")
	fmt.Println(strings.Repeat("=", 72))
	printVersions(cfg)
}

func collectConfig() (Config, error) {
	cfg := Config{
		UpdateSystem:     true,
		InstallPackages:  true,
		CreateUser:       true,
		PasswordlessSudo: true,
		SetupSSH:         true,
		InstallNode:      true,
		InstallDocker:    true,
		SSHPort:          22,
		SSHPasswordAuth:  false,
		NodeMajor:        22,
		Packages: []string{
			"git", "curl", "wget", "nano", "vim", "htop", "tree", "unzip",
			"zip", "jq", "ca-certificates", "gnupg", "lsb-release",
			"software-properties-common", "build-essential",
		},
	}

	clearScreen()
	fmt.Println("┌──────────────────────────────────────────────────────────────┐")
	fmt.Printf("│ %-60s │\n", appName)
	fmt.Println("│ Interaktive Grundeinrichtung für Ubuntu                      │")
	fmt.Println("└──────────────────────────────────────────────────────────────┘")

	operations := []struct {
		label string
		value *bool
	}{
		{"System aktualisieren: apt update && apt upgrade", &cfg.UpdateSystem},
		{"Standardpakete installieren", &cfg.InstallPackages},
		{"Benutzer anlegen und zur sudo-Gruppe hinzufügen", &cfg.CreateUser},
		{"sudo ohne Passworteingabe aktivieren", &cfg.PasswordlessSudo},
		{"OpenSSH-Server installieren und konfigurieren", &cfg.SetupSSH},
		{"Node.js 22+ installieren", &cfg.InstallNode},
		{"Docker Engine und Docker Compose installieren", &cfg.InstallDocker},
	}

	for {
		fmt.Println("\nOperationen auswählen. Zahl eingeben = umschalten, Enter = weiter:\n")
		for i, op := range operations {
			mark := " "
			if *op.value {
				mark = "x"
			}
			fmt.Printf("  %d) [%s] %s\n", i+1, mark, op.label)
		}
		fmt.Print("\nAuswahl: ")
		input := readLine()
		if input == "" {
			break
		}
		n, err := strconv.Atoi(input)
		if err != nil || n < 1 || n > len(operations) {
			fmt.Println("Ungültige Auswahl.")
			continue
		}
		*operations[n-1].value = !*operations[n-1].value
	}

	needsUser := cfg.CreateUser || cfg.PasswordlessSudo || cfg.SetupSSH || cfg.InstallDocker
	if needsUser {
		defaultUser := detectSuggestedUser()
		for {
			cfg.Username = promptDefault("Benutzername", defaultUser)
			if validLinuxUsername(cfg.Username) {
				break
			}
			fmt.Println("Nur Kleinbuchstaben, Zahlen, _ und -; Beginn mit Buchstabe oder _.")
		}

		if cfg.CreateUser {
			cfg.SetUserPassword = confirm("Nach dem Anlegen ein Benutzerpasswort setzen?", true)
		}
	}

	if cfg.InstallPackages {
		current := strings.Join(cfg.Packages, " ")
		raw := promptDefault("Zu installierende Pakete", current)
		cfg.Packages = uniqueFields(raw)
	}

	if cfg.SetupSSH {
		for {
			raw := promptDefault("SSH-Port", strconv.Itoa(cfg.SSHPort))
			port, err := strconv.Atoi(raw)
			if err == nil && port >= 1 && port <= 65535 {
				cfg.SSHPort = port
				break
			}
			fmt.Println("Port muss zwischen 1 und 65535 liegen.")
		}

		fmt.Println("\nOptionaler SSH Public Key für den Benutzer.")
		fmt.Println("Beispiel: ssh-ed25519 AAAAC3... name@computer")
		cfg.SSHPublicKey = strings.TrimSpace(prompt("Public Key, leer zum Überspringen"))
		cfg.SSHPasswordAuth = confirm("SSH-Anmeldung mit Benutzerpasswort erlauben?", cfg.SSHPublicKey == "")

		if !cfg.SSHPasswordAuth && cfg.SSHPublicKey == "" {
			return cfg, errors.New("Passwort-Login kann ohne hinterlegten SSH-Key nicht deaktiviert werden")
		}
	}

	if cfg.InstallNode {
		for {
			raw := promptDefault("Node.js Hauptversion", "22")
			major, err := strconv.Atoi(raw)
			if err == nil && major >= 22 {
				cfg.NodeMajor = major
				break
			}
			fmt.Println("Bitte Version 22 oder höher eingeben.")
		}
	}

	return cfg, nil
}

func buildSteps(cfg Config) []Step {
	var steps []Step

	if cfg.UpdateSystem {
		steps = append(steps, Step{"System aktualisieren", updateSystem})
	}
	if cfg.InstallPackages {
		steps = append(steps, Step{"Standardpakete installieren", installPackages})
	}
	if cfg.CreateUser {
		steps = append(steps, Step{"Benutzer anlegen", createUser})
	}
	if cfg.PasswordlessSudo {
		steps = append(steps, Step{"Passwortloses sudo konfigurieren", configurePasswordlessSudo})
	}
	if cfg.SetupSSH {
		steps = append(steps, Step{"OpenSSH einrichten", setupSSH})
	}
	if cfg.InstallNode {
		steps = append(steps, Step{"Node.js installieren", installNode})
	}
	if cfg.InstallDocker {
		steps = append(steps, Step{"Docker und Compose installieren", installDocker})
	}

	return steps
}

func updateSystem(_ Config) error {
	env := append(os.Environ(), "DEBIAN_FRONTEND=noninteractive")
	if err := runEnv(env, "apt-get", "update"); err != nil {
		return err
	}
	return runEnv(env, "apt-get", "-y", "upgrade")
}

func installPackages(cfg Config) error {
	if len(cfg.Packages) == 0 {
		fmt.Println("Keine Pakete angegeben.")
		return nil
	}
	args := append([]string{"install", "-y"}, cfg.Packages...)
	env := append(os.Environ(), "DEBIAN_FRONTEND=noninteractive")
	return runEnv(env, "apt-get", args...)
}

func createUser(cfg Config) error {
	exists := commandSuccess("id", "-u", cfg.Username)
	if exists {
		fmt.Printf("Benutzer %q existiert bereits.\n", cfg.Username)
	} else {
		if err := run("useradd", "-m", "-s", "/bin/bash", cfg.Username); err != nil {
			return err
		}
	}

	if err := run("usermod", "-aG", "sudo", cfg.Username); err != nil {
		return err
	}

	if cfg.SetUserPassword {
		fmt.Printf("\nPasswort für %s setzen:\n", cfg.Username)
		cmd := exec.Command("passwd", cfg.Username)
		cmd.Stdin = os.Stdin
		cmd.Stdout = os.Stdout
		cmd.Stderr = os.Stderr
		if err := cmd.Run(); err != nil {
			return fmt.Errorf("passwd fehlgeschlagen: %w", err)
		}
	}
	return nil
}

func configurePasswordlessSudo(cfg Config) error {
	if !commandSuccess("id", "-u", cfg.Username) {
		return fmt.Errorf("Benutzer %q existiert nicht", cfg.Username)
	}

	path := "/etc/sudoers.d/90-" + cfg.Username + "-nopasswd"
	content := fmt.Sprintf("%s ALL=(ALL:ALL) NOPASSWD: ALL\n", cfg.Username)

	if err := writeFileAtomic(path, []byte(content), 0440); err != nil {
		return err
	}

	if err := run("visudo", "-cf", path); err != nil {
		_ = os.Remove(path)
		return fmt.Errorf("ungültige sudoers-Konfiguration entfernt: %w", err)
	}
	return nil
}

func setupSSH(cfg Config) error {
	env := append(os.Environ(), "DEBIAN_FRONTEND=noninteractive")
	if err := runEnv(env, "apt-get", "install", "-y", "openssh-server"); err != nil {
		return err
	}

	if cfg.SSHPublicKey != "" {
		if err := installAuthorizedKey(cfg.Username, cfg.SSHPublicKey); err != nil {
			return err
		}
	}

	passwordAuth := "no"
	if cfg.SSHPasswordAuth {
		passwordAuth = "yes"
	}

	config := fmt.Sprintf(`# Managed by ubuntu-setup
Port %d
PermitRootLogin no
PubkeyAuthentication yes
PasswordAuthentication %s
KbdInteractiveAuthentication no
UsePAM yes
`, cfg.SSHPort, passwordAuth)

	configPath := "/etc/ssh/sshd_config.d/99-ubuntu-setup.conf"
	if err := writeFileAtomic(configPath, []byte(config), 0644); err != nil {
		return err
	}

	if err := run("sshd", "-t"); err != nil {
		return fmt.Errorf("SSH-Konfiguration ist ungültig: %w", err)
	}
	if err := run("systemctl", "enable", "--now", "ssh"); err != nil {
		return err
	}
	return run("systemctl", "reload", "ssh")
}

func installAuthorizedKey(username, key string) error {
	if !validPublicKey(key) {
		return errors.New("der eingegebene SSH Public Key hat kein unterstütztes Format")
	}

	u, err := user.Lookup(username)
	if err != nil {
		return fmt.Errorf("Benutzer %q nicht gefunden: %w", username, err)
	}

	uid, _ := strconv.Atoi(u.Uid)
	gid, _ := strconv.Atoi(u.Gid)
	sshDir := filepath.Join(u.HomeDir, ".ssh")
	authFile := filepath.Join(sshDir, "authorized_keys")

	if err := os.MkdirAll(sshDir, 0700); err != nil {
		return err
	}
	if err := os.Chown(sshDir, uid, gid); err != nil {
		return err
	}

	existing, _ := os.ReadFile(authFile)
	lines := strings.Split(strings.TrimSpace(string(existing)), "\n")
	found := false
	for _, line := range lines {
		if strings.TrimSpace(line) == key {
			found = true
			break
		}
	}
	if !found {
		f, err := os.OpenFile(authFile, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0600)
		if err != nil {
			return err
		}
		if len(existing) > 0 && !strings.HasSuffix(string(existing), "\n") {
			_, _ = f.WriteString("\n")
		}
		_, err = f.WriteString(key + "\n")
		closeErr := f.Close()
		if err != nil {
			return err
		}
		if closeErr != nil {
			return closeErr
		}
	}

	if err := os.Chmod(authFile, 0600); err != nil {
		return err
	}
	return os.Chown(authFile, uid, gid)
}

func installNode(cfg Config) error {
	arch, err := nodeArchitecture()
	if err != nil {
		return err
	}

	baseURL := fmt.Sprintf("https://nodejs.org/dist/latest-v%d.x", cfg.NodeMajor)
	shasumsURL := baseURL + "/SHASUMS256.txt"
	tempDir, err := os.MkdirTemp("", "ubuntu-setup-node-*")
	if err != nil {
		return err
	}
	defer os.RemoveAll(tempDir)

	shasumsPath := filepath.Join(tempDir, "SHASUMS256.txt")
	if err := downloadFile(shasumsURL, shasumsPath); err != nil {
		return err
	}

	data, err := os.ReadFile(shasumsPath)
	if err != nil {
		return err
	}

	pattern := regexp.MustCompile(`(?m)^([a-f0-9]{64})  (node-v[0-9.]+-linux-` + regexp.QuoteMeta(arch) + `\.tar\.xz)$`)
	match := pattern.FindStringSubmatch(string(data))
	if len(match) != 3 {
		return fmt.Errorf("kein Node.js-v%d-Binary für Architektur %s gefunden", cfg.NodeMajor, arch)
	}

	expectedHash := match[1]
	filename := match[2]
	archivePath := filepath.Join(tempDir, filename)
	if err := downloadFile(baseURL+"/"+filename, archivePath); err != nil {
		return err
	}

	actualHash, err := sha256File(archivePath)
	if err != nil {
		return err
	}
	if actualHash != expectedHash {
		return fmt.Errorf("SHA256-Prüfsumme stimmt nicht: erwartet %s, erhalten %s", expectedHash, actualHash)
	}

	extractDir := filepath.Join(tempDir, "extract")
	if err := os.MkdirAll(extractDir, 0755); err != nil {
		return err
	}
	if err := run("tar", "-xJf", archivePath, "-C", extractDir, "--strip-components=1"); err != nil {
		return err
	}

	// Installation nach /usr/local; bestehende Dateien derselben Namen werden aktualisiert.
	for _, dir := range []string{"bin", "include", "lib", "share"} {
		source := filepath.Join(extractDir, dir)
		if _, err := os.Stat(source); err == nil {
			if err := run("cp", "-a", source+"/.", filepath.Join("/usr/local", dir)+"/"); err != nil {
				return err
			}
		}
	}

	if !commandSuccess("node", "--version") {
		return errors.New("Node.js wurde kopiert, ist aber nicht über PATH erreichbar")
	}
	return run("node", "--version")
}

func installDocker(cfg Config) error {
	env := append(os.Environ(), "DEBIAN_FRONTEND=noninteractive")

	// Konfliktpakete entfernen. Nicht installierte Pakete werden ignoriert.
	conflicts := []string{
		"docker.io", "docker-compose", "docker-compose-v2",
		"docker-doc", "podman-docker", "containerd", "runc",
	}
	args := append([]string{"remove", "-y"}, conflicts...)
	_ = runEnv(env, "apt-get", args...)

	if err := runEnv(env, "apt-get", "update"); err != nil {
		return err
	}
	if err := runEnv(env, "apt-get", "install", "-y", "ca-certificates", "curl"); err != nil {
		return err
	}

	if err := os.MkdirAll("/etc/apt/keyrings", 0755); err != nil {
		return err
	}
	if err := runShell(`curl -fsSL https://download.docker.com/linux/ubuntu/gpg -o /etc/apt/keyrings/docker.asc
chmod a+r /etc/apt/keyrings/docker.asc`); err != nil {
		return err
	}

	repo := `Types: deb
URIs: https://download.docker.com/linux/ubuntu
Suites: ${UBUNTU_CODENAME:-$VERSION_CODENAME}
Components: stable
Architectures: $(dpkg --print-architecture)
Signed-By: /etc/apt/keyrings/docker.asc
`
	script := fmt.Sprintf(`set -eu
. /etc/os-release
cat > /etc/apt/sources.list.d/docker.sources <<EOF
%s
EOF
`, repo)
	if err := runShell(script); err != nil {
		return err
	}

	if err := runEnv(env, "apt-get", "update"); err != nil {
		return err
	}
	if err := runEnv(env, "apt-get", "install", "-y",
		"docker-ce", "docker-ce-cli", "containerd.io",
		"docker-buildx-plugin", "docker-compose-plugin"); err != nil {
		return err
	}

	if err := run("systemctl", "enable", "--now", "docker"); err != nil {
		return err
	}

	if cfg.Username != "" && commandSuccess("id", "-u", cfg.Username) {
		if err := run("usermod", "-aG", "docker", cfg.Username); err != nil {
			return err
		}
		fmt.Printf("Benutzer %s wurde zur docker-Gruppe hinzugefügt.\n", cfg.Username)
		fmt.Println("Die Gruppenänderung gilt nach einer neuen Anmeldung.")
	}

	if err := run("docker", "version"); err != nil {
		return err
	}
	return run("docker", "compose", "version")
}

func printSummary(cfg Config) {
	fmt.Printf("\n%s\nZusammenfassung\n%s\n", strings.Repeat("=", 72), strings.Repeat("=", 72))
	printChoice("System aktualisieren", cfg.UpdateSystem)
	printChoice("Standardpakete installieren", cfg.InstallPackages)
	printChoice("Benutzer anlegen", cfg.CreateUser)
	printChoice("sudo ohne Passwort", cfg.PasswordlessSudo)
	printChoice("SSH einrichten", cfg.SetupSSH)
	printChoice(fmt.Sprintf("Node.js %d installieren", cfg.NodeMajor), cfg.InstallNode)
	printChoice("Docker und Compose installieren", cfg.InstallDocker)

	if cfg.Username != "" {
		fmt.Printf("  Benutzer:             %s\n", cfg.Username)
	}
	if cfg.InstallPackages {
		fmt.Printf("  Pakete:               %s\n", strings.Join(cfg.Packages, " "))
	}
	if cfg.SetupSSH {
		fmt.Printf("  SSH-Port:             %d\n", cfg.SSHPort)
		fmt.Printf("  SSH-Passwortlogin:    %s\n", yesNo(cfg.SSHPasswordAuth))
		fmt.Printf("  SSH-Key hinterlegt:   %s\n", yesNo(cfg.SSHPublicKey != ""))
	}
}

func printVersions(cfg Config) {
	fmt.Println("\nInstallierte Versionen:")
	if cfg.InstallNode {
		_ = run("node", "--version")
		_ = run("npm", "--version")
	}
	if cfg.InstallDocker {
		_ = run("docker", "--version")
		_ = run("docker", "compose", "version")
	}
	if cfg.SetupSSH {
		_ = run("sshd", "-V")
	}
}

func requireUbuntu() error {
	data, err := os.ReadFile("/etc/os-release")
	if err != nil {
		return fmt.Errorf("/etc/os-release konnte nicht gelesen werden: %w", err)
	}
	content := string(data)
	if !strings.Contains(content, "ID=ubuntu") {
		return errors.New("dieses Programm unterstützt ausschließlich Ubuntu")
	}
	return nil
}

func run(name string, args ...string) error {
	return runEnv(os.Environ(), name, args...)
}

func runEnv(env []string, name string, args ...string) error {
	fmt.Printf("$ %s %s\n", name, strings.Join(args, " "))
	cmd := exec.Command(name, args...)
	cmd.Env = env
	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("%s fehlgeschlagen: %w", name, err)
	}
	return nil
}

func runShell(script string) error {
	fmt.Println("$ /bin/bash -c <Installationsschritte>")
	cmd := exec.Command("/bin/bash", "-c", "set -euo pipefail\n"+script)
	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("Shell-Schritt fehlgeschlagen: %w", err)
	}
	return nil
}

func commandSuccess(name string, args ...string) bool {
	return exec.Command(name, args...).Run() == nil
}

func downloadFile(url, target string) error {
	fmt.Printf("Lade %s\n", url)
	return run("curl", "-fL", "--retry", "3", "--connect-timeout", "15", "-o", target, url)
}

func sha256File(path string) (string, error) {
	f, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer f.Close()

	h := sha256.New()
	if _, err := io.Copy(h, f); err != nil {
		return "", err
	}
	return fmt.Sprintf("%x", h.Sum(nil)), nil
}

func writeFileAtomic(path string, data []byte, mode os.FileMode) error {
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return err
	}

	temp, err := os.CreateTemp(dir, ".ubuntu-setup-*")
	if err != nil {
		return err
	}
	tempName := temp.Name()
	defer os.Remove(tempName)

	if _, err := temp.Write(data); err != nil {
		temp.Close()
		return err
	}
	if err := temp.Sync(); err != nil {
		temp.Close()
		return err
	}
	if err := temp.Close(); err != nil {
		return err
	}
	if err := os.Chmod(tempName, mode); err != nil {
		return err
	}

	if _, err := os.Stat(path); err == nil {
		backup := path + ".bak-" + time.Now().Format("20060102-150405")
		if err := copyFile(path, backup); err != nil {
			return fmt.Errorf("Backup von %s fehlgeschlagen: %w", path, err)
		}
		fmt.Printf("Backup erstellt: %s\n", backup)
	}

	return os.Rename(tempName, path)
}

func copyFile(source, target string) error {
	in, err := os.Open(source)
	if err != nil {
		return err
	}
	defer in.Close()

	info, err := in.Stat()
	if err != nil {
		return err
	}

	out, err := os.OpenFile(target, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, info.Mode())
	if err != nil {
		return err
	}
	_, copyErr := io.Copy(out, in)
	closeErr := out.Close()
	if copyErr != nil {
		return copyErr
	}
	return closeErr
}

func nodeArchitecture() (string, error) {
	switch runtime.GOARCH {
	case "amd64":
		return "x64", nil
	case "arm64":
		return "arm64", nil
	case "386":
		return "x86", nil
	case "arm":
		return "armv7l", nil
	default:
		return "", fmt.Errorf("nicht unterstützte CPU-Architektur: %s", runtime.GOARCH)
	}
}

func validLinuxUsername(name string) bool {
	matched, _ := regexp.MatchString(`^[a-z_][a-z0-9_-]{0,31}$`, name)
	return matched
}

func validPublicKey(key string) bool {
	allowed := []string{
		"ssh-ed25519 ",
		"ssh-rsa ",
		"ecdsa-sha2-nistp256 ",
		"ecdsa-sha2-nistp384 ",
		"ecdsa-sha2-nistp521 ",
		"sk-ssh-ed25519@openssh.com ",
		"sk-ecdsa-sha2-nistp256@openssh.com ",
	}
	for _, prefix := range allowed {
		if strings.HasPrefix(key, prefix) && len(strings.Fields(key)) >= 2 {
			return true
		}
	}
	return false
}

func detectSuggestedUser() string {
	if sudoUser := os.Getenv("SUDO_USER"); sudoUser != "" && sudoUser != "root" {
		return sudoUser
	}
	return "michi"
}

func uniqueFields(input string) []string {
	seen := map[string]bool{}
	var result []string
	for _, value := range strings.Fields(input) {
		if !seen[value] {
			seen[value] = true
			result = append(result, value)
		}
	}
	return result
}

func prompt(label string) string {
	fmt.Printf("%s: ", label)
	return readLine()
}

func promptDefault(label, defaultValue string) string {
	fmt.Printf("%s [%s]: ", label, defaultValue)
	value := readLine()
	if value == "" {
		return defaultValue
	}
	return value
}

func confirm(label string, defaultValue bool) bool {
	suffix := "[j/N]"
	if defaultValue {
		suffix = "[J/n]"
	}
	for {
		fmt.Printf("%s %s: ", label, suffix)
		value := strings.ToLower(readLine())
		if value == "" {
			return defaultValue
		}
		switch value {
		case "j", "ja", "y", "yes":
			return true
		case "n", "nein", "no":
			return false
		default:
			fmt.Println("Bitte j oder n eingeben.")
		}
	}
}

func readLine() string {
	value, err := reader.ReadString('\n')
	if err != nil && !errors.Is(err, io.EOF) {
		fatal(fmt.Sprintf("Eingabe konnte nicht gelesen werden: %v", err))
	}
	return strings.TrimSpace(value)
}

func printChoice(label string, enabled bool) {
	mark := " "
	if enabled {
		mark = "x"
	}
	fmt.Printf("  [%s] %s\n", mark, label)
}

func yesNo(value bool) string {
	if value {
		return "Ja"
	}
	return "Nein"
}

func clearScreen() {
	fmt.Print("\033[2J\033[H")
}

func fatal(message string) {
	fmt.Fprintln(os.Stderr, "Fehler:", message)
	os.Exit(1)
}
