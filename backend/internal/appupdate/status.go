package appupdate

// ReadStatusForVersion drops terminal results made obsolete by an installed
// version, including manual FPK upgrades. Active workers remain responsible for
// reporting their own result; their restart gap must not be mistaken for success.
func ReadStatusForVersion(dir, current string) (Status, error) {
	status, err := ReadStatus(dir)
	if err != nil {
		return Status{}, err
	}
	reconciled := reconcileStatus(status, current)
	if reconciled != status {
		if err := WriteStatus(dir, reconciled); err != nil {
			return Status{}, err
		}
	}
	return reconciled, nil
}

func reconcileStatus(status Status, current string) Status {
	if status.Stage != "error" && status.Stage != "success" {
		return status
	}
	installed, err := Version(current)
	if err != nil {
		return status
	}
	target, err := Version(status.TargetVersion)
	if err != nil {
		return status
	}
	if Newer(installed, target) {
		return Status{Stage: "idle"}
	}
	if installed != target {
		return status
	}
	// The new helper is running at the submitted version, resolving a lost
	// daemon response. Preserve success so the old page can finish and reload.
	if status.OutcomeUnknown {
		status.Stage, status.Progress, status.Error, status.OutcomeUnknown = "success", 100, "", false
		return status
	}
	if status.Stage == "error" {
		return Status{Stage: "idle"}
	}
	return status
}
