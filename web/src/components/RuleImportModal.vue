<script setup lang="ts">
import AppSelect from '@/components/AppSelect.vue'
import PolicySelect from '@/components/PolicySelect.vue'
import { computed, nextTick, onBeforeUnmount, ref, watch } from 'vue'
import BaseModal from '@/components/BaseModal.vue'
import { api, errorMessage, isAbortError, jsonRequest } from '@/services/api'
import { t } from '@/services/i18n'
import { notify } from '@/services/toast'
import { streamProfileJob } from '@/services/profile-jobs'
import { baseEntries, effectiveEntries, parseSequence } from '@/services/profile-sequences'
import type { SequenceEditorData } from '@/services/profile-sequences'
import type { ProfileItem, ProfileJob, ProfilesResponse } from '@/types/api'

type Issue = { line?: number; message: string; content?: string }
type ImportPreview = {
  token: string; format: string; ruleCount: number; providerRuleCount: number; replace: boolean
  providers: { name: string; behavior: string; format: string; count: number; sample: string[] }[]
  rules: string[]; groups: { name: string; type: string; proxies?: string[]; filter?: string }[]
  warnings: Issue[]; errors: Issue[]; extensions: Record<string, string>
}
const props = defineProps<{ open: boolean; profile?: ProfileItem | null }>()
const emit = defineEmits<{ close: []; saved: [] }>()
const profiles = ref<ProfileItem[]>([]), profileId = ref(''), source = ref<'file' | 'url' | 'text'>('file')
const file = ref<File | null>(null), fileInput = ref<HTMLInputElement | null>(null), text = ref(''), url = ref(''), name = ref(''), policy = ref('DIRECT')
const behavior = ref('auto'), placement = ref('auto'), loading = ref(false), busy = ref(false), saving = ref(false)
const preview = ref<ImportPreview | null>(null), acknowledged = ref(false), error = ref(''), policyOptions = ref(['DIRECT', 'REJECT', 'REJECT-DROP'])
const profileSelect = ref<{ focus: () => void } | null>(null), previewHeading = ref<HTMLHeadingElement | null>(null), progress = ref('')
let controller: AbortController | null = null, generation = 0, policyGeneration = 0, openGeneration = 0
const sources = ['file', 'url', 'text'] as const
const sourceLabels = { file: '文件', url: '链接', text: '粘贴文本' }
const selectedProfile = computed(() => profiles.value.find(item => item.id === profileId.value))
const canPreview = computed(() => !!profileId.value && (source.value === 'file' ? !!file.value : source.value === 'url' ? !!url.value.trim() : !!text.value.trim()))
const canSave = computed(() => !!preview.value && !preview.value.errors.length && (!preview.value.warnings.length || acknowledged.value) && !busy.value && !saving.value)
function invalidate() { generation++; controller?.abort(); controller = null; preview.value = null; acknowledged.value = false; busy.value = false; error.value = '' }
watch([profileId, source, file, text, url, name, policy, behavior, placement], invalidate)
async function loadPolicies() {
  const id = profileId.value, current = ++policyGeneration
  policyOptions.value = ['DIRECT', 'REJECT', 'REJECT-DROP']
  if (!id) return
  try {
    const data = await api<SequenceEditorData>(`/api/profiles/${id}/extensions/groups/editor`)
    if (current !== policyGeneration || !props.open) return
    const names: string[] = []
    for (const kind of ['groups', 'proxies'] as const) {
      const entries = effectiveEntries(baseEntries(data.base, kind), parseSequence(data.sequences?.[kind] || '', kind))
      for (const item of entries) if (typeof item !== 'string') names.push(item.name)
    }
    policyOptions.value = [...new Set([...policyOptions.value, ...names])]
  } catch { /* The preview validates the effective profile, including overrides. */ }
}
watch(profileId, loadPolicies)
watch(() => props.open, async open => {
  invalidate(); policyGeneration++
  const currentOpen = ++openGeneration
  if (!open) return
  loading.value = true
  profileId.value = ''; file.value = null; text.value = ''; url.value = ''; name.value = ''; policy.value = 'DIRECT'; behavior.value = 'auto'; placement.value = 'auto'; source.value = 'file'
  try {
    const data = await api<ProfilesResponse>('/api/profiles')
    if (!props.open || currentOpen !== openGeneration) return
    profiles.value = data.items || []
    profileId.value = props.profile?.id || profiles.value.find(item => item.current)?.id || profiles.value[0]?.id || ''
    await nextTick(); profileSelect.value?.focus()
  } catch (cause) { if (props.open && currentOpen === openGeneration) error.value = errorMessage(cause) }
  finally { if (currentOpen === openGeneration) loading.value = false }
})
async function chooseFile(event: Event) {
  file.value = (event.target as HTMLInputElement).files?.[0] || null
  if (file.value && !name.value) name.value = file.value.name.replace(/\.(yaml|yml|txt|list|mrs|ini)$/i, '')
}
async function requestPreview() {
  invalidate()
  if (!canPreview.value) return
  busy.value = true
  const current = generation, id = profileId.value
  controller = new AbortController()
  try {
    let content = text.value, encoding = ''
    if (source.value === 'file' && file.value) {
      if (file.value.size > 8 * 1024 * 1024) throw new Error(t('文件不能超过 8 MiB'))
      const bytes = new Uint8Array(await file.value.arrayBuffer())
      const parts: string[] = []
      for (let offset = 0; offset < bytes.length; offset += 32768) parts.push(String.fromCharCode(...bytes.subarray(offset, offset + 32768)))
      content = btoa(parts.join('')); encoding = 'base64'
    }
    if (current !== generation) return
    const result = await api<ImportPreview>(`/api/profiles/${id}/rule-import/preview`, { ...jsonRequest('POST', { source: source.value, content, encoding, url: url.value.trim(), name: name.value.trim() || 'Imported', policy: policy.value, behavior: behavior.value, placement: placement.value }), signal: controller!.signal })
    if (current !== generation || !props.open) return
    preview.value = result
    await nextTick(); previewHeading.value?.focus()
  } catch (cause) { if (current === generation && !isAbortError(cause)) error.value = errorMessage(cause) }
  finally { if (current === generation) { busy.value = false; controller = null } }
}
async function save(apply: boolean) {
  if (!canSave.value || !preview.value) return
  saving.value = true; error.value = ''; progress.value = ''
  let persisted = false
  try {
    let result = await api<ProfileJob & { ok?: boolean }>(`/api/profiles/${profileId.value}/rule-import/save`, jsonRequest('POST', { token: preview.value.token, acknowledge: acknowledged.value, apply }))
    // The enhancements are saved before the apply job starts. Clear the consumed
    // preview even if Core validation fails so the user cannot submit it again.
    preview.value = null
    persisted = true
    if (result.jobId) {
      result = await streamProfileJob(result.jobId, job => { progress.value = job.message || '' }, new AbortController().signal) as ProfileJob
      if (result.state === 'failed') throw new Error(t('规则增强已保存，但应用失败：{arg0}', { arg0: result.error || t('未知错误') }))
    }
    notify(t(apply ? '规则增强已保存并应用' : '规则增强已保存，点击订阅的应用按钮后生效'))
    emit('close')
  } catch (cause) { error.value = errorMessage(cause) }
  finally { saving.value = false; progress.value = ''; if (persisted) emit('saved') }
}
function close() { if (saving.value) return; invalidate(); emit('close') }
function keydown(event: KeyboardEvent) {
  if (event.key === 'Escape' && !saving.value) { event.preventDefault(); event.stopPropagation(); close() }
  if (event.key === 'Tab') {
    const dialog = (event.currentTarget as HTMLElement).closest('[role="dialog"]')
    const elements = [...(dialog?.querySelectorAll<HTMLElement>('button:not(:disabled),input:not(:disabled),select:not(:disabled),textarea:not(:disabled),summary,[tabindex="0"]') || [])].filter(element => element.tabIndex >= 0 && !element.matches(':disabled') && element.getClientRects().length)
    const first = elements[0], last = elements[elements.length - 1]
    if (event.shiftKey && document.activeElement === first) { event.preventDefault(); last?.focus() }
    if (!event.shiftKey && document.activeElement === last) { event.preventDefault(); first?.focus() }
  }
}
onBeforeUnmount(() => { generation++; policyGeneration++; controller?.abort() })
</script>

