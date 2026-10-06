import { describe, expect, it } from 'vitest'
import { coreUpdateFailure } from './core-update'

describe('core update failure progress', () => {
  it('uses the terminal status even if SSE has not delivered it yet', () => {
    const result = coreUpdateFailure({ id: 'one', progress: 20 }, { id: 'one', stage: 'error', message: '下载超时', downloadedBytes: 40, totalBytes: 100, progress: 40, attempt: 3 }, 'one', 'HTTP error')
    expect(result).toMatchObject({ stage: 'error', active: false, message: '下载超时', progress: 40, downloadedBytes: 40, attempt: 3 })
  })
  it('keeps the last known bytes when status lookup fails or belongs to another update', () => {
    for (const latest of [null, { id: 'other', progress: 99 }]) {
      expect(coreUpdateFailure({ id: 'one', progress: 40, downloadedBytes: 40 }, latest, 'one', '下载超时')).toMatchObject({ id: 'one', stage: 'error', progress: 40, downloadedBytes: 40, message: '更新失败：下载超时' })
    }
  })
})
