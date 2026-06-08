// term.go — small shim around golang.org/x/term so the imports stay
// in a single file and main.go doesn't grow a giant import list.
//
// ReadPassword is the only call we make; we don't expose term.IsTerminal
// here because the implementation differs slightly between BSD and
// Linux terms.

package main

import (
	"fmt"

	"golang.org/x/term"
)

func termIsTerminal(fd int) bool {
	return term.IsTerminal(fd)
}

func termReadPassword(fd int) ([]byte, error) {
	// term.ReadPassword closes stdin after reading — restore it for
	// any subsequent reads (envcfg doesn't read stdin, but be safe).
	oldState, err := term.MakeRaw(fd)
	if err != nil {
		return nil, fmt.Errorf("make raw: %w", err)
	}
	defer func() { _ = term.Restore(fd, oldState) }()
	return term.ReadPassword(fd)
}
