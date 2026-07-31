package main

import (
	"errors"
	"fmt"
	"net"
	"os"
	"os/exec"
	"regexp"
	"strings"
	"time"
)

func requireUbuntu() error {
	data, err := os.ReadFile("/etc/os-release")
	if err != nil {
		return err
	}
	if !regexp.MustCompile(`(?m)^ID=ubuntu$`).Match(data) {
		return errors.New("dieses Programm unterstützt ausschließlich Ubuntu")
	}
	return nil
}
func currentHostname() string { h, _ := os.Hostname(); return h }
func suggestedUser() string {
	if u := os.Getenv("SUDO_USER"); u != "" && u != "root" {
		return u
	}
	return "michi"
}
func validUsername(v string) bool {
	return regexp.MustCompile(`^[a-z_][a-z0-9_-]{0,31}$`).MatchString(v)
}
func yesNo(v bool) string {
	if v {
		return "ja"
	}
	return "nein"
}
func parseYes(v string) bool {
	switch strings.ToLower(strings.TrimSpace(v)) {
	case "j", "ja", "y", "yes", "1", "true":
		return true
	}
	return false
}
func isYesNo(v string) bool {
	switch strings.ToLower(strings.TrimSpace(v)) {
	case "j", "ja", "y", "yes", "1", "true", "n", "nein", "no", "0", "false":
		return true
	}
	return false
}
func commandSuccess(name string, args ...string) bool {
	return exec.Command(name, args...).Run() == nil
}
func commandOutput(name string, args ...string) string {
	out, err := exec.Command(name, args...).CombinedOutput()
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(out))
}
func primaryIP() string {
	conn, err := net.Dial("udp", "1.1.1.1:80")
	if err == nil {
		defer conn.Close()
		if a, ok := conn.LocalAddr().(*net.UDPAddr); ok {
			return a.IP.String()
		}
	}
	addrs, _ := net.InterfaceAddrs()
	for _, a := range addrs {
		ip, _, e := net.ParseCIDR(a.String())
		if e == nil && ip.IsPrivate() && !ip.IsLoopback() {
			return ip.String()
		}
	}
	return ""
}
func runLogged(ctx *installContext, name string, args ...string) error {
	ctx.Log("$ " + name + " " + strings.Join(args, " "))
	cmd := exec.Command(name, args...)
	cmd.Stdout = &logWriter{log: ctx.Log}
	cmd.Stderr = &logWriter{log: ctx.Log}
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("%s: %w", name, err)
	}
	return nil
}
func runInput(ctx *installContext, input, name string, args ...string) error {
	ctx.Log("$ " + name + " " + strings.Join(args, " ") + " <verdeckte Eingabe>")
	cmd := exec.Command(name, args...)
	cmd.Stdin = strings.NewReader(input)
	cmd.Stdout = &logWriter{log: ctx.Log}
	cmd.Stderr = &logWriter{log: ctx.Log}
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("%s: %w", name, err)
	}
	return nil
}
func runShell(ctx *installContext, script string) error {
	cmd := exec.Command("/bin/bash", "-c", "set -euo pipefail\n"+script)
	cmd.Stdout = &logWriter{log: ctx.Log}
	cmd.Stderr = &logWriter{log: ctx.Log}
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("Shell-Schritt: %w", err)
	}
	return nil
}

type logWriter struct{ log func(string) }

func (w *logWriter) Write(p []byte) (int, error) {
	for _, s := range strings.Split(strings.TrimRight(string(p), "\r\n"), "\n") {
		if s != "" {
			w.log(s)
		}
	}
	return len(p), nil
}
func apt(ctx *installContext, args ...string) error {
	var last error
	for i := 1; i <= 3; i++ {
		full := append([]string{"-o", "DPkg::Lock::Timeout=120", "-o", "Acquire::Retries=3"}, args...)
		cmd := exec.Command("apt-get", full...)
		cmd.Env = append(os.Environ(), "DEBIAN_FRONTEND=noninteractive")
		cmd.Stdout = &logWriter{log: ctx.Log}
		cmd.Stderr = &logWriter{log: ctx.Log}
		ctx.Log("$ apt-get " + strings.Join(full, " "))
		last = cmd.Run()
		if last == nil {
			return nil
		}
		ctx.Log(fmt.Sprintf("APT-Versuch %d fehlgeschlagen; erneuter Versuch …", i))
		time.Sleep(time.Duration(i*3) * time.Second)
	}
	return last
}
