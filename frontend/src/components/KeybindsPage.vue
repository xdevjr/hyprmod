<script setup lang="ts">
import { computed, ref, watch } from 'vue'
import { useI18n } from '../i18n'
import { AlertTriangle, Eraser, Keyboard, Pencil, Save, Search, Trash2 } from '@lucide/vue'
import * as Api from '../../bindings/github.com/juniorgs/hyprmod/internal/api/service'
import type { Schema } from '../../bindings/github.com/juniorgs/hyprmod/internal/schema/models'
import type { Document } from '../../bindings/github.com/juniorgs/hyprmod/internal/config/models'
import { useKeyCapture, MOUSE_CODE_REFERENCE } from '../lib/keycapture'
import DispatcherMenu from './DispatcherMenu.vue'

const { t } = useI18n()

const props = defineProps<{ schema: Schema | null; document: Document | null }>()
const emit = defineEmits<{ saved: [message?: string]; error: [message: string] }>()
const query = ref('')
// The form starts empty: the user either captures a shortcut or writes the
// whole declaration in the Lua block below.
const keys = ref('')
const dispatcher = ref('')
const submitting = ref(false)

const code = ref('')
const codeTouched = ref(false)
const editing = ref<string | null>(null)

const { start: startCapture, stop: stopCapture, active: capturing, preview } = useKeyCapture(combo => {  keys.value = combo
  codeTouched.value = false
})

const capturedTokens = computed(() => {
  const tokens = new Set<string>()
  for (const token of keys.value.split('+')) {
    const clean = token.trim().toUpperCase()
    if (clean) tokens.add(clean)
  }
  return tokens
})

const binds = computed(() => props.document?.binds ?? [])
const conflicts = computed(() => {
  const counts = new Map<string, number>()
  for (const bind of binds.value) counts.set(bind.keys.toLowerCase(), (counts.get(bind.keys.toLowerCase()) ?? 0) + 1)
  return counts
})
const filtered = computed(() => {
  const q = query.value.trim().toLowerCase()
  return binds.value.filter(bind => !q || `${bind.keys} ${bind.dispatcher.name} ${bind.file}`.toLowerCase().includes(q))
})
const dispatcherList = computed(() => props.schema?.Dispatchers ?? [])

const keyRows = [
  ['ESC', '1', '2', '3', '4', '5', '6', '7', '8', '9', '0', '-', '=', 'BACK'],
  ['TAB', 'Q', 'W', 'E', 'R', 'T', 'Y', 'U', 'I', 'O', 'P', '[', ']', '\\'],
  ['CAPS', 'A', 'S', 'D', 'F', 'G', 'H', 'J', 'K', 'L', ';', "'", 'ENTER'],
  ['SHIFT', 'Z', 'X', 'C', 'V', 'B', 'N', 'M', ',', '.', '/', 'SHIFT'],
  ['CTRL', 'SUPER', 'ALT', 'SPACE', 'ALT', 'SUPER', 'CTRL'],
]
const activeKeyNames = computed(() => {
  const names = new Set<string>()
  for (const bind of binds.value) {
    if (!bind.enabled) continue
    const split = bind.keys.split('+').map(token => token.trim().toUpperCase())
    for (const token of split) names.add(token.replace(/^CONTROL$/, 'CTRL').replace(/^MOD4$/, 'SUPER'))
  }
  // Keys typed into the capture field are highlighted too, so the user can see
  // where the shortcut lands on a real keyboard.
  for (const token of capturedTokens.value) {
    names.add(token.toUpperCase().replace(/^CONTROL$/, 'CTRL').replace(/^META$/, 'SUPER'))
  }
  return names
})

const conflictingBind = computed(() => {
  const target = normalizeCombo(keys.value)
  if (!target) return undefined
  return binds.value.find(bind => normalizeCombo(bind.keys) === target)
})

const usedCombos = computed(() => {
  const seen = new Set<string>()
  return binds.value.filter(bind => {
    const normalized = normalizeCombo(bind.keys)
    if (!normalized || seen.has(normalized)) return false
    seen.add(normalized)
    return true
  })
})

