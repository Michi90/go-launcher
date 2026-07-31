package main

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

func opUpdateSystem(ctx *installContext) error {
	_ = runLogged(ctx, "dpkg", "--configure", "-a")
	_ = apt(ctx, "-f", "install", "-y")
	if err := apt(ctx, "update"); err != nil {
		return err
	}
	return apt(ctx, "upgrade", "-y")
}
func opTimezone(ctx *installContext) error {
	current := commandOutput("timedatectl", "show", "-p", "Timezone", "--value")
	if current == ctx.Config.Timezone {
		ctx.Log("Zeitzone bereits korrekt: " + current)
		return nil
	}
	return runLogged(ctx, "timedatectl", "set-timezone", ctx.Config.Timezone)
}
func opHostname(ctx *installContext) error {
	if currentHostname() == ctx.Config.Hostname {
		ctx.Log("Hostname bereits korrekt")
		return nil
	}
	return runLogged(ctx, "hostnamectl", "set-hostname", ctx.Config.Hostname)
}
func opLocale(ctx *installContext) error {
	if strings.Contains(commandOutput("locale"), ctx.Config.Locale) {
		ctx.Log("Locale bereits aktiv")
		return nil
	}
	if err := apt(ctx, "install", "-y", "locales"); err != nil {
		return err
	}
	if err := runLogged(ctx, "locale-gen", ctx.Config.Locale); err != nil {
		return err
	}
	return runLogged(ctx, "update-locale", "LANG="+ctx.Config.Locale)
}
func opNTP(ctx *installContext) error {
	if err := apt(ctx, "install", "-y", "systemd-timesyncd"); err != nil {
		return err
	}
	if err := runLogged(ctx, "systemctl", "enable", "--now", "systemd-timesyncd"); err != nil {
		return err
	}
	return runLogged(ctx, "timedatectl", "set-ntp", "true")
}
func opBasePackages(ctx *installContext) error {
	return apt(ctx, "install", "-y", "git", "curl", "wget", "nano", "jq", "unzip", "htop", "ca-certificates", "gnupg")
}
func opMonitoring(ctx *installContext) error { return apt(ctx, "install", "-y", "btop", "htop") }
func opWriteReport(ctx *installContext) error {
	path := reportPath(ctx.Config.Username)
	ip := primaryIP()
	body := fmt.Sprintf(`# Server Setup

- Hostname: %s
- IP: %s
- Benutzer: %s
- Zeitzone: %s
- Locale: %s

## SSH

`+"```bash"+`
ssh -p %d %s@%s
`+"```"+`

## Samba

`+"```text"+`
\\%s\%s
`+"```"+`

## Installierte Versionen

`, currentHostname(), ip, ctx.Config.Username, ctx.Config.Timezone, ctx.Config.Locale,
		ctx.Config.SSHPort, ctx.Config.Username, ip, ip, ctx.Config.SambaShareName)
	for name, version := range collectVersions() {
		body += fmt.Sprintf("- %s: %s\n", name, version)
	}
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		return err
	}
	if err := os.WriteFile(path, []byte(body), 0644); err != nil {
		return err
	}
	if ctx.Config.Username != "" {
		_ = runLogged(ctx, "chown", ctx.Config.Username+":"+ctx.Config.Username, path)
	}
	return nil
}
