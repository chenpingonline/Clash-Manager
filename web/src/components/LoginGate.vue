<script setup lang="ts">
import { onBeforeUnmount, onMounted, ref } from 'vue'
import App from '@/App.vue'
import { api, errorMessage, jsonRequest } from '@/services/api'
import { appDisplayName, loadRuntime } from '@/services/runtime'

const ready = ref(false), loading = ref(true), busy = ref(false), required = ref(false), error = ref('')
const username = ref('admin'), password = ref('')
async function initialize() {
  loading.value = true; error.value = ''
  try {
    const session = await api<{ required: boolean; authenticated: boolean }>('/api/auth/session')
    required.value = session.required
    if (session.authenticated) { await loadRuntime(); ready.value = true }
  } catch (cause) { error.value = errorMessage(cause) }
  finally { loading.value = false }
}
async function login() {
  if (busy.value) return
  busy.value = true; error.value = ''
  try {
    await api('/api/auth/login', jsonRequest('POST', { username: username.value, password: password.value }))
    password.value = ''; await loadRuntime(); ready.value = true
  } catch (cause) { error.value = errorMessage(cause) }
  finally { busy.value = false }
}
function expired(event: Event) {
  const wasReady = ready.value
  ready.value = false; required.value = true
  if (event instanceof CustomEvent && event.detail?.loggedOut) error.value = ''
  else if (wasReady) error.value = '登录已过期，请重新登录'
}
onMounted(() => { window.addEventListener('clash-auth-expired', expired); void initialize() })
onBeforeUnmount(() => window.removeEventListener('clash-auth-expired', expired))
</script>

<template>
  <App v-if="ready" />
  <div v-else class="manager-login">
    <form class="manager-login-card" @submit.prevent="login">
      <h1>{{ appDisplayName }}</h1>
      <p class="muted">{{ loading ? '正在连接…' : '登录后管理订阅、代理与网络设置' }}</p>
      <template v-if="!loading && required">
        <label for="manager-username">用户名</label>
        <input id="manager-username" v-model="username" autocomplete="username" required :disabled="busy" />
        <label for="manager-password">密码</label>
        <input id="manager-password" v-model="password" type="password" autocomplete="current-password" required :disabled="busy" />
        <button type="submit" :disabled="busy">{{ busy ? '正在登录…' : '登录' }}</button>
      </template>
      <p v-if="error" class="warn-text" role="alert">{{ error }}</p>
      <button v-if="!loading && !required" type="button" @click="initialize">重新连接</button>
    </form>
  </div>
</template>

<style scoped>
.manager-login { grid-column:1 / -1; min-width:0; width:100%; min-height:100vh; min-height:100dvh; display:grid; place-items:center; padding:24px; background:var(--bg); }
.manager-login-card { width:min(100%,360px); display:flex; flex-direction:column; gap:12px; padding:28px; border:1px solid var(--line); border-radius:16px; background:var(--panel); }
h1 { margin:0; font-size:22px; }
p { margin:0; font-size:13px; line-height:1.6; }
label { font-size:13px; }
input { width:100%; }
button { margin-top:8px; }
</style>
