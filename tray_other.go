//go:build !windows

package main

import (
	"fmt"
	"os"
)

// runWithTrayIcon 在非 Windows 平台直接以命令行方式运行
func runWithTrayIcon(config *Config) {
	runForwarders(config)
}

// AttachParentConsole 非 Windows 平台无需附加控制台，空实现
func AttachParentConsole() {}

// showFatal 在非 Windows 平台直接输出到 stderr
func showFatal(msg string) {
	fmt.Fprintln(os.Stderr, msg)
}
