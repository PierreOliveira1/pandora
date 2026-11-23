//go:build !windows

package daemon

import "syscall"

func detachedProcess() *syscall.SysProcAttr {
	return &syscall.SysProcAttr{Setsid: true}
}
