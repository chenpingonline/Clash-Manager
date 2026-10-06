export interface CoreUpdateProgress {
  id?: string; active?: boolean; stage?: string; message?: string; progress?: number
  downloadedBytes?: number; totalBytes?: number; attempt?: number; maxAttempts?: number
  retrying?: boolean; failureStage?: string
}

// The final status request can arrive before the last SSE event. Preserve byte
// counts from this operation, never from another update sharing the status URL.
export function coreUpdateFailure(current: CoreUpdateProgress, latest: CoreUpdateProgress | null, id: string, error: string): CoreUpdateProgress {
  const snapshot = latest?.id === id ? latest : current
  return { ...snapshot, active: false, stage: 'error', message: snapshot.stage === 'error' && snapshot.message ? snapshot.message : `更新失败：${error}` }
}
