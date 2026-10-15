<script setup lang="ts">
import { Link2, Search } from 'lucide-vue-next'
import { countLabel, t } from '@/services/i18n'

import { computed, nextTick, onMounted, reactive, ref } from 'vue'
import AsyncState from '@/components/AsyncState.vue'
import CollapseTransition from '@/components/CollapseTransition.vue'
import BaseModal from '@/components/BaseModal.vue'
import { useDelayTests } from '@/composables/useDelayTests'
import { useGroupDelaySettings } from '@/composables/useGroupDelaySettings'
import { api, errorMessage, jsonRequest } from '@/services/api'
import { providerUpdatedText } from '@/services/format'
import { parseProxyGroupSortPreferences, shouldShowProxyNode, sortProxyNodeNames, type ProxyNodeSort } from '@/services/proxy-view'
import { latencyClass } from '@/services/latency'
import { delayTestGroupBatchRunning, delayTestGroupRunning } from '@/services/delay-tests'
import { notify } from '@/services/toast'
import type { ProxiesResponse, ProxyNode, ProxyProvider, ProxyProvidersResponse } from '@/types/api'

type ProxyGroup = { name: string; proxy: ProxyNode }
const GROUP_SORT_STORAGE_KEY = 'clash-for-fnos.proxy-group-sorts.v1'

function storedGroupSorts() {
  try { return parseProxyGroupSortPreferences(window.localStorage.getItem(GROUP_SORT_STORAGE_KEY)) }
  catch { return {} }
}

const loading = ref(true), error = ref(''), query = ref('')
const groups = ref<ProxyGroup[]>([])
const expanded = reactive(new Set<string>())
const delayTests = useDelayTests()
const delaySettings = useGroupDelaySettings(delayTests.context, delayTests.setContext)
async function toggleCustomDelay(group: string) {
  await delaySettings.toggle(group)
  if (!delaySettings.isCustom(group)) return
  await nextTick()
  const card = Array.from(document.querySelectorAll<HTMLElement>('.proxy-card')).find(item => item.dataset.groupName === group)
  card?.querySelector<HTMLInputElement>('.proxy-delay-url')?.focus({ preventScroll: true })
}
function delayFor(name: string, group: string) { return delayTests.get(name, group) }
const groupFilters = reactive(new Map<string, string>())
const groupSorts = reactive(new Map<string, ProxyNodeSort>(Object.entries(storedGroupSorts())))
const showTimeoutNodes = reactive(new Map<string, boolean>())
const providerOpen = ref(false), providerLoading = ref(false), providersLoaded = ref(false), providerError = ref('')
const providers = ref<Record<string, ProxyProvider>>({}), updatingProviders = ref(new Set<string>()), checkingProviders = ref(new Set<string>())

const filtered = computed(() => {
  const needle = query.value.trim().toLowerCase()
  return groups.value.map(group => {
    const groupMatches = group.name.toLowerCase().includes(needle)
    const localNeedle = groupFilter(group.name).trim().toLowerCase()
    const nodes = (group.proxy.all || []).filter(node =>
      (!needle || groupMatches || node.toLowerCase().includes(needle)) &&
      (!localNeedle || node.toLowerCase().includes(localNeedle)) &&
      shouldShowProxyNode(delayFor(node, group.name)?.state, groupShowsTimeoutNodes(group.name)),
    )
    return {
      ...group,
      nodes: sortProxyNodeNames(nodes, groupSort(group.name), name => delayFor(name, group.name)?.value || 0),
    }
  }).filter(group => !needle || group.name.toLowerCase().includes(needle) || group.nodes.length)
})
const visibleNodeCount = computed(() => filtered.value.reduce((sum, group) => sum + group.nodes.length, 0))
const allExpanded = computed(() => groups.value.length > 0 && groups.value.every(group => expanded.has(group.name)))
const providerEntries = computed(() => Object.entries(providers.value).sort(([a], [b]) => a.localeCompare(b)))
const delayTestBusy = delayTests.testing
const allGroupsTesting = computed(() => delayTestGroupBatchRunning(delayTests.status.value, delayTests.context.value?.scope))

