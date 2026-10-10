import { describe, expect, it } from 'vitest'
import { describeConfigError } from './config-error'

function failure(cause: string) {
  return `编辑代理组 · pe已保存，但应用当前配置失败：备份或校验启动配置失败: Mihomo 配置校验失败: time="2026-10-11T00:21:04+08:00" level=info msg="Start initial configuration in progress"\ntime="2026-10-11T00:21:04+08:00" level=error msg=${JSON.stringify(cause)}\nconfiguration file /tmp/.config.yaml.new test failed`
}

describe('configuration failure explanations', () => {
  it('identifies the group and missing member in the screenshot without exposing info logs in the summary', () => {
    const raw = failure("proxy group[1]: 🤖AI网站: '🚀节点选择' not found")
    const result = describeConfigError(raw)!
    expect(result.params).toEqual({ arg0: '🤖AI网站', arg1: '🚀节点选择' })
    expect(result.suggestionParams).toEqual({ arg0: '🚀节点选择', arg1: '🤖AI网站' })
    expect(result.saved).toBe(true)
    expect(result.raw).toBe(raw)
    expect(result.reason).not.toContain('time=')
  })
  it('preserves names containing colons and escaped quotes', () => {
    const result = describeConfigError(failure(`proxy group[1]: AI: "测试": 'Node: 特殊' not found`))!
    expect(result.params).toEqual({ arg0: 'AI: "测试"', arg1: 'Node: 特殊' })
  })
  it('explains a missing rule policy without guessing the rule numbering', () => {
    const result = describeConfigError(failure('rules[37] [DOMAIN-KEYWORD,admarvel,🛑广告拦截] error: proxy [🛑广告拦截] not found'))!
    expect(result.params).toEqual({ arg0: 'DOMAIN-KEYWORD,admarvel,🛑广告拦截', arg1: '🛑广告拦截' })
    expect(result.suggestionParams).toEqual({ arg0: '🛑广告拦截' })
  })
  it('retains an unrecognized Core error as the reason and all later errors in details', () => {
    const raw = failure('unsupported proxy type: example') + '\nlevel=error msg="another error"'
    const result = describeConfigError(raw)!
    expect(result.params).toEqual({ arg0: 'unsupported proxy type: example' })
    expect(result.raw).toContain('another error')
  })
  it('does not infer that a profile was saved when applying it fails', () => {
    const result = describeConfigError('应用失败: Mihomo 配置校验失败: unknown output')!
    expect(result.saved).toBe(false)
    expect(result.params).toBeUndefined()
    expect(result.raw).toContain('unknown output')
  })
  it('leaves network, download and ordinary form errors unchanged', () => {
    for (const raw of ['HTTP 503', '订阅下载失败', '代理组名称不能为空', null]) expect(describeConfigError(raw)).toBeNull()
  })
})
