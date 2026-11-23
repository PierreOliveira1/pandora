package cli

import (
	"bufio"
	"errors"
	"fmt"
	"io"
	"strings"

	"golang.org/x/term"
)

func terminalFd(r io.Reader) (uintptr, bool) {
	f, ok := r.(interface{ Fd() uintptr })
	if !ok {
		return 0, false
	}
	fd := f.Fd()
	if !term.IsTerminal(int(fd)) {
		return 0, false
	}
	return fd, true
}

func isTerminal(r io.Reader) bool {
	_, ok := terminalFd(r)
	return ok
}

func promptSecret(stdin io.Reader, stderr io.Writer, name string) (string, error) {
	fd, ok := terminalFd(stdin)
	if !ok {
		return "", fmt.Errorf("value for %s required", name)
	}

	fmt.Fprintf(stderr, "Value for %s: ", name)
	value, err := term.ReadPassword(int(fd))
	fmt.Fprintln(stderr)
	if err != nil {
		return "", fmt.Errorf("reading value for %s: %w", name, err)
	}
	return string(value), nil
}

func readConfirmation(stdin io.Reader) (bool, error) {
	reader := bufio.NewReader(stdin)
	answer, err := reader.ReadString('\n')
	if err != nil && !errors.Is(err, io.EOF) {
		return false, fmt.Errorf("reading confirmation: %w", err)
	}
	answer = strings.ToLower(strings.TrimSpace(answer))
	return answer == "y" || answer == "yes", nil
}