function normalizeCombo(combo: string): string {
  return combo.split('+').map(token => token.trim().toUpperCase()).filter(Boolean).sort().join('+')
}

// The dispatcher arguments and flags are authored as JSON, but the code block
// below is the authoritative representation the user may hand-edit.
function toLuaLiteral(value: unknown): string {
  if (typeof value === 'string') return JSON.stringify(value)
  if (typeof value === 'boolean' || typeof value === 'number') return String(value)
  if (value === null || value === undefined) return 'nil'
  if (Array.isArray(value)) return `{ ${value.map(toLuaLiteral).join(', ')} }`
  if (typeof value === 'object') {
    const entries = Object.entries(value as Record<string, unknown>)
    return `{ ${entries.map(([key, item]) => `${luaKey(key)} = ${toLuaLiteral(item)}`).join(', ')} }`
  }
  return 'nil'
}

const LUA_RESERVED = new Set(['and', 'break', 'do', 'else', 'elseif', 'end', 'false', 'for', 'function', 'goto', 'if', 'in', 'local', 'nil', 'not', 'or', 'repeat', 'return', 'then', 'true', 'until', 'while'])

function luaKey(key: string): string {
  if (!/^[A-Za-z_]\w*$/.test(key) || LUA_RESERVED.has(key)) return `[${JSON.stringify(key)}]`
  return key
}

// The Lua block is the single source of truth: dispatcher arguments and bind
// flags are authored there, so the form only keeps the keys and the dispatcher
// in sync with it instead of duplicating them in JSON fields.
function rebuild(): string {
  if (!keys.value.trim() || !dispatcher.value) return ''
  return `hl.bind(${JSON.stringify(keys.value.trim())}, hl.dsp.${dispatcher.value}())`
}

const firstStringArg = /"(?:[^"\\]|\\.)*"/
const dispatcherRef = /hl\.dsp\.[A-Za-z0-9_.]*/

function patchFirstString(codeText: string, value: string): string | null {
  if (!firstStringArg.test(codeText)) return null
  return codeText.replace(firstStringArg, JSON.stringify(value))
}

function patchDispatcher(codeText: string, name: string): string | null {
  if (!dispatcherRef.test(codeText)) return null
  return codeText.replace(dispatcherRef, `hl.dsp.${name}`)
}

function syncCode() {
  if (!codeTouched.value) {
    code.value = rebuild()
    return
  }
  const withKeys = keys.value.trim() ? patchFirstString(code.value, keys.value.trim()) : null
  const base = withKeys ?? code.value
  const withDispatcher = dispatcher.value ? patchDispatcher(base, dispatcher.value) : null
  code.value = (withDispatcher ?? base).trim()
}

watch([keys, dispatcher], syncCode)

// Reloading a bind from the list restores its original arguments and flags.
// A dispatcher that is an inline Lua callback is carried over verbatim.
function buildFromBind(bind: { keys: string; dispatcher: { name: string; args?: unknown[] | null; callback?: string | null }; opts?: Record<string, unknown> | null }): string {
  const args = Array.isArray(bind.dispatcher.args) ? bind.dispatcher.args : []
  const flags = bind.opts && !Array.isArray(bind.opts) ? bind.opts : {}
  const target = bind.dispatcher.callback
    ? bind.dispatcher.callback
    : `hl.dsp.${bind.dispatcher.name}(${args.map(toLuaLiteral).join(', ')})`
  const parts = [JSON.stringify(bind.keys), target]
  if (Object.keys(flags).length) parts.push(toLuaLiteral(flags))
  return `hl.bind(${parts.join(', ')})`
}

