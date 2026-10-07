import { configMatchRange, configSearchIndex, configTextDocument, type ConfigSearchIndex, type ConfigTextDocument } from './config-text'
import { configPreview, type ConfigFormat } from './config-preview'

export type ConfigTextRequest = {
  kind: 'document' | 'search' | 'locate'; documentId: number; searchId: number; query: string; content?: string; format?: ConfigFormat; match?: number
}
export type ConfigTextResponse = {
  documentId: number; searchId: number; document?: ConfigTextDocument; search?: ConfigSearchIndex; match?: number; range?: { from: number; to: number } | null
}

let documentId = 0, document: ConfigTextDocument = { lines: [], widthLine: '' }
let search: ConfigSearchIndex = { prefix: [0], count: 0 }
onmessage = (event: MessageEvent<ConfigTextRequest>) => {
  const request = event.data
  if (request.kind === 'document') {
    documentId = request.documentId
    const content = configPreview(request.content || '', request.format || 'original')
    document = configTextDocument(content)
  }
  if (documentId !== request.documentId) return
  if (request.kind === 'locate') {
    postMessage({ documentId, searchId: request.searchId, match: request.match, range: configMatchRange(document, search, request.match || 0, request.query) } satisfies ConfigTextResponse)
    return
  }
  search = configSearchIndex(document.lines, request.query)
  const response: ConfigTextResponse = {
    documentId, searchId: request.searchId,
    ...(request.kind === 'document' ? { document } : {}),
    search,
  }
  postMessage(response)
}
