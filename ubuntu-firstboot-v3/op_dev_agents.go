package main

import (
	"errors"
	"fmt"
)

func opBuildTools(ctx *installContext) error {
	return apt(ctx, "install", "-y", "build-essential", "gcc", "make", "pkg-config")
}
func opPython(ctx *installContext) error {
	return apt(ctx, "install", "-y", "python3", "python3-pip", "python3-venv")
}
func opGo(ctx *installContext) error {
	if commandSuccess("go", "version") {
		ctx.Log("Go bereits vorhanden: " + commandOutput("go", "version"))
		return nil
	}
	return apt(ctx, "install", "-y", "golang-go")
}
func opNode(ctx *installContext) error {
	if commandSuccess("node", "--version") && nodeMajorAtLeast22() {
		ctx.Log("Node.js bereits passend: " + commandOutput("node", "--version"))
		return nil
	}
	return runShell(ctx, `curl -fsSL https://deb.nodesource.com/setup_22.x | bash -
apt-get install -y nodejs
node --version
npm --version`)
}
func nodeMajorAtLeast22() bool {
	v := commandOutput("node", "--version")
	var major int
	_, _ = fmt.Sscanf(v, "v%d", &major)
	return major >= 22
}
func opUV(ctx *installContext) error {
	if commandSuccess("uv", "--version") {
		ctx.Log("uv bereits vorhanden: " + commandOutput("uv", "--version"))
		return nil
	}
	return runShell(ctx, `curl -LsSf https://astral.sh/uv/install.sh | env UV_INSTALL_DIR=/usr/local/bin sh
uv --version`)
}
func requireNPM() error {
	if !commandSuccess("npm", "--version") {
		return errors.New("npm fehlt; Node.js 22 auswählen")
	}
	return nil
}
func npmGlobal(ctx *installContext, command, pkg string, ignoreScripts bool) error {
	if commandSuccess(command, "--version") {
		ctx.Log(command + " bereits vorhanden: " + commandOutput(command, "--version"))
		return nil
	}
	if err := requireNPM(); err != nil {
		return err
	}
	args := []string{"install", "-g"}
	if ignoreScripts {
		args = append(args, "--ignore-scripts")
	}
	args = append(args, pkg)
	return runLogged(ctx, "npm", args...)
}
func opAgentPi(ctx *installContext) error {
	return npmGlobal(ctx, "pi", "@earendil-works/pi-coding-agent", true)
}
func opAgentClaude(ctx *installContext) error {
	return npmGlobal(ctx, "claude", "@anthropic-ai/claude-code", false)
}
func opAgentGemini(ctx *installContext) error {
	return npmGlobal(ctx, "gemini", "@google/gemini-cli", false)
}
func opAgentCodex(ctx *installContext) error {
	return npmGlobal(ctx, "codex", "@openai/codex", false)
}
