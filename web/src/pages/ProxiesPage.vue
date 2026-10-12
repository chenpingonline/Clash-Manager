<script setup lang="ts">
import { countLabel, t } from '@/services/i18n'

import { computed, nextTick, onMounted, reactive, ref } from 'vue'
import AsyncState from '@/components/AsyncState.vue'
import BaseModal from '@/components/BaseModal.vue'
import { useDelayTests } from '@/composables/useDelayTests'
import { api, errorMessage, jsonRequest } from '@/services/api'
import { providerUpdatedText } from '@/services/format'
import { parseProxyGroupSortPreferences, shouldShowProxyNode, sortProxyNodeNames, type ProxyNodeSort } from '@/services/proxy-view'
import { latencyClass } from '@/services/latency'
import { notify } from '@/services/toast'
import type { ProxiesResponse, ProxyNode, ProxyProvider, ProxyProvidersResponse, GroupDelayContext } from '@/types/api'

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
const settingGroup = ref('')
const settingMode = ref<'auto' | 'global' | 'custom'>('auto')
const settingURL = ref('')
const settingBusy = ref(false)
const settingError = ref('')
const settingContext = ref<GroupDelayContext | null>(null)
const selectedTarget = computed(() => settingContext.value?.groups[settingGroup.value])
const previewURL = computed(() => settingMode.value === 'custom' ? settingURL.value : settingMode.value === 'global' ? settingContext.value?.globalUrl : selectedTarget.value?.configUrl || settingContext.value?.globalUrl)
function openDelaySettings(group: ProxyGroup) {
  settingGroup.value = group.name
  settingContext.value = delayTests.context.value ? JSON.parse(JSON.stringify(delayTests.context.value)) : null
  settingMode.value = selectedTarget.value?.mode || 'auto'
  settingURL.value = settingMode.value === 'custom' ? selectedTarget.value?.url || '' : ''
  settingError.value = ''
}
async function saveDelaySettings() {
  settingBusy.value = true
  settingError.value = ''
  try {
    const context = await api<GroupDelayContext>('/api/delay-settings', jsonRequest('PUT', { scope: settingContext.value?.scope, group: settingGroup.value, mode: settingMode.value, url: settingURL.value }))
    delayTests.setContext(context)
    settingGroup.value = ''
    notify('测速设置已保存')
  } catch (cause) { settingError.value = errorMessage(cause) }
  finally { settingBusy.value = false }
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
  if (!testable(name)) return
  try {
    await delayTests.start([name], group)
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
    await delayTests.start(queue, group)
    notify(label)
  } catch (cause) {
    notify(errorMessage(cause), true)
  }
}
async function testAllGroups() {
 const requests = groups.value.map(group => ({ group: group.name, names: [...new Set(group.proxy.all || [])].filter(testable) })).filter(group => group.names.length)
 if (!requests.length) return notify('没有可测速节点')
 try { await delayTests.startGroups(requests); notify('全部节点测速完成') }
 catch (cause) { notify(errorMessage(cause), true) }
}
function groupTesting(name: string) {
  const context = delayTests.context.value
  const target = context?.groups[name]
  const members = groups.value.find(group => group.name === name)?.proxy.all || []
  return delayTests.testing.value && Boolean(delayTests.status.value.targets?.some(item =>
    item.scope === context?.scope && item.url === target?.url && (item.expected || '') === (target?.expected || '') && members.includes(item.name),
  ))
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
        <span aria-hidden="true">⌕</span>
        <input v-model="query" :placeholder="t('搜索代理组或节点')" :aria-label="t('搜索代理组或节点')" autocomplete="off">
        <button v-if="query" type="button" class="search-clear" :aria-label="t('清空搜索')" @click="query = ''">×</button>
      </div>
      <span class="search-result">{{ t(query ? `${filtered.length} 组 · ${visibleNodeCount} 个节点` : '') }}</span>
      <button class="ghost rule-provider-trigger" @click="openProviderManager">{{ t("策略组") }} <span v-if="providersLoaded">{{ t(providerEntries.length) }}</span></button>
      <button class="ghost" :disabled="loading || !groups.length" @click="toggleAll">{{ t(allExpanded ? '全部收起' : '全部展开') }}</button>
      <button class="ghost" :disabled="loading || !groups.length || delayTestBusy" @click="testAllGroups">{{ t(delayTestBusy ? '测试中…' : '延迟测试') }}</button>
    </div>
  </Teleport>

  <AsyncState :loading="loading" :error="t(error)">
    <div class="proxy-groups">
      <section v-for="group in filtered" :key="group.name" class="card proxy-card" :class="{ expanded: expanded.has(group.name), 'search-expanded': query }" :data-group-name="group.name">
        <div class="proxy-card-head" @click="expanded.has(group.name) ? expanded.delete(group.name) : expanded.add(group.name)">
          <div class="proxy-summary"><div class="proxy-name-row"><h3>{{ group.name }}</h3><span class="tag">{{ t(group.proxy.type || 'Selector') }}</span></div><div class="proxy-current"><span>{{ t("当前") }}</span><strong>{{ group.proxy.now || '-' }}</strong><span v-if="snapshot(group.proxy.now || '', group.name).text !== '--'" class="current-delay" :class="snapshot(group.proxy.now || '', group.name).className">{{ t(snapshot(group.proxy.now || '', group.name).text) }}</span></div></div>
          <div class="proxy-head-actions">
            <button class="proxy-group-tool proxy-locate ghost small" :disabled="!group.proxy.now" :aria-label="t('定位当前节点')" :title="t('定位当前节点')" @click.stop="locateCurrent(group)"><svg viewBox="0 0 24 24" aria-hidden="true"><circle cx="12" cy="12" r="6.5" /><path d="M12 2.5v3M12 18.5v3M2.5 12h3M18.5 12h3" /></svg></button>
            <button class="proxy-group-tool ghost small" :disabled="!delayTests.context.value" :aria-label="t('测速设置')" :title="t('测速设置')" @click.stop="openDelaySettings(group)">⚙</button>
            <button class="proxy-group-tool proxy-latency-test ghost small" :disabled="delayTestBusy" :title="t('测试本组全部可测速节点')" @click.stop="testGroup(group)">{{ t(groupTesting(group.name) ? '测试中…' : '延迟测试') }}</button>
            <label class="proxy-sort-control" :title="t('设置本组节点排序方式')" @click.stop>
              <span>{{ t("排序") }}</span>
              <select :value="groupSort(group.name)" :aria-label="t(`${group.name} 排序方式`)" @click.stop @change.stop="updateGroupSort(group.name, $event)">
                <option value="default">{{ t("默认") }}</option>
                <option value="delay">{{ t("按延迟") }}</option>
                <option value="name">{{ t("按名称") }}</option>
              </select>
            </label>
            <span class="node-count" :title="countLabel(group.proxy.all?.length || 0, '个节点')">{{ t(group.proxy.all?.length || 0) }}</span>
            <span class="proxy-chevron" aria-hidden="true"><svg viewBox="0 0 20 20"><path d="m5.5 7.5 4.5 4.5 4.5-4.5" /></svg></span>
          </div>
        </div>
        <div class="proxy-body">
          <div class="proxy-group-filter">
            <label class="proxy-filter-search"><span>{{ t("筛选") }}</span><input :value="groupFilter(group.name)" :aria-label="t(`${group.name} 筛选节点`)" :placeholder="t('输入节点名称')" @input="updateGroupFilter(group.name, $event)"></label>
            <button v-if="groupFilter(group.name)" class="ghost small" @click="groupFilters.delete(group.name)">{{ t("清除") }}</button>
            <label class="proxy-timeout-toggle" :title="t('关闭后隐藏测速结果为超时的节点')"><input type="checkbox" :checked="groupShowsTimeoutNodes(group.name)" @change="updateTimeoutVisibility(group.name, $event)"><span>{{ t("显示超时节点") }}</span></label>
            <span>{{ t(group.nodes.length) }} / {{ t(group.proxy.all?.length || 0) }} {{ t("个节点") }}</span>
          </div>
          <div v-if="group.nodes.length" class="node-list"><div v-for="name in group.nodes" :key="name" class="node-row" :class="{ active: group.proxy.now === name }" :data-node-name="name"><button class="node-select" @click="select(group, name)"><span class="node-copy"><span class="node-name" :title="name">{{ name }}</span><span v-if="rawProxies[name]?.type" class="node-type">{{ rawProxies[name]?.type }}</span></span></button><button class="node-delay" :class="snapshot(name, group.name).className" :disabled="delayTestBusy" :title="snapshot(name, group.name).error || t('单独测试该节点延迟')" @click="testOne(name, group.name)">{{ t(snapshot(name, group.name).text) }}</button></div></div>
          <div v-else class="proxy-group-empty">{{ t("没有匹配的节点") }}</div>
        </div>
      </section>
    </div>
  </AsyncState>

  <BaseModal :open="Boolean(settingGroup)" :title="t('测速设置') + ' · ' + settingGroup" :closable="!settingBusy" @close="settingGroup = ''">
    <form class="group-delay-settings" @submit.prevent="saveDelaySettings">
      <div class="field"><label for="group-delay-mode">{{ t('测试地址') }}</label><select id="group-delay-mode" v-model="settingMode" :disabled="settingBusy"><option value="auto">{{ t('自动：优先使用组配置') }}</option><option value="global">{{ t('使用全局地址') }}</option><option value="custom">{{ t('自定义') }}</option></select></div>
      <div v-if="settingMode === 'custom'" class="field"><label for="group-delay-url">{{ t('自定义测试地址') }}</label><input id="group-delay-url" v-model="settingURL" type="url" required maxlength="4096" :disabled="settingBusy" placeholder="https://example.com/generate_204"></div>
      <div class="field"><label>{{ t('实际测试地址') }}</label><span class="group-delay-url mono">{{ previewURL }}</span></div>
      <p>{{ t('设置仅作用于当前订阅的面板手动测速，不改变内核自动健康检查。') }}</p>
      <p v-if="settingError" class="error" role="alert">{{ t(settingError) }}</p>
      <div class="actions"><button type="button" class="ghost" :disabled="settingBusy" @click="settingMode = 'auto'; settingURL = ''">{{ t('恢复默认') }}</button><button type="button" class="ghost" :disabled="settingBusy" @click="settingGroup = ''">{{ t('取消') }}</button><button type="submit" :disabled="settingBusy">{{ t(settingBusy ? '保存中…' : '保存') }}</button></div>
    </form>
  </BaseModal>

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
.group-delay-settings { display: grid; gap: 16px; }
.group-delay-settings p { margin: 0; color: var(--muted); font-size: 12px; line-height: 1.6; }
.group-delay-url { overflow-wrap: anywhere; font-size: 12px; }
.group-delay-settings .error { color: var(--bad); }
</style>
