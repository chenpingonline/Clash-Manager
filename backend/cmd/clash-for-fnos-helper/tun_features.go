package main

import (
	"fmt"
	"strings"
)

func tunVersionFeatures(version string) map[string]any {
	var major, minor, patch int
	known := false
	if n, _ := fmt.Sscanf(strings.TrimPrefix(version, "v"), "%d.%d.%d", &major, &minor, &patch); n == 3 {
		known = true
	}
	atLeast := func(requiredPatch int) bool {
		return known && (major > 1 || (major == 1 && (minor > 19 || (minor == 19 && patch >= requiredPatch))))
	}
	defaultStack := "gvisor"
	if atLeast(32) {
		defaultStack = "mips"
	}
	if !known {
		defaultStack = ""
	}
	return map[string]any{"coreVersion": version, "defaultStack": defaultStack, "mips": atLeast(31), "congestionController": atLeast(32)}
}
