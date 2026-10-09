import fs from 'node:fs'
import path from 'node:path'
import { fileURLToPath } from 'node:url'
import ts from 'typescript'
import { parse as parseSfc } from 'vue/compiler-sfc'
import { parse as parseTemplate } from '@vue/compiler-dom'

const root = fileURLToPath(new URL('../src/', import.meta.url))
const read = name => JSON.parse(fs.readFileSync(path.join(root, 'locales', name), 'utf8'))
const chinese = read('zh-CN.json'), english = read('en-US.json'), slots = read('message-params.json')
const errors = [], keys = Object.keys(chinese)
const parameters = message => [...message.matchAll(/\{arg\d+\}/g)].map(match => match[0]).sort().join(',')
if (keys.sort().join('\n') !== Object.keys(english).sort().join('\n')) errors.push('Chinese and English catalog keys differ.')
for (const key of keys) {
  if (chinese[key] !== key) errors.push(`Chinese source differs: ${key}`)
  if (typeof english[key] !== 'string') errors.push(`Missing English text: ${key}`)
  else {
    if (/[\u3400-\u9fff]/.test(english[key])) errors.push(`Untranslated English text: ${key}`)
    if (parameters(key) !== parameters(english[key])) errors.push(`Parameter mismatch: ${key}`)
    // This suffix is intentionally empty: the English prefix already says "Attempt".
    if (!english[key] && key !== '次尝试') errors.push(`Empty English text: ${key}`)
  }
}
for (const [key, names] of Object.entries(slots)) {
  if (!(key in english)) errors.push(`Unknown parameter message: ${key}`)
  for (const name of names) if (!key.includes(`{${name}}`)) errors.push(`Unknown parameter ${name}: ${key}`)
}
function check(value, file) {
  if (!/[\u3400-\u9fff]/.test(value) || value === '简体中文') return
  // Executable examples are illustrative content; their surrounding help is translated.
  if (value.includes('\n') && /(function main|proxy-groups:|rules:|rule-providers:)/.test(value)) return
  const key = value.trim()
  if (key && !(key in english)) errors.push(`${path.relative(root, file)}: missing message ${key}`)
}
function script(code, file) {
  const source = ts.createSourceFile(file + '.ts', code, ts.ScriptTarget.Latest, true, ts.ScriptKind.TS)
  function walk(node) {
    if (ts.isStringLiteral(node) || ts.isNoSubstitutionTemplateLiteral(node)) check(node.text, file)
    else if (ts.isTemplateExpression(node)) {
      check(node.head.text + node.templateSpans.map((span, index) => `{arg${index}}${span.literal.text}`).join(''), file)
    }
    ts.forEachChild(node, walk)
  }
  walk(source)
}
function template(node, file) {
  if (node.type === 2) check(node.content, file)
  if (node.type === 6 && node.value) check(node.value.content, file)
  if (node.type === 5) script(node.content.content, file)
  if (node.type === 7 && node.exp) script(node.exp.content, file)
  for (const prop of node.props || []) template(prop, file)
  for (const child of node.children || []) template(child, file)
}
function visit(dir) {
  for (const item of fs.readdirSync(dir, { withFileTypes: true })) {
    const file = path.join(dir, item.name)
    if (item.isDirectory()) visit(file)
    else if (item.name.endsWith('.vue')) {
      const { descriptor } = parseSfc(fs.readFileSync(file, 'utf8'))
      if (descriptor.scriptSetup) script(descriptor.scriptSetup.content, file)
      if (descriptor.script) script(descriptor.script.content, file)
      if (descriptor.template) template(parseTemplate(descriptor.template.content), file)
    } else if (item.name.endsWith('.ts') && !item.name.endsWith('.test.ts')) script(fs.readFileSync(file, 'utf8'), file)
  }
}
visit(root)
if (errors.length) { console.error([...new Set(errors)].join('\n')); process.exit(1) }
console.log(`i18n: ${keys.length} messages; catalog parity, parameters and frontend source coverage passed.`)
