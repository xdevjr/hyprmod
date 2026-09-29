<script setup lang="ts">
import { computed, ref, watch } from 'vue'
import { useI18n } from '../i18n'
import { ChevronRight, LoaderCircle, RefreshCw, Search, SlidersHorizontal } from '@lucide/vue'
import * as Api from '../../bindings/github.com/juniorgs/hyprmod/internal/api/service'
import type { DeviceView, SectionView } from '../../bindings/github.com/juniorgs/hyprmod/internal/api/models'
import type { Value } from '../../bindings/github.com/juniorgs/hyprmod/internal/config/models'
import type { Option, Schema } from '../../bindings/github.com/juniorgs/hyprmod/internal/schema/models'
import FieldEditor from './FieldEditor.vue'

const { t } = useI18n()

const props = defineProps<{ schema: Schema | null; initialSection: string }>()
const emit = defineEmits<{ saved: [message?: string]; error: [message: string] }>()
const selected = ref('general')
const view = ref<SectionView | null>(null)
const query = ref('')
const loading = ref(false)
const loadError = ref('')
const leafOptions = ref<Option[]>([])
const devices = ref<DeviceView | null>(null)
const isInput = computed(() => selected.value === 'input')

const sections = computed(() => (props.schema?.Sections ?? []).filter((item): item is Option => !!item && !!item.children?.length))
const selectedSchema = computed(() => sections.value.find(item => item.path === selected.value) ?? null)
const valueMap = computed(() => new Map((view.value?.values ?? []).map(item => [item.path, item])))
const visibleGroups = computed(() => {
  const groups = new Map<string, Option[]>()
  const q = query.value.trim().toLowerCase()
  for (const option of leafOptions.value) {
    if (q && !`${option.name} ${option.path} ${option.raw}`.toLowerCase().includes(q)) continue
    const rest = option.path.split('.').slice(1)
    const name = rest.length > 1 ? rest[0] : 'General'
    const group = groups.get(name) ?? []
    group.push(option)
    groups.set(name, group)
  }
  return [...groups.entries()]
})
const loadedCount = computed(() => (view.value?.values ?? []).filter((v: Value) => v.resolved !== null && v.resolved !== undefined).length)

function flatten(nodes: (Option | null)[]): Option[] {
  const result: Option[] = []
  for (const node of nodes) {
    if (!node) continue
    const children = node.children ?? []
    if (children.length) result.push(...flatten(children))
    else result.push(node)
  }
  return result
}

async function loadSection(path = selected.value) {
  selected.value = path
  loading.value = true
  loadError.value = ''
  try {
    const [nextView, schemaNode] = await Promise.all([Api.GetSection(path), Promise.resolve(sections.value.find(item => item.path === path) ?? null)])
    view.value = nextView
    leafOptions.value = schemaNode ? flatten(schemaNode.children ?? []) : []
    devices.value = path === 'input' ? await Api.GetDevices().catch(() => null) : null
  } catch (err) {
    loadError.value = String(err)
    view.value = null
    leafOptions.value = []
    devices.value = null
  } finally {
    loading.value = false
  }
}

async function setOption(path: string, value: unknown) {
  try {
    await Api.SetOption(path, value)
    emit('saved', `${path} applied`)
    await loadSection()
  } catch (err) {
    emit('error', String(err))
  }
}

async function previewOption(path: string, value: unknown) {
  try {
    await Api.PreviewOption(path, value)
    emit('saved', 'Temporary preview applied; a config reload will revert it')
  } catch (err) {
    emit('error', String(err))
  }
}

watch(() => props.initialSection, (path) => {
  if (sections.value.some(item => item.path === path)) void loadSection(path)
}, { immediate: true })
watch(sections, (items) => {
  if (!items.length) return
  if (!items.some(item => item.path === selected.value)) void loadSection(items[0].path)
}, { immediate: true })
</script>

