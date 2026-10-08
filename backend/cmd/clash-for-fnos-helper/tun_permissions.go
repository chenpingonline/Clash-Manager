package main

import (
	"strconv"
	"strings"
)

func hasNetAdmin(status string) bool {
	for _, line := range strings.Split(status, "\n") {
		fields := strings.Fields(line)
		if len(fields) == 2 && fields[0] == "CapEff:" {
			bits, err := strconv.ParseUint(fields[1], 16, 64)
			return err == nil && bits&(1<<12) != 0
		}
	}
	return false
}
