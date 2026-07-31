# Ubuntu Setup TUI

## Unter Windows für Linux x86_64 bauen

```powershell
$env:GOOS="linux"
$env:GOARCH="amd64"
$env:CGO_ENABLED="0"

go mod tidy
go build -ldflags="-s -w" -o ubuntu-setup .
```

## Auf den Server kopieren

```powershell
scp .\ubuntu-setup root@SERVER-IP:/root/
```

## Starten

```bash
chmod +x /root/ubuntu-setup
/root/ubuntu-setup
```

Navigation:

- `↑` / `↓`: navigieren
- `Leertaste`: Operation auswählen
- `Enter`: weiter
- `Esc`: zurück
- `Strg+C`: abbrechen
