package main

import (
	"fmt"
	"net"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
)

type tunInterfaceState struct {
	Name    string `json:"name"`
	Checked bool   `json:"checked"`
	Exists  bool   `json:"exists"`
	Up      bool   `json:"up"`
	TUN     bool   `json:"tun"`
	Reason  string `json:"reason,omitempty"`
}

// Read-only: no interface allocation and no route changes. Do not treat an
// arbitrary UP interface, or a TUN in a different namespace, as Core's device.
func inspectTunInterface(name, sysRoot string, sameNamespace bool) tunInterfaceState {
	s := tunInterfaceState{Name: name}
	if !sameNamespace {
		s.Reason = "core-network-namespace-differs"
		return s
	}
	if name == "" || len(name) > 15 || name == "." || name == ".." || strings.ContainsAny(name, "/\\\x00\r\n") {
		s.Reason = "invalid-interface-name"
		return s
	}
	s.Checked = true
	data, err := os.ReadFile(filepath.Join(sysRoot, name, "tun_flags"))
	if os.IsNotExist(err) {
		if _, existsErr := os.Stat(filepath.Join(sysRoot, name)); existsErr == nil {
			s.Exists = true
			s.Reason = "not-a-tun-interface"
		}
		return s
	}
	if err != nil {
		s.Checked = false
		s.Reason = err.Error()
		return s
	}
	s.Exists = true
	flags, err := strconv.ParseUint(strings.TrimSpace(string(data)), 0, 32)
	if err != nil {
		s.Checked = false
		s.Reason = "invalid-tun-flags"
		return s
	}
	s.TUN = flags&1 != 0
	oper, err := os.ReadFile(filepath.Join(sysRoot, name, "flags"))
	if err != nil {
		s.Checked = false
		s.Reason = err.Error()
		return s
	}
	bits, err := strconv.ParseUint(strings.TrimSpace(string(oper)), 0, 32)
	if err != nil {
		s.Checked = false
		s.Reason = "invalid-interface-flags"
		return s
	}
	s.Up = bits&uint64(net.FlagUp) != 0
	return s
}
func (h *helper) tunInterface(name string) tunInterfaceState {
	if runtime.GOOS != "linux" {
		return tunInterfaceState{Name: name, Reason: "interface-check-unsupported"}
	}
	same := true
	if p := h.primary(); p != nil && runtime.GOOS == "linux" {
		own, e1 := os.Readlink("/proc/self/ns/net")
		core, e2 := os.Readlink(fmt.Sprintf("/proc/%d/ns/net", p.PID))
		same = e1 == nil && e2 == nil && own == core
	}
	return inspectTunInterface(name, "/sys/class/net", same)
}
