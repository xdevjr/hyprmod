<script setup lang="ts">
import { computed, ref } from 'vue'
import { useI18n } from '../i18n'
import { Braces, CircleAlert, Plus, Search } from '@lucide/vue'
import * as Api from '../../bindings/github.com/juniorgs/hyprmod/internal/api/service'
import type { Document, Rule } from '../../bindings/github.com/juniorgs/hyprmod/internal/config/models'

const { t } = useI18n()

const props = defineProps<{
  kind: 'window-rules' | 'workspace-rules' | 'layer-rules'
  document: Document | null
}>()
const emit = defineEmits<{ saved: [message?: string]; error: [message: string] }>()
const query = ref('')
const specText = ref(`{
  "match": { "class": "^firefox$" },
  "float": true
}`)
const saving = ref(false)
const advanced = ref(false)
const ruleName = ref('')
const matchField = ref('class')
const matchValue = ref('')
const floatWindow = ref(false)
const fullscreenWindow = ref(false)
const pinWindow = ref(false)
const centerWindow = ref(false)
const noAnimation = ref(false)
const blurLayer = ref(false)
const workspace = ref('')
const workspaceMonitor = ref('')
const workspaceDefault = ref(false)

const apiKind = computed(() => props.kind === 'window-rules' ? 'window' : props.kind === 'workspace-rules' ? 'workspace' : 'layer')
const kindLabel = computed(() => apiKind.value)
const rules = computed(() => (props.document?.rules ?? []).filter((rule: Rule) => rule.kind === apiKind.value))
const filtered = computed(() => {
  const q = query.value.trim().toLowerCase()
  return rules.value.filter(rule => !q || `${rule.name ?? ''} ${JSON.stringify(rule.match ?? {})} ${JSON.stringify(rule.fields ?? {})}`.toLowerCase().includes(q))
})
const example = computed(() => {
  if (apiKind.value === 'window') return '{ "name": "picture-in-picture", "match": { "title": "Picture-in-Picture" }, "float": true, "pin": true }'
  if (apiKind.value === 'workspace') return '{ "workspace": "name:media", "monitor": "DP-1", "default": true }'
  return '{ "match": { "namespace": "^notifications$" }, "no_anim": true }'
})

async function addRule() {
  saving.value = true
  try {
    const spec = advanced.value ? JSON.parse(specText.value) : buildVisualSpec()
    if (!spec || typeof spec !== 'object' || Array.isArray(spec)) throw new Error('Rule must be a JSON object.')
    if (apiKind.value === 'window') await Api.AddWindowRule(spec)
    else if (apiKind.value === 'workspace') await Api.AddWorkspaceRule(spec)
    else await Api.AddLayerRule(spec)
    emit('saved', t('rules.added', { kind: kindLabel.value }))
  } catch (err) {
    emit('error', `Could not add rule: ${String(err)}`)
  } finally {
    saving.value = false
  }
}

function buildVisualSpec(): Record<string, unknown> {
  if (apiKind.value === 'window') {
    const spec: Record<string, unknown> = {}
    if (ruleName.value.trim()) spec.name = ruleName.value.trim()
    if (matchValue.value.trim()) spec.match = { [matchField.value]: matchValue.value.trim() }
    if (floatWindow.value) spec.float = true
    if (fullscreenWindow.value) spec.fullscreen = true
    if (pinWindow.value) spec.pin = true
    if (centerWindow.value) spec.center = true
    return spec
  }
  if (apiKind.value === 'workspace') {
    const spec: Record<string, unknown> = {}
    if (workspace.value.trim()) spec.workspace = workspace.value.trim()
    if (workspaceMonitor.value.trim()) spec.monitor = workspaceMonitor.value.trim()
    if (workspaceDefault.value) spec.default = true
    return spec
  }
  const spec: Record<string, unknown> = {}
  if (ruleName.value.trim()) spec.name = ruleName.value.trim()
  if (matchValue.value.trim()) spec.match = { namespace: matchValue.value.trim() }
  if (noAnimation.value) spec.no_anim = true
  if (blurLayer.value) spec.blur = true
  return spec
}
</script>