<template>
  <div class="variables-layout">
    <aside class="variable-index card">
      <div class="variable-index-head">
        <div class="section-kicker">{{ t('variables.apiSections') }}</div>
        <span>{{ sections.length }}</span>
      </div>
      <button
        v-for="section in sections"
        :key="section.path"
        class="variable-index-item"
        :class="{ active: selected === section.path }"
        @click="loadSection(section.path)"
      >
        <span class="index-name">{{ section.name }}</span>
        <span class="index-count">{{ flatten(section.children ?? []).length }}</span>
        <ChevronRight :size="13" />
      </button>
      <div class="variable-index-foot">
        <SlidersHorizontal :size="13" />
        <span>{{ t('variables.generatedFrom') }}</span>
        <code>hl.meta.lua</code>
      </div>
    </aside>

    <div class="variable-main">
      <section class="card device-card">
        <div class="card-header">
          <div><div class="section-kicker">{{ t('variables.devices') }}</div><h3 class="card-title">{{ t('app.nav.input') }}</h3><p class="card-subtitle">{{ t('variables.devicesSubtitle') }}</p></div>
          <button class="icon-button" :title="t('variables.reload')" :disabled="!isInput" @click="isInput && loadSection()"><RefreshCw :size="14" /></button>
        </div>
        <div v-if="!isInput" class="device-hint">{{ t('app.page.input.description') }}</div>
        <template v-else>
          <div v-if="devices && !devices.available" class="device-hint">{{ t('variables.devicesOffline') }} <span>{{ devices.message }}</span></div>
          <div v-else-if="devices" class="device-groups">
            <div v-if="devices.keyboards?.length" class="device-group">
              <span class="device-group-label">{{ t('variables.keyboard') }}</span>
              <div v-for="(keyboard, index) in devices.keyboards" :key="`kb-${index}`" class="device-row">
                <strong>{{ keyboard.name || t('variables.noName') }}</strong>
                <span class="device-facts"><em>{{ t('variables.layout') }}</em> {{ keyboard.layout || '--' }}<em>{{ t('variables.variant') }}</em> {{ keyboard.variant || '--' }}<em>{{ t('variables.optionsLabel') }}</em> {{ keyboard.options || '--' }}</span>
              </div>
            </div>
            <div v-if="devices.mice?.length" class="device-group">
              <span class="device-group-label">{{ t('variables.mouse') }}</span>
              <div v-for="(mouse, index) in devices.mice" :key="`ms-${index}`" class="device-row">
                <strong>{{ mouse.name || t('variables.noName') }}</strong>
                <span class="device-facts"><em>{{ t('variables.speed') }}</em> {{ mouse.defaultSpeed }}<em>{{ t('variables.scroll') }}</em> {{ mouse.scrollFactor }}</span>
              </div>
            </div>
            <div v-if="devices.tablets?.length" class="device-group">
              <span class="device-group-label">{{ t('variables.tablet') }}</span>
              <div v-for="(tablet, index) in devices.tablets" :key="`tb-${index}`" class="device-row"><strong>{{ tablet?.name || t('variables.noName') }}</strong></div>
            </div>
            <div v-if="devices.touch?.length" class="device-group">
              <span class="device-group-label">{{ t('variables.touch') }}</span>
              <div v-for="(touch, index) in devices.touch" :key="`tc-${index}`" class="device-row"><strong>{{ touch?.name || t('variables.noName') }}</strong></div>
            </div>
            <div v-if="!devices.keyboards?.length && !devices.mice?.length && !devices.tablets?.length && !devices.touch?.length" class="empty-state">{{ t('variables.noDevices') }}</div>
          </div>
          <div v-else class="device-hint">{{ t('common.loading') }}</div>
        </template>
      </section>

      <section class="variable-summary card">
        <div class="variable-summary-title">
          <div class="section-kicker">{{ t('variables.liveSection') }}</div>
          <h2>{{ selectedSchema?.name || selected }}</h2>
          <p>{{ selectedSchema?.raw || selected }}</p>
        </div>
        <div class="variable-summary-right">
          <div class="summary-stat"><strong>{{ leafOptions.length }}</strong><span>{{ t('variables.options') }}</span></div>
          <div class="summary-stat"><strong>{{ loadedCount }}</strong><span>{{ t('variables.resolved') }}</span></div>
          <button class="icon-button" :title="t('variables.reload')" @click="loadSection()"><RefreshCw :size="14" /></button>
        </div>
      </section>

      <section class="variable-toolbar">
        <div class="variable-search"><Search :size="14" /><input v-model="query" :placeholder="t('variables.filter')" /></div>
        <div class="runtime-label" :class="{ offline: !view?.runtimeAvailable }"><span></span>{{ view?.runtimeAvailable ? t('variables.runtimeValues') : t('variables.configOnly') }}</div>
      </section>

      <div v-if="loadError" class="load-error card">
        <span>{{ loadError }}</span><button class="button-secondary" @click="loadSection()">{{ t('common.retry') }}</button>
      </div>
      <div v-else-if="loading && !view" class="variable-loading card"><LoaderCircle :size="17" class="spin" /> {{ t('variables.reading') }}</div>
      <div v-else-if="!visibleGroups.length" class="empty-state card">{{ t('variables.noMatch') }}</div>
      <div v-else class="variable-groups">
        <section v-for="[group, options] in visibleGroups" :key="group" class="variable-group card">
          <div class="variable-group-head">
            <div><span class="section-kicker">{{ selected }} /</span><h3>{{ group }}</h3></div>
            <span class="group-count">{{ t('variables.fields', { count: options.length }) }}</span>
          </div>
          <FieldEditor
            v-for="option in options"
            :key="option.path"
            :option="option"
            :value="valueMap.get(option.path)"
            @save="setOption"
            @preview="previewOption"
            @error="(message) => emit('error', message)"
          />
        </section>
      </div>
      <div class="variable-footnote"><span class="tag amber">{{ t('common.dynamic') }}</span> {{ t('variables.dynamicNote') }}</div>
    </div>
  </div>
</template>