function groupFilter(name: string) { return groupFilters.get(name) || '' }
function groupSort(name: string): ProxyNodeSort { return groupSorts.get(name) || 'default' }
function groupShowsTimeoutNodes(name: string) { return showTimeoutNodes.get(name) !== false }
function updateGroupFilter(name: string, event: Event) {
  const value = (event.target as HTMLInputElement).value
  if (value) groupFilters.set(name, value)
  else groupFilters.delete(name)
}
function updateGroupSort(name: string, event: Event) {
  const sort = (event.target as HTMLSelectElement).value as ProxyNodeSort
  if (sort === 'default') groupSorts.delete(name)
  else groupSorts.set(name, sort)
  try { window.localStorage.setItem(GROUP_SORT_STORAGE_KEY, JSON.stringify(Object.fromEntries(groupSorts))) }
  catch { /* The current selection still works when storage is unavailable. */ }
  expanded.add(name)
}
function updateTimeoutVisibility(name: string, event: Event) {
  showTimeoutNodes.set(name, (event.target as HTMLInputElement).checked)
}
function snapshot(name: string, group: string) {
  const item = delayFor(name, group)
  if (item?.state === 'testing') return { text: '测试中…', className: 'testing' }
  if (item?.state === 'timeout') return { text: '超时', className: 'bad', error: item.error }
  if (item?.state === 'error') return { text: '失败', className: 'bad', error: item.error }
  return item?.value ? { text: `${item.value} ms`, className: latencyClass(item.value) } : { text: '--', className: '' }
}
function testable(name: string) {
  const node = groups.value.flatMap(group => group.proxy.all || []).includes(name)
  const proxy = rawProxies.value[name] || {}
  return node && !['direct', 'reject', 'pass'].includes(String(proxy.type || name).toLowerCase())
}
const rawProxies = ref<Record<string, ProxyNode>>({})