<template>
  <BaseModal :open="open" :title="t('导入规则集')" card-class="rule-import-modal" :closable="!saving" @close="close">
    <div class="rule-import-body" @keydown="keydown">
      <p class="muted">{{ t('支持 YAML、文本列表、MRS 和 ACL4SSR INI。预览后保存到所选订阅增强，订阅更新后保留。') }}</p>
      <div class="rule-import-fields">
        <label class="field"><span>{{ t('目标订阅') }}</span><AppSelect :aria-label="t('目标订阅')" ref="profileSelect" v-model="profileId" :disabled="loading || saving" :options="[{ value: '', label: (t(loading ? '正在读取…' : '请选择订阅')), disabled: true }, ...(profiles).map((item) => ({ value: item.id, label: (item.name) + (item.current ? ` · ${t('当前使用')}` : '') }))]" /></label>
        <label class="field"><span>{{ t('规则集名称 / INI 名称前缀') }}</span><input v-model="name" :disabled="saving" placeholder="Imported"></label>
      </div>
      <p v-if="!loading && !profiles.length" class="sequence-error">{{ t('请先在订阅配置中添加或导入一个订阅。') }}</p>
      <div class="rule-import-tabs" role="group" :aria-label="t('导入来源')"><button v-for="item in sources" :key="item" type="button" class="ghost small" :class="{ active: source === item }" :aria-pressed="source === item" :disabled="saving" @click="source = item">{{ t(sourceLabels[item]) }}</button></div>
      <label v-if="source === 'file'" class="field"><span>{{ t('选择规则文件') }}</span><input ref="fileInput" type="file" accept=".yaml,.yml,.txt,.list,.mrs,.ini" :disabled="saving" @change="chooseFile"><small class="muted">{{ t('按内容识别格式，单个文件最多 8 MiB。') }}</small></label>
      <label v-else-if="source === 'url'" class="field"><span>{{ t('规则文件链接') }}</span><input v-model="url" type="url" :disabled="saving" placeholder="https://raw.githubusercontent.com/…"><small class="muted">{{ t('支持 HTTP/HTTPS 和 GitHub 文件页；远程规则集保留自动更新来源。') }}</small></label>
      <label v-else class="field"><span>{{ t('粘贴规则内容') }}</span><textarea v-model="text" class="editor rule-import-text" :disabled="saving" spellcheck="false" :placeholder="t('粘贴 payload、域名 / IP 列表、完整规则或 INI 内容')" /></label>
      <div class="rule-import-fields rule-import-options">
        <PolicySelect v-model="policy" :options="policyOptions" :disabled="saving" label="目标策略" allow-custom />
        <label class="field"><span>{{ t('规则类型') }}</span><AppSelect :aria-label="t('规则类型')" v-model="behavior" :disabled="saving" :options="[{ value: 'auto', label: (t('自动识别')) }, { value: 'domain', label: 'domain' }, { value: 'ipcidr', label: 'ipcidr' }, { value: 'classical', label: 'classical' }]" /></label>
        <label class="field"><span>{{ t('导入方式') }}</span><AppSelect :aria-label="t('导入方式')" v-model="placement" :disabled="saving" :options="[{ value: 'auto', label: (t('默认前置，INI 遵循模板设置')) }, { value: 'prepend', label: (t('前置合并')) }, { value: 'append', label: (t('后置合并')) }, { value: 'replace', label: (t('替换原有规则')) }]" /></label>
      </div>
      <small class="muted">{{ t('目标策略用于单个规则集；完整规则和 INI 保留各自的策略。') }}</small>
      <div class="actions"><button class="small" :disabled="!canPreview || busy || saving" @click="requestPreview">{{ t(busy ? '正在识别并生成预览…' : '识别并预览') }}</button><span v-if="busy" class="muted" role="status">{{ t('正在检查格式和远程规则源，请稍候。') }}</span></div>
      <p v-if="error" class="sequence-error" role="alert">{{ t(error) }}</p>
      <section v-if="preview" class="rule-import-preview" aria-live="polite">
        <h4 ref="previewHeading" tabindex="-1">{{ t('导入预览') }}</h4>
        <div class="rule-import-summary"><span class="tag">{{ preview.format }}</span><span>{{ t('{arg0} 个规则集 · {arg1} 条匹配内容 · {arg2} 条分流规则 · {arg3} 个代理组', { arg0: preview.providers.length, arg1: preview.providerRuleCount, arg2: preview.ruleCount, arg3: preview.groups.length }) }}</span><strong>{{ t(preview.replace ? '替换原有规则' : '合并到订阅增强') }}</strong></div>
        <div v-if="preview.providers.length" class="rule-import-provider-list"><details v-for="item in preview.providers" :key="item.name"><summary>{{ item.name }} · {{ item.behavior }} / {{ item.format }} · {{ item.count }}</summary><pre v-if="item.sample.length">{{ item.sample.join('\n') }}</pre><p v-else class="muted">{{ t('MRS 已校验版本、类型、计数和数据结构；二进制内容保持原格式。') }}</p></details></div>
        <details v-if="preview.rules.length"><summary>{{ t('分流规则预览（最多 100 条）') }}</summary><pre>{{ preview.rules.join('\n') }}</pre></details>
        <details v-if="preview.groups.length"><summary>{{ t('导入的代理组') }}</summary><div v-for="item in preview.groups" :key="item.name" class="rule-import-group"><strong>{{ item.name }}</strong> · {{ item.type }}<div class="muted">{{ (item.proxies || []).join(' · ') }}<template v-if="item.filter"> · {{ item.filter }}</template></div></div></details>
        <div v-if="preview.errors.length" class="rule-import-issues errors" role="alert"><strong>{{ t('以下问题需要修正后才能保存') }}</strong><ul><li v-for="(item, index) in preview.errors" :key="index"><span v-if="item.line">{{ t('第 {arg0} 行：', { arg0: item.line }) }}</span>{{ t(item.message) }}<code v-if="item.content">{{ item.content }}</code></li></ul></div>
        <div v-if="preview.warnings.length" class="rule-import-issues"><strong>{{ t('导入提示 / 未导入的条目') }}</strong><ul><li v-for="(item, index) in preview.warnings" :key="index"><span v-if="item.line">{{ t('第 {arg0} 行：', { arg0: item.line }) }}</span>{{ t(item.message) }}<code v-if="item.content">{{ item.content }}</code></li></ul><label class="rule-import-ack"><input v-model="acknowledged" type="checkbox" :disabled="saving">{{ t('我已阅读以上提示，确认按预览内容导入') }}</label></div>
        <details><summary>{{ t('将保存的订阅增强配置') }}</summary><div v-for="(content, kind) in preview.extensions" :key="kind"><strong>{{ kind }}</strong><pre>{{ content }}</pre></div></details>
      </section>
      <div class="actions rule-import-footer"><span class="muted">{{ selectedProfile?.name }}</span><button class="small" :disabled="!canSave" @click="save(false)">{{ t(saving ? '保存中…' : '保存到订阅') }}</button><button class="ghost small" :disabled="!canSave" @click="save(true)">{{ t('保存并应用') }}</button><button class="ghost small" :disabled="saving" @click="close">{{ t('取消') }}</button></div>
      <p v-if="progress" class="muted" role="status">{{ t(progress) }}</p>
    </div>
  </BaseModal>
