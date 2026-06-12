//go:build windows

package main

import (
	"unsafe"

	"golang.org/x/sys/windows"
)

const singleInstanceMutexName = `Global\TongKaTong_GUI_SingleInstance_v1`

type singleInstanceLock struct {
	handle windows.Handle
}

func acquireSingleInstance() (*singleInstanceLock, bool, error) {
	name, err := windows.UTF16PtrFromString(singleInstanceMutexName)
	if err != nil {
		return nil, false, err
	}

	sa := windows.SecurityAttributes{
		Length:        uint32(unsafe.Sizeof(windows.SecurityAttributes{})),
		InheritHandle: 0,
	}

	handle, err := windows.CreateMutex(&sa, true, name)
	if err != nil && err != windows.ERROR_ALREADY_EXISTS {
		return nil, false, err
	}
	if err == windows.ERROR_ALREADY_EXISTS {
		if handle != 0 {
			_ = windows.CloseHandle(handle)
		}
		return nil, true, nil
	}

	return &singleInstanceLock{handle: handle}, false, nil
}

func (l *singleInstanceLock) Release() {
	if l == nil || l.handle == 0 {
		return
	}
	_ = windows.CloseHandle(l.handle)
	l.handle = 0
}

func showAlreadyRunningMessage(title, message string) {
	captionPtr, _ := windows.UTF16PtrFromString(title)
	messagePtr, _ := windows.UTF16PtrFromString(message)
	_, _ = windows.MessageBox(0, messagePtr, captionPtr, windows.MB_OK|windows.MB_ICONWARNING)
}
