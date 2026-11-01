import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { loadLogDisplayLimit, saveLogDisplayLimit, logDisplayLimit, logDisplayReady, logDisplayLoading, logDisplaySaving, logDisplayError } from './log-view'
const response = (limit: number) => new Response(JSON.stringify({ limit }), { headers: { 'Content-Type': 'application/json' } })
beforeEach(() => { logDisplayLimit.value = 100; logDisplayReady.value = false; logDisplayLoading.value = false; logDisplaySaving.value = false; logDisplayError.value = '' })
afterEach(() => vi.unstubAllGlobals())
describe('server shared display preference', () => {
 it('reads and writes the server preference and picks up another client change', async () => {
  let server = 500
  const fetch = vi.fn(async (_url: string, options?: RequestInit) => {
   if (options?.method === 'PUT') server = JSON.parse(String(options.body)).limit
   return response(server)
  })
  vi.stubGlobal('fetch', fetch)
  await loadLogDisplayLimit(); expect(logDisplayLimit.value).toBe(500)
  expect(await saveLogDisplayLimit(800)).toBe(true); expect(server).toBe(800)
  server = 200; await loadLogDisplayLimit(); expect(logDisplayLimit.value).toBe(200)
  expect(fetch.mock.calls.every(([url]) => url === '/api/logs/display')).toBe(true)
 })
 it('keeps the confirmed value when persistence fails', async () => {
  vi.stubGlobal('fetch', vi.fn().mockResolvedValueOnce(response(500)).mockRejectedValueOnce(new Error('disk full')))
  await loadLogDisplayLimit()
  expect(await saveLogDisplayLimit(2000)).toBe(false)
  expect(logDisplayLimit.value).toBe(500); expect(logDisplayError.value).toBe('disk full')
 })
 it('ignores stale reads that finish after the latest read', async () => {
  let finish!: (value: Response) => void
  vi.stubGlobal('fetch', vi.fn().mockImplementationOnce(() => new Promise<Response>(resolve => { finish = resolve })).mockResolvedValueOnce(response(800)))
  const old = loadLogDisplayLimit(); await loadLogDisplayLimit(); finish(response(200)); await old
  expect(logDisplayLimit.value).toBe(800)
 })
 it('does not overwrite the server until an initial read succeeds', async () => {
  const fetch = vi.fn().mockRejectedValue(new Error('offline')); vi.stubGlobal('fetch', fetch)
  await expect(loadLogDisplayLimit()).rejects.toThrow('offline')
  expect(await saveLogDisplayLimit(200)).toBe(false)
  expect(fetch).toHaveBeenCalledTimes(1); expect(logDisplayLimit.value).toBe(100)
 })
})
