import { t, getLocale } from './i18n'
import { parseDocument, stringify } from 'yaml'

export type SequenceKind = 'rules' | 'proxies' | 'groups'
export type NamedEntry = Record<string, unknown> & { name: string; type: string }
export type SequenceEntry = string | NamedEntry
export interface SequenceExtension { prepend: SequenceEntry[]; append: SequenceEntry[]; delete: string[]; [key: string]: unknown }
export interface SequenceEditorData { content: string; customized: boolean; base: Record<string, unknown>; sequences: Record<SequenceKind, string>; warning?: string }
export const emptySequence = (): SequenceExtension => ({ prepend: [], append: [], delete: [] })
export function isNamed(value: unknown): value is NamedEntry {
  return Boolean(value && typeof value === 'object' && !Array.isArray(value) && typeof (value as NamedEntry).name === 'string' && typeof (value as NamedEntry).type === 'string')
}
export function parseSequence(content: string, kind: SequenceKind): SequenceExtension {
  const doc = parseDocument(content, { uniqueKeys: true })
  if (doc.errors.length) throw new Error('YAML 格式错误：' + doc.errors[0]!.message)
  const value: unknown = doc.toJS({ maxAliasCount: 50 })
  if (value === null || value === undefined) return emptySequence()
  if (typeof value !== 'object' || Array.isArray(value)) throw new Error('增强配置必须包含 prepend、append、delete 列表')
  const record = value as Record<string, unknown>
  for (const key of ['prepend', 'append', 'delete']) {
    if (record[key] === undefined || record[key] === null) record[key] = []
    if (!Array.isArray(record[key])) throw new Error(`${key} 必须是列表`)
  }
  for (const key of ['prepend', 'append'] as const) {
    if ((record[key] as unknown[]).some(item => kind === 'rules' ? typeof item !== 'string' : !isNamed(item) || !item.name.trim() || !item.type.trim())) {
      throw new Error(kind === 'rules' ? `${key} 中的规则必须是字符串` : `${key} 中的条目必须包含 name 和 type；请在高级模式修正`)
    }
  }
  if ((record.delete as unknown[]).some(item => typeof item !== 'string')) throw new Error('delete 中的条目必须是规则文本或名称')
  return record as SequenceExtension
}
export const serializeSequence = (value: SequenceExtension): string => stringify(value, { lineWidth: 0 })
export const entryIdentity = (item: SequenceEntry): string => typeof item === 'string' ? item : item.name
export const baseEntries = (base: Record<string, unknown>, kind: SequenceKind): SequenceEntry[] => {
  const list = base[kind === 'groups' ? 'proxy-groups' : kind]
  if (!Array.isArray(list)) return []
  return list.filter((value): value is SequenceEntry => kind === 'rules' ? typeof value === 'string' : isNamed(value))
}
export function effectiveEntries(base: SequenceEntry[], model: SequenceExtension): SequenceEntry[] {
  return [...model.prepend, ...base.filter(item => !model.delete.includes(entryIdentity(item))), ...model.append]
}
// Logical rule operands contain commas inside parentheses.
export function ruleParts(rule: string): string[] {
  const result: string[] = []; let depth = 0, start = 0
  for (let index = 0; index < rule.length; index++) {
    const char = rule[index]
    if (char === '(') depth++
    if (char === ')') depth--
    if (char === ',' && depth === 0) { result.push(rule.slice(start, index)); start = index + 1 }
  }
  result.push(rule.slice(start)); return result
}
export const ruleTypes = ['DOMAIN', 'DOMAIN-SUFFIX', 'DOMAIN-KEYWORD', 'DOMAIN-REGEX', 'GEOSITE', 'GEOIP', 'SRC-GEOIP', 'IP-ASN', 'SRC-IP-ASN', 'IP-CIDR', 'IP-CIDR6', 'SRC-IP-CIDR', 'IP-SUFFIX', 'SRC-IP-SUFFIX', 'SRC-PORT', 'DST-PORT', 'IN-PORT', 'DSCP', 'PROCESS-NAME', 'PROCESS-PATH', 'PROCESS-NAME-REGEX', 'PROCESS-PATH-REGEX', 'NETWORK', 'UID', 'IN-TYPE', 'IN-USER', 'IN-NAME', 'RULE-SET', 'SUB-RULE', 'AND', 'OR', 'NOT', 'MATCH']
export function makeRule(type: string, payload: string, policy: string, noResolve = false): string {
  payload = payload.trim(); policy = policy.trim()
  if (!ruleTypes.includes(type) || !policy || policy.includes(',')) throw new Error('请选择规则类型和有效的代理策略')
  if (type !== 'MATCH' && !payload) throw new Error('请填写规则内容')
  if (!['AND', 'OR', 'NOT'].includes(type) && payload.includes(',')) throw new Error('规则内容不能包含逗号，复杂规则请使用高级模式')
  if (['AND', 'OR', 'NOT'].includes(type) && (!payload.startsWith('(') || ruleParts(payload).length !== 1 || [...payload].reduce((depth, char) => depth + (char === '(' ? 1 : char === ')' ? -1 : 0), 0) !== 0)) throw new Error('逻辑规则的括号不完整')
  if (type === 'NETWORK' && !['tcp', 'udp'].includes(payload.toLowerCase())) throw new Error('网络类型只能是 tcp 或 udp')
  if (['UID', 'IP-ASN', 'SRC-IP-ASN', 'DSCP'].includes(type) && !/^\d+$/.test(payload)) throw new Error('规则内容必须是非负整数')
  if (['SRC-PORT', 'DST-PORT', 'IN-PORT'].includes(type) && !payload.split('/').every(part => { const ends = part.split('-').map(Number); return ends.length <= 2 && ends.every(port => Number.isInteger(port) && port >= 1 && port <= 65535) && (ends.length === 1 || ends[0]! <= ends[1]!) })) throw new Error('端口需为 1–65535，可使用范围或 / 分隔')
  return [type, ...(type === 'MATCH' ? [] : [payload]), policy, ...(noResolve ? ['no-resolve'] : [])].join(',')
}
export function moveEntry(model: SequenceExtension, side: 'prepend' | 'append', index: number, delta: number): SequenceExtension {
  const list = [...model[side]], target = index + delta
  if (target < 0 || target >= list.length) return model
  ;[list[index], list[target]] = [list[target]!, list[index]!]
  return { ...model, [side]: list }
}

