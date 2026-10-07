import { formatConfigYaml, type ConfigFormat } from './config-preview'

export interface ConfigFormatRequest { id: number; content: string; format: Exclude<ConfigFormat, 'original'> }
export interface ConfigFormatResponse { id: number; content?: string; error?: string }
onmessage = (event: MessageEvent<ConfigFormatRequest>) => {
  const { id, content, format } = event.data
  try { postMessage({ id, content: formatConfigYaml(content, format) } satisfies ConfigFormatResponse) }
  catch (error) { postMessage({ id, error: error instanceof Error ? error.message : '无法调整配置格式' } satisfies ConfigFormatResponse) }
}