async function load() {
  loading.value = true
  try {
    const data = await api<ProxiesResponse>('/api/proxies')
    rawProxies.value = data.proxies || {}
    if (data.delayContext) delayTests.setContext(data.delayContext)
    const entries = Object.entries(rawProxies.value).filter(([, proxy]) => proxy.all?.length)
    const map = new Map(entries), seen = new Set<string>(), ordered: ProxyGroup[] = []
    for (const name of data.groupOrder || []) if (map.has(name) && !seen.has(name)) { ordered.push({ name, proxy: map.get(name)! }); seen.add(name) }
    for (const [name, proxy] of entries) if (!seen.has(name)) ordered.push({ name, proxy })
    groups.value = ordered
    error.value = ''
  } catch (cause) { error.value = errorMessage(cause) }
  finally { loading.value = false }
}
async function select(group: ProxyGroup, name: string) {
  try { await api(`/api/proxies/${encodeURIComponent(group.name)}`, jsonRequest('PUT', { name })); group.proxy.now = name; notify(`已切换到 ${name}`) }
  catch (cause) { notify(errorMessage(cause), true) }
}
async function testOne(name: string, group: string) {
  if (delayTestBusy.value || !testable(name)) return
  try {
    if (!await delayTests.start([name], group, () => delaySettings.save(group))) return
    const failure = delayFor(name, group)
    if (failure?.error) notify(failure.error, true)
  } catch (cause) {
    notify(errorMessage(cause), true)
  }
}
async function testMany(names: string[], label: string, group: string) {
  const queue = [...new Set(names)].filter(testable)
  if (!queue.length) return notify('没有可测速节点')
  try {
    if (!await delayTests.start(queue, group, () => delaySettings.save(group))) return
    notify(label)
  } catch (cause) {
    notify(errorMessage(cause), true)
  }
}
async function testAllGroups() {
 if (delayTestBusy.value) return
 const requests = groups.value.map(group => ({ group: group.name, names: [...new Set(group.proxy.all || [])].filter(testable) })).filter(group => group.names.length)
 if (!requests.length) return notify('没有可测速节点')
 try {
   const result = await delayTests.startGroups(requests, async () => {
     for (const group of groups.value) if (!await delaySettings.save(group.name)) return false
     return true
   })
   if (result) notify('全部节点测速完成')
 }
 catch (cause) { notify(errorMessage(cause), true) }
}
function groupTesting(name: string) {
  return delayTestGroupRunning(delayTests.status.value, delayTests.context.value?.scope, name)
}
async function testGroup(group: ProxyGroup) {
  if (delayTestBusy.value) return
  await testMany(group.proxy.all || [], `${group.name} 测速完成`, group.name)
}
async function locateCurrent(group: ProxyGroup) {
  const current = group.proxy.now?.trim()
  if (!current) return notify(`${group.name} 暂无当前节点`)
  expanded.add(group.name)
  if (groupFilter(group.name) && !current.toLowerCase().includes(groupFilter(group.name).trim().toLowerCase())) groupFilters.delete(group.name)
  if (!groupShowsTimeoutNodes(group.name) && delayFor(current, group.name)?.state === 'timeout') showTimeoutNodes.set(group.name, true)
  const needle = query.value.trim().toLowerCase()
  if (needle && !group.name.toLowerCase().includes(needle) && !current.toLowerCase().includes(needle)) query.value = ''
  await nextTick()
  const card = Array.from(document.querySelectorAll<HTMLElement>('.proxy-card')).find(item => item.dataset.groupName === group.name)
  const row = Array.from(card?.querySelectorAll<HTMLElement>('.node-row') || []).find(item => item.dataset.nodeName === current)
  if (!row) return notify(`未找到当前节点：${current}`, true)
  // Wait for the expanded height before computing the scroll destination.
  await Promise.allSettled(Array.from(card?.querySelectorAll<HTMLElement>('.expand-panel') || [])
    .flatMap(panel => panel.getAnimations().map(animation => animation.finished)))
  if (!row.isConnected || !expanded.has(group.name)) return
  row.scrollIntoView({ block: 'center', behavior: 'smooth' })
  row.querySelector<HTMLElement>('.node-select')?.focus({ preventScroll: true })
}
function toggleAll() {
  if (allExpanded.value) expanded.clear()
  else groups.value.forEach(group => expanded.add(group.name))
}
async function loadProviders(showLoading = true) {
  if (showLoading) providerLoading.value = true
  try { const result = await api<ProxyProvidersResponse>('/api/providers'); providers.value = result.providers || {}; providersLoaded.value = true; providerError.value = '' }
  catch (cause) { providerError.value = errorMessage(cause) }
  finally { providerLoading.value = false }
}
function openProviderManager() { providerOpen.value = true; if (!providersLoaded.value) void loadProviders(true) }
async function updateProvider(name: string, silent = false) {
  updatingProviders.value = new Set(updatingProviders.value).add(name)
  try { await api(`/api/providers/${encodeURIComponent(name)}/update`, { method: 'PUT' }); if (!silent) { notify(`${name} 更新成功`); await loadProviders(false) }; return true }
  catch (cause) { if (!silent) notify(`${name}: ${errorMessage(cause)}`, true); return false }
  finally { const next = new Set(updatingProviders.value); next.delete(name); updatingProviders.value = next }
}
async function updateAllProviders() {
  const names = providerEntries.value.map(([name]) => name)
  updatingProviders.value = new Set(names)
  try {
    const result = await api<{ success: number; failed: number }>('/api/providers/update-all', jsonRequest('POST', { names }))
    await loadProviders(false)
    notify(`代理组更新完成：成功 ${result.success}${result.failed ? `，失败 ${result.failed}` : ''}`, result.failed > 0)
  } catch (cause) {
    notify(errorMessage(cause), true)
  } finally {
    updatingProviders.value = new Set()
  }
}
async function healthcheckProvider(name: string) {
  checkingProviders.value = new Set(checkingProviders.value).add(name)
  try { await api(`/api/providers/${encodeURIComponent(name)}/healthcheck`); notify(`${name} 健康检查完成`) }
  catch (cause) { notify(`${name}: ${errorMessage(cause)}`, true) }
  finally { const next = new Set(checkingProviders.value); next.delete(name); checkingProviders.value = next }
}
onMounted(() => { void delayTests.restore(); void load() })
</script>

