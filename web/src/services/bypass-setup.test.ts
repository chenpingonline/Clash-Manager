import { afterEach, describe, expect, it, vi } from 'vitest'
import { bypassRequest } from './bypass-setup'
const json = (body: unknown, status = 200) => new Response(JSON.stringify(body), { status, headers: { 'Content-Type': 'application/json' } })
afterEach(() => vi.unstubAllGlobals())
describe('bypass request CSRF handshake', () => {
  it('fetches a fresh nonce for all three actions and preserves the payload', async () => {
    const fetch = vi.fn()
    for (let n = 0; n < 3; n++) fetch.mockResolvedValueOnce(json({ csrfToken: `nonce-${n}` })).mockResolvedValueOnce(json({ ok: true }))
    vi.stubGlobal('fetch', fetch)
    for (const [n, action] of (['preview', 'configure', 'stop'] as const).entries()) {
      const body = { mode: 'tproxy', interface: 'eth0', confirmed: action === 'configure' }
      await expect(bypassRequest(action, body)).resolves.toEqual({ ok: true })
      expect(fetch.mock.calls[n * 2]).toEqual(['/api/network/bypass/csrf', { cache: 'no-store' }])
      const [url, request] = fetch.mock.calls[n * 2 + 1]!
      expect(url).toBe(`/api/network/bypass/${action}`)
      expect(request).toMatchObject({ method: 'POST', headers: { 'Content-Type': 'application/json', 'X-Clash-Bypass-Token': `nonce-${n}` }, body: JSON.stringify(body) })
    }
  })
  it('does not mutate if the nonce cannot be obtained', async () => {
    const fetch = vi.fn().mockResolvedValueOnce(json({}))
    vi.stubGlobal('fetch', fetch)
    await expect(bypassRequest('configure', {})).rejects.toThrow('获取接管请求令牌失败')
    expect(fetch).toHaveBeenCalledTimes(1)
  })
  it('reports rejection without retrying the mutation', async () => {
    const fetch = vi.fn().mockResolvedValueOnce(json({ csrfToken: 'nonce' })).mockResolvedValueOnce(json({ error: '拒绝跨站请求' }, 403))
    vi.stubGlobal('fetch', fetch)
    await expect(bypassRequest('stop', {})).rejects.toThrow('拒绝跨站请求')
    expect(fetch).toHaveBeenCalledTimes(2)
  })
})
it('uses the same CSRF guard for keep preferences and forwards abort signals', async () => {
  const fetch = vi.fn().mockResolvedValueOnce(json({ csrfToken: 'nonce' })).mockResolvedValueOnce(json({ ok: true }))
  vi.stubGlobal('fetch', fetch)
  const controller = new AbortController()
  await bypassRequest('preferences', { keepEnabled: true }, controller.signal)
  expect(fetch.mock.calls[0]![1]).toMatchObject({ cache: 'no-store', signal: controller.signal })
  expect(fetch.mock.calls[1]![0]).toBe('/api/network/bypass/preferences')
  expect(fetch.mock.calls[1]![1]).toMatchObject({ signal: controller.signal, body: '{"keepEnabled":true}', headers: { 'X-Clash-Bypass-Token': 'nonce' } })
})
