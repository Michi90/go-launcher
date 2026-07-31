# Ubuntu First Boot V3

Mehrteilige Bubble-Tea-TUI für die Grundeinrichtung eines Ubuntu-Servers oder LXC.

## Windows → Linux x86_64 bauen

```powershell
$env:GOOS="linux"
$env:GOARCH="amd64"
$env:CGO_ENABLED="0"

go mod tidy
go build -ldflags="-s -w" -o ubuntu-firstboot .
```

Danach:

```powershell
scp .\ubuntu-firstboot root@SERVER-IP:/root/
```

```bash
chmod +x /root/ubuntu-firstboot
/root/ubuntu-firstboot
```

## Bedienung

- Pfeiltasten: navigieren
- Leertaste: auswählen
- Enter: weiter
- Esc: zurück
- Strg+C: abbrechen

## Struktur

- `model.go`: TUI-Zustand und Navigation
- `ui.go`: Darstellung
- `status.go`: Erkennung vorhandener Software
- `installer.go`: Installationsablauf
- `system.go`: gemeinsame Systemfunktionen
- `ops/`: getrennte Operationsdateien

## Hinweis

Die TUI prüft vorhandene Programme und Konfigurationen vorab. Jede Operation prüft zusätzlich direkt vor der Änderung erneut, soweit dies sinnvoll möglich ist.
