package appupdate

import (
	"os"
	"path/filepath"
	"testing"
)

func TestReadStatusForVersion(t *testing.T) {
	tests := []struct {
		name, current string
		status        Status
		want          Status
	}{
		{"manual install supersedes old failure", "1.3.23", Status{Stage: "error", TargetVersion: "1.3.22", Progress: 88, Error: "old mismatch", Downloaded: 42, Total: 42}, Status{Stage: "idle"}},
		{"manual install reaches failed target", "v1.3.23", Status{Stage: "error", TargetVersion: "1.3.23", Error: "old failure"}, Status{Stage: "idle"}},
		{"next version failure remains", "1.3.23", Status{Stage: "error", TargetVersion: "1.3.24", Error: "current failure"}, Status{Stage: "error", TargetVersion: "1.3.24", Error: "current failure"}},
		{"active restart remains", "1.3.23", Status{Stage: "restarting", TargetVersion: "1.3.23", Progress: 95, OutcomeUnknown: true}, Status{Stage: "restarting", TargetVersion: "1.3.23", Progress: 95, OutcomeUnknown: true}},
		{"current success remains for page reload", "1.3.23", Status{Stage: "success", TargetVersion: "1.3.23", Progress: 100}, Status{Stage: "success", TargetVersion: "1.3.23", Progress: 100}},
		{"old success cleared", "1.3.23", Status{Stage: "success", TargetVersion: "1.3.22", Progress: 100}, Status{Stage: "idle"}},
		{"installed target resolves unknown result", "1.3.23", Status{Stage: "error", TargetVersion: "v1.3.23", Progress: 90, Error: "lost response", AppCenterTaskID: "task", OutcomeUnknown: true}, Status{Stage: "success", TargetVersion: "v1.3.23", Progress: 100, AppCenterTaskID: "task"}},
		{"superseded unknown result cleared", "1.3.23", Status{Stage: "error", TargetVersion: "1.3.22", AppCenterTaskID: "task", OutcomeUnknown: true}, Status{Stage: "idle"}},
		{"future unknown result still blocks retry", "1.3.23", Status{Stage: "error", TargetVersion: "1.3.24", AppCenterTaskID: "task", OutcomeUnknown: true}, Status{Stage: "error", TargetVersion: "1.3.24", AppCenterTaskID: "task", OutcomeUnknown: true}},
		{"numeric version comparison", "1.3.10", Status{Stage: "error", TargetVersion: "1.3.9"}, Status{Stage: "idle"}},
		{"invalid current version preserved", "dev", Status{Stage: "error", TargetVersion: "1.3.22", OutcomeUnknown: true}, Status{Stage: "error", TargetVersion: "1.3.22", OutcomeUnknown: true}},
		{"missing target preserved", "1.3.23", Status{Stage: "error", Error: "unknown task", OutcomeUnknown: true}, Status{Stage: "error", Error: "unknown task", OutcomeUnknown: true}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			dir := t.TempDir()
			if err := WriteStatus(dir, tt.status); err != nil {
				t.Fatal(err)
			}
			logPath := filepath.Join(dir, "install.log")
			if err := os.WriteFile(logPath, []byte("diagnostic history"), 0o600); err != nil {
				t.Fatal(err)
			}
			got, err := ReadStatusForVersion(dir, tt.current)
			if err != nil {
				t.Fatal(err)
			}
			got.UpdatedAt = 0
			if got != tt.want {
				t.Fatalf("got %+v, want %+v", got, tt.want)
			}
			persisted, err := ReadStatus(dir)
			if err != nil {
				t.Fatal(err)
			}
			persisted.UpdatedAt = 0
			if persisted != tt.want {
				t.Fatalf("persisted %+v, want %+v", persisted, tt.want)
			}
			log, err := os.ReadFile(logPath)
			if err != nil || string(log) != "diagnostic history" {
				t.Fatalf("diagnostic log changed: %q, %v", log, err)
			}
		})
	}
}
