// URI conversion follows Clash Verge Rev's protocol handling. See
// https://github.com/clash-verge-rev/clash-verge-rev/tree/af808a677c7dd952742d217799a63d2b77723953/src/utils/uri-parser
import type { NamedEntry } from './profile-sequences'
function decode64(value: string): string {
  const clean = value.replace(/\s/g, '').replace(/-/g, '+').replace(/_/g, '/')
  return new TextDecoder('utf-8', { fatal: true }).decode(Uint8Array.from(atob(clean.padEnd(Math.ceil(clean.length / 4) * 4, '=')), char => char.charCodeAt(0)))
}
function bool(value: string): boolean { return value === '' || /^(1|true)$/i.test(value) }
function port(value: unknown, fallback = 443): number {
  const number = value === undefined || value === '' ? fallback : Number(value)
  if (!Number.isInteger(number) || number < 1 || number > 65535) throw new Error('端口无效')
  return number
}
function transport(proxy: NamedEntry, params: URLSearchParams) {
  const network = params.get('type') || params.get('network') || 'tcp', host = params.get('host'), path = params.get('path')
  if (!['tcp', 'ws', 'grpc', 'http', 'h2'].includes(network)) throw new Error('传输方式暂不支持，请使用高级 YAML')
  proxy.network = network
  if (network === 'ws') proxy['ws-opts'] = { ...(path ? { path } : {}), ...(host ? { headers: { Host: host } } : {}) }
  if (network === 'grpc') proxy['grpc-opts'] = { 'grpc-service-name': params.get('serviceName') || path || '' }
  if (network === 'h2') proxy['h2-opts'] = { ...(host ? { host } : {}), ...(path ? { path } : {}) }
  if (network === 'http') proxy['http-opts'] = { ...(path ? { path: [path] } : {}), ...(host ? { headers: { Host: [host] } } : {}) }
}
export function parseProxyURI(input: string): NamedEntry {
  input = input.trim().replace(/^([A-Z0-9]+):\/\//i, (_, scheme: string) => scheme.toLowerCase() + '://')
  if (input.startsWith('vmess://')) {
    const value = JSON.parse(decode64(input.slice(8))) as Record<string, unknown>
    if (!value.id || !value.add) throw new Error('VMess 缺少服务器或 UUID')
    const result: NamedEntry = { name: String(value.ps || `VMess ${value.add}`), type: 'vmess', server: String(value.add), port: port(value.port), uuid: String(value.id), alterId: Number(value.aid || 0), cipher: String(value.scy || 'auto'), tls: value.tls === 'tls' }
    const p = new URLSearchParams({ type: String(value.net || 'tcp'), host: String(value.host || ''), path: String(value.path || '') })
    transport(result, p)
    if (value.sni) result.servername = String(value.sni)
    if (value.alpn) result.alpn = String(value.alpn).split(',')
    if (value.fp) result['client-fingerprint'] = value.fp
    return result
  }
  if (input.startsWith('ss://')) {
    let [body, fragment] = input.slice(5).split('#')
    const [main, query] = body!.split('?')
    body = main!.includes('@') ? main! : decode64(main!)
    const at = body.lastIndexOf('@')
    if (at < 0) throw new Error('Shadowsocks 链接缺少认证信息')
    let auth = decodeURIComponent(body.slice(0, at))
    if (!auth.includes(':')) auth = decode64(auth)
    const colon = auth.indexOf(':'); if (colon < 1) throw new Error('Shadowsocks 加密方式无效')
    const address = new URL('http://' + body.slice(at + 1))
    const result: NamedEntry = { type: 'ss', name: fragment ? decodeURIComponent(fragment) : `SS ${address.hostname}`, server: address.hostname.replace(/^\[|\]$/g, ''), port: port(address.port, 8388), cipher: auth.slice(0, colon), password: auth.slice(colon + 1) }
    const plugin = new URLSearchParams(query).get('plugin')
    if (plugin) {
      const [name, ...values] = plugin.split(';'), opts: Record<string, unknown> = {}
      for (const item of values) { const equal = item.indexOf('='); opts[equal < 0 ? item : item.slice(0, equal)] = equal < 0 ? true : item.slice(equal + 1) }
      if (name === 'obfs-local' || name === 'simple-obfs') { result.plugin = 'obfs'; result['plugin-opts'] = { mode: opts.obfs, host: opts['obfs-host'] } }
      else if (name === 'v2ray-plugin') { result.plugin = name; result['plugin-opts'] = { mode: 'websocket', ...opts } }
      else throw new Error('Shadowsocks 插件暂不支持，请使用高级 YAML')
    }
    return result
  }
  const url = new URL(input), scheme = url.protocol.slice(0, -1)
  const aliases: Record<string, string> = { hy2: 'hysteria2', https: 'http', socks: 'socks5' }
  const type = aliases[scheme] || scheme
  if (!['vless', 'trojan', 'anytls', 'hysteria2', 'tuic', 'http', 'socks5'].includes(type)) throw new Error('链接协议暂不支持，请使用高级 YAML')
  const params = url.searchParams
  const result: NamedEntry = { name: decodeURIComponent(url.hash.slice(1)) || `${type} ${url.hostname}`, type, server: url.hostname.replace(/^\[|\]$/g, ''), port: port(url.port, type === 'http' ? scheme === 'https' ? 443 : 80 : type === 'socks5' ? 1080 : 443) }
  const auth = decodeURIComponent(url.username), password = decodeURIComponent(url.password)
  if (!auth && !['http', 'socks5'].includes(type)) throw new Error('链接缺少认证信息')
  if (type === 'vless') { result.uuid = auth; if (params.get('flow')) result.flow = params.get('flow') }
  else if (type === 'tuic') { if (!password) throw new Error('TUIC 缺少密码'); result.uuid = auth; result.password = password }
  else if (type === 'http' || type === 'socks5') { if (auth) result.username = auth; if (password) result.password = password; if (scheme === 'https') result.tls = true }
  else result.password = password ? auth + ':' + password : auth
  if (type === 'anytls') { result.udp = true; result.password = password || auth }
  const security = params.get('security')
  if (type === 'vless') result.tls = security === 'tls' || security === 'reality'
  const sni = params.get('sni') || params.get('peer')
  if (sni) result[type === 'vless' ? 'servername' : 'sni'] = sni
  const insecure = params.get('allowInsecure') ?? params.get('insecure') ?? params.get('skip-cert-verify')
  if (insecure !== null) result['skip-cert-verify'] = bool(insecure)
  if (params.get('alpn')) result.alpn = params.get('alpn')!.split(',')
  if (params.get('fp')) result['client-fingerprint'] = params.get('fp')
  if (security === 'reality') {
    if (!params.get('pbk')) throw new Error('Reality 缺少公钥')
    result['reality-opts'] = { 'public-key': params.get('pbk'), ...(params.get('sid') ? { 'short-id': params.get('sid') } : {}) }
  }
  if (['trojan', 'vless'].includes(type)) transport(result, params)
  if (type === 'hysteria2') {
    for (const [query, key] of [['obfs', 'obfs'], ['obfs-password', 'obfs-password'], ['mport', 'ports']] as const) if (params.get(query)) result[key] = params.get(query)
  }
  if (type === 'tuic') {
    for (const key of ['congestion-controller', 'udp-relay-mode']) if (params.get(key)) result[key] = params.get(key)
  }
  return result
}
export function parseProxyInput(input: string): NamedEntry[] {
  if (!input.trim()) throw new Error('请粘贴节点链接')
  let value = input.trim()
  if (!value.includes('://')) { try { value = decode64(value) } catch { throw new Error('Base64 内容无效') } }
  const lines = value.split(/\r?\n/).map(line => line.trim()).filter(Boolean)
  if (!lines.length) throw new Error('节点列表为空')
  const result: NamedEntry[] = [], names = new Set<string>()
  lines.forEach((line, index) => {
    let proxy: NamedEntry
    try { proxy = parseProxyURI(line) } catch { throw new Error(`第 ${index + 1} 行节点链接无效或协议暂不支持，请检查链接或使用高级 YAML`) }
    if (names.has(proxy.name)) throw new Error(`第 ${index + 1} 行节点名称重复，请修改名称后重试`)
    names.add(proxy.name); result.push(proxy)
  })
  return result
}
