// 面向用户文案的中文化护栏：扫描 src 下 tsx/ts，拦截"会显示给用户的纯英文文案"。
// 仅检查 antd 文案类 props 的字符串字面量取值，不解析 JSX 文本节点（正则无法可靠区分代码与文本，误报过多）。
// 命中条件：DISPLAY_PROPS 中的 prop 取值为纯拉丁字符短语（含空格、不含中文/假名/韩文）、且不在白名单内。
// 不报：代码注释、console.*、throw new Error、import 路径、CSS 类名、变量/类型名、
//       后端枚举 key、比较用的字面量、表达式 {} 形式（静态扫描不展开）、白名单内的专有名词/缩写。
// 维护说明：误报时优先把词加进 ALLOW_PHRASES；新增展示型 props 时补进 DISPLAY_PROPS。

import { readdir, readFile } from 'node:fs/promises'
import { dirname, join, relative } from 'node:path'
import { fileURLToPath } from 'node:url'

const ROOT = join(dirname(fileURLToPath(import.meta.url)), '..', 'src')

const ALLOW_PHRASES = new Set([
  // 通用缩写 / 专有名词 / 系统名
  'HR', 'OKR', 'KPI', 'OA', 'IP', 'ID', 'IDs', 'URL', 'URI', 'API', 'APIs',
  'Excel', 'CSV', 'JSON', 'XML', 'PDF', 'PNG', 'JPG', 'JPEG', 'SQL',
  'TODO', 'FIXME', 'Q1', 'Q2', 'Q3', 'Q4', 'H1', 'H2',
  'ManualLeave', 'dingtalk', 'DingTalk', 'github', 'GitHub', 'git',
  'token', 'Token', 'org', 'Org', 'org_id', 'user_id',
  // Excel/HTML 导出里的 MIME / meta 值，非用户文案
  'text/html; charset=utf-8', 'text/csv; charset=utf-8', 'charset=utf-8',
])

// 这些 props 的值会直接渲染给用户看，纯英文即报错。
const DISPLAY_PROPS = new Set([
  'placeholder', 'title', 'subtitle', 'subTitle', 'okText', 'cancelText',
  'label', 'description', 'content', 'message', 'tooltip', 'extra',
  'summary', 'aria-label', 'emptyText',
])

const isCjk = (s) => /[一-鿿぀-ヿ가-힯]/.test(s)

// "纯英文短语"判定：含拉丁字母、不含 CJK、带空格（排除单 token 的枚举 key/字段名/变量名误报）、不在白名单。
function isUserFacingEnglish(text) {
  const trimmed = text.trim()
  if (!trimmed) return false
  if (isCjk(trimmed)) return false
  if (!/[A-Za-z]/.test(trimmed)) return false
  if (!/\s/.test(trimmed)) return false // 只报"带空格的英文短语"，单 token 不报，大幅降误报
  if (ALLOW_PHRASES.has(trimmed)) return false
  return true
}

async function* walk(dir) {
  const entries = await readdir(dir, { withFileTypes: true })
  for (const e of entries) {
    const full = join(dir, e.name)
    if (e.isDirectory()) {
      yield* walk(full)
    } else if (/\.[tj]sx?$/.test(e.name) && !/\.(test|spec)\.[tj]sx?$/.test(e.name)) {
      yield full
    }
  }
}

// 匹配 prop="..." / prop='...' 形式的展示型属性（不匹配反引号模板字符串，因其可能含插值）。
const PROP_RE = new RegExp(
  `\\b(${[...DISPLAY_PROPS].join('|')})\\s*=\\s*(?:"([^"]*)"|'([^']*)')`,
  'g',
)

async function scanFile(file) {
  const src = await readFile(file, 'utf8')
  // 去掉注释，降低噪声
  const code = src.replace(/^\s*\/\/.*$/gm, '').replace(/\/\*[\s\S]*?\*\//g, '')
  const findings = []

  for (const m of code.matchAll(PROP_RE)) {
    const value = m[2] ?? m[3] ?? ''
    if (isUserFacingEnglish(value)) {
      findings.push({ kind: `prop:${m[1]}`, value })
    }
  }
  return findings
}

const files = []
for await (const f of walk(ROOT)) files.push(f)

let total = 0
for (const f of files) {
  const findings = await scanFile(f)
  if (!findings.length) continue
  const rel = relative(ROOT, f).replace(/\\/g, '/')
  for (const fnd of findings) {
    total++
    console.log(`✗ ${rel}  [${fnd.kind}]  ${fnd.value}`)
  }
}

if (total > 0) {
  console.log(`\n发现 ${total} 处疑似面向用户的英文文案。`)
  console.log('若为误报（专有名词/缩写/后端枚举），请加进 scripts/check-user-facing-en.mjs 的 ALLOW_PHRASES。')
  console.log('若为真实用户可见文案，请改为中文。')
  process.exit(1)
}
console.log('✓ 未发现面向用户的纯英文文案。')
