<script setup lang="ts">
import { computed, nextTick, onBeforeUnmount, ref, watch } from 'vue'
import { useI18n } from '../i18n'
import { Code2, FileCode2, Save, ShieldCheck } from '@lucide/vue'
import { EditorState } from '@codemirror/state'
import { EditorView, keymap } from '@codemirror/view'
import { defaultKeymap, indentWithTab } from '@codemirror/commands'
import { StreamLanguage } from '@codemirror/language'
import { lua as luaMode } from '@codemirror/legacy-modes/mode/lua'
import * as Api from '../../bindings/github.com/juniorgs/hyprmod/internal/api/service'
import type { Document } from '../../bindings/github.com/juniorgs/hyprmod/internal/config/models'
import type { SourceFile } from '../../bindings/github.com/juniorgs/hyprmod/internal/api/models'

const props = defineProps<{ document: Document | null }>()
const emit = defineEmits<{ saved: [message?: string]; error: [message: string] }>()
const { t } = useI18n()
const host = ref<HTMLElement | null>(null)
const source = ref<SourceFile | null>(null)
const selectedPath = ref('')
const loading = ref(false)
const loadError = ref('')
const saving = ref(false)
const validated = ref(false)
const editor = ref<EditorView | null>(null)
const editedText = ref('')
const files = computed(() => props.document?.files ?? [])
const dirty = computed(() => editedText.value !== (source.value?.content ?? ''))

const showEditor = computed(() => !loading.value && !loadError.value && files.value.length > 0)

function mountEditor(content: string) {
  editor.value?.destroy()
  editor.value = null
  if (!host.value) {
    // Defensive retry: never leave the editor area blank because the host had
    // not been mounted yet.
    void nextTick(() => {
      if (host.value) mountEditor(content)
    })
    return
  }
  const theme = EditorView.theme({
    '&': { height: '100%', color: '#d7dbe4', backgroundColor: '#13161d', fontSize: '11px' },
    '.cm-content': { fontFamily: 'var(--mono)', padding: '16px 0' },
    '.cm-gutters': { backgroundColor: '#151820', color: '#596171', border: 'none' },
    '.cm-activeLineGutter': { backgroundColor: '#20242d', color: '#aab1be' },
    '.cm-activeLine': { backgroundColor: '#1d222b' },
    '.cm-selectionBackground': { backgroundColor: '#566c3c66 !important' },
    '.cm-cursor': { borderLeftColor: '#c4f274' },
    '&.cm-focused': { outline: 'none' },
  }, { dark: true })
  editor.value = new EditorView({
    state: EditorState.create({
      doc: content,
      extensions: [
        StreamLanguage.define(luaMode),
        theme,
        EditorView.lineWrapping,
        keymap.of([...defaultKeymap, indentWithTab]),
        EditorView.updateListener.of(update => {
          if (update.docChanged) editedText.value = update.state.doc.toString()
        }),
      ],
    }),
    parent: host.value,
  })
  editedText.value = content
}

async function loadFile(path: string) {
  if (!path) return
  loading.value = true
  validated.value = false
  loadError.value = ''
  try {
    const file = await Api.ReadSource(path)
    source.value = file
    selectedPath.value = file.path
  } catch (err) {
    source.value = null
    loadError.value = String(err)
    emit('error', String(err))
  } finally {
    loading.value = false
  }
  // The editor host only exists once the loading branch is gone from the DOM,
  // so the view has to be mounted after the state settles and the DOM updates.
  await nextTick()
  if (source.value) mountEditor(source.value.content)
}

async function saveFile() {
  if (!source.value || !dirty.value) return
  saving.value = true
  validated.value = false
  try {
    await Api.WriteSource(source.value.path, editor.value?.state.doc.toString() ?? editedText.value, source.value.digest)
    source.value = await Api.ReadSource(source.value.path)
    editedText.value = source.value.content
    if (editor.value && editor.value.state.doc.toString() !== source.value.content) {
      editor.value.dispatch({ changes: { from: 0, to: editor.value.state.doc.length, insert: source.value.content } })
    }
    validated.value = true
    emit('saved', t('source.validated'))
  } catch (err) {
    emit('error', String(err))
  } finally {
    saving.value = false
  }
}

async function validate() {
  try {
    const result = await Api.ValidateConfig()
    const errors = result.diagnostics?.filter(item => item.severity === 'error') ?? []
    if (errors.length) throw new Error(errors.map(item => item.message).join('\n'))
    validated.value = true
    emit('saved', t('source.validConfig'))
  } catch (err) {
    emit('error', String(err))
  }
}

watch(files, (items) => {
  if (!items.length) return
  if (!selectedPath.value || !items.some(file => file.path === selectedPath.value)) {
    const entry = props.document?.entry
    void loadFile(items.find(file => file.path === entry)?.path ?? items[0].path)
  }
}, { immediate: true })

onBeforeUnmount(() => editor.value?.destroy())
</script>

