package main

import (
	"context"
	"errors"
	"reflect"
	"testing"
)

func TestTunRepairStages(t *testing.T) {
	for _, tc := range []struct {
		name                        string
		initial                     tunProbe
		allow, loadFail, createFail bool
		want                        []string
		wantErr                     bool
	}{
		{"missing driver and device", tunProbe{}, true, false, false, []string{"load", "create"}, false},
		{"missing node", tunProbe{Driver: true}, true, false, false, []string{"create"}, false},
		{"already usable", tunProbe{Device: true, Driver: true, Usable: true}, true, false, false, nil, false},
		{"container", tunProbe{}, false, false, false, nil, true},
		{"kernel unsupported", tunProbe{}, true, true, false, []string{"load"}, true},
		{"node creation denied", tunProbe{Driver: true}, true, false, true, []string{"create"}, true},
		{"invalid existing node", tunProbe{Device: true, Error: "invalid device"}, true, false, false, nil, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			p := tc.initial
			var calls []string
			ops := tunRepairOps{probe: func() tunProbe { return p }, load: func(context.Context) error {
				calls = append(calls, "load")
				if tc.loadFail {
					return errors.New("unsupported")
				}
				p.Driver = true
				return nil
			}, create: func() error {
				calls = append(calls, "create")
				if tc.createFail {
					return errors.New("denied")
				}
				p.Device = true
				p.Usable = true
				return nil
			}}
			_, err := repairTun(context.Background(), tc.allow, ops)
			if (err != nil) != tc.wantErr || !reflect.DeepEqual(calls, tc.want) {
				t.Fatalf("calls=%v err=%v", calls, err)
			}
		})
	}
}
func TestTunRepairRequiresRegisteredDriverAndRechecks(t *testing.T) {
	created := false
	_, err := repairTun(context.Background(), true, tunRepairOps{func() tunProbe { return tunProbe{} }, func(context.Context) error { return nil }, func() error { created = true; return nil }})
	if err == nil || created {
		t.Fatal("created a node without a registered driver")
	}
	_, err = repairTun(context.Background(), true, tunRepairOps{func() tunProbe { return tunProbe{Driver: true} }, nil, func() error { return nil }})
	if err == nil {
		t.Fatal("reported success without usable device")
	}
}