<template>
  <Teleport defer to="#page-actions">
    <div class="proxy-topbar-tools">
      <div class="proxy-search">
        <Search :size="14" :stroke-width="1.8" aria-hidden="true" />
        <input v-model="query" :placeholder="t('搜索代理组或节点')" :aria-label="t('搜索代理组或节点')" autocomplete="off">
        <button v-if="query" type="button" class="search-clear" :aria-label="t('清空搜索')" @click="query = ''">×</button>
      </div>
      <span class="search-result">{{ t(query ? `${filtered.length} 组 · ${visibleNodeCount} 个节点` : '') }}</span>
      <button class="ghost rule-provider-trigger" @click="openProviderManager">{{ t("策略组") }} <span v-if="providersLoaded">{{ t(providerEntries.length) }}</span></button>
      <button class="ghost" :disabled="loading || !groups.length" @click="toggleAll">{{ t(allExpanded ? '全部收起' : '全部展开') }}</button>
      <button class="ghost primary" :disabled="loading || !groups.length || delayTestBusy" :aria-busy="allGroupsTesting" :title="delayTestBusy && !allGroupsTesting ? t('已有测速任务正在进行') : undefined" @click="testAllGroups">{{ t(allGroupsTesting ? '测试中…' : '延迟测试') }}</button>
    </div>
  </Teleport>

  <AsyncState :loading="loading" :error="t(error)">
    <div class="proxy-groups">
      <section v-for="group in filtered" :key="group.name" class="card proxy-card" :class="{ expanded: expanded.has(group.name), 'search-expanded': query }" :data-group-name="group.name">
        <div class="proxy-card-head" @click="expanded.has(group.name) ? expanded.delete(group.name) : expanded.add(group.name)">
          <div class="proxy-summary"><div class="proxy-name-row"><h3>{{ group.name }}</h3><span class="tag">{{ t(group.proxy.type || 'Selector') }}</span><span class="node-count" :title="countLabel(group.proxy.all?.length || 0, '个节点')">{{ t(group.proxy.all?.length || 0) }}</span></div><div class="proxy-current"><span>{{ t("当前") }}</span><strong>{{ group.proxy.now || '-' }}</strong><span v-if="snapshot(group.proxy.now || '', group.name).text !== '--'" class="current-delay" :class="snapshot(group.proxy.now || '', group.name).className">{{ t(snapshot(group.proxy.now || '', group.name).text) }}</span></div></div>
          <div class="proxy-head-actions">
            <input v-if="delaySettings.isCustom(group.name)" class="proxy-delay-url" :value="delaySettings.url(group.name)" type="url" maxlength="4096" autocomplete="off" spellcheck="false" :aria-label="t('自定义测试地址') + ' · ' + group.name" :aria-invalid="Boolean(delaySettings.error(group.name))" :placeholder="delaySettings.defaultUrl(group.name) || t('输入测速网址')" :title="t('输入网址后按回车或移开焦点保存')" :disabled="delayTestBusy || delaySettings.isSaving(group.name)" @click.stop @input="delaySettings.edit(group.name, ($event.target as HTMLInputElement).value)" @blur="delaySettings.save(group.name)" @keydown.enter.stop.prevent="delaySettings.save(group.name)">
            <button class="proxy-group-tool proxy-locate ghost small" :disabled="!group.proxy.now" :aria-label="t('定位当前节点')" :title="t('定位当前节点')" @click.stop="locateCurrent(group)"><svg viewBox="0 0 24 24" aria-hidden="true"><circle cx="12" cy="12" r="7" /><circle class="proxy-locate-dot" cx="12" cy="12" r="3" /><path d="M12 2v3M12 19v3M2 12h3M19 12h3" /></svg></button>
            <button class="proxy-group-tool proxy-delay-custom ghost small" :class="{ active: delaySettings.isCustom(group.name) }" :disabled="!delayTests.context.value || delayTestBusy || delaySettings.isSaving(group.name)" :aria-pressed="delaySettings.isCustom(group.name)" :aria-label="t('自定义测速网址') + ' · ' + group.name" :title="t('自定义测速网址')" @mousedown.prevent @click.stop="toggleCustomDelay(group.name)"><Link2 :size="16" :stroke-width="1.8" aria-hidden="true" /></button>
            <button class="proxy-group-tool proxy-latency-test ghost small" :disabled="delayTestBusy" :aria-busy="groupTesting(group.name)" :title="t(delayTestBusy && !groupTesting(group.name) ? '已有测速任务正在进行' : '测试本组全部可测速节点')" @click.stop="testGroup(group)">{{ t(groupTesting(group.name) ? '测试中…' : '延迟测试') }}</button>
            <label class="proxy-sort-control" :title="t('设置本组节点排序方式')" @click.stop>
              <select :value="groupSort(group.name)" :aria-label="t(`${group.name} 排序方式`)" @click.stop @change.stop="updateGroupSort(group.name, $event)">
                <option value="default">{{ t("默认排序") }}</option>
                <option value="delay">{{ t("按延迟") }}</option>
                <option value="name">{{ t("按名称") }}</option>
              </select>
            </label>
            <button type="button" class="proxy-chevron ghost" :aria-expanded="expanded.has(group.name) || Boolean(query)" :aria-label="t(expanded.has(group.name) || query ? '收起分组 {arg0}' : '展开分组 {arg0}', { arg0: group.name })" :disabled="Boolean(query)" :title="query ? t('搜索时分组自动展开') : undefined" @click.stop="expanded.has(group.name) ? expanded.delete(group.name) : expanded.add(group.name)"><svg viewBox="0 0 20 20" aria-hidden="true"><path d="m5.5 7.5 4.5 4.5 4.5-4.5" /></svg></button>
          </div>
        </div>
        <p v-if="delaySettings.error(group.name)" class="proxy-delay-error" role="alert">{{ t(delaySettings.error(group.name)) }}</p>
        <CollapseTransition><div v-show="expanded.has(group.name) || Boolean(query)" class="expand-panel"><div class="proxy-body">
          <div class="proxy-group-filter">
            <label class="proxy-filter-search"><span>{{ t("筛选") }}</span><input :value="groupFilter(group.name)" :aria-label="t(`${group.name} 筛选节点`)" :placeholder="t('输入节点名称')" @input="updateGroupFilter(group.name, $event)"></label>
            <button v-if="groupFilter(group.name)" class="ghost small" @click="groupFilters.delete(group.name)">{{ t("清除") }}</button>
            <label class="proxy-timeout-toggle" :title="t('关闭后隐藏测速结果为超时的节点')"><input type="checkbox" :checked="groupShowsTimeoutNodes(group.name)" @change="updateTimeoutVisibility(group.name, $event)"><span>{{ t("显示超时节点") }}</span></label>
            <span>{{ t(group.nodes.length) }} / {{ t(group.proxy.all?.length || 0) }} {{ t("个节点") }}</span>
          </div>
          <div v-if="group.nodes.length" class="node-list"><div v-for="name in group.nodes" :key="name" class="node-row" :class="{ active: group.proxy.now === name }" :data-node-name="name"><button class="node-select" @click="select(group, name)"><span class="node-copy"><span class="node-name" :title="name">{{ name }}</span><span v-if="rawProxies[name]?.type" class="node-type">{{ rawProxies[name]?.type }}</span></span></button><button class="node-delay" :class="snapshot(name, group.name).className" :disabled="delayTestBusy" :title="snapshot(name, group.name).error || t(snapshot(name, group.name).text === '--' ? '尚未测速，点击测试此节点' : '单独测试该节点延迟')" @click="testOne(name, group.name)">{{ t(snapshot(name, group.name).text === '--' ? '未测速' : snapshot(name, group.name).text) }}</button></div></div>
          <div v-else class="proxy-group-empty">{{ t("没有匹配的节点") }}</div>
        </div></div></CollapseTransition>
      </section>
    </div>
  </AsyncState>

  <BaseModal :open="providerOpen" :title="t(`代理组 · ${providerEntries.length}`)" @close="providerOpen = false">
    <div class="provider-modal-head"><p>{{ t("管理当前配置中的 Proxy Providers，可单独更新或执行健康检查。") }}</p><button v-if="providerEntries.length" class="ghost small" :disabled="updatingProviders.size > 0" @click="updateAllProviders">{{ t(updatingProviders.size ? '更新中…' : '全部更新') }}</button></div>
    <AsyncState :loading="providerLoading" :error="t(providerError)">
      <div v-if="providerEntries.length" class="rule-provider-list">
        <div v-for="[name, provider] in providerEntries" :key="name" class="rule-provider-row proxy-provider-row">
          <div class="rule-provider-main"><strong :title="name">{{ name }}</strong><span>{{ t(provider.vehicleType || provider.type || 'Proxy Provider') }} · {{ countLabel(provider.proxies?.length || 0, "个节点") }}</span></div>
          <span class="rule-provider-updated">{{ t(providerUpdatedText(provider.updatedAt)) }}</span>
          <div class="proxy-provider-actions"><button class="ghost small" :disabled="checkingProviders.has(name) || updatingProviders.has(name)" @click="healthcheckProvider(name)">{{ t(checkingProviders.has(name) ? '检测中…' : '健康检查') }}</button><button class="ghost small" :disabled="updatingProviders.has(name) || checkingProviders.has(name)" @click="updateProvider(name)">{{ t(updatingProviders.has(name) ? '更新中…' : '更新') }}</button></div>
        </div>
      </div>
      <div v-else class="empty compact">{{ t("当前配置没有 Proxy Provider") }}</div>
    </AsyncState>
    <div class="actions provider-modal-actions"><button class="ghost" @click="providerOpen = false">{{ t("关闭") }}</button></div>
  </BaseModal>
