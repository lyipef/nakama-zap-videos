//go:build !windows

package main

import "os/exec"

func prepareFFmpeg(tmp string) (string, error) { return exec.LookPath("ffmpeg") }
func setupConsole()                            {}
func hideWindow(cmd *exec.Cmd)                 {}
func pickFolder() string                       { return "" }
func openFolder(p string)                      {}
