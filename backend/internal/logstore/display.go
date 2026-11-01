package logstore

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
)

// DisplaySettings are shared by all clients of this installation. They do not
// affect retention policy or the number of logs written by the core.
type DisplaySettings struct {
	Limit int `json:"limit"`
}

func DefaultDisplaySettings() DisplaySettings { return DisplaySettings{Limit: 100} }
func (s DisplaySettings) Validate() error {
	switch s.Limit {
	case 100, 200, 500, 800, 2000:
		return nil
	}
	return fmt.Errorf("日志显示行数须为 100、200、500、800 或 2000")
}
func DisplaySettingsPath(etcDir string) string { return filepath.Join(etcDir, "log-display.json") }
func LoadDisplaySettings(file string) (DisplaySettings, error) {
	s := DefaultDisplaySettings()
	body, err := os.ReadFile(file)
	if os.IsNotExist(err) {
		return s, nil
	}
	if err != nil {
		return s, err
	}
	if err = json.Unmarshal(body, &s); err != nil {
		return DefaultDisplaySettings(), err
	}
	return s, s.Validate()
}
func SaveDisplaySettings(file string, s DisplaySettings) error {
	if err := s.Validate(); err != nil {
		return err
	}
	return saveSettingsJSON(file, s)
}
