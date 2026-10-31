package main

import (
	"os"
	"path/filepath"
	"testing"
)

func TestTunInterfaceRequiresExactUpTunDevice(t *testing.T) {
	root := t.TempDir()
	dir := filepath.Join(root, "Meta")
	os.Mkdir(dir, 0700)
	os.WriteFile(filepath.Join(dir, "tun_flags"), []byte("0x1001\n"), 0600)
	os.WriteFile(filepath.Join(dir, "flags"), []byte("0x1001\n"), 0600)
	s := inspectTunInterface("Meta", root, true)
	if !s.Checked || !s.Exists || !s.Up || !s.TUN {
		t.Fatal(s)
	}
	os.WriteFile(filepath.Join(dir, "flags"), []byte("0x1000"), 0600)
	if inspectTunInterface("Meta", root, true).Up {
		t.Fatal("accepted a DOWN interface")
	}
	os.WriteFile(filepath.Join(dir, "tun_flags"), []byte("0x2"), 0600)
	if inspectTunInterface("Meta", root, true).TUN {
		t.Fatal("accepted TAP as TUN")
	}
	if inspectTunInterface("other", root, true).Exists {
		t.Fatal("matched another name")
	}
	if inspectTunInterface("Meta", root, false).Checked {
		t.Fatal("checked a different Core namespace")
	}
	for _, name := range []string{"", "..", "../Meta", "a/b"} {
		if inspectTunInterface(name, root, true).Checked {
			t.Fatal("unsafe device name accepted")
		}
	}
	os.Remove(filepath.Join(dir, "tun_flags"))
	if s := inspectTunInterface("Meta", root, true); !s.Exists || s.TUN {
		t.Fatal("accepted ordinary network interface", s)
	}
}
