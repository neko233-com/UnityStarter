//go:build !windows

package main

import (
	"context"
	"fmt"
)

func queryVisualStudioInstances(context.Context) ([]byte, []byte, error) {
	return nil, nil, fmt.Errorf("Visual Studio is only available on Windows")
}

func startVisualStudioModifier(editorModule) error {
	return fmt.Errorf("Visual Studio Installer is only available on Windows")
}
