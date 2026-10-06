package main

func (g *gateway) setCoreUpdateStage(id, stage, message string, progress *int, active bool) {
	g.networkOperationMu.Lock()
	defer g.networkOperationMu.Unlock()
	if g.coreOperation.ID != id {
		g.coreOperation = networkSaveStatus{ID: id}
	}
	g.coreOperation.Active, g.coreOperation.Message = active, message
	if stage == "error" {
		g.coreOperation.FailureStage = g.coreOperation.Stage
	}
	g.coreOperation.Stage = stage
	if progress != nil {
		g.coreOperation.Progress = progress
	}
}