<template>
  <div class="rules-layout">
    <section class="rules-intro card">
      <div class="rules-intro-symbol"><Braces :size="20" /></div>
      <div class="rules-intro-copy"><div class="section-kicker">{{ t('rules.declarativeMatchers') }}</div><h2>{{ t('rules.builderTitle', { kind: kindLabel }) }}</h2><p>{{ t('rules.builderSubtitle') }}</p></div>
      <div class="rules-count"><strong>{{ rules.length }}</strong><span>{{ t('rules.loaded') }}</span></div>
    </section>

    <section class="card rules-create">
      <div class="card-header"><div><div class="section-kicker">{{ t('rules.newRule') }}</div><h3 class="card-title">{{ t('rules.addRule', { kind: kindLabel }) }}</h3><p class="card-subtitle">{{ t('rules.addRuleSubtitle') }}</p></div><button class="button-quiet mode-toggle" @click="advanced = !advanced">{{ advanced ? t('rules.visualBuilder') : t('rules.advancedJson') }}</button></div>
      <div class="rules-create-body">
        <div class="rules-hints">
          <div><span>01</span><strong>{{ t('rules.matcher') }}</strong><small>{{ t('rules.matcherSub') }}</small></div>
          <div><span>02</span><strong>{{ t('rules.actions') }}</strong><small>{{ t('rules.actionsSub') }}</small></div>
          <div class="example-box"><span>{{ t('rules.example') }}</span><code>{{ example }}</code></div>
        </div>
        <div v-if="!advanced" class="visual-rule-form">
          <label v-if="apiKind !== 'workspace'" class="form-label">{{ t('rules.ruleName') }}<input v-model="ruleName" class="form-input" :placeholder="t('rules.optionalLabel')" /></label>
          <template v-if="apiKind === 'window'">
            <div class="match-builder"><label class="form-label">{{ t('rules.matchField') }}<select v-model="matchField" class="form-select"><option value="class">class</option><option value="title">title</option><option value="initial_class">initial_class</option><option value="initial_title">initial_title</option><option value="xwayland">xwayland</option></select></label><label class="form-label">{{ t('rules.pattern') }}<input v-model="matchValue" class="form-input mono" placeholder="^firefox$" /></label></div>
            <div class="rule-toggle-grid">
              <label><input v-model="floatWindow" type="checkbox" /><span>{{ t('rules.floating') }}</span></label><label><input v-model="fullscreenWindow" type="checkbox" /><span>{{ t('rules.fullscreen') }}</span></label><label><input v-model="pinWindow" type="checkbox" /><span>{{ t('rules.pinned') }}</span></label><label><input v-model="centerWindow" type="checkbox" /><span>{{ t('rules.centered') }}</span></label>
            </div>
          </template>
          <template v-else-if="apiKind === 'workspace'">
            <label class="form-label">{{ t('rules.workspaceId') }}<input v-model="workspace" class="form-input mono" placeholder="name:media" /></label>
            <label class="form-label">{{ t('rules.monitorBinding') }}<input v-model="workspaceMonitor" class="form-input mono" placeholder="DP-1" /></label>
            <label class="rule-check"><input v-model="workspaceDefault" type="checkbox" /><span>{{ t('rules.defaultWorkspace') }}</span></label>
          </template>
          <template v-else>
            <label class="form-label">{{ t('rules.namespace') }}<input v-model="matchValue" class="form-input mono" placeholder="^notifications$" /></label>
            <div class="rule-toggle-grid"><label><input v-model="noAnimation" type="checkbox" /><span>{{ t('rules.noAnimation') }}</span></label><label><input v-model="blurLayer" type="checkbox" /><span>{{ t('rules.blurSurface') }}</span></label></div>
          </template>
        </div>
        <label v-else class="form-label rules-json-label">{{ t('rules.ruleSpec') }} <span class="label-hint">{{ t('rules.toLua') }}</span><textarea v-model="specText" class="form-textarea rules-json" spellcheck="false"></textarea></label>
      </div>
      <div class="rules-create-footer"><span class="muted">{{ t('rules.unknownKeys') }}</span><button class="button-primary" :disabled="saving" @click="addRule"><Plus :size="14" /> {{ t('rules.adding') }}</button></div>
    </section>

    <section class="card rules-list">
      <div class="card-header rules-list-head">
        <div><div class="section-kicker">{{ t('rules.activeConfig') }}</div><h3 class="card-title">{{ t('rules.loadedRules') }}</h3></div>
        <div class="bind-search"><Search :size="13" /><input v-model="query" :placeholder="t('rules.filter')" /></div>
      </div>
      <div v-if="!filtered.length" class="empty-state">{{ t('rules.noRules', { kind: kindLabel }) }}</div>
      <article v-for="(rule, index) in filtered" :key="`${rule.file}:${rule.line}:${index}`" class="rule-record">
        <div class="rule-record-top">
          <div class="rule-record-name"><span class="rule-index">{{ String(index + 1).padStart(2, '0') }}</span><strong>{{ rule.name || `${kindLabel} rule` }}</strong><span v-if="!rule.editable" class="tag amber">{{ t('common.dynamic') }}</span></div>
          <span class="rule-origin">{{ rule.file.split('/').slice(-2).join('/') }}:{{ rule.line }}</span>
        </div>
        <div class="rule-record-content">
          <div v-if="rule.match && Object.keys(rule.match).length" class="rule-match"><span>{{ t('rules.matchLabel') }}</span><code>{{ JSON.stringify(rule.match) }}</code></div>
          <div v-if="rule.fields && Object.keys(rule.fields).length" class="rule-actions"><span>{{ t('rules.actionsLabel') }}</span><div class="rule-tags"><span v-for="[key, value] in Object.entries(rule.fields)" :key="key" class="rule-action-tag"><b>{{ key }}</b> {{ JSON.stringify(value) }}</span></div></div>
        </div>
      </article>
      <div class="rules-footnote"><CircleAlert :size="13" /> {{ t('rules.removalNote') }}</div>
    </section>
  </div>
