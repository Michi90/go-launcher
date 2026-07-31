package main

import (
	"fmt"
	"os"
	"os/user"
	"regexp"
	"strings"
)

func opDockerInstall(ctx *installContext) error {
	if commandSuccess("docker", "compose", "version") {
		ctx.Log("Docker und Compose bereits vorhanden")
	} else {
		if err := runShell(ctx, `apt-get update
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
systemctl enable --now docker`); err != nil {
			return err
		}
	}
	if ctx.Config.Username != "" && commandSuccess("id", "-u", ctx.Config.Username) {
		return runLogged(ctx, "usermod", "-aG", "docker", ctx.Config.Username)
	}
	return nil
}
func opSambaSetup(ctx *installContext) error {
	if err := apt(ctx, "install", "-y", "samba"); err != nil {
		return err
	}
	if err := os.MkdirAll(ctx.Config.SambaPath, 0770); err != nil {
		return err
	}
	if err := runLogged(ctx, "chown", "-R", ctx.Config.Username+":"+ctx.Config.Username, ctx.Config.SambaPath); err != nil {
		return err
	}

	path := "/etc/samba/smb.conf"
	data, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	tag := regexp.QuoteMeta(ctx.Config.SambaShareName)
	re := regexp.MustCompile(`(?s)\n?# BEGIN ubuntu-firstboot ` + tag + `.*?# END ubuntu-firstboot ` + tag + `\n?`)
	clean := re.ReplaceAllString(string(data), "\n")
	block := fmt.Sprintf(`
# BEGIN ubuntu-firstboot %s
[%s]
   path = %s
   browseable = yes
   read only = no
   valid users = %s
   force user = %s
   create mask = 0660
   directory mask = 0770
# END ubuntu-firstboot %s
`, ctx.Config.SambaShareName, ctx.Config.SambaShareName, ctx.Config.SambaPath,
		ctx.Config.Username, ctx.Config.Username, ctx.Config.SambaShareName)
	if err := os.WriteFile(path, []byte(strings.TrimRight(clean, "\n")+block), 0644); err != nil {
		return err
	}
	if err := runInput(ctx, ctx.Config.Password+"\n"+ctx.Config.Password+"\n", "smbpasswd", "-s", "-a", ctx.Config.Username); err != nil {
		return err
	}
	if err := runLogged(ctx, "testparm", "-s"); err != nil {
		return err
	}
	if err := runLogged(ctx, "systemctl", "enable", "--now", "smbd"); err != nil {
		return err
	}
	return runLogged(ctx, "systemctl", "restart", "smbd")
}
func opGitConfig(ctx *installContext) error {
	u, err := user.Lookup(ctx.Config.Username)
	if err != nil {
		return err
	}
	env := fmt.Sprintf("HOME=%s", u.HomeDir)
	script := fmt.Sprintf(`sudo -u %q env %s git config --global user.name %q
sudo -u %q env %s git config --global user.email %q
sudo -u %q env %s git config --global init.defaultBranch %q`,
		ctx.Config.Username, env, ctx.Config.GitName,
		ctx.Config.Username, env, ctx.Config.GitEmail,
		ctx.Config.Username, env, ctx.Config.GitDefaultBranch)
	return runShell(ctx, script)
}