<template>
  <div class="lua-editor-layout">
    <section class="editor-intro card">
      <div class="editor-intro-icon"><Code2 :size="19" /></div>
      <div><div class="section-kicker">{{ t('source.advancedMode') }}</div><h2>{{ t('source.headline') }}</h2><p>{{ t('source.subtitle') }}</p></div>
      <div class="editor-safety"><ShieldCheck :size="14" /> {{ t('source.validateBeforeWrite') }}</div>
    </section>

    <section class="source-editor card">
      <div class="source-toolbar">
        <div class="source-select-group">
          <FileCode2 :size="14" />
          <select v-model="selectedPath" class="source-select" @change="loadFile(selectedPath)">
            <option v-for="file in files" :key="file.path" :value="file.path">{{ file.rel }}</option>
          </select>
          <span v-if="source" class="source-bytes">{{ source.bytes.toLocaleString() }} B</span>
        </div>
        <div class="source-actions">
          <span v-if="validated" class="validated-label"><span></span>{{ t('source.validating') }}</span>
          <span v-if="dirty" class="dirty-label">{{ t('source.unsaved') }}</span>
          <button class="button-secondary" :disabled="loading" @click="validate"><ShieldCheck :size="12" /> {{ t('source.validate') }}</button>
          <button class="button-primary" :disabled="!dirty || saving" @click="saveFile"><Save :size="12" /> {{ saving ? t('source.saving') : t('source.save') }}</button>
        </div>
      </div>
      <div class="source-stage">
        <div ref="host" class="code-editor" v-show="showEditor"></div>
        <div v-if="loading" class="source-overlay source-loading">{{ t('source.loading') }}</div>
        <div v-else-if="loadError" class="source-overlay source-error">
          <strong>{{ loadError }}</strong>
          <span>{{ t('source.readError') }}</span>
          <button class="button-secondary" @click="loadFile(selectedPath)">{{ t('common.retry') }}</button>
        </div>
        <div v-else-if="!files.length" class="source-overlay empty-state">{{ t('source.noModules') }}</div>
      </div>
      <div class="source-footer"><span>{{ source?.path || t('source.noSource') }}</span><span>{{ t('source.syntaxHint') }}</span></div>
    </section>
  </div>
</template>

<style scoped>
.lua-editor-layout { display: flex; flex-direction: column; gap: 13px; }
.editor-intro { min-height: 85px; display: flex; align-items: center; gap: 12px; padding: 14px 18px; border-color: #3b414d; background: linear-gradient(105deg, #222731, #1b1e26); }
.editor-intro-icon { width: 37px; height: 37px; display: grid; place-items: center; border: 1px solid #465541; border-radius: 9px; background: #2b3525; color: #bfde91; }
.editor-intro > div:nth-child(2) { flex: 1; }
.editor-intro h2 { margin: 4px 0 3px; color: #e5e8ee; font-size: 12px; }
.editor-intro p { margin: 0; color: #848b99; font-size: 8.5px; line-height: 1.5; }
.editor-safety { display: flex; align-items: center; gap: 6px; padding: 7px 8px; border: 1px solid #3f4936; border-radius: 6px; background: #252c21; color: #bfdc93; font: 7px var(--mono); white-space: nowrap; }
.source-editor { overflow: hidden; display: flex; flex-direction: column; height: min(680px, calc(100vh - 300px)); min-height: 380px; }
.source-toolbar { min-height: 44px; display: flex; align-items: center; justify-content: space-between; gap: 10px; padding: 0 11px; border-bottom: 1px solid #30343e; background: #20232b; }
.source-select-group, .source-actions { display: flex; align-items: center; gap: 8px; min-width: 0; }
.source-select-group { color: #a6b785; }
.source-select { max-width: min(390px, 40vw); border: 0; outline: 0; background: transparent; color: #d9dce3; font: 8.5px var(--mono); }
.source-select option { background: #1c1f27; }
.source-bytes { color: #777e8c; font: 7px var(--mono); }
.source-actions .button-secondary, .source-actions .button-primary { min-height: 27px; padding-inline: 8px; font-size: 8px; }
.validated-label, .dirty-label { display: flex; align-items: center; gap: 5px; color: #b5d68b; font: 7px var(--mono); letter-spacing: .07em; }
.validated-label span { width: 5px; height: 5px; border-radius: 50%; background: var(--lime); }
.dirty-label { color: var(--amber); }
.source-stage { position: relative; flex: 1 1 auto; min-height: 0; display: flex; }
.code-editor { flex: 1 1 auto; min-height: 0; height: 100%; overflow: hidden; }
.source-overlay { position: absolute; inset: 0; z-index: 2; display: grid; place-items: center; align-content: center; gap: 9px; padding: 20px; background: #1b1e26; }
.source-loading { color: #858c99; font-size: 10px; }
.source-error { color: #e0a0a4; font-size: 9.5px; text-align: center; }
.source-error span { max-width: 460px; color: #9b8385; }
.source-footer { min-height: 28px; display: flex; align-items: center; justify-content: space-between; gap: 12px; padding: 0 11px; border-top: 1px solid #2a2e37; background: #171a21; color: #656d7a; font: 7px var(--mono); }
.source-footer span { overflow: hidden; text-overflow: ellipsis; white-space: nowrap; }
@media (max-width: 760px) { .editor-safety, .source-footer span:nth-child(2) { display: none; } .source-toolbar { align-items: flex-start; flex-direction: column; padding: 9px; } .source-actions { width: 100%; justify-content: flex-end; } }
</style>
