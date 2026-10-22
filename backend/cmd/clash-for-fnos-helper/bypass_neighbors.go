package main

import (
	"context"
	"encoding/json"
	"net"
	"os/exec"
	"time"
)

type bypassNeighbor struct {
	IP        string `json:"ip"`
	MAC       string `json:"mac"`
	Interface string `json:"interface"`
	State     string `json:"state"`
}

func readBypassNeighbors(ctx context.Context, interfaces []bypassInterface) ([]bypassNeighbor, error) {
	readCtx, cancel := context.WithTimeout(ctx, 2*time.Second)
	defer cancel()
	raw, err := exec.CommandContext(readCtx, "ip", "-j", "-4", "neigh", "show").Output()
	if err != nil {
		return nil, err
	}
	return parseBypassNeighbors(raw, interfaces)
}
func parseBypassNeighbors(raw []byte, interfaces []bypassInterface) ([]bypassNeighbor, error) {
	var entries []struct {
		IP        string   `json:"dst"`
		MAC       string   `json:"lladdr"`
		Interface string   `json:"dev"`
		State     []string `json:"state"`
	}
	if err := json.Unmarshal(raw, &entries); err != nil {
		return nil, err
	}
	out := []bypassNeighbor{}
	seen := map[string]bool{}
	usable := map[string]bool{"REACHABLE": true, "STALE": true, "DELAY": true, "PROBE": true, "PERMANENT": true}
	for _, entry := range entries {
		ip := net.ParseIP(entry.IP)
		mac, err := net.ParseMAC(entry.MAC)
		if ip == nil || ip.To4() == nil || err != nil || len(mac) != 6 || mac[0]&1 != 0 || mac.String() == "00:00:00:00:00:00" {
			continue
		}
		allowed := false
		for _, iface := range interfaces {
			_, subnet, e := net.ParseCIDR(iface.CIDR)
			if e == nil && iface.Name == entry.Interface && subnet.Contains(ip) && iface.Address != ip.String() {
				allowed = true
			}
		}
		if !allowed {
			continue
		}
		state := ""
		for _, value := range entry.State {
			if usable[value] {
				state = value
				break
			}
		}
		if state == "" {
			continue
		}
		key := entry.Interface + "/" + ip.String() + "/" + mac.String()
		if seen[key] {
			continue
		}
		seen[key] = true
		out = append(out, bypassNeighbor{ip.String(), mac.String(), entry.Interface, state})
	}
	return out, nil
}