export type SequenceSource = 'prepend' | 'base' | 'append'
export type SequenceSourceFilter = 'all' | SequenceSource | 'deleted'
export interface SequenceRow { item: SequenceEntry; side: SequenceSource; index: number; order: number; key: string; deleted: boolean }
export function sequenceRows(base: SequenceEntry[], model: SequenceExtension): SequenceRow[] {
  const deleted = new Set(model.delete), rows: SequenceRow[] = []
  for (const side of ['prepend', 'base', 'append'] as const) {
    const entries = side === 'base' ? base : model[side]
    entries.forEach((item, index) => rows.push({ item, side, index, order: rows.length + 1, key: `${side}:${index}`, deleted: side === 'base' && deleted.has(entryIdentity(item)) }))
  }
  return rows
}
export function filterSequenceRows(rows: SequenceRow[], search: string, source: SequenceSourceFilter = 'all'): SequenceRow[] {
  const term = search.trim().toLocaleLowerCase()
  return rows.filter(row => (source === 'all' || (source === 'deleted' ? row.deleted : row.side === source)) && (!term || (typeof row.item === 'string' ? row.item : `${row.item.name} ${row.item.type}`).toLocaleLowerCase().includes(term)))
}
export const ruleTypeLabels: Record<string, string> = {
  DOMAIN: '完整域名', 'DOMAIN-SUFFIX': '域名后缀', 'DOMAIN-KEYWORD': '域名关键词', 'DOMAIN-REGEX': '域名正则', GEOSITE: '域名分类', GEOIP: '目标 IP 国家', 'SRC-GEOIP': '来源 IP 国家', 'IP-ASN': '目标 ASN', 'SRC-IP-ASN': '来源 ASN', 'IP-CIDR': '目标 IP 网段', 'IP-CIDR6': '目标 IPv6 网段', 'SRC-IP-CIDR': '来源 IP 网段', 'IP-SUFFIX': '目标 IP 后缀', 'SRC-IP-SUFFIX': '来源 IP 后缀', 'SRC-PORT': '来源端口', 'DST-PORT': '目标端口', 'IN-PORT': '入站端口', DSCP: 'DSCP 标记', 'PROCESS-NAME': '进程名称', 'PROCESS-PATH': '进程路径', 'PROCESS-NAME-REGEX': '进程名称正则', 'PROCESS-PATH-REGEX': '进程路径正则', NETWORK: '网络类型', UID: '用户 ID', 'IN-TYPE': '入站类型', 'IN-USER': '入站用户', 'IN-NAME': '入站名称', 'RULE-SET': '规则集', 'SUB-RULE': '子规则', AND: '同时满足', OR: '任一满足', NOT: '不满足', MATCH: '所有其他流量',
}

// Table summaries expose routing metadata, never node credentials or full URIs.
export function sequenceEntrySummary(item: SequenceEntry, kind: SequenceKind): string {
  if (typeof item === 'string') { const parts = ruleParts(item); return parts[parts[0] === 'MATCH' ? 1 : 2] || '' }
  if (kind === 'proxies') {
    const server = typeof item.server === 'string' ? item.server : ''
    const port = typeof item.port === 'number' || typeof item.port === 'string' ? String(item.port) : ''
    return server ? `${server.includes(':') && !server.startsWith('[') ? `[${server}]` : server}${port ? `:${port}` : ''}` : '—'
  }
  const entries = [
    ...(Array.isArray(item.proxies) ? item.proxies.filter((entry): entry is string => typeof entry === 'string') : []),
    ...(Array.isArray(item.use) ? item.use.filter((entry): entry is string => typeof entry === 'string').map(entry => t('集合：{arg0}', { arg0: entry })) : []),
  ]
  if (item['include-all'] === true) entries.unshift(t('全部节点与集合'))
  return entries.length ? entries.slice(0, 3).join(getLocale() === 'en-US' ? ', ' : '、') + (entries.length > 3 ? ` ${t('等 {arg0} 项', { arg0: entries.length })}` : '') : t('未配置成员')
}
