<script setup lang="ts">
import { t } from '@/services/i18n'
import { runtime } from '@/services/runtime'

import { computed, reactive, ref, watch } from 'vue'
import TunStackFields from '@/components/settings/TunStackFields.vue'
import BaseModal from '@/components/BaseModal.vue'
import HelpPopover from '@/components/HelpPopover.vue'
import SettingToggle from '@/components/settings/SettingToggle.vue'
import { useOperationProgress } from '@/composables/useOperationProgress'
import { api, errorMessage, jsonRequest } from '@/services/api'
import { notify } from '@/services/toast'
import type { NetworkSettingsResponse, TunFeatures, TunSetting } from '@/types/api'

type TunForm = Required<TunSetting>

const defaultTun = (): TunForm => ({
  enabled: false,
  stack: '',
  congestionController: '',
  mtu: 1500,
  routeExcludeAddress: [],
  autoRoute: true,
  autoRedirect: true,
  autoDetectInterface: true,
  dnsHijack: false,
  strictRoute: false,
})

const props = defineProps<{ open: boolean; initialSettings?: NetworkSettingsResponse | null }>()
const emit = defineEmits<{ close: []; saved: [response: NetworkSettingsResponse] }>()
const loading = ref(false)
const saving = ref(false)
const error = ref('')
const offline = ref(false)
const dnsEnabled = ref(false)
const features = ref<TunFeatures>({})
const capability = ref<NonNullable<NetworkSettingsResponse['tunCapability']>>({ supported: false })
const routeExcludeText = ref('')
const form = reactive<TunForm>(defaultTun())
const operation = useOperationProgress()
let loadRequest = 0
let hasSettings = false

const supported = computed(() => capability.value.supported === true)
const controlsDisabled = computed(() => loading.value || saving.value || !supported.value)
const capabilityText = computed(() => capability.value.message || (supported.value ? '当前环境支持 TUN。' : '当前环境暂不支持 TUN。'))

function applyResult(result: NetworkSettingsResponse) {
  hasSettings = true
  Object.assign(form, defaultTun(), result.settings?.tun || {})
  features.value = result.tunFeatures || {}
  if (result.tunCapability) capability.value = result.tunCapability
  offline.value = result.offline === true
  dnsEnabled.value = result.settings?.dns?.enable === true
  routeExcludeText.value = form.routeExcludeAddress.join('\n')
}

if (props.initialSettings) applyResult(props.initialSettings)

async function load() {
  const request = ++loadRequest
  loading.value = !hasSettings
  error.value = ''
  try {
    const result = await api<NetworkSettingsResponse>('/api/network/settings')
    if (request === loadRequest) applyResult(result)
  } catch (cause) {
    if (request === loadRequest) error.value = errorMessage(cause)
  } finally {
    if (request === loadRequest) loading.value = false
  }
}

function close() {
  if (!saving.value) emit('close')
}

function parseRouteExcludes() {
  return [...new Set(routeExcludeText.value
    .split(/[\n,]+/)
    .map(item => item.trim())
    .filter(Boolean))]
}

async function save() {
  if (!Number.isInteger(form.mtu) || form.mtu < 1280 || form.mtu > 65535) {
    error.value = 'MTU 必须是 1280–65535 之间的整数'
    return
  }
  if (!supported.value) {
    error.value = capabilityText.value
    return
  }
  saving.value = true
  error.value = ''
  form.routeExcludeAddress = parseRouteExcludes()
  if (!form.autoRoute) form.autoRedirect = false
  try {
    const result = await operation.request<NetworkSettingsResponse>(
      '/api/network/settings',
      jsonRequest('PUT', { tun: { ...form, routeExcludeAddress: [...form.routeExcludeAddress] } }),
      '/api/network/settings/status',
      '正在校验并保存 TUN 设置…',
    )
    applyResult(result)
    emit('saved', result)
    notify(result.activation === 'saved-only' && result.activationReason !== 'tun-disabled' ? 'TUN 设置已保存，Core 启动后生效' : form.enabled ? 'TUN 设置已保存并生效' : 'TUN 参数已保存，开启 TUN 后生效')
    emit('close')
  } catch (cause) {
    error.value = errorMessage(cause)
    notify(error.value, true)
  } finally {
    saving.value = false
  }
}

watch(() => props.open, open => {
  if (open) {
    if (props.initialSettings) applyResult(props.initialSettings)
    void load()
  }
  else ++loadRequest
})
</script>

