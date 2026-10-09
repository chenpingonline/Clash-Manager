<script setup lang="ts">
import { countLabel, t } from '@/services/i18n'

import { computed, onBeforeUnmount, onMounted, ref, shallowRef } from 'vue'
import AsyncState from '@/components/AsyncState.vue'
import BaseModal from '@/components/BaseModal.vue'
import RuleVirtualList from '@/components/RuleVirtualList.vue'
import { api, errorMessage, jsonRequest } from '@/services/api'
import { providerUpdatedText } from '@/services/format'
import { containsRule, normalizeRules } from '@/services/rules'
import type { NormalizedRule } from '@/services/rules'
import { notify } from '@/services/toast'
import type { RuleProvider, RulesResponse } from '@/types/api'

const RULE_MEMORY_TTL = 60_000
let ruleCache: { value: RulesResponse; expiresAt: number } | null = null
let ruleCacheTimer = 0

function rememberRules(value: RulesResponse) {
  ruleCache = { value, expiresAt: Date.now() + RULE_MEMORY_TTL }
  window.clearTimeout(ruleCacheTimer)
  ruleCacheTimer = window.setTimeout(() => { ruleCache = null }, RULE_MEMORY_TTL)
}

function currentRuleCache() {
  if (ruleCache && ruleCache.expiresAt > Date.now()) return ruleCache.value
  ruleCache = null
  return null
}

const rulesLoading = ref(true)
const rulesError = ref('')
const rawRules = shallowRef<RulesResponse['rules']>([])
const ruleScope = ref('')
const query = ref('')
const providerOpen = ref(false)
const providerLoading = ref(false)
const providersLoaded = ref(false)
const providerError = ref('')
const providers = ref<Record<string, RuleProvider>>({})
const updating = ref(new Set<string>())
const toggling = ref(false)
const helpOpen = ref(false)
let timer = 0

const rules = computed(() => normalizeRules(rawRules.value || []))
const filteredRules = computed(() => {
  if (!query.value.trim()) return rules.value
  return rules.value.filter(rule => containsRule(rule, query.value))
})
const providerEntries = computed(() => Object.entries(providers.value).sort(([a], [b]) => a.localeCompare(b)))

async function toggleRule(rule: NormalizedRule) {
  if (toggling.value || !rule.canToggle || rule.index === null) return
  toggling.value = true
  try {
    const value = await api<RulesResponse>('/api/rules/disable', jsonRequest('PATCH', {
      scope: ruleScope.value, index: rule.index, type: rule.type, payload: rule.payload, proxy: rule.proxy, disabled: !rule.disabled,
    }))
    rawRules.value = value.rules || []
    ruleScope.value = value.scope || ''
    rememberRules(value)
    notify(rule.disabled ? `第 ${rule.lineNo} 条规则已启用并保存` : `第 ${rule.lineNo} 条规则已禁用并保存`)
  } catch (cause) {
    notify(errorMessage(cause), true)
    await loadRules(false, true)
  } finally { toggling.value = false }
}

async function loadRules(showLoading = true, refresh = false) {
  if (showLoading) rulesLoading.value = true
  try {
    const value = await api<RulesResponse>(`/api/rules${refresh ? '?refresh=1' : ''}`)
    rawRules.value = value.rules || []
    ruleScope.value = value.scope || ''
    rememberRules(value)
    rulesError.value = ''
  } catch (cause) { rulesError.value = errorMessage(cause) }
  finally { rulesLoading.value = false }
}

async function loadProviders(showLoading = true) {
  if (showLoading) providerLoading.value = true
  try {
    const value = await api<{ providers?: Record<string, RuleProvider> }>('/api/rule-providers')
    providers.value = value.providers || {}
    providersLoaded.value = true
    providerError.value = ''
  } catch (cause) { providerError.value = errorMessage(cause) }
  finally { providerLoading.value = false }
}

function openProviders() {
  providerOpen.value = true
  if (!providersLoaded.value) void loadProviders(true)
}

function scheduleReload() {
  window.clearTimeout(timer)
  timer = window.setTimeout(() => {
    void loadRules(false, true)
    if (providerOpen.value) void loadProviders(false)
  }, 800)
}

async function updateOne(name: string, silent = false) {
  updating.value = new Set(updating.value).add(name)
  try {
    const result = await api<{ method?: string }>(`/api/rule-providers/${encodeURIComponent(name)}/update`, { method: 'PUT' })
    if (!silent) notify(result.method === 'direct-fallback' ? `${name} 更新成功（常规通道失败，已通过直连完成）` : `${name} 更新已触发`)
    if (!silent) scheduleReload()
    return result.method === 'direct-fallback'
  } catch (cause) {
    if (!silent) notify(`${name}: ${errorMessage(cause)}`, true)
    throw cause
  } finally {
    const next = new Set(updating.value)
    next.delete(name)
    updating.value = next
  }
}

async function updateAll() {
  const names = providerEntries.value.map(([name]) => name)
  updating.value = new Set(names)
  try {
    const result = await api<{ success: number; failed: number; fallback: number }>('/api/rule-providers/update-all', jsonRequest('POST', { names }))
    notify(`规则集更新完成：成功 ${result.success}${result.fallback ? `（直连兜底 ${result.fallback}）` : ''}${result.failed ? `，失败 ${result.failed}` : ''}`, result.failed > 0)
    scheduleReload()
  } catch (cause) {
    notify(errorMessage(cause), true)
  } finally {
    updating.value = new Set()
  }
}

