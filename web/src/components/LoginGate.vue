<script setup lang="ts">
import { t } from '@/services/i18n'

import { onBeforeUnmount, onMounted } from 'vue'
import App from '@/App.vue'
import { appDisplayName } from '@/services/runtime'
import { useStartupSession } from '@/composables/useStartupSession'

const { ready, loading, busy, required, error, username, password, initialize, login, expire, dispose } = useStartupSession()
function expired(event: Event) {
  expire(event instanceof CustomEvent && Boolean(event.detail?.loggedOut))
}
onMounted(() => { window.addEventListener('clash-auth-expired', expired); void initialize() })
onBeforeUnmount(() => { dispose(); window.removeEventListener('clash-auth-expired', expired) })
</script>

<template>
  <App v-if="ready" />
  <div v-else class="manager-login">
    <form class="manager-login-card" @submit.prevent="login">
      <h1>{{ t(appDisplayName) }}</h1>
      <p class="muted">{{ t(loading ? '正在连接…' : error && !required ? '连接失败，请检查服务状态后重试' : '登录后管理订阅、代理与网络设置') }}</p>
      <template v-if="!loading && required">
        <label for="manager-username">{{ t("用户名") }}</label>
        <input id="manager-username" v-model="username" autocomplete="username" required :disabled="busy" />
        <label for="manager-password">{{ t("密码") }}</label>
        <input id="manager-password" v-model="password" type="password" autocomplete="current-password" required :disabled="busy" />
        <button type="submit" :disabled="busy">{{ t(busy ? '正在登录…' : '登录') }}</button>
      </template>
      <p v-if="error" class="warn-text" role="alert">{{ t(error) }}</p>
      <button v-if="!loading && (!required || error)" type="button" :disabled="busy" @click="initialize">{{ t("重新连接") }}</button>
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
