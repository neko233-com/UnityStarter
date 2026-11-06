//go:build windows

package main

import (
	"fmt"
	"runtime"
	"syscall"
	"unsafe"
)

var downloadKernel = syscall.NewLazyDLL("kernel32.dll")
var createDownloadMutex = downloadKernel.NewProc("CreateMutexW")
var releaseDownloadMutex = downloadKernel.NewProc("ReleaseMutex")

// Windows mutexes are thread-owned; keep this download coordinator on the
// same OS thread until it releases the mutex. Other downloads remain parallel.
func acquireDownloadLock(key string) (func(), error) {
	runtime.LockOSThread()
	name, err := syscall.UTF16PtrFromString(`Local\UnityStarterDownload-` + key)
	if err != nil {
		runtime.UnlockOSThread()
		return nil, err
	}
	h, _, callErr := createDownloadMutex.Call(0, 0, uintptr(unsafe.Pointer(name)))
	if h == 0 {
		runtime.UnlockOSThread()
		return nil, fmt.Errorf("create download lock: %w", callErr)
	}
	handle := syscall.Handle(h)
	status, waitErr := syscall.WaitForSingleObject(handle, 0)
	if waitErr != nil || (status != syscall.WAIT_OBJECT_0 && status != 0x80) {
		syscall.CloseHandle(handle)
		runtime.UnlockOSThread()
		if waitErr != nil {
			return nil, waitErr
		}
		return nil, fmt.Errorf("download already running in another process")
	}
	return func() {
		releaseDownloadMutex.Call(uintptr(handle))
		syscall.CloseHandle(handle)
		runtime.UnlockOSThread()
	}, nil
}