</template>

<style scoped>
.proxy-summary { flex: 1; }
.proxy-name-row .tag, .proxy-name-row .node-count, .proxy-current > span { flex: none; white-space: nowrap; }
.proxy-current strong { min-width: 0; }
.proxy-sort-control { padding-left: 8px; }
.proxy-sort-control select { width: 68px; padding-right: 4px; }
.proxy-sort-control select:lang(en) { width: 88px; }
.proxy-head-actions .proxy-locate { display: inline-flex; align-items: center; justify-content: center; width: 30px; height: 30px; padding: 0; border-color: var(--line); color: var(--text); }
.proxy-head-actions .proxy-locate:hover:not(:disabled) { border-color: var(--accent-line, var(--accent)); background: var(--accent-soft, rgba(124,156,255,.08)); color: var(--accent-ink, var(--accent)); }
.proxy-locate svg { width: 18px; height: 18px; stroke-width: 2; }
.proxy-locate-dot { fill: currentColor; stroke: none; }
.proxy-head-actions .proxy-delay-url { width: clamp(160px, 20vw, 280px); min-width: 0; height: 30px; padding: 0 10px; border-radius: 8px; font-size: 11px; user-select: text; }
.proxy-head-actions .proxy-delay-url:focus { border-color: var(--accent); outline: none; box-shadow: none; }
.proxy-head-actions .proxy-delay-url[aria-invalid="true"] { border-color: var(--bad); }
.proxy-head-actions .proxy-delay-custom { display: inline-flex; align-items: center; justify-content: center; width: 30px; height: 30px; padding: 0; line-height: 1; }
.proxy-delay-custom svg { flex: none; color: var(--muted); }
.proxy-head-actions .proxy-delay-custom.active { border-color: var(--accent-line, var(--accent)); background: var(--accent-soft, rgba(124,156,255,.10)); color: var(--accent-ink, var(--accent)); }
.proxy-delay-custom.active svg { color: inherit; }
.proxy-delay-error { margin: -5px 17px 12px; color: var(--bad); font-size: 11px; overflow-wrap: anywhere; }
@media (max-width: 680px) {
  .proxy-card-head { position: relative; }
  .proxy-summary { max-width: calc(100% - 34px); }
  .proxy-head-actions { flex-wrap: wrap; overflow-x: visible; }
  .proxy-head-actions .proxy-delay-url { width: 100%; order: 1; }
  .proxy-head-actions .proxy-chevron { position: absolute; top: 13px; right: 13px; }
}
</style>