// X11 keycodes are valid Hyprland syntax, but "code:82" means nothing to a
// reader, so the common keypad block is labelled for the UI only.
const KEYCODE_LABELS: Record<string, string> = {
  '77': 'num 7 / home', '78': 'num 8 / ↑', '79': 'num 9 / pgup',
  '81': 'num 4 / ←', '82': 'num 5 / num -', '83': 'num 6 / →',
  '84': 'num 1 / end', '85': 'num 2 / ↓', '86': 'num 3 / pgdn',
  '87': 'num 0 / ins', '88': 'num . / del', '89': 'F11', '90': 'F12',
}

function keycodeHint(keys: string): string | undefined {
  const match = keys.match(/code:(\d+)/)
  if (!match) return undefined
  return KEYCODE_LABELS[match[1]] ? `keycode ${match[1]} = ${KEYCODE_LABELS[match[1]]}` : `keycode ${match[1]}`
}

const editedKeysDynamic = ref(false)
const formRef = ref<HTMLElement | null>(null)

// Codes already present in the user's own config are listed first, since those
// are the ones their mouse actually reports to Hyprland.
const mouseCodeChips = computed(() => {
  const used = new Set<string>()
  for (const bind of binds.value) {
    for (const match of bind.keys.matchAll(/mouse:\d+/g)) used.add(match[0])
  }
  const rest = MOUSE_CODE_REFERENCE.filter(item => !used.has(item.code))
  return [...MOUSE_CODE_REFERENCE.filter(item => used.has(item.code)), ...rest]
})

function addToken(token: string) {
  const current = keys.value.trim()
  keys.value = current && !current.includes(token) ? `${current} + ${token}` : current || token
}

function scrollToForm() {
  requestAnimationFrame(() => {
    formRef.value?.scrollIntoView({ behavior: 'smooth', block: 'start' })
  })
}

function resetForm() {
  keys.value = ''
  dispatcher.value = ''
  code.value = ''
  codeTouched.value = false
  editedKeysDynamic.value = false
  editing.value = null
}

function loadBind(bind: { keys: string; keysDynamic?: boolean; dispatcher: { name: string; args?: unknown[] | null; callback?: string | null }; opts?: Record<string, unknown> | null }) {
  keys.value = bind.keys
  dispatcher.value = bind.dispatcher.callback ? '' : bind.dispatcher.name
  // Keep the original declaration verbatim: its arguments and flags were
  // authored in the Lua block, not in the form.
  code.value = buildFromBind(bind)
  codeTouched.value = true
  editing.value = normalizeCombo(bind.keys)
  editedKeysDynamic.value = bind.keysDynamic === true
  query.value = ''
  scrollToForm()
}

// The keys may live only inside the hand-written block, so fall back to the
// first string literal of the declaration before saving.
function extractKeys(codeText: string): string {
  const match = codeText.match(firstStringArg)
  if (!match) return ''
  try {
    return String(JSON.parse(match[0]))
  } catch {
    return match[0].slice(1, -1)
  }
}

async function submitBind() {
  submitting.value = true
  try {
    const payload = code.value.trim() || rebuild()
    if (!payload) throw new Error(t('keybinds.codeRequired'))
    const effectiveKeys = keys.value.trim() || extractKeys(payload)
    if (!effectiveKeys) throw new Error(t('keybinds.codeRequired'))
    await Api.SetBind(effectiveKeys, payload)
    emit('saved', t(editing.value ? 'keybinds.updated' : 'keybinds.added', { keys: effectiveKeys }))
    resetForm()
  } catch (err) {
    emit('error', `${t('common.error')}: ${String(err)}`)
  } finally {
    submitting.value = false
  }
}

async function addBind() {
  await submitBind()
}

async function removeBind(key: string) {
  if (!window.confirm(t('keybinds.confirmUnbind', { keys: key }))) return
  try {
    await Api.RemoveBind(key)
    emit('saved', t('keybinds.unbound', { keys: key }))
  } catch (err) {
    emit('error', String(err))
  }
}
</script>

