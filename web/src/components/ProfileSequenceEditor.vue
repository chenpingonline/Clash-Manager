<script setup lang="ts">
import { countLabel, t } from '@/services/i18n'

import { computed, nextTick, onMounted, onBeforeUnmount, ref, watch } from 'vue'
import { baseEntries, effectiveEntries, emptySequence, entryIdentity, makeRule, moveEntry, parseSequence, ruleParts, ruleTypes, ruleTypeLabels, sequenceRows, filterSequenceRows, sequenceEntrySummary } from '@/services/profile-sequences'
import type { NamedEntry, SequenceEditorData, SequenceEntry, SequenceExtension, SequenceKind, SequenceRow, SequenceSourceFilter } from '@/services/profile-sequences'
import { parseProxyInput } from '@/services/proxy-uri'
import { errorMessage } from '@/services/api'
import PolicySelect from './PolicySelect.vue'

const props = defineProps<{ kind: SequenceKind; modelValue: SequenceExtension; data: SequenceEditorData; disabled: boolean }>()
const emit = defineEmits<{ 'update:modelValue': [value: SequenceExtension]; advanced: [] }>()
const search = ref(''), error = ref(''), input = ref('')
const addSide = ref<'prepend' | 'append'>('prepend'), sourceFilter = ref<SequenceSourceFilter>('all')
const sourceOptions: { value: SequenceSourceFilter; label: string }[] = [{ value: 'all', label: '全部' }, { value: 'prepend', label: '前置' }, { value: 'base', label: '订阅原始' }, { value: 'append', label: '后置' }, { value: 'deleted', label: '已排除' }]
const ruleType = ref('DOMAIN'), payload = ref(''), policy = ref('DIRECT'), noResolve = ref(false)
const groupType = ref('select'), name = ref(''), icon = ref(''), members = ref<string[]>([]), providers = ref<string[]>([])
const url = ref('https://www.gstatic.com/generate_204'), interval = ref(300), timeout = ref(5000), tolerance = ref(50), expectedStatus = ref('*'), lazy = ref(true), strategy = ref('consistent-hashing')
const groupExtra = ref(false), filter = ref(''), excludeFilter = ref(''), includeAll = ref(false), hidden = ref(false)
const editIndex = ref<number | null>(null), editSide = ref<'prepend' | 'append'>('prepend'), originalObject = ref<NamedEntry | null>(null)
const originals = computed(() => baseEntries(props.data.base, props.kind))
const noun = computed(() => props.kind === 'rules' ? '规则' : props.kind === 'proxies' ? '节点' : '代理组')
function sequence(kind: SequenceKind) {
  if (kind === props.kind) return props.modelValue
  try { return parseSequence(props.data.sequences[kind] || 'prepend: []\nappend: []\ndelete: []', kind) } catch { return emptySequence() }
}
const availableProxies = computed(() => [...new Set(['DIRECT', 'REJECT', 'REJECT-DROP', 'PASS', ...effectiveEntries(baseEntries(props.data.base, 'groups'), sequence('groups')).map(entryIdentity), ...effectiveEntries(baseEntries(props.data.base, 'proxies'), sequence('proxies')).map(entryIdentity)])])
const providerNames = computed(() => Object.keys((props.data.base['proxy-providers'] || {}) as object))
const ruleProviderNames = computed(() => Object.keys((props.data.base['rule-providers'] || {}) as object))
const subRuleNames = computed(() => Object.keys((props.data.base['sub-rules'] || {}) as object))
const providerInput = ref(''), memberInput = ref('')
function addMember() {
  const value = memberInput.value.trim()
  if (value && value !== name.value.trim() && !members.value.includes(value)) members.value.push(value)
  memberInput.value = ''
}
function addProvider() {
  const value = providerInput.value.trim()
  if (value && !providers.value.includes(value)) providers.value.push(value)
  providerInput.value = ''
}
const canNoResolve = computed(() => ['GEOIP', 'IP-ASN', 'IP-CIDR', 'IP-CIDR6', 'IP-SUFFIX', 'RULE-SET'].includes(ruleType.value))
type Row = SequenceRow
const allRows = computed(() => sequenceRows(originals.value, props.modelValue))
const rows = computed(() => filterSequenceRows(allRows.value, search.value, sourceFilter.value))
const rulePlaceholder = computed(() => {
  if (ruleType.value === 'RULE-SET') return '规则集名称'
  if (ruleType.value === 'SUB-RULE') return '子规则名称'
  if (ruleType.value.includes('CIDR')) return ruleType.value === 'IP-CIDR6' ? '2001:db8::/32' : '192.168.1.0/24'
  if (ruleType.value.includes('PORT')) return '443 或 1000-2000'
  if (ruleType.value === 'NETWORK') return 'tcp 或 udp'
  if (ruleType.value.includes('GEO')) return ruleType.value === 'GEOSITE' ? 'cn' : 'CN'
  return ruleType.value.startsWith('DOMAIN') ? 'example.com' : '请输入匹配内容'
})
const appendWarning = computed(() => addSide.value === 'append' && effectiveEntries(originals.value, props.modelValue).some(item => typeof item === 'string' && ruleParts(item)[0] === 'MATCH'))
function describe(item: SequenceEntry): string { return typeof item === 'string' ? item : `${item.name} ${item.type}` }
function details(item: SequenceEntry) {
  if (typeof item !== 'string') return { title: item.name, type: item.type, policy: '' }
  const parts = ruleParts(item), match = parts[0] === 'MATCH'
  return { title: match ? t('所有其他流量') : parts[1] || item, type: parts[0], policy: parts[match ? 1 : 2] || '' }
}
const viewport = ref<HTMLElement | null>(null), scrollTop = ref(0), viewportHeight = ref(440)
const rowHeight = 26
const start = computed(() => Math.max(0, Math.min(rows.value.length - 1, Math.floor(scrollTop.value / rowHeight)) - 4))
const end = computed(() => Math.min(rows.value.length, start.value + Math.ceil(viewportHeight.value / rowHeight) + 8))
const visibleRows = computed(() => rows.value.slice(start.value, end.value))
watch([search, sourceFilter], () => { scrollTop.value = 0; if (viewport.value) viewport.value.scrollTop = 0 })
let resizeObserver: ResizeObserver | undefined
onMounted(() => { if (viewport.value) { resizeObserver = new ResizeObserver(entries => { viewportHeight.value = entries[0]?.contentRect.height || 440 }); resizeObserver.observe(viewport.value) } })
onBeforeUnmount(() => resizeObserver?.disconnect())
function remove(row: Row) {
  if (row.side === 'base') {
    const identity = entryIdentity(row.item), deleted = new Set(props.modelValue.delete)
    if (deleted.has(identity)) deleted.delete(identity); else deleted.add(identity)
    emit('update:modelValue', { ...props.modelValue, delete: [...deleted] })
  } else {
    emit('update:modelValue', { ...props.modelValue, [row.side]: props.modelValue[row.side].filter((_, index) => index !== row.index) })
    cancelEdit()
  }
}
function cancelEdit() { editIndex.value = null; originalObject.value = null; error.value = '' }
function edit(row: Row) {
  if (row.side === 'base') return
  if (props.kind === 'proxies') { emit('advanced'); return }
  editIndex.value = row.index; editSide.value = row.side; addSide.value = row.side
  if (typeof row.item === 'string') {
    const parts = ruleParts(row.item); ruleType.value = parts[0]!; payload.value = parts[0] === 'MATCH' ? '' : parts[1] || ''; policy.value = parts[parts[0] === 'MATCH' ? 1 : 2] || ''; noResolve.value = parts.includes('no-resolve')
  } else {
    const value = row.item; originalObject.value = value
    name.value = value.name; groupType.value = value.type; icon.value = String(value.icon || '')
    members.value = Array.isArray(value.proxies) ? [...value.proxies] as string[] : []; providers.value = Array.isArray(value.use) ? [...value.use] as string[] : []
    url.value = String(value.url || 'https://www.gstatic.com/generate_204'); interval.value = Number(value.interval ?? 300); timeout.value = Number(value.timeout ?? 5000); tolerance.value = Number(value.tolerance ?? 50); lazy.value = value.lazy !== false; expectedStatus.value = String(value['expected-status'] || '*'); strategy.value = String(value.strategy || 'consistent-hashing')
    filter.value = String(value.filter || ''); excludeFilter.value = String(value['exclude-filter'] || ''); includeAll.value = value['include-all'] === true; hidden.value = value.hidden === true
  }
}
async function add(side: 'prepend' | 'append') {
  try {
    let additions: SequenceEntry[]
    if (props.kind === 'rules') additions = [makeRule(ruleType.value, payload.value, policy.value, canNoResolve.value && noResolve.value)]
    else if (props.kind === 'proxies') additions = parseProxyInput(input.value)
    else {
      if (!name.value.trim() || name.value.includes(',')) throw new Error('请填写有效的代理组名称')
      if (!members.value.length && !providers.value.length && !includeAll.value) throw new Error('请引入代理节点或代理集合')
      if (members.value.includes(name.value.trim())) throw new Error('代理组不能引用自身')
      if (![interval.value, timeout.value, tolerance.value].every(Number.isInteger) || interval.value < 0 || timeout.value <= 0 || tolerance.value < 0) throw new Error('检查间隔、超时和容差必须是有效整数')
      if (url.value) { const parsed = new URL(url.value); if (!['http:', 'https:'].includes(parsed.protocol)) throw new Error('健康检查地址必须使用 HTTP 或 HTTPS') }
      const group: NamedEntry = { ...(originalObject.value || {}), name: name.value.trim(), type: groupType.value, proxies: [...members.value], use: [...providers.value] }
      if (icon.value.trim()) group.icon = icon.value.trim(); else delete group.icon
      group.url = url.value.trim(); group.interval = interval.value; group.timeout = timeout.value; group.lazy = lazy.value; group['expected-status'] = expectedStatus.value.trim() || '*'
      if (groupType.value === 'url-test') group.tolerance = tolerance.value
      if (groupType.value === 'load-balance') group.strategy = strategy.value
      for (const [key, value] of [['filter', filter.value], ['exclude-filter', excludeFilter.value]] as const) { if (value) group[key] = value; else delete group[key] }
      group['include-all'] = includeAll.value; group.hidden = hidden.value
      additions = [group]
    }
    const next = { ...props.modelValue, prepend: [...props.modelValue.prepend], append: [...props.modelValue.append] }
    const editing = editIndex.value !== null
    if (editing) next[editSide.value].splice(editIndex.value!, 1)
    const existing = new Set(effectiveEntries(originals.value, next).map(entryIdentity))
    if (props.kind !== 'rules') {
      const otherKind = props.kind === 'groups' ? 'proxies' : 'groups'
      for (const item of effectiveEntries(baseEntries(props.data.base, otherKind), sequence(otherKind))) existing.add(entryIdentity(item))
      for (const item of ['DIRECT', 'REJECT', 'REJECT-DROP', 'PASS', 'GLOBAL']) existing.add(item)
    }
    for (const item of additions) if (existing.has(entryIdentity(item))) throw new Error(`${noun.value}已存在；修改订阅原始条目时请先排除原条目`)
    if (editing && editSide.value === side) next[side].splice(editIndex.value!, 0, ...additions)
    else if (side === 'prepend') next.prepend.unshift(...additions)
    else next.append.push(...additions)
    emit('update:modelValue', next); error.value = ''; cancelEdit(); input.value = ''; payload.value = ''
    sourceFilter.value = 'all'; search.value = ''
    await nextTick()
    if (viewport.value) viewport.value.scrollTop = side === 'prepend' ? 0 : rows.value.length * rowHeight
  } catch (cause) { error.value = errorMessage(cause) }
}
function reorder(row: Row, delta: number) {
  if (row.side !== 'base') emit('update:modelValue', moveEntry(props.modelValue, row.side, row.index, delta))
  cancelEdit()
}
</script>

