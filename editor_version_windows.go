//go:build windows

package main

import (
	"encoding/binary"
	"fmt"
	"runtime"
	"syscall"
	"unsafe"
)

var versionDLL = syscall.NewLazyDLL("version.dll")
var getFileVersionInfoSize = versionDLL.NewProc("GetFileVersionInfoSizeW")
var getFileVersionInfo = versionDLL.NewProc("GetFileVersionInfoW")
var queryFileVersion = versionDLL.NewProc("VerQueryValueW")

// The editor's product version identifies installations in custom folders;
// the numeric Windows file version does not contain Unity's release channel.
func editorExecutableVersion(exe string) string {
	path, err := syscall.UTF16PtrFromString(exe)
	if err != nil {
		return ""
	}
	size, _, _ := getFileVersionInfoSize.Call(uintptr(unsafe.Pointer(path)), 0)
	if size == 0 || size > 4<<20 {
		return ""
	}
	data := make([]byte, int(size))
	if ok, _, _ := getFileVersionInfo.Call(uintptr(unsafe.Pointer(path)), 0, size, uintptr(unsafe.Pointer(&data[0]))); ok == 0 {
		return ""
	}
	query := func(name string, unitSize uintptr) (unsafe.Pointer, uint32) {
		key, err := syscall.UTF16PtrFromString(name)
		if err != nil {
			return nil, 0
		}
		var value unsafe.Pointer
		var length uint32
		ok, _, _ := queryFileVersion.Call(uintptr(unsafe.Pointer(&data[0])), uintptr(unsafe.Pointer(key)), uintptr(unsafe.Pointer(&value)), uintptr(unsafe.Pointer(&length)))
		start, end := uintptr(unsafe.Pointer(&data[0])), uintptr(unsafe.Pointer(&data[0]))+uintptr(len(data))
		pos := uintptr(value)
		if ok == 0 || length == 0 || pos < start || pos >= end || uintptr(length) > (end-pos)/unitSize {
			return nil, 0
		}
		return value, length
	}
	table := "040904b0"
	if ptr, n := query(`\VarFileInfo\Translation`, 1); n >= 4 {
		translation := unsafe.Slice((*byte)(ptr), n)
		table = fmt.Sprintf("%04x%04x", binary.LittleEndian.Uint16(translation), binary.LittleEndian.Uint16(translation[2:]))
	}
	ptr, n := query(`\StringFileInfo\`+table+`\ProductVersion`, 2)
	if ptr == nil {
		return ""
	}
	product := syscall.UTF16ToString(unsafe.Slice((*uint16)(ptr), n))
	runtime.KeepAlive(data)
	return parseEditorProductVersion(product)
}
