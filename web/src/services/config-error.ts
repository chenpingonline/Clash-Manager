export interface ConfigDiagnostic {
  reason: string
  params?: Record<string, string>
  suggestion: string
  suggestionParams?: Record<string, string>
  saved: boolean
  raw: string
}

/** Interpret known Core validation failures; retain the original diagnostic verbatim. */
export function describeConfigError(message: unknown): ConfigDiagnostic | null {
  const raw = String(message ?? '')
  if (!/Mihomo 配置校验失败|configuration file [\s\S]*test failed/i.test(raw)) return null
  const messages = [...raw.matchAll(/\blevel=(?:error|fatal)\b[^\r\n]*?\bmsg=("(?:\\.|[^"\\])*"|[^\r\n]*)/g)].map(match => {
    try { return JSON.parse(match[1]!) as string } catch { return match[1]!.trim() }
  })
  const cause = messages[0] || ''
  const saved = /已保存[\s\S]*?(?:应用[\s\S]*?失败|读取当前配置失败)/.test(raw)
  const group = /^proxy group\[\d+\]:\s*(.+):\s*'(.+)' not found$/s.exec(cause)
  if (group) return {
    reason: '代理组「{arg0}」引用的「{arg1}」不存在或已被排除。',
    params: { arg0: group[1]!, arg1: group[2]! },
    suggestion: '恢复「{arg0}」，或在「{arg1}」中移除／替换对它的引用，再应用配置。',
    suggestionParams: { arg0: group[2]!, arg1: group[1]! }, saved, raw,
  }
  const rule = /^rules\[\d+\]\s*\[(.+)\]\s*error:\s*proxy\s*\[(.+)\]\s*not found$/s.exec(cause)
  if (rule) return {
    reason: '规则「{arg0}」引用的策略「{arg1}」不存在或已被排除。',
    params: { arg0: rule[1]!, arg1: rule[2]! },
    suggestion: '恢复「{arg0}」，或将这条规则的策略改为已有节点／代理组，再应用配置。',
    suggestionParams: { arg0: rule[2]! }, saved, raw,
  }
  return {
    reason: cause ? '内核校验错误：{arg0}' : '配置未通过 Mihomo 校验，请展开详情查看原始错误。',
    params: cause ? { arg0: cause } : undefined,
    suggestion: '请检查最近修改的配置；修正后重新应用。', saved, raw,
  }
}
