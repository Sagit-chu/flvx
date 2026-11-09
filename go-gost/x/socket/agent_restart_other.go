//go:build !linux

package socket

import "fmt"

func checkAgentDetach() error               { return fmt.Errorf("OpenRC Agent 重启仅支持 Linux") }
func startDetachedAgentHelper(string) error { return checkAgentDetach() }
