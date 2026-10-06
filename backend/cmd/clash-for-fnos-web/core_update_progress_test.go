package main

import (
	"context"
	"testing"
)

func TestCoreUpdateFailureRetainsDownloadProgress(t *testing.T) {
	percent := 40
	g := &gateway{coreOperation: networkSaveStatus{ID: "one", Stage: "downloading", Active: true, Progress: &percent, DownloadedBytes: 4, TotalBytes: 10, Attempt: 3}}
	g.setCoreUpdateStage("one", "error", "下载超时", nil, false)
	got := g.coreOperation
	if got.Active || got.Stage != "error" || got.FailureStage != "downloading" || got.Progress == nil || *got.Progress != 40 || got.DownloadedBytes != 4 || got.Attempt != 3 {
		t.Fatalf("%+v", got)
	}
	g.setCoreUpdateStage("two", "checking", "检查更新", nil, true)
	if g.coreOperation.Progress != nil || g.coreOperation.DownloadedBytes != 0 || g.coreOperation.Attempt != 0 || g.coreOperation.FailureStage != "" {
		t.Fatalf("new update inherited previous bytes: %+v", g.coreOperation)
	}
}

func TestCoreUpdateRejectsConcurrentUpdate(t *testing.T) {
	g := &gateway{}
	g.coreUpdateMu.Lock()
	defer g.coreUpdateMu.Unlock()
	if _, err := g.updateCore(context.Background(), false, false, "second-update"); err == nil {
		t.Fatal("accepted a concurrent update")
	}
	if g.coreOperation.ID != "" {
		t.Fatal("rejected update overwrote operation status")
	}
}