<template>
  <div class="sequence-editor sequence-rule-editor" :class="`sequence-${kind}-editor`">
    <fieldset class="sequence-form" :disabled="disabled">
      <template v-if="kind === 'rules'">
        <h4 class="rule-form-title">{{ t(editIndex !== null ? '编辑自定义规则' : '添加规则') }}</h4>
        <div class="rule-add-bar">
          <label>{{ t("规则类型") }}<select v-model="ruleType"><option v-for="type in ruleTypes" :key="type" :value="type">{{ t(ruleTypeLabels[type]) }} · {{ t(type) }}</option></select></label>
          <label>{{ t("规则内容") }}<input v-model="payload" :disabled="ruleType === 'MATCH'" :list="ruleType === 'RULE-SET' ? 'sequence-rule-providers' : ruleType === 'SUB-RULE' ? 'sequence-sub-rules' : undefined" :placeholder="t(ruleType === 'MATCH' ? '无需填写匹配内容' : rulePlaceholder)" @keydown.enter.prevent="add(addSide)" /></label>
          <PolicySelect v-model="policy" :options="availableProxies" :disabled="disabled" />
          <div class="rule-position-field"><span id="rule-position-label">{{ t("添加位置") }}</span><div class="rule-position" role="group" aria-labelledby="rule-position-label"><button type="button" :aria-pressed="addSide === 'prepend'" :class="{ active: addSide === 'prepend' }" @click="addSide = 'prepend'">{{ t("前置") }}</button><button type="button" :aria-pressed="addSide === 'append'" :class="{ active: addSide === 'append' }" @click="addSide = 'append'">{{ t("后置") }}</button></div></div>
          <button class="small rule-add-submit" type="button" @click="add(addSide)">{{ t(editIndex !== null ? '更新规则' : '添加规则') }}</button>
        </div>
        <datalist id="sequence-rule-providers"><option v-for="item in ruleProviderNames" :key="item" :value="item" /></datalist>
        <datalist id="sequence-sub-rules"><option v-for="item in subRuleNames" :key="item" :value="item" /></datalist>
        <div class="rule-add-help"><span class="muted">{{ t("前置优先匹配，后置在订阅规则后。") }}</span><label v-if="canNoResolve" class="sequence-check"><input v-model="noResolve" type="checkbox" />{{ t("不解析域名（no-resolve）") }}</label><button v-if="editIndex !== null" type="button" class="rule-text-action" @click="cancelEdit">{{ t("取消条目编辑") }}</button></div>
        <p v-if="appendWarning" class="rule-order-warning">{{ t("已有 MATCH 兜底规则，后置规则可能无法匹配；建议使用前置。") }}</p>
        <p v-if="error" class="sequence-error" role="alert">{{ t(error) }}</p>
      </template>
      <template v-else-if="kind === 'proxies'">
        <h4 class="rule-form-title">{{ t("添加节点") }}</h4>
        <div class="sequence-node-add"><label>{{ t("节点链接") }}<textarea v-model="input" spellcheck="false" :placeholder="t('每行一条 URI，也可粘贴 Base64 编码的节点列表')" /></label><div class="sequence-entry-actions"><div class="rule-position-field"><span id="node-position-label">{{ t("添加位置") }}</span><div class="rule-position" role="group" aria-labelledby="node-position-label"><button type="button" :aria-pressed="addSide === 'prepend'" :class="{ active: addSide === 'prepend' }" @click="addSide = 'prepend'">{{ t("前置") }}</button><button type="button" :aria-pressed="addSide === 'append'" :class="{ active: addSide === 'append' }" @click="addSide = 'append'">{{ t("后置") }}</button></div></div><button type="button" class="small rule-add-submit" @click="add(addSide)">{{ t("添加节点") }}</button></div></div>
        <p class="sequence-node-hint muted">{{ t("支持 SS、VMess、VLESS、Trojan、AnyTLS、Hysteria2、TUIC、HTTP 和 SOCKS5；其他节点参数可在高级 YAML 中编辑。") }}</p>
      </template>
      <template v-else>
        <div class="sequence-group-heading"><h4 class="rule-form-title">{{ t(editIndex !== null ? '编辑自定义代理组' : '添加代理组') }}</h4><button type="button" class="ghost small" :aria-expanded="groupExtra" aria-controls="sequence-group-options" @click="groupExtra = !groupExtra">{{ t(groupExtra ? '收起更多设置' : '更多设置') }}</button></div>
        <div class="sequence-group-fields">
          <label>{{ t("代理组类型") }}<select v-model="groupType"><option value="select">{{ t("手动选择 · select") }}</option><option value="url-test">{{ t("自动选择 · url-test") }}</option><option value="fallback">{{ t("故障转移 · fallback") }}</option><option value="load-balance">{{ t("负载均衡 · load-balance") }}</option></select></label>
          <label>{{ t("代理组名称") }}<input v-model="name" :placeholder="t('我的代理组')" /></label>
          <div class="sequence-member-field"><span>{{ t("引入代理") }}</span><div class="sequence-provider-input"><PolicySelect v-model="memberInput" :options="availableProxies.filter(item => item !== name.trim())" :disabled="disabled" :label="t('引入代理')" :placeholder="t('选择或输入名称')" :search-placeholder="t('搜索代理组、节点或输入名称')" hide-label allow-custom /><button type="button" class="ghost small" @click="addMember">{{ t("引入代理") }}</button></div><div class="sequence-member-chips"><span v-if="!members.length" class="muted">{{ t("尚未引入代理") }}</span><span v-for="item in members" :key="item" class="sequence-member-chip"><span :title="item">{{ item }}</span><button type="button" :aria-label="t(`移除代理 ${item}`)" @click="members = members.filter(value => value !== item)">×</button></span></div></div>
          <div class="sequence-member-field"><span>{{ t("引入代理集合") }}</span><div class="sequence-provider-input"><PolicySelect v-model="providerInput" :options="providerNames" :disabled="disabled" :label="t('引入代理集合')" :placeholder="t('选择或输入集合名称')" :search-placeholder="t('搜索集合或输入名称')" hide-label allow-custom /><button type="button" class="ghost small" @click="addProvider">{{ t("引入集合") }}</button></div><div class="sequence-member-chips"><span v-if="!providers.length" class="muted">{{ t("尚未引入集合") }}</span><span v-for="item in providers" :key="item" class="sequence-member-chip"><span :title="item">{{ item }}</span><button type="button" :aria-label="t(`移除集合 ${item}`)" @click="providers = providers.filter(value => value !== item)">×</button></span></div></div>
        </div>
        <div v-if="groupExtra" id="sequence-group-options" class="sequence-group-options">
          <label>{{ t("代理组图标") }}<input v-model="icon" :placeholder="t('图标 URL（可选）')" /></label><label>{{ t("健康检查地址") }}<input v-model="url" type="url" /></label><label>{{ t("期望状态码") }}<input v-model="expectedStatus" :placeholder="t('* 或 200 / 204')" /></label><label>{{ t("检查间隔（秒）") }}<input v-model.number="interval" type="number" min="0" /></label><label>{{ t("超时（毫秒）") }}<input v-model.number="timeout" type="number" min="1" /></label>
          <label v-if="groupType === 'url-test'">{{ t("切换容差（毫秒）") }}<input v-model.number="tolerance" type="number" min="0" /></label><label v-if="groupType === 'load-balance'">{{ t("负载均衡策略") }}<select v-model="strategy"><option>consistent-hashing</option><option>round-robin</option><option>sticky-sessions</option></select></label><label>{{ t("节点过滤") }}<input v-model="filter" :placeholder="t('正则表达式')" /></label><label>{{ t("排除节点") }}<input v-model="excludeFilter" :placeholder="t('正则表达式')" /></label><label class="sequence-check"><input v-model="lazy" type="checkbox" />{{ t("仅在使用时检查") }}</label><label class="sequence-check"><input v-model="includeAll" type="checkbox" />{{ t("包含所有节点与代理集合") }}</label><label class="sequence-check"><input v-model="hidden" type="checkbox" />{{ t("隐藏代理组") }}</label>
        </div>
        <div class="sequence-entry-actions sequence-group-actions"><span class="muted">{{ t("成员可多次引入，前置 / 后置决定列表顺序。") }}</span><div class="rule-position-field"><span id="group-position-label">{{ t("添加位置") }}</span><div class="rule-position" role="group" aria-labelledby="group-position-label"><button type="button" :aria-pressed="addSide === 'prepend'" :class="{ active: addSide === 'prepend' }" @click="addSide = 'prepend'">{{ t("前置") }}</button><button type="button" :aria-pressed="addSide === 'append'" :class="{ active: addSide === 'append' }" @click="addSide = 'append'">{{ t("后置") }}</button></div></div><button type="button" class="small rule-add-submit" @click="add(addSide)">{{ t(editIndex !== null ? '更新代理组' : '添加代理组') }}</button><button v-if="editIndex !== null" type="button" class="ghost small" @click="cancelEdit">{{ t("取消条目编辑") }}</button></div>
      </template>
      <p v-if="kind !== 'rules' && error" class="sequence-error" role="alert">{{ t(error) }}</p>
    </fieldset>
    <div class="sequence-list-panel">
      <div class="rule-list-toolbar">
        <input v-model="search" :aria-label="t(`搜索${noun}`)" :placeholder="t(kind === 'rules' ? '搜索规则、类型或策略' : `搜索${noun}名称或类型`)" />
        <div class="rule-source-filters" role="group" :aria-label="t('{arg0}来源', { arg0: t(noun) })"><button v-for="option in sourceOptions" :key="option.value" type="button" :aria-pressed="sourceFilter === option.value" :class="{ active: sourceFilter === option.value }" @click="sourceFilter = option.value">{{ t(option.label) }}<span v-if="option.value !== 'all'">{{ option.value === 'base' ? originals.length : option.value === 'deleted' ? allRows.filter(row => row.deleted).length : modelValue[option.value].length }}</span></button></div>
        <span class="sequence-count muted">{{ countLabel(rows.length, kind === 'rules' ? '条规则' : kind === 'proxies' ? '个节点' : '个代理组') }}</span>
      </div>
      <div v-if="data.warning" class="muted">{{ t(data.warning) }}</div>
      <div class="sequence-table-shell" role="table" :aria-label="t(`订阅${noun}`)" :aria-rowcount="rows.length + 1">
        <div class="rule-table-heading rule-table-grid" :class="{ 'sequence-named-grid': kind !== 'rules' }" role="row" aria-rowindex="1"><span role="columnheader">{{ t("序号") }}</span><span role="columnheader">{{ kind === 'rules' ? t('规则内容') : t('{arg0}名称', { arg0: t(noun) }) }}</span><span role="columnheader">{{ t("类型") }}</span><span role="columnheader">{{ t(kind === 'rules' ? '策略' : kind === 'proxies' ? '服务器' : '成员 / 集合') }}</span><span role="columnheader">{{ t("来源") }}</span><span role="columnheader">{{ t("操作") }}</span></div>
        <div ref="viewport" class="sequence-list rule-table-body" role="rowgroup" @scroll="scrollTop = ($event.target as HTMLElement).scrollTop">
          <div v-if="!rows.length" class="sequence-empty muted">{{ t(search || sourceFilter !== 'all' ? '没有匹配的条目' : '暂无条目') }}</div>
          <div :style="{ height: `${start * rowHeight}px` }" />
          <div v-for="(row, offset) in visibleRows" :key="row.key" class="rule-table-row rule-table-grid" :class="{ 'sequence-named-grid': kind !== 'rules', 'rule-excluded': row.deleted, 'rule-editing': editIndex === row.index && editSide === row.side }" role="row" :aria-rowindex="start + offset + 2">
            <span class="muted" role="cell">{{ t(row.order) }}</span><span class="rule-table-content" role="cell" :title="describe(row.item)">{{ details(row.item).title }}</span><span role="cell" :title="details(row.item).type">{{ details(row.item).type }}</span><span role="cell" :title="sequenceEntrySummary(row.item, kind)">{{ sequenceEntrySummary(row.item, kind) }}</span><span role="cell" class="rule-table-source">{{ t(row.deleted ? '已排除' : row.side === 'base' ? '订阅原始' : row.side === 'prepend' ? '前置' : '后置') }}</span>
            <div class="rule-table-actions" role="cell">
              <template v-if="row.side !== 'base'"><button class="rule-text-action" :disabled="disabled" :aria-label="t(`编辑 ${entryIdentity(row.item)}`)" :title="t(kind === 'proxies' ? '在高级 YAML 中编辑节点参数' : undefined)" @click="edit(row)">{{ t("编辑") }}</button><button class="rule-text-action rule-move" :disabled="disabled || row.index === 0" :aria-label="t(`上移 ${entryIdentity(row.item)}`)" :title="t('上移')" @click="reorder(row, -1)">↑</button><button class="rule-text-action rule-move" :disabled="disabled || row.index === modelValue[row.side].length - 1" :aria-label="t(`下移 ${entryIdentity(row.item)}`)" :title="t('下移')" @click="reorder(row, 1)">↓</button></template>
              <button class="rule-text-action" :class="{ 'rule-delete-action': row.side !== 'base', 'rule-exclude-action': row.side === 'base' && !row.deleted, 'rule-restore-action': row.deleted }" :disabled="disabled" :aria-label="`${t(row.deleted ? '恢复' : row.side === 'base' ? '排除' : '删除')} ${entryIdentity(row.item)}`" @click="remove(row)">{{ t(row.deleted ? '恢复' : row.side === 'base' ? '排除' : '删除') }}</button>
            </div>
          </div>
          <div :style="{ height: `${Math.max(0, rows.length - end) * rowHeight}px` }" />
        </div>
      </div>
    </div>
  </div>
</template>
