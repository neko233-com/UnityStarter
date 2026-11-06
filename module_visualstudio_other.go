//go:build !windows

package main

import (
	"context"
	"fmt"
)

func checkVisualStudioModules(context.Context, []editorModule) {}

func startVisualStudioModifier(editorModule) error {
	return fmt.Errorf("Visual Studio Installer is only available on Windows")
}