<template>
  <BaseModal :open="open" :title="t('虚拟网卡(TUN)设置')" card-class="tun-settings-modal-card" :closable="!saving" @close="close">
    <template #header>
      <div class="settings-modal-header">
        <div class="settings-modal-heading">
          <h3>{{ t("虚拟网卡(TUN)设置") }}</h3>
          <span v-if="saving" class="tun-settings-save-progress" role="status" aria-live="polite">{{ t(operation.message) }}</span>
        </div>
        <div class="settings-modal-header-actions">
          <button class="ghost" type="button" :disabled="saving" @click="close">{{ t("取消") }}</button>
          <button type="button" :disabled="loading || saving || !supported" @click="save">{{ t(saving ? '保存中…' : '保存') }}</button>
        </div>
      </div>
    </template>
    <div class="tun-settings-modal-content">
      <div v-if="loading" class="tun-settings-modal-loading">{{ t("正在读取 TUN 设置…") }}</div>
      <div v-else class="tun-settings-modal-scroll">
        <div v-if="error" class="local-warning">{{ t(error) }}</div>

        <div class="tun-capability" :class="supported ? 'ok' : 'warn'">
          <strong>{{ t(supported ? (form.enabled ? '已开启' : '已关闭') : '不可用') }}</strong>
          <span>{{ t(capabilityText) }}</span>
        </div>
        <p v-if="!form.enabled && supported" class="tun-settings-disabled-hint">{{ t("当前 TUN 未开启，修改会先保存为预配置；之后开启 TUN 时生效。") }}</p>
        <p v-if="offline" class="tun-settings-disabled-hint">{{ t("Core 已停止；保存后将在下次启动时生效。") }}</p>

        <div class="tun-settings-main-grid">
          <TunStackFields v-model:stack="form.stack" v-model:congestion-controller="form.congestionController" :features="features" field-class="tun-settings-field" :disabled="controlsDisabled" />
          <div class="field tun-settings-field">
            <div class="field-label-row">
              <label>MTU</label>
              <HelpPopover label="MTU"><strong>{{ t("单个网络数据包的最大传输尺寸") }}</strong><span>{{ t("默认值为") }} <b>1500</b>{{ t("。VPN、PPPoE 或多层隧道环境异常时，可尝试") }} <b>1400</b>。</span></HelpPopover>
            </div>
            <input v-model.number="form.mtu" type="number" min="1280" max="65535" :disabled="controlsDisabled" aria-label="TUN MTU">
          </div>
        </div>

        <div class="tun-settings-option-grid">
          <SettingToggle v-model="form.autoRoute" :title="t('自动路由')" :description="t('自动把系统流量路由到 TUN')" :disabled="controlsDisabled" />
          <SettingToggle v-model="form.autoRedirect" title="Auto Redirect" :description="t('Linux 自动配置 nftables/iptables TCP 重定向')" :disabled="controlsDisabled || !form.autoRoute" />
          <SettingToggle v-model="form.autoDetectInterface" :title="t('自动检测出口网卡')" :description="t('自动选择实际的外网出口接口')" :disabled="controlsDisabled" />
          <SettingToggle v-model="form.dnsHijack" :title="t('DNS 劫持')" :description="t('劫持 UDP/TCP 53 到 Mihomo DNS 模块')" :disabled="controlsDisabled" />
          <SettingToggle v-model="form.strictRoute" :title="t('严格路由')" :description="t('减少流量/DNS 泄漏；复杂网络可能影响其他虚拟网卡')" :disabled="controlsDisabled" />
        </div>

        <div class="field tun-settings-route-exclude">
          <div class="field-label-row">
            <label>{{ t("排除自定义网段") }}</label>
            <HelpPopover :label="t('排除自定义网段')"><strong>{{ t("让指定目标网段绕过 TUN 自动路由") }}</strong><span>{{ t("仅在开启“自动路由”时生效。支持 IPv4/IPv6 CIDR，每行填写一个。") }}</span></HelpPopover>
          </div>
          <textarea v-model="routeExcludeText" placeholder="192.168.0.0/16&#10;10.0.0.0/8&#10;fc00::/7" :disabled="controlsDisabled || !form.autoRoute" />
          <span class="field-note">{{ t("每行一个 IPv4/IPv6 CIDR；留空表示不额外排除。") }}</span>
        </div>

        <div v-if="form.enabled && form.dnsHijack && !dnsEnabled" class="tun-capability warn"><strong>DNS</strong><span>{{ t("开启 DNS 劫持前建议先启用 Mihomo DNS。") }}</span></div>
        <div class="tun-note"><strong>{{ t("注意") }}</strong><span>{{ t(runtime.platform === 'docker' ? 'TUN 修改所在网络命名空间的路由。Host 网络下作用于宿主；Bridge 网络下作用于容器。' : 'TUN 会修改 fnOS 的系统路由与 DNS 流向。') }} {{ t('默认关闭；配置不可用时可能影响访问互联网。') }}</span></div>
      </div>
    </div>
  </BaseModal>
</template>
