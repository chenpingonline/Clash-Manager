import { APP_PREFIX } from './api'

export function openStatusStream<T>(path: string, onUpdate: (status: T) => void, onError?: () => void): () => void {
  const source = new EventSource(`${APP_PREFIX}${path}/stream`)
  source.onerror = () => onError?.()
  source.onmessage = event => {
    try { onUpdate(JSON.parse(event.data) as T) }
    catch { /* Ignore one malformed event and keep the operation request authoritative. */ }
  }
  return () => source.close()
}
