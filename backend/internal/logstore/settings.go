// Package logstore implements bounded application-owned log storage.
package logstore

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
)

const MiB int64 = 1 << 20

type Policy struct {
	Days   int `json:"days"`
	MaxMiB int `json:"maxMiB"`
}
type Settings struct {
	History   Policy `json:"history"`
	Core      Policy `json:"core"`
	SaveLevel string `json:"saveLevel"`
}

func Defaults() Settings {
	return Settings{History: Policy{7, 20}, Core: Policy{7, 10}, SaveLevel: "info"}
}
func (p Policy) Validate() error {
	if p.Days < 0 || p.Days > 365 || p.MaxMiB < 1 || p.MaxMiB > 1024 {
		return fmt.Errorf("日志保留天数须为 0–365，容量须为 1–1024 MiB")
	}
	return nil
}
func (s Settings) Validate() error {
	if err := s.History.Validate(); err != nil {
		return err
	}
	if err := s.Core.Validate(); err != nil {
		return err
	}
	switch s.SaveLevel {
	case "debug", "info", "warning", "error":
		return nil
	}
	return fmt.Errorf("日志保存级别无效")
}
func SettingsPath(etcDir string) string { return filepath.Join(etcDir, "log-retention.json") }
func LoadSettings(file string) (Settings, error) {
	s := Defaults()
	body, err := os.ReadFile(file)
	if os.IsNotExist(err) {
		return s, nil
	}
	if err != nil {
		return s, err
	}
	if err := json.Unmarshal(body, &s); err != nil {
		return Defaults(), err
	}
	return s, s.Validate()
}
func SaveSettings(file string, s Settings) error {
	if err := s.Validate(); err != nil {
		return err
	}
	body, err := json.MarshalIndent(s, "", "  ")
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(file), 0o700); err != nil {
		return err
	}
	f, err := os.CreateTemp(filepath.Dir(file), ".log-settings-*")
	if err != nil {
		return err
	}
	name := f.Name()
	defer os.Remove(name)
	if err = f.Chmod(0o600); err == nil {
		_, err = f.Write(append(body, '\n'))
	}
	if err == nil {
		err = f.Sync()
	}
	closeErr := f.Close()
	if err != nil {
		return err
	}
	if closeErr != nil {
		return closeErr
	}
	return os.Rename(name, file)
}
