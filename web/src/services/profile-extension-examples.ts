import type { ProfileExtensionKind } from '@/types/api'

export interface ExtensionExample { title: string; language: 'YAML' | 'JavaScript'; description: string; content: string; notes: string[] }
export const extensionExamples: Record<ProfileExtensionKind, ExtensionExample> = {
  rules: {
    title: '规则增强示例', language: 'YAML',
    description: '高级模式填写完整规则：类型,内容,目标策略。可视化模式的规则内容只填写域名、网段或规则集名称。',
    content: `prepend:
  - DOMAIN-SUFFIX,example.com,DIRECT

append: []

delete:
  - RULE-SET,geosite-netflix,Netflix`,
    notes: ['prepend 优先匹配；append 放在订阅规则之后，前面已有 MATCH 时不会匹配后置规则。', 'delete 填订阅原始规则的全文，类型、内容和目标策略必须一致。示例中的规则集和策略须在订阅中存在。', '[] 表示空列表。有条目时使用示例中的缩进列表。'],
  },
  proxies: {
    title: '节点增强示例', language: 'YAML',
    description: '高级模式填写节点对象。可视化模式可按行粘贴节点 URI 或 Base64 节点列表。',
    content: `prepend:
  - name: 自定义节点
    type: trojan
    server: node.example.com
    port: 443
    password: 请替换为节点密码
    sni: node.example.com
    udp: true

append: []

delete:
  - 订阅中要排除的节点名称`,
    notes: ['prepend / append 分别放在原始节点列表前 / 后；delete 填节点名称，不填节点链接。', '请替换示例服务器、密码与名称，节点名称不能与现有节点、代理组或内置策略重复。', '新增节点会加入首个手动选择代理组；排除原始节点时，会同时移除代理组中的对应节点引用。'],
  },
  groups: {
    title: '代理组增强示例', language: 'YAML',
    description: 'proxies 引用节点或代理组，use 引用代理集合。',
    content: `prepend:
  - name: 自定义代理组
    type: select
    proxies:
      - DIRECT
      - REJECT

append: []

delete:
  - 订阅中要排除的代理组名称`,
    notes: ['支持 select、url-test、fallback、load-balance。', 'delete 填原始组名；组名须唯一，引用须存在且不能引用自身。', '排除代理组后，请同步调整规则和其他代理组中的引用。'],
  },
  override: {
    title: '扩展覆写配置示例', language: 'YAML',
    description: '填写要覆盖的配置项即可，无需复制完整订阅配置。',
    content: `# 对象递归合并；数组整体替换
allow-lan: true
unified-delay: true

profile:
  store-selected: true`,
    notes: ['覆写对象会递归合并，未填写的字段保留原值；数组会整体替换，不会自动追加。', '增删规则、节点或代理组，优先使用对应增强编辑器的 prepend / append / delete。', '最终配置还会应用已保存的 Core 用户设置及启用的 DNS / Hosts 覆盖；重复设置同一项时，以后续处理结果为准。'],
  },
  script: {
    title: '扩展脚本示例', language: 'JavaScript',
    description: '定义 main(config, profileName)，接收配置对象与订阅名称，并返回配置对象。',
    content: `function main(config, profileName) {
  const rule = "DOMAIN-SUFFIX,example.com,DIRECT";
  const rules = config.rules || [];
  config.rules = [rule, ...rules.filter(item => item !== rule)];
  return config;
}`,
    notes: ['示例将直连规则放到最前面，并移除相同规则的重复项；请替换 example.com。', '必须返回配置对象，不使用 module.exports 或 export default；也可以根据 profileName 区分订阅。', '脚本在 JavaScript 沙箱中执行，不提供浏览器或 Node.js API；执行时间上限为 5 秒。'],
  },
}
