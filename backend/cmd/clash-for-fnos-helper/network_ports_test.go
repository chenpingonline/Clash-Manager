package main

import (
	"net"
	"strconv"
	"strings"
	"testing"
)

func TestNetworkPortsRejectDuplicate(t *testing.T) {
	raw := "external-controller: 127.0.0.1:9090\nmixed-port: 9090\n"
	if err := validateNetworkPorts(raw, raw, false); err == nil || !strings.Contains(err.Error(), "Controller API") {
		t.Fatalf("expected duplicate error, got %v", err)
	}
}
func TestNetworkPortsProbeChangedButNotUnchanged(t *testing.T) {
	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer l.Close()
	raw := "external-controller: " + l.Addr().String() + "\n"
	if err := validateNetworkPorts(raw, raw, false); err != nil {
		t.Fatal(err)
	}
	if err := validateNetworkPorts(raw, raw, true); err == nil {
		t.Fatal("offline occupied port accepted")
	}
	if err := validateNetworkPorts(raw, "external-controller: 127.0.0.1:1\n", false); err == nil {
		t.Fatal("changed occupied port accepted")
	}
}

func TestNetworkPortsAllowLanTransitionDoesNotProbeCurrentCore(t *testing.T) {
	for _, test := range []struct {
		name          string
		listenAddress string
		previousAllow bool
		nextAllow     bool
	}{
		{name: "enable", listenAddress: "127.0.0.1:0", previousAllow: false, nextAllow: true},
		{name: "disable", listenAddress: "0.0.0.0:0", previousAllow: true, nextAllow: false},
	} {
		t.Run(test.name, func(t *testing.T) {
			listener, err := net.Listen("tcp", test.listenAddress)
			if err != nil {
				t.Fatal(err)
			}
			defer listener.Close()
			_, port, err := net.SplitHostPort(listener.Addr().String())
			if err != nil {
				t.Fatal(err)
			}
			previous := "external-controller: 127.0.0.1:9090\nmixed-port: " + port + "\nallow-lan: " + strconv.FormatBool(test.previousAllow) + "\n"
			next := "external-controller: 127.0.0.1:9090\nmixed-port: " + port + "\nallow-lan: " + strconv.FormatBool(test.nextAllow) + "\n"
			if err := validateNetworkPorts(next, previous, false); err != nil {
				t.Fatalf("allow-lan transition rejected the current Core listener: %v", err)
			}
		})
	}
}

func TestNetworkPortsCheckUDP(t *testing.T) {
	l, err := net.ListenPacket("udp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer l.Close()
	_, port, _ := net.SplitHostPort(l.LocalAddr().String())
	old := "external-controller: 127.0.0.1:9090\n"
	err = validateNetworkPorts(old+"mixed-port: "+port+"\n", old, false)
	if err == nil || !strings.Contains(err.Error(), "udp") {
		t.Fatalf("expected UDP conflict: %v", err)
	}
}

func TestCustomListenerUDPAndCurrentCoreOwnership(t *testing.T) {
	udp, err := net.ListenPacket("udp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer udp.Close()
	_, port, _ := net.SplitHostPort(udp.LocalAddr().String())
	base := "external-controller: 127.0.0.1:9191\n"
	raw := base + "listeners: [{name: own, type: socks, listen: 127.0.0.1, udp: true, port: " + port + "}]\n"
	if err := validateNetworkPorts(raw, base, false); err == nil || !strings.Contains(err.Error(), "udp") {
		t.Fatal("missed custom UDP occupancy", err)
	}
	if err := validateNetworkPorts(raw, raw, false); err != nil {
		t.Fatal("probed current Core against itself", err)
	}
}
func TestCustomListenerRangesAndAddressConflicts(t *testing.T) {
	raw := "listeners: [{name: range, type: mixed, listen: 127.0.0.1, udp: true, port: '41000-41001,41003'}]"
	listeners, err := customNetworkListeners(raw)
	if err != nil || len(listeners) != 6 {
		t.Fatal(listeners, err)
	}
	for _, value := range []string{"0", "65536", "10-1", "1-65535", "invalid"} {
		if _, err := customNetworkListeners("listeners: [{port: '" + value + "'}]"); err == nil {
			t.Fatal("accepted invalid/unbounded ports", value)
		}
	}
	if !networkListenersOverlap(networkListener{"tcp", "[::]:41000", ""}, networkListener{"tcp", "127.0.0.1:41000", ""}) {
		t.Fatal("missed dual-stack wildcard conflict")
	}
	if networkListenersOverlap(networkListener{"tcp", "127.0.0.1:41000", ""}, networkListener{"tcp", "127.0.0.2:41000", ""}) {
		t.Fatal("different specific addresses conflict")
	}
}
