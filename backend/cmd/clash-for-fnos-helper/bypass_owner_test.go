package main

import (
	"context"
	"errors"
	"strings"
	"testing"
)

func TestBypassTextOwnerStaysAtTableScope(t *testing.T) {
	for _, tc := range []struct {
		text, owner string
		ok          bool
	}{
		{`table ip clash_manager_bypass {
    comment "clash-manager:abc"
    chain capture { }
  }`, "clash-manager:abc", true},
		{`table ip clash_manager_bypass { # handle 5
    comment "another-owner";
  }`, "another-owner", true},
		{`table ip other { comment "clash-manager:abc" }`, "", false},
		{`table ip clash_manager_bypass {
    chain capture {
      comment "clash-manager:abc"
    }
  }`, "", false},
		{`table ip clash_manager_bypass {
    chain capture { meta l4proto { tcp, udp } counter comment "} {" }
  }`, "", false},
	} {
		owner, err := bypassOwnerFromText(tc.text)
		if (err == nil) != tc.ok || owner != tc.owner {
			t.Fatalf("%s: owner=%q err=%v", tc.text, owner, err)
		}
	}
}
func TestBypassLegacyJSONOwnershipAndRecovery(t *testing.T) {
	for _, scenario := range []string{"matching", "foreign", "unreadable", "missing-marker"} {
		t.Run(scenario, func(t *testing.T) {
			h := testHelper(t)
			s := testBypassPlan("redir")
			if err := h.saveBypassState(s); err != nil {
				t.Fatal(err)
			}
			legacy := []byte(`{"nftables":[{"table":{"family":"ip","name":"clash_manager_bypass"}}]}`)
			deleted := false
			h.bypassSystem = &bypassSystem{
				readForward:  func() (string, error) { return "0", nil },
				writeForward: func(string) error { t.Fatal("unexpected forward mutation"); return nil },
				run: func(_ context.Context, _ string, name string, args ...string) ([]byte, error) {
					switch name + " " + strings.Join(args, " ") {
					case "nft -j list tables":
						return legacy, nil
					case "nft -nn list table ip clash_manager_bypass":
						if scenario == "unreadable" {
							return nil, errors.New("unavailable")
						}
						owner := "clash-manager:" + s.Token
						if scenario == "foreign" {
							owner = "clash-manager:" + strings.Repeat("b", 32)
						}
						if scenario == "missing-marker" {
							return []byte("table ip clash_manager_bypass {\n chain capture { }\n}\n"), nil
						}
						return []byte("table ip clash_manager_bypass {\n comment \"" + owner + "\"\n chain capture { }\n}\n"), nil
					case "nft delete table ip clash_manager_bypass":
						deleted = true
						return nil, nil
					default:
						t.Fatalf("unexpected command: %s %v", name, args)
						return nil, nil
					}
				},
			}
			status := h.bypassManagedStatus(context.Background(), legacy)
			if scenario == "matching" && status["rulesPresent"] != true {
				t.Fatal(status)
			}
			if scenario == "foreign" && status["rulesPresent"] != false {
				t.Fatal(status)
			}
			if (scenario == "unreadable" || scenario == "missing-marker") && status["rulesPresent"] != nil {
				t.Fatal("unverified must not be reported as mismatch", status)
			}
			err := h.cleanupBypass(context.Background(), 0)
			if scenario == "matching" {
				if err != nil || !deleted || fileExists(h.bypassJournalPath()) {
					t.Fatal(err, deleted)
				}
			} else if err == nil || deleted || !fileExists(h.bypassJournalPath()) {
				t.Fatal("unverified/foreign rules must remain", err, deleted)
			}
		})
	}
}
func TestBypassExplicitForeignJSONDoesNotFallBack(t *testing.T) {
	h := testHelper(t)
	h.bypassSystem = &bypassSystem{run: func(context.Context, string, string, ...string) ([]byte, error) {
		t.Fatal("must not override explicit owner")
		return nil, nil
	}}
	exists, owner, err := h.bypassOwner(context.Background(), []byte(`{"nftables":[{"table":{"family":"ip","name":"clash_manager_bypass","comment":"foreign"}}]}`))
	if !exists || owner != "foreign" || err != nil {
		t.Fatal(exists, owner, err)
	}
}
