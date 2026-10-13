import { TrimApp } from '@trimjs/web-app'
import { onBeforeUnmount, onMounted } from 'vue'

import { runtime } from '@/services/runtime'

type Theme = 'dark' | 'light'

function systemTheme(): Theme {
  return matchMedia('(prefers-color-scheme: dark)').matches ? 'dark' : 'light'
}

function normalizeTheme(value: unknown): Theme | null {
  if (value === 'dark' || value === 'light') return value
  if (!value || typeof value !== 'object') return null

  const nested = (value as { theme?: unknown }).theme
  if (nested === 'dark' || nested === 'light') return nested
  if (nested && typeof nested === 'object') {
    const theme = (nested as { theme?: unknown }).theme
    if (theme === 'dark' || theme === 'light') return theme
  }
  return null
}

function applyTheme(theme: Theme, mode: Theme | 'system'): void {
  const root = document.documentElement
  root.dataset.themeMode = 'system'
  root.dataset.fnosThemeMode = mode
  root.dataset.theme = theme
  root.style.colorScheme = theme
}

export function useFnosTheme(): void {
  const media = matchMedia('(prefers-color-scheme: dark)')
  const sdk = runtime.value.platform === 'fnos' ? new TrimApp() : null
  let disposed = false
  let hostThemeActive = false
  let subscribed = false
  // Preserve the first-paint cache while the host answers; it is not an override.
  let hostThemePending = Boolean(sdk && !sdk.isStandaloneWeb
    && document.documentElement.dataset.bootThemeSource === 'host-cache')

  const applyHostTheme = (value: unknown) => {
    if (disposed) return
    const theme = normalizeTheme(value)
    if (!theme) return
    hostThemeActive = true
    applyTheme(theme, theme)
    if (sdk && !sdk.isStandaloneWeb) {
      try { localStorage.setItem('clash-fnos-host-theme', theme) } catch { /* Optional first-paint cache. */ }
    }
  }
  const applySystemTheme = () => {
    if (!hostThemeActive && !hostThemePending && !disposed) applyTheme(systemTheme(), 'system')
  }

  onMounted(() => {
    applySystemTheme()
    media.addEventListener('change', applySystemTheme)

    if (!sdk) return
    void sdk.getPlatformConfig()
      .then(async (config) => {
        if (disposed) return
        hostThemePending = false
        applyHostTheme(config.theme)
        applySystemTheme()
        if (sdk.isWeb && !sdk.isStandaloneWeb) {
          await sdk.$on('os/theme', applyHostTheme)
          if (disposed) {
            await sdk.$off('os/theme', applyHostTheme)
            return
          }
          subscribed = true
        }
      })
      .catch(() => {
        // Standalone browsers and older fnOS hosts keep using the media-query fallback.
        hostThemePending = false
        applySystemTheme()
      })
  })

  onBeforeUnmount(() => {
    disposed = true
    media.removeEventListener('change', applySystemTheme)
    if (subscribed && sdk) void sdk.$off('os/theme', applyHostTheme).catch(() => undefined)
  })
}
