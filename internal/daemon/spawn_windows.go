//go:build windows

package daemon

import "syscall"

const detachedProcessFlag = 0x00000008

func detachedProcess() *syscall.SysProcAttr {
	return &syscall.SysProcAttr{
		CreationFlags: syscall.CREATE_NEW_PROCESS_GROUP | detachedProcessFlag,
		HideWindow:    true,
	}
}
