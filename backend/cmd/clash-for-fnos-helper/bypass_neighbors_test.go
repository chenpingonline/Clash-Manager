package main

import (
	"testing"
)

func TestBypassNeighborsOnlyIncludesUsableLANRecords(t *testing.T) {
	raw := []byte(`[
 {"dst":"192.168.1.100","lladdr":"02:11:22:33:44:55","dev":"eth0","state":["REACHABLE"]},
 {"dst":"192.168.1.101","lladdr":"02:11:22:33:44:66","dev":"eth0","state":["STALE"]},
 {"dst":"192.168.1.100","lladdr":"02:11:22:33:44:55","dev":"eth0","state":["REACHABLE"]},
 {"dst":"192.168.1.102","dev":"eth0","state":["INCOMPLETE"]},
 {"dst":"192.168.1.103","lladdr":"02:11:22:33:44:77","dev":"eth0","state":["FAILED"]},
 {"dst":"192.168.1.104","lladdr":"02:11:22:33:44:88","dev":"other","state":["REACHABLE"]},
 {"dst":"192.168.2.100","lladdr":"02:11:22:33:44:99","dev":"eth0","state":["REACHABLE"]},
 {"dst":"192.168.1.20","lladdr":"02:11:22:33:44:aa","dev":"eth0","state":["PERMANENT"]}
 ]`)
	rows, err := parseBypassNeighbors(raw, []bypassInterface{{Name: "eth0", Address: "192.168.1.20", CIDR: "192.168.1.0/24"}})
	if err != nil {
		t.Fatal(err)
	}
	if len(rows) != 2 || rows[0].IP != "192.168.1.100" || rows[1].State != "STALE" {
		t.Fatalf("unexpected records: %v", rows)
	}
	if _, err := parseBypassNeighbors([]byte("not-json"), nil); err == nil {
		t.Fatal("malformed neighbor data ignored")
	}
}
