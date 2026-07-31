package main

import (
	"fmt"
	"os"
	"os/user"
	"path/filepath"
	"strconv"
	"strings"
)

func opUserCreate(ctx *installContext) error {
	u := ctx.Config.Username
	if !commandSuccess("id", "-u", u) {
		if err := runLogged(ctx, "useradd", "-m", "-s", "/bin/bash", u); err != nil {
			return err
		}
	} else {
		ctx.Log("Benutzer bereits vorhanden: " + u)
	}
	if err := runLogged(ctx, "usermod", "-aG", "sudo", u); err != nil {
		return err
	}
	return runInput(ctx, u+":"+ctx.Config.Password+"\n", "chpasswd")
}
func opSudoNoPassword(ctx *installContext) error {
	path := "/etc/sudoers.d/90-" + ctx.Config.Username + "-nopasswd"
	content := fmt.Sprintf("%s ALL=(ALL:ALL) NOPASSWD: ALL\n", ctx.Config.Username)
	if b, _ := os.ReadFile(path); string(b) == content {
		ctx.Log("sudo-Regel bereits korrekt")
		return nil
	}
	if err := os.WriteFile(path, []byte(content), 0440); err != nil {
		return err
	}
	return runLogged(ctx, "visudo", "-cf", path)
}
func opSSHSetup(ctx *installContext) error {
	if err := apt(ctx, "install", "-y", "openssh-server"); err != nil {
		return err
	}
	if strings.TrimSpace(ctx.Config.SSHPublicKey) != "" {
		if err := installSSHKey(ctx.Config.Username, ctx.Config.SSHPublicKey); err != nil {
			return err
		}
	}
	root := "no"
	if ctx.Config.PermitRootLogin {
		root = "yes"
	}
	pass := "no"
	if ctx.Config.SSHPasswordAuth && !ctx.Config.SSHPubkeyOnly {
		pass = "yes"
	}
	conf := fmt.Sprintf(`# Managed by ubuntu-firstboot-v3
Port %d
PermitRootLogin %s
PubkeyAuthentication yes
PasswordAuthentication %s
KbdInteractiveAuthentication no
UsePAM yes
`, ctx.Config.SSHPort, root, pass)
	_ = os.MkdirAll("/etc/ssh/sshd_config.d", 0755)
	path := "/etc/ssh/sshd_config.d/99-ubuntu-firstboot.conf"
	if old, _ := os.ReadFile(path); string(old) != conf {
		if err := os.WriteFile(path, []byte(conf), 0644); err != nil {
			return err
		}
	} else {
		ctx.Log("SSH-Konfiguration bereits korrekt")
	}
	if err := runLogged(ctx, "sshd", "-t"); err != nil {
		return err
	}
	if err := runLogged(ctx, "systemctl", "enable", "--now", "ssh"); err != nil {
		return err
	}
	return runLogged(ctx, "systemctl", "reload", "ssh")
}
func installSSHKey(username, key string) error {
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
	data, _ := os.ReadFile(file)
	if !strings.Contains(string(data), strings.TrimSpace(key)) {
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
func opAliases(ctx *installContext) error {
	u, err := user.Lookup(ctx.Config.Username)
	if err != nil {
		return err
	}
	path := filepath.Join(u.HomeDir, ".bashrc")
	data, _ := os.ReadFile(path)
	start := "# BEGIN ubuntu-firstboot aliases"
	end := "# END ubuntu-firstboot aliases"
	block := start + `
alias ll='ls -alF'
alias la='ls -A'
alias l='ls -CF'
alias ..='cd ..'
alias ...='cd ../..'
alias update='sudo apt update && sudo apt upgrade'
alias dps='docker ps --format "table {{.Names}}\t{{.Status}}\t{{.Ports}}"'
` + end
	text := string(data)
	if strings.Contains(text, start) {
		a := strings.Index(text, start)
		b := strings.Index(text, end)
		if b >= a {
			text = text[:a] + block + text[b+len(end):]
		}
	} else {
		text = strings.TrimRight(text, "\n") + "\n\n" + block + "\n"
	}
	if err := os.WriteFile(path, []byte(text), 0644); err != nil {
		return err
	}
	return runLogged(ctx, "chown", ctx.Config.Username+":"+ctx.Config.Username, path)
}
