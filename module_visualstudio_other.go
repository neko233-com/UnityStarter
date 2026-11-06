//go:build !windows

package main

import "context"

func checkVisualStudioModules(context.Context, []editorModule) {}