</template>

<style scoped>
:global(.modal-card.rule-import-modal){width:min(900px,calc(100vw - 32px));max-height:calc(100dvh - 32px);overflow:auto}
.rule-import-body{display:flex;flex-direction:column;gap:14px;min-width:0}
.rule-import-body>p{margin:0}
.rule-import-fields{display:grid;grid-template-columns:repeat(2,minmax(0,1fr));gap:12px}
.rule-import-options{grid-template-columns:repeat(3,minmax(0,1fr))}
.field{display:flex;flex-direction:column;gap:6px;min-width:0}
.field input,.field .app-select{width:100%;min-width:0}
.rule-import-tabs{display:flex;gap:8px}
.rule-import-tabs .active{background:color-mix(in srgb,var(--accent) 12%,transparent);border-color:var(--accent);color:var(--accent)}
.rule-import-text{min-height:160px;max-height:260px;resize:vertical;width:100%}
.rule-import-preview{border:1px solid var(--line);border-radius:12px;padding:14px;display:flex;flex-direction:column;gap:12px;min-width:0}
.rule-import-preview h4{margin:0}
.rule-import-summary{display:flex;align-items:center;gap:8px;flex-wrap:wrap;font-size:13px}
.rule-import-summary strong{margin-left:auto}
.rule-import-preview details{border-top:1px solid var(--line);padding-top:8px;min-width:0}
.rule-import-preview summary{cursor:pointer;font-size:13px;overflow-wrap:anywhere}
.rule-import-preview pre{font-size:12px;line-height:1.6;white-space:pre-wrap;overflow-wrap:anywhere;max-height:240px;overflow:auto;background:var(--bg);padding:10px;border-radius:7px}
.rule-import-provider-list,.rule-import-group{display:flex;flex-direction:column;gap:8px;overflow-wrap:anywhere}
.rule-import-issues{padding:12px;background:var(--bg);border:1px solid var(--line);border-radius:8px;font-size:13px;overflow-wrap:anywhere}
.rule-import-issues.errors{border-color:var(--bad);color:var(--bad)}
.rule-import-issues ul{padding-left:20px;max-height:240px;overflow:auto;line-height:1.6}
.rule-import-issues code{display:block;white-space:pre-wrap;color:var(--muted);font-size:12px}
.rule-import-ack{display:flex;gap:8px;align-items:flex-start}
.rule-import-ack input{margin-top:3px;flex:none}
.rule-import-footer{justify-content:flex-end;border-top:1px solid var(--line);padding-top:12px;flex-wrap:wrap}
.rule-import-footer>span{margin-right:auto}
@media(max-width:650px){.rule-import-fields,.rule-import-options{grid-template-columns:1fr}.rule-import-summary strong{margin-left:0}.rule-import-footer>span{width:100%}}
</style>
