//go:build windows

package main

import (
	"syscall"
)

func recentEditorProjects() []recentEditorProject {
	var out []recentEditorProject
	for _, source := range []struct{ key, kind string }{
		{`Software\Unity Technologies\Unity Editor 5.x`, kindUnity},
		{`Software\You San Di Technology\Tuanjie Editor 5.x`, kindTuanjie},
	} {
		subkey, err := syscall.UTF16PtrFromString(source.key)
		if err != nil {
			continue
		}
		var handle syscall.Handle
		if syscall.RegOpenKeyEx(syscall.HKEY_CURRENT_USER, subkey, 0, syscall.KEY_READ, &handle) != nil {
			continue
		}
		entries := collectRecentEditorProjects(source.kind, func(name string) []byte {
			key, err := syscall.UTF16PtrFromString(name)
			if err != nil {
				return nil
			}
			var kind, size uint32
			if syscall.RegQueryValueEx(handle, key, nil, &kind, nil, &size) != nil || kind != syscall.REG_BINARY || size == 0 || size > 32768 {
				return nil
			}
			data := make([]byte, size)
			if syscall.RegQueryValueEx(handle, key, nil, &kind, &data[0], &size) != nil || kind != syscall.REG_BINARY {
				return nil
			}
			return data[:size]
		})
		_ = syscall.RegCloseKey(handle)
		out = append(out, entries...)
	}
	return out
}