<template>
  <div class="keybind-layout">
    <section class="card keymap-card">
      <div class="card-header">
        <div><div class="section-kicker">{{ t('keybinds.keyMap') }}</div><h3 class="card-title">{{ t('keybinds.density') }}</h3><p class="card-subtitle">{{ t('keybinds.densitySubtitle') }}</p></div>
        <span class="tag"><Keyboard :size="11" /> {{ t('keybinds.binds', { count: binds.length }) }}</span>
      </div>
      <div class="keymap">
        <div v-for="(row, index) in keyRows" :key="index" class="key-row">
          <span v-for="(key, keyIndex) in row" :key="`${key}-${keyIndex}`" class="keycap" :class="{ active: activeKeyNames.has(key), wide: key.length > 1 }">{{ key }}</span>
        </div>
      </div>
      <div class="keymap-foot"><span class="legend-key"></span> {{ t('keybinds.configuredKey') }} <span class="legend-empty"></span> {{ t('keybinds.unused') }}</div>
    </section>

    <section class="card add-bind-card">
      <div class="card-header">
        <div><div class="section-kicker">{{ t('keybinds.newDeclaration') }}</div><h3 class="card-title">{{ t('keybinds.addBind') }}</h3><p class="card-subtitle">{{ t('keybinds.addBindSubtitle') }}</p></div>
        <span class="tag lime">{{ t('keybinds.safeOverride') }}</span>
      </div>
      <form ref="formRef" class="bind-form" @submit.prevent="submitBind">
        <label class="form-label capture-field">
          {{ t('keybinds.keys') }}
          <div class="capture-row">
            <input v-model="keys" class="form-input mono" placeholder="SUPER + Q" :class="{ capturing }" />
            <button type="button" class="button-secondary capture-button" :class="{ active: capturing }" @click="capturing ? stopCapture() : startCapture(keys)">
              <Keyboard :size="13" /> {{ capturing ? t('keybinds.captureStop') : t('keybinds.capture') }}
            </button>
            <button type="button" class="button-quiet clear-button" :title="t('keybinds.clearField')" @click="resetForm">
              <Eraser :size="13" />
            </button>
          </div>
          <span v-if="editedKeysDynamic" class="capture-hint dynamic">
            <AlertTriangle :size="11" /> {{ t('keybinds.dynamicKeys') }}
          </span>
          <div v-if="capturing" class="mouse-codes">
            <span class="mouse-codes-label">{{ t('keybinds.mouseCodes') }}</span>
            <button
              v-for="item in mouseCodeChips"
              :key="item.code"
              type="button"
              class="mouse-chip"
              :class="{ used: binds.some(bind => bind.keys.includes(item.code)) }"
              @click="addToken(item.code)"
            >
              {{ item.code }}<em>{{ t(`keybinds.mouse_${item.label}`) }}</em>
            </button>
          </div>
          <span v-if="capturing" class="capture-hint">
            <span class="capture-live">{{ preview || t('keybinds.capturePrompt') }}</span>
            <em>{{ t('keybinds.captureHint') }}</em>
          </span>
          <span v-else-if="editing" class="capture-hint editing">
            <Pencil :size="11" /> {{ t('keybinds.editing', { keys: keys }) }}
          </span>
          <span v-else-if="conflictingBind" class="capture-hint conflict">
            <AlertTriangle :size="11" />
            <span>{{ t('keybinds.conflict', { dispatcher: conflictingBind.dispatcher.name }) }}</span>
          </span>
          <div v-if="capturing && usedCombos.length" class="used-strip">
            <span class="used-strip-label">{{ t('keybinds.takenByHypr') }}</span>
            <button v-for="(bind, index) in usedCombos.slice(0, 8)" :key="`${bind.file}-${bind.line}-${index}`" class="used-chip" type="button" @click="keys = bind.keys">
              {{ bind.keys }}
            </button>
            <span v-if="usedCombos.length > 8" class="used-more">+{{ usedCombos.length - 8 }}</span>
          </div>
        </label>
        <label class="form-label">{{ t('keybinds.dispatcher') }}<DispatcherMenu v-model="dispatcher" :options="dispatcherList" :placeholder="t('keybinds.pickDispatcher')" /></label>
        <label class="form-label code-field">
          <span class="code-field-head">
            {{ t('keybinds.code') }}
            <span v-if="codeTouched" class="tag amber">{{ t('keybinds.codeEdited') }}</span>
            <span v-else class="code-generated">{{ t('keybinds.codeGenerated') }}</span>
          </span>
          <textarea v-model="code" class="form-textarea code-block" spellcheck="false" :placeholder="t('keybinds.codePlaceholder')" @input="codeTouched = true"></textarea>
          <em class="code-note">{{ t('keybinds.codeNote') }}</em>
        </label>
        <div class="bind-submit-row">
          <button class="button-primary" type="submit" :disabled="submitting || !keys.trim() || editedKeysDynamic">
            <Save :size="14" /> {{ editing ? t('keybinds.saveEdit') : t('keybinds.adding') }}
          </button>
          <button v-if="editing" type="button" class="button-quiet" @click="resetForm">{{ t('keybinds.cancelEdit') }}</button>
        </div>
      </form>
    </section>

    <section class="card bind-list-card">
      <div class="card-header bind-list-header">
        <div><div class="section-kicker">{{ t('keybinds.inventory') }}</div><h3 class="card-title">{{ t('keybinds.addBind') }}</h3></div>
        <div class="bind-search"><Search :size="13" /><input v-model="query" :placeholder="t('keybinds.search')" /></div>
      </div>
      <div class="bind-table-head"><span>{{ t('keybinds.key') }}</span><span>{{ t('keybinds.dispatcher') }}</span><span>{{ t('keybinds.optionsCol') }}</span><span>{{ t('keybinds.source') }}</span><span></span></div>
      <div v-if="!filtered.length" class="empty-state">{{ t('keybinds.noMatch') }}</div>
      <div v-for="(bind, index) in filtered" :key="`${bind.file}:${bind.line}:${index}`" class="bind-row" :class="{ disabled: !bind.enabled, editing: editing === normalizeCombo(bind.keys) }" role="button" tabindex="0" @click="loadBind(bind)" @keydown.enter="loadBind(bind)">
        <div class="bind-key">
          <code>{{ bind.keys }}</code>
          <AlertTriangle v-if="(conflicts.get(bind.keys.toLowerCase()) ?? 0) > 1" :size="12" />
          <Pencil v-if="editing === normalizeCombo(bind.keys)" :size="11" class="edit-marker" />
          <small v-if="keycodeHint(bind.keys)" class="keycode-hint">{{ keycodeHint(bind.keys) }}</small>
        </div>
        <div class="bind-dispatch">
          <template v-if="bind.dispatcher.callback">
            <code class="callback-name">callback</code>
            <small class="callback-body">{{ bind.dispatcher.callback.replace(/\s+/g, ' ') }}</small>
          </template>
          <template v-else>
            <code>{{ bind.dispatcher.name }}</code>
            <small v-if="bind.dispatcher.args?.length">{{ JSON.stringify(bind.dispatcher.args) }}</small>
          </template>
        </div>
        <div class="bind-options"><span v-for="flag in Object.keys(bind.opts ?? {})" :key="flag" class="tag">{{ flag }}</span><span v-if="!Object.keys(bind.opts ?? {}).length" class="muted">--</span></div>
        <div class="bind-source"><span>{{ bind.file.split('/').slice(-2).join('/') }}:{{ bind.line }}</span><span v-if="bind.keysDynamic" class="tag amber">{{ t('keybinds.dynamic') }}</span><span v-else-if="!bind.editable" class="tag amber">{{ t('common.dynamic') }}</span></div>
        <button class="row-icon-button" :title="t('keybinds.confirmUnbind', { keys: bind.keys })" @click.stop="removeBind(bind.keys)"><Trash2 :size="13" /></button>
      </div>
    </section>
  </div>
