//go:build !windows

package main

import "fmt"

func isManagedInstall() bool       { return false }
func startUpdateHelper(int) error  { return fmt.Errorf("MSI updates require Windows") }
func runUpdateHelper([]string) int { return 1 }
