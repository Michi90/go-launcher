package main

import (
	"fmt"
	"os"

	tea "github.com/charmbracelet/bubbletea"
)

func main() {
	if os.Geteuid() != 0 {
		fmt.Fprintln(os.Stderr, "Bitte als root starten: sudo ./ubuntu-firstboot")
		os.Exit(1)
	}
	if err := requireUbuntu(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}

	m := newModel()
	p := tea.NewProgram(m, tea.WithAltScreen())
	if _, err := p.Run(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