</template>

<style scoped>
.keybind-layout { display: flex; flex-direction: column; gap: 13px; }
.keymap-card { overflow: hidden; background: radial-gradient(ellipse at 48% 95%, #2d3a2366, transparent 42%), var(--panel); }
.keymap { display: flex; flex-direction: column; gap: 5px; padding: 6px 18px 12px; }
.key-row { display: flex; justify-content: center; gap: 5px; }
.keycap { min-width: 31px; height: 26px; display: grid; place-items: center; padding: 0 5px; border: 1px solid #3a3f49; border-bottom-width: 3px; border-radius: 5px; background: #242832; color: #858c99; font: 7.5px var(--mono); transition: .15s; }
.keycap.wide { min-width: 47px; }
.keycap.active { border-color: #6c8549; border-bottom-color: #8caa5f; background: #344128; color: #d4ecae; box-shadow: 0 0 13px #c4f27412; }
.keymap-foot { display: flex; align-items: center; justify-content: center; gap: 6px; padding: 0 16px 14px; color: #737a87; font-size: 8px; }
.legend-key, .legend-empty { width: 8px; height: 8px; border: 1px solid #6c8549; border-radius: 2px; background: #344128; }
.legend-empty { margin-left: 9px; border-color: #3a3f49; background: #242832; }
.add-bind-card { padding-bottom: 14px; }
.bind-form { display: grid; grid-template-columns: 1fr 1.3fr; gap: 12px; padding: 0 18px; }
.bind-form-row { grid-column: 1 / -1; display: grid; grid-template-columns: 1fr 1fr; gap: 12px; }
.bind-form-row .form-textarea { min-height: 65px; }
.label-hint { color: #626a77; font: 8px var(--mono); font-weight: 400; }
.capture-field { min-width: 0; }
.capture-row { display: flex; gap: 6px; }
.capture-row .form-input { min-width: 0; flex: 1; }
.capture-row .form-input.capturing { border-color: #72894c; background: #22291c; color: #d3f0a4; }
.capture-button { min-width: 92px; }
.clear-button { min-width: 30px; padding: 0; }
.capture-hint.editing { flex-direction: row; align-items: center; gap: 6px; color: var(--blue); }
.edit-marker { color: var(--blue); }
.code-field { grid-column: 1 / -1; }
.code-field-head { display: flex; align-items: center; gap: 8px; }
.code-generated { color: #6b7280; font: 7.5px var(--mono); font-weight: 400; }
.code-block { min-height: 74px; color: #cbe79c; }
.code-note { color: #6d7481; font-style: normal; font-size: 7.5px; font-weight: 400; }
.bind-submit-row { display: flex; align-items: center; gap: 8px; }
.bind-row { cursor: pointer; }
.bind-row:hover { background: #21252e; }
.bind-row.editing { background: #22301c; box-shadow: inset 2px 0 0 var(--lime); }
.capture-button.active { border-color: #c4f274; background: #c4f274; color: #20291a; }
.capture-hint { display: flex; flex-direction: column; gap: 3px; }
.capture-live { color: #c3dc98; font: 9px var(--mono); }
.capture-hint em { color: #6d7481; font-style: normal; font-size: 7.5px; font-weight: 400; }
.capture-hint.conflict { flex-direction: row; align-items: center; gap: 6px; color: var(--amber); }
.capture-hint.conflict button { border: 0; background: transparent; color: #c7d68f; cursor: pointer; font-size: 7.5px; text-decoration: underline; }
.used-strip { display: flex; flex-wrap: wrap; align-items: center; gap: 4px; margin-top: 3px; }
.used-strip-label { color: #8a6f45; font-size: 7.5px; font-weight: 400; }
.used-chip { padding: 3px 6px; border: 1px solid #4a3a26; border-radius: 4px; background: #241d15; color: #c3a37a; cursor: pointer; font: 7.5px var(--mono); }
.used-chip:hover { border-color: #8a6c42; color: #e6c79b; }
.used-more { color: #6d7481; font: 7.5px var(--mono); font-weight: 400; }
.mouse-codes { display: flex; flex-wrap: wrap; align-items: center; gap: 4px; margin-top: 4px; }
.mouse-codes-label { color: #7f8794; font-size: 7.5px; font-weight: 400; }
.mouse-chip { display: inline-flex; align-items: center; gap: 4px; padding: 3px 6px; border: 1px solid #333945; border-radius: 4px; background: #1e222a; color: #b3bac6; cursor: pointer; font: 7.5px var(--mono); }
.mouse-chip em { color: #6f7683; font-style: normal; font-size: 6.5px; }
.mouse-chip:hover { border-color: #6d7d4c; color: #d3e9a8; }
.mouse-chip.used { border-color: #4a5836; background: #232b1c; color: #c0dc93; }
.mouse-chip.used em { color: #8a9c6a; }
.bind-form > button { justify-self: start; }
.bind-list-card { overflow: hidden; }
.bind-list-header { align-items: center; }
.bind-search { width: 214px; height: 29px; display: flex; align-items: center; gap: 7px; padding: 0 8px; border: 1px solid #353a45; border-radius: 6px; background: #15181f; color: #737b89; }
.bind-search input { width: 100%; border: 0; outline: 0; background: transparent; color: #dce0e6; font-size: 8.5px; }
.bind-table-head, .bind-row { display: grid; grid-template-columns: 1.05fr 1.3fr .9fr 1.1fr 26px; align-items: center; gap: 10px; padding: 0 15px; }
.bind-table-head { height: 29px; border-top: 1px solid #2a2e37; border-bottom: 1px solid #2a2e37; background: #20232b; color: #707785; font: 7px var(--mono); letter-spacing: .1em; }
.bind-row { min-height: 43px; border-bottom: 1px solid #292d36; }
.bind-row.disabled { opacity: .48; }
.bind-row.disabled .bind-key code { text-decoration: line-through; }
.capture-hint.dynamic { flex-direction: row; align-items: center; gap: 6px; color: var(--amber); }
.keycode-hint { color: #7a6b4d; font: 7px var(--mono); white-space: nowrap; }
.callback-name { padding: 3px 5px; border: 1px solid #414d34; border-radius: 4px; background: #262e1f; color: #bcd992; font-size: 7.5px; }
.callback-body { overflow: hidden; color: #8a917d; font: 7px var(--mono); text-overflow: ellipsis; white-space: nowrap; }
.bind-key { display: flex; align-items: center; gap: 6px; }
.bind-key code, .bind-dispatch code { color: #e0e3e9; font: 8.5px var(--mono); }
.bind-key svg { color: var(--amber); }
.bind-dispatch { min-width: 0; display: flex; flex-direction: column; gap: 3px; }
.bind-dispatch small { overflow: hidden; color: #78808e; font: 7px var(--mono); text-overflow: ellipsis; white-space: nowrap; }
.bind-options { display: flex; flex-wrap: wrap; gap: 4px; }
.bind-source { min-width: 0; display: flex; align-items: center; gap: 6px; overflow: hidden; color: #7e8592; font: 7px var(--mono); text-overflow: ellipsis; white-space: nowrap; }
.row-icon-button { width: 25px; height: 25px; display: grid; place-items: center; border: 1px solid transparent; border-radius: 5px; background: transparent; color: #777e8c; cursor: pointer; }
.row-icon-button:hover { border-color: #5b383c; background: #302024; color: #ef9ca1; }
@media (max-width: 850px) { .bind-table-head, .bind-row { grid-template-columns: 1fr 1.1fr 1.2fr 27px; } .bind-table-head span:nth-child(4), .bind-source { display: none; } }
@media (max-width: 620px) { .bind-form { grid-template-columns: 1fr; } .bind-form-row { grid-column: auto; grid-template-columns: 1fr; } .bind-table-head, .bind-row { grid-template-columns: 1fr 1.2fr 27px; } .bind-table-head span:nth-child(3), .bind-options { display: none; } .bind-search { width: 145px; } }
</style>