</template>

<style scoped>
.rules-layout { display: flex; flex-direction: column; gap: 13px; }
.rules-intro { min-height: 97px; display: flex; align-items: center; gap: 13px; padding: 16px 19px; }
.rules-intro-symbol { width: 39px; height: 39px; display: grid; place-items: center; border: 1px solid #414c34; border-radius: 9px; background: #2a3322; color: var(--lime); }
.rules-intro-copy { flex: 1; }
.rules-intro-copy h2 { margin: 5px 0 3px; color: #e7e9ee; font-size: 13px; }
.rules-intro-copy p { max-width: 600px; margin: 0; color: #808795; font-size: 8.5px; line-height: 1.5; }
.rules-count { display: flex; flex-direction: column; align-items: flex-end; gap: 2px; padding-right: 4px; }
.rules-count strong { color: #eff1f4; font: 20px var(--mono); }
.rules-count span { color: #7a818e; font: 7px var(--mono); letter-spacing: .1em; }
.rules-create { overflow: hidden; }
.rules-create-body { display: grid; grid-template-columns: .85fr 1.15fr; gap: 18px; padding: 0 19px 13px; }
.rules-hints { display: flex; flex-direction: column; gap: 8px; }
.rules-hints > div:not(.example-box) { display: grid; grid-template-columns: 24px 1fr; gap: 1px 8px; padding: 8px 0; border-top: 1px solid #2d313a; }
.rules-hints > div > span { grid-row: span 2; color: #8ea56d; font: 8px var(--mono); }
.rules-hints strong { color: #d0d4dc; font-size: 9px; }
.rules-hints small { color: #777e8c; font-size: 8px; line-height: 1.45; }
.example-box { display: flex; flex-direction: column; gap: 7px; padding: 9px; border: 1px solid #333844; border-radius: 7px; background: #171a21; }
.example-box span { color: #829364; font: 7px var(--mono); letter-spacing: .1em; }
.example-box code { color: #9ea5b2; font: 7px/1.55 var(--mono); overflow-wrap: anywhere; }
.rules-json-label { min-width: 0; }
.rules-json { min-height: 174px; }
.visual-rule-form { display: flex; flex-direction: column; gap: 10px; }
.match-builder { display: grid; grid-template-columns: .8fr 1.2fr; gap: 9px; }
.rule-toggle-grid { display: grid; grid-template-columns: 1fr 1fr; gap: 7px; }
.rule-toggle-grid label, .rule-check { min-height: 30px; display: flex; align-items: center; gap: 7px; padding: 0 8px; border: 1px solid #353a44; border-radius: 6px; background: #1a1d24; color: #aeb4bf; font-size: 8px; }
.rule-toggle-grid input, .rule-check input { accent-color: #c4f274; }
.mode-toggle { align-self: center; font-size: 8px; }
.rules-create-footer { min-height: 44px; display: flex; align-items: center; justify-content: space-between; gap: 10px; padding: 0 19px; border-top: 1px solid #2a2e37; }
.rules-create-footer > span { font-size: 8px; }
.rules-list { overflow: hidden; }
.rules-list-head { align-items: center; }
.rule-record { padding: 12px 18px; border-top: 1px solid #2a2e37; }
.rule-record-top { display: flex; align-items: center; justify-content: space-between; gap: 10px; }
.rule-record-name { display: flex; align-items: center; gap: 8px; min-width: 0; }
.rule-record-name strong { overflow: hidden; color: #d9dce3; font-size: 9.5px; text-overflow: ellipsis; white-space: nowrap; }
.rule-index { color: #7b846a; font: 8px var(--mono); }
.rule-origin { overflow: hidden; color: #707785; font: 7px var(--mono); text-overflow: ellipsis; white-space: nowrap; }
.rule-record-content { display: flex; flex-direction: column; gap: 7px; margin: 9px 0 0 26px; }
.rule-match, .rule-actions { display: flex; align-items: flex-start; gap: 9px; }
.rule-match > span, .rule-actions > span { width: 40px; padding-top: 2px; color: #75816a; font: 7px var(--mono); }
.rule-match code { color: #b4bac6; font: 8px var(--mono); overflow-wrap: anywhere; }
.rule-tags { display: flex; flex-wrap: wrap; gap: 5px; }
.rule-action-tag { padding: 4px 6px; border: 1px solid #353a45; border-radius: 4px; background: #22262f; color: #acb2bd; font: 7px var(--mono); }
.rule-action-tag b { color: #c2d794; font-weight: 500; }
.rules-footnote { display: flex; align-items: center; gap: 7px; padding: 12px 18px; border-top: 1px solid #2a2e37; color: #a99469; font-size: 8px; }
@media (max-width: 760px) { .rules-create-body { grid-template-columns: 1fr; } .rules-intro-copy p { display: none; } }
</style>