<style scoped>
.variables-layout { display: grid; grid-template-columns: 194px minmax(0, 1fr); align-items: start; gap: 13px; }
.device-card { overflow: hidden; }
.device-hint { display: flex; flex-wrap: wrap; gap: 7px; padding: 11px 18px 14px; color: #848b99; font-size: 9px; }
.device-hint span { color: #6a7180; font: 8px var(--mono); }
.device-groups { display: flex; flex-direction: column; }
.device-group { padding: 4px 18px 10px; }
.device-group-label { display: block; padding: 9px 0 5px; color: #7b8467; font: 7.5px var(--mono); letter-spacing: .1em; }
.device-row { min-height: 32px; display: flex; align-items: center; gap: 11px; border-top: 1px solid #282c35; }
.device-row strong { min-width: 0; overflow: hidden; color: #d4d8e0; font: 9px var(--mono); text-overflow: ellipsis; white-space: nowrap; }
.device-facts { min-width: 0; display: flex; flex-wrap: wrap; align-items: center; gap: 5px; color: #9aa1ae; font: 7.5px var(--mono); }
.device-facts em { color: #6b7280; font-style: normal; }
.variable-index { position: sticky; top: 0; overflow: hidden; padding: 12px 8px 8px; }
.variable-index-head { display: flex; align-items: center; justify-content: space-between; padding: 3px 8px 10px; }
.variable-index-head > span { color: #747b88; font: 8px var(--mono); }
.variable-index-item { width: 100%; height: 31px; display: flex; align-items: center; gap: 6px; padding: 0 8px; border: 0; border-radius: 5px; background: transparent; color: #a1a6b1; text-align: left; cursor: pointer; font: 9px var(--mono); }
.variable-index-item:hover { background: #252933; color: #e1e3e8; }
.variable-index-item.active { background: #2b3522; color: var(--lime); }
.index-name { flex: 1; overflow: hidden; text-overflow: ellipsis; }
.index-count { color: #69717e; font-size: 7px; }
.variable-index-foot { display: flex; flex-wrap: wrap; align-items: center; gap: 5px; margin: 12px 3px 0; padding: 10px 5px 2px; border-top: 1px solid #30343e; color: #737a88; font-size: 7px; }
.variable-index-foot code { width: 100%; color: #a8b98c; font: 8px var(--mono); }
.variable-main { min-width: 0; display: flex; flex-direction: column; gap: 12px; }
.variable-summary { display: flex; align-items: center; justify-content: space-between; padding: 15px 18px; }
.variable-summary-title h2 { margin: 4px 0 2px; color: #e9ebef; font-size: 16px; letter-spacing: -.4px; }
.variable-summary-title p { margin: 0; color: #7f8694; font: 8px var(--mono); }
.variable-summary-right { display: flex; align-items: center; gap: 17px; }
.summary-stat { display: flex; flex-direction: column; align-items: flex-end; gap: 2px; }
.summary-stat strong { color: #e6e8ed; font: 14px var(--mono); }
.summary-stat span { color: #737a88; font: 7px var(--mono); letter-spacing: .1em; }
.variable-summary-right .icon-button { border-color: #30343e; background: #22252e; }
.variable-toolbar { display: flex; align-items: center; justify-content: space-between; gap: 12px; }
.variable-search { flex: 1; height: 32px; display: flex; align-items: center; gap: 8px; padding: 0 9px; border: 1px solid #30343f; border-radius: 6px; background: #191c23; color: #79808d; }
.variable-search input { width: 100%; border: 0; outline: 0; background: transparent; color: #dfe1e6; font-size: 9px; }
.variable-search input::placeholder { color: #747b88; }
.runtime-label { display: flex; align-items: center; gap: 6px; color: #9ab877; font: 7.5px var(--mono); letter-spacing: .08em; white-space: nowrap; }
.runtime-label span { width: 5px; height: 5px; border-radius: 50%; background: var(--lime); }
.runtime-label.offline { color: #c2a36c; }
.runtime-label.offline span { background: var(--amber); }
.variable-groups { display: flex; flex-direction: column; gap: 11px; }
.variable-group { overflow: hidden; }
.variable-group-head { min-height: 47px; display: flex; align-items: center; justify-content: space-between; padding: 8px 14px; background: #20232b; }
.variable-group-head h3 { display: inline; margin: 0 0 0 5px; color: #dce0e7; font-size: 10px; }
.group-count { color: #727987; font: 8px var(--mono); }
.variable-loading { min-height: 120px; display: flex; align-items: center; justify-content: center; gap: 10px; color: #9299a6; font-size: 10px; }
.spin { animation: spin 1.2s linear infinite; }
@keyframes spin { to { transform: rotate(360deg); } }
.load-error { display: flex; align-items: center; justify-content: space-between; gap: 15px; padding: 13px; color: #e4a8ab; font-size: 9px; }
.variable-footnote { display: flex; align-items: center; gap: 8px; color: #7a818f; font-size: 8px; line-height: 1.5; }
@media (max-width: 900px) { .variables-layout { grid-template-columns: 1fr; } .variable-index { position: static; display: flex; flex-wrap: wrap; gap: 3px; } .variable-index-head, .variable-index-foot { display: none; } .variable-index-item { width: auto; } }
@media (max-width: 650px) { .variable-summary { align-items: flex-start; flex-direction: column; gap: 12px; } .variable-summary-right { align-self: stretch; justify-content: space-between; } .variable-toolbar { align-items: flex-start; flex-direction: column; } }
</style>
