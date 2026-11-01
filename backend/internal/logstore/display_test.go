package logstore

import (
	"os"
	"path/filepath"
	"testing"
)

func TestDisplaySettingsPersistWithoutChangingRetention(t *testing.T) {
	dir := t.TempDir()
	retention := Defaults()
	if err := SaveSettings(SettingsPath(dir), retention); err != nil {
		t.Fatal(err)
	}
	before, err := os.ReadFile(SettingsPath(dir))
	if err != nil {
		t.Fatal(err)
	}
	file := DisplaySettingsPath(dir)
	initial, err := LoadDisplaySettings(file)
	if err != nil || initial.Limit != 100 {
		t.Fatalf("default %+v %v", initial, err)
	}
	for _, limit := range []int{100, 200, 500, 800, 2000} {
		if err = SaveDisplaySettings(file, DisplaySettings{Limit: limit}); err != nil {
			t.Fatal(err)
		}
		got, err := LoadDisplaySettings(file)
		if err != nil || got.Limit != limit {
			t.Fatalf("reload %+v %v", got, err)
		}
	}
	for _, limit := range []int{0, -1, 101, 2001} {
		if err = SaveDisplaySettings(file, DisplaySettings{Limit: limit}); err == nil {
			t.Fatalf("accepted %d", limit)
		}
	}
	got, _ := LoadDisplaySettings(file)
	if got.Limit != 2000 {
		t.Fatal("invalid save overwrote value")
	}
	after, _ := os.ReadFile(SettingsPath(dir))
	if string(after) != string(before) {
		t.Fatal("display setting changed retention")
	}
	info, _ := os.Stat(file)
	if info.Mode().Perm() != 0600 {
		t.Fatalf("mode %v", info.Mode())
	}
	if err = os.WriteFile(file, []byte(`{"limit":123}`), 0600); err != nil {
		t.Fatal(err)
	}
	if _, err = LoadDisplaySettings(file); err == nil {
		t.Fatal("invalid persisted value accepted")
	}
	if err = os.WriteFile(file, []byte(`{`), 0600); err != nil {
		t.Fatal(err)
	}
	if _, err = LoadDisplaySettings(file); err == nil {
		t.Fatal("corrupt file accepted")
	}
	if err = SaveDisplaySettings(filepath.Join(file, "blocked"), DisplaySettings{Limit: 100}); err == nil {
		t.Fatal("write failure ignored")
	}
}
