package cli

import (
	"bufio"
	"errors"
	"fmt"
	"io"
	"os"
	"strings"
	"time"

	"golang.org/x/term"
)

const secretReadTimeout = 5 * time.Second

type secretReader struct {
	stdin  io.Reader
	stderr io.Writer
	lines  *bufio.Reader
}

func newSecretReader(stdin io.Reader, stderr io.Writer) *secretReader {
	return &secretReader{
		stdin:  stdin,
		stderr: stderr,
		lines:  bufio.NewReader(stdin),
	}
}

func (r *secretReader) Read(name string) (string, error) {
	if fd, ok := terminalFd(r.stdin); ok {
		fmt.Fprintf(r.stderr, "Value for %s: ", name)
		value, err := term.ReadPassword(int(fd))
		fmt.Fprintln(r.stderr)
		if err != nil {
			return "", fmt.Errorf("reading value for %s: %w", name, err)
		}
		return string(value), nil
	}

	value, ok := r.readLineFromStdin()
	if !ok {
		return "", fmt.Errorf("value for %s required: run this command in a terminal, where Pandora asks for it without showing it, or pipe the value on stdin", name)
	}
	return value, nil
}

func (r *secretReader) readLineFromStdin() (string, bool) {
	file, ok := r.stdin.(*os.File)
	if !ok {
		return r.readLine()
	}
	info, err := file.Stat()
	if err == nil && info.Mode()&os.ModeCharDevice != 0 {
		return "", false
	}

	type result struct {
		value string
		ok    bool
	}
	done := make(chan result, 1)
	go func() {
		value, ok := r.readLine()
		done <- result{value: value, ok: ok}
	}()
	select {
	case line := <-done:
		return line.value, line.ok
	case <-time.After(secretReadTimeout):
		return "", false
	}
}

func (r *secretReader) readLine() (string, bool) {
	line, err := r.lines.ReadString('\n')
	if err != nil && !errors.Is(err, io.EOF) {
		return "", false
	}
	value := strings.TrimRight(line, "\r\n")
	return value, value != ""
}

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

func readConfirmation(stdin io.Reader) (bool, error) {
	reader := bufio.NewReader(stdin)
	answer, err := reader.ReadString('\n')
	if err != nil && !errors.Is(err, io.EOF) {
		return false, fmt.Errorf("reading confirmation: %w", err)
	}
	answer = strings.ToLower(strings.TrimSpace(answer))
	return answer == "y" || answer == "yes", nil
}