defineExpose({ refreshPage: () => loadRules(false, true) })
onMounted(() => {
  const cached = currentRuleCache()
  if (cached) {
    rawRules.value = cached.rules || []
    ruleScope.value = cached.scope || ''
    rulesLoading.value = false
    void loadRules(false, true)
  } else {
    void loadRules(true, true)
  }
})
onBeforeUnmount(() => window.clearTimeout(timer))
</script>

<template>
  <Teleport defer to="#page-title-meta">
    <span class="rule-count">{{ rulesLoading ? t('加载中…') : rulesError ? '—' : countLabel(rules.length, '条') }}</span>
  </Teleport>

  <Teleport defer to="#page-actions">
    <div class="rule-topbar-tools">
      <label class="rule-search">
        <span aria-hidden="true">⌕</span>
        <input v-model="query" :placeholder="t('搜索规则、类型或策略')" :aria-label="t('搜索规则、类型或策略')" autocomplete="off">
        <button v-if="query" type="button" class="search-clear" :aria-label="t('清空搜索')" @click="query = ''">×</button>
      </label>
      <button class="ghost rule-provider-trigger" @click="openProviders">{{ t("规则集") }} <span v-if="providersLoaded">{{ t(providerEntries.length) }}</span></button>
      <button class="ghost" @click="helpOpen = true">{{ t("规则说明") }}</button>
    </div>
  </Teleport>

  <section class="card rules-panel">
    <AsyncState :loading="rulesLoading" :error="t(rulesError)">
      <RuleVirtualList v-if="filteredRules.length" :items="filteredRules" :busy="toggling" :query="query" @toggle="toggleRule" />
      <div v-else class="empty rules-empty">{{ t(query ? `没有匹配“${query}”的规则` : '当前运行配置没有生效规则') }}</div>
    </AsyncState>
  </section>

  <BaseModal :open="helpOpen" :title="t('规则来源与开关')" @close="helpOpen = false">
    <div class="config-help-copy">
      <p>{{ t("这里展示当前内核加载的规则。远程订阅可以包含完整的节点、代理组和分流规则，因此导入后出现大量规则通常来自订阅本身。") }}</p>
      <p>{{ t("软件默认不会添加代理组或分流规则；你设置的订阅增强、全局覆写或脚本也可能修改它们。可对照订阅原始 YAML 中的 proxy-groups、rules 和 rule-providers 查看来源。") }}</p>
      <p>{{ t("开关状态保存在软件中，按规则类型、内容和目标策略匹配；订阅更新或规则顺序变化后仍可恢复，每个订阅独立保存设置，切换订阅时只恢复对应订阅的记录。规则内容变化或相同规则的重复数量变化时跳过恢复。关闭 RULE-SET 会跳过整个引用的规则集；关闭兜底规则可能改变剩余流量的去向。") }}</p>
      <p v-if="rules.length && !rules.some(rule => rule.canToggle)">{{ t("当前内核不支持规则开关，请升级内核。") }}</p>
      <p>{{ t("要修改规则内容或分流策略，请在「订阅配置 → 编辑 → 扩展覆写配置／扩展脚本」中设置。扩展覆写中的 rules 会整体替换原规则列表，脚本可按规则内容过滤。直接编辑当前配置可能被下一次订阅应用覆盖。") }}</p>
    </div>
    <div class="actions"><button class="ghost" @click="helpOpen = false">{{ t("关闭") }}</button></div>
  </BaseModal>

  <BaseModal :open="providerOpen" :title="t(`规则集 · ${providerEntries.length}`)" @close="providerOpen = false">
    <div class="provider-modal-head">
      <p>{{ t("管理当前配置中的 Rule Providers；更新后会同步刷新生效规则。") }}</p>
      <button v-if="providerEntries.length" class="ghost small" :disabled="updating.size > 0" @click="updateAll">{{ t(updating.size ? '更新中…' : '全部更新') }}</button>
    </div>
    <AsyncState :loading="providerLoading" :error="t(providerError)">
      <div v-if="providerEntries.length" class="rule-provider-list">
        <div v-for="[name, provider] in providerEntries" :key="name" class="rule-provider-row">
          <div class="rule-provider-main"><strong :title="name">{{ name }}</strong><span>{{ t(provider.behavior || provider.vehicleType || provider.type || 'Rule Provider') }} · {{ t(Number(provider.ruleCount || 0)) }} {{ t("条") }}</span></div>
          <span class="rule-provider-updated">{{ t(providerUpdatedText(provider.updatedAt)) }}</span>
          <button class="ghost small" :disabled="updating.has(name)" @click="updateOne(name)">{{ t(updating.has(name) ? '处理中…' : '更新') }}</button>
        </div>
      </div>
      <div v-else class="empty compact">{{ t("当前配置没有 Rule Provider") }}</div>
    </AsyncState>
    <div class="actions provider-modal-actions"><button class="ghost" @click="providerOpen = false">{{ t("关闭") }}</button></div>
  </BaseModal>
</template>
