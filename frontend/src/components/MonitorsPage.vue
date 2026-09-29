<script setup lang="ts">
import { computed, onMounted, ref, watch } from 'vue'
import { useI18n } from '../i18n'
import { Cable, CircleHelp, Grip, Monitor, Move, RefreshCw, Save } from '@lucide/vue'
import * as Api from '../../bindings/github.com/juniorgs/hyprmod/internal/api/service'
import type { MonitorView } from '../../bindings/github.com/juniorgs/hyprmod/internal/api/models'
import type { Document } from '../../bindings/github.com/juniorgs/hyprmod/internal/config/models'
import type { Monitor as RuntimeMonitor } from '../../bindings/github.com/juniorgs/hyprmod/internal/hyprctl/models'

const { t } = useI18n()

const props = defineProps<{ document: Document | null }>()
const emit = defineEmits<{ saved: [message?: string]; error: [message: string] }>()
const view = ref<MonitorView | null>(null)
const selected = ref('')
const positions = ref<Record<string, { x: number; y: number }>>({})
const saving = ref(false)
const loading = ref(false)
const scaleDraft = ref(1)
const modeDraft = ref('')
const vrrDraft = ref(false)

const activeMonitors = computed(() => view.value?.active ?? [])
const configured = computed(() => view.value?.configured ?? [])
const selectedMonitor = computed(() => activeMonitors.value.find(monitor => monitor.name === selected.value) ?? activeMonitors.value[0] ?? null)
const selectedConfig = computed(() => configured.value.find(item => item.output === selectedMonitor.value?.name) ?? null)

function parsePosition(text?: string) {
  const match = text?.match(/^\s*(-?\d+)\s*x\s*(-?\d+)\s*$/i)
  if (!match) return null
  return { x: Number(match[1]), y: Number(match[2]) }
}

function positionFor(monitor: RuntimeMonitor) {
  if (positions.value[monitor.name]) return positions.value[monitor.name]
  const configPosition = configured.value.find(item => item.output === monitor.name)?.position
  return parsePosition(configPosition) ?? { x: monitor.x, y: monitor.y }
}

function logicalSize(monitor: RuntimeMonitor) {
  const scale = monitor.scale || 1
  return { w: monitor.width / scale, h: monitor.height / scale }
}

// Monitors may touch edges but must never overlap, so a drag resolves to the
// closest free position: full move, then one axis, otherwise the last valid one.
function overlaps(x: number, y: number, monitor: RuntimeMonitor) {
  const { w, h } = logicalSize(monitor)
  return activeMonitors.value.some(other => {
    if (other.name === monitor.name) return false
    const pos = positionFor(other)
    const size = logicalSize(other)
    return x < pos.x + size.w && x + w > pos.x && y < pos.y + size.h && y + h > pos.y
  })
}

function resolveDragPosition(candidate: { x: number; y: number }): { x: number; y: number } {
  if (!drag) return candidate
  const monitor = activeMonitors.value.find(item => item.name === drag?.name)
  if (!monitor) return candidate
  const snap = (x: number, y: number) => ({ x: Math.round(x), y: Math.round(y) })
  if (!overlaps(candidate.x, candidate.y, monitor)) return snap(candidate.x, candidate.y)
  if (!overlaps(candidate.x, drag.y, monitor)) return snap(candidate.x, drag.y)
  if (!overlaps(drag.x, candidate.y, monitor)) return snap(drag.x, candidate.y)
  return { x: drag.x, y: drag.y }
}

const modeOptions = computed(() => {
  const monitor = selectedMonitor.value
  if (!monitor) return []
  const options = (monitor.availableModes ?? [])
    .map(mode => mode.replace(/Hz$/i, '').trim())
    .filter(Boolean)
  const current = modeDraft.value
  if (current && !options.includes(current)) options.unshift(current)
  return options
})

const stage = computed(() => {
  if (!activeMonitors.value.length) return { items: [] as Array<{ monitor: RuntimeMonitor; x: number; y: number; w: number; h: number; left: number; top: number }>, width: 0, height: 0, zoom: 0.25 }
  const list = activeMonitors.value.map(monitor => ({ monitor, pos: positionFor(monitor), logicalW: monitor.width / (monitor.scale || 1), logicalH: monitor.height / (monitor.scale || 1) }))
  const minX = Math.min(...list.map(item => item.pos.x))
  const minY = Math.min(...list.map(item => item.pos.y))
  const maxX = Math.max(...list.map(item => item.pos.x + item.logicalW))
  const maxY = Math.max(...list.map(item => item.pos.y + item.logicalH))
  const totalW = Math.max(maxX - minX, 1)
  const totalH = Math.max(maxY - minY, 1)
  const zoom = Math.min(0.34, 700 / totalW, 330 / totalH)
  const items = list.map(item => ({
    monitor: item.monitor,
    x: item.pos.x,
    y: item.pos.y,
    w: Math.max(item.logicalW * zoom, 92),
    h: Math.max(item.logicalH * zoom, 64),
    left: (item.pos.x - minX) * zoom,
    top: (item.pos.y - minY) * zoom,
  }))
  return { items, width: totalW * zoom, height: totalH * zoom, zoom }
})

let drag: { name: string; pointer: number; startX: number; startY: number; x: number; y: number; zoom: number; moved: boolean } | null = null

async function load() {
  loading.value = true
  try {
    view.value = await Api.GetMonitors()
    if (!selected.value || !activeMonitors.value.some(monitor => monitor.name === selected.value)) selected.value = activeMonitors.value[0]?.name ?? ''
    syncDrafts()
  } catch (err) {
    emit('error', String(err))
  } finally { loading.value = false }
}

function syncDrafts() {
  if (!selectedMonitor.value) return
  const conf = configured.value.find(item => item.output === selectedMonitor.value?.name)
  modeDraft.value = conf?.mode ?? `${selectedMonitor.value.width}x${selectedMonitor.value.height}@${selectedMonitor.value.refreshRate.toFixed(2)}`
  scaleDraft.value = Number(conf?.scale ?? selectedMonitor.value.scale ?? 1)
  vrrDraft.value = conf?.vrr === true || conf?.vrr === 1
}

function startDrag(event: PointerEvent, monitor: RuntimeMonitor) {
  if (event.button !== 0) return
  event.preventDefault()
  selected.value = monitor.name
  const pos = positionFor(monitor)
  drag = { name: monitor.name, pointer: event.pointerId, startX: event.clientX, startY: event.clientY, x: pos.x, y: pos.y, zoom: Math.max(stage.value.zoom, 0.01), moved: false }
  ;(event.currentTarget as HTMLElement).setPointerCapture(event.pointerId)
}

function moveDrag(event: PointerEvent) {
  if (!drag || drag.pointer !== event.pointerId) return
  const zoom = drag.zoom
  const candidate = {
    x: drag.x + (event.clientX - drag.startX) / zoom,
    y: drag.y + (event.clientY - drag.startY) / zoom,
  }
  const resolved = resolveDragPosition(candidate)
  if (resolved.x !== drag.x || resolved.y !== drag.y) drag.moved = true
  positions.value = { ...positions.value, [drag.name]: resolved }
}

async function endDrag(event: PointerEvent) {
  if (!drag || drag.pointer !== event.pointerId) return
  const name = drag.name
  const moved = drag.moved
  drag = null
  // A click that only selects a monitor must not touch the configuration.
  if (!moved) return
  const pos = positions.value[name]
  if (!pos) return
  await saveField(name, 'position', `${pos.x}x${pos.y}`)
}

async function saveField(output: string, field: string, value: unknown) {
  saving.value = true
  try {
    await Api.SetMonitorField(output, field, value)
    emit('saved', t('monitors.savedField', { output, field }))
    await load()
  } catch (err) {
    emit('error', String(err))
  } finally { saving.value = false }
}

async function saveDetails() {
  if (!selectedMonitor.value) return
  const monitor = selectedMonitor.value
  const pos = positionFor(monitor)
  saving.value = true
  try {
    await Api.SetMonitorFields(monitor.name, {
      mode: modeDraft.value,
      scale: scaleDraft.value,
      vrr: vrrDraft.value ? 1 : 0,
      position: `${pos.x}x${pos.y}`,
    })
    emit('saved', t('monitors.savedAll', { output: monitor.name }))
    await load()
  } catch (err) {
    emit('error', String(err))
  } finally { saving.value = false }
}

watch(selected, syncDrafts)
watch(configured, syncDrafts)
onMounted(load)
</script>

<template>
  <div class="monitors-layout">
    <section class="monitor-summary card">
      <div class="monitor-summary-icon"><Monitor :size="19" /></div>
      <div class="monitor-summary-copy"><div class="section-kicker">{{ t('monitors.outputTopology') }}</div><h2>{{ t('monitors.activeOutputs', { count: activeMonitors.length, noun: t(activeMonitors.length === 1 ? 'monitors.output' : 'monitors.outputs') }) }}</h2><p>{{ t('monitors.subtitle') }}</p></div>
      <div class="monitor-summary-meta"><span class="tag" :class="view?.runtimeError ? 'amber' : 'lime'"><Cable :size="11" /> {{ view?.runtimeError ? t('monitors.runtimeUnavailable') : t('monitors.liveGeometry') }}</span><button class="icon-button" :title="t('app.refreshState')" :disabled="loading" @click="load"><RefreshCw :size="14" /></button></div>
    </section>

    <div v-if="view?.runtimeError" class="monitor-warning card"><CircleHelp :size="14" /> {{ view.runtimeError }} <span>{{ t('monitors.stillVisible') }}</span></div>

    <div class="monitor-workspace">
      <section class="card monitor-canvas-card">
        <div class="card-header"><div><div class="section-kicker">{{ t('monitors.arrangement') }}</div><h3 class="card-title">{{ t('monitors.displayCanvas') }}</h3><p class="card-subtitle">{{ t('monitors.canvasSubtitle') }}</p></div><span class="tag"><Move :size="10" /> {{ t('monitors.dragToMove') }}</span></div>
        <div class="monitor-stage-wrap">
          <div v-if="!stage.items.length" class="empty-state">{{ loading ? t('monitors.querying') : t('monitors.noOutputs') }}</div>
          <div v-else class="monitor-stage" :style="{ width: `${Math.max(stage.width, 120)}px`, height: `${Math.max(stage.height, 90)}px` }">
            <div v-for="item in stage.items" :key="item.monitor.name" class="monitor-panel" :class="{ selected: item.monitor.name === selected }" :style="{ left: `${item.left}px`, top: `${item.top}px`, width: `${item.w}px`, height: `${item.h}px` }" @pointerdown="startDrag($event, item.monitor)" @pointermove="moveDrag" @pointerup="endDrag">
              <div class="monitor-screen">
                <div class="monitor-screen-lines"></div>
                <span class="monitor-index">{{ String(item.monitor.id).padStart(2, '0') }}</span>
                <span class="monitor-resolution">{{ item.monitor.width }} × {{ item.monitor.height }}</span>
              </div>
              <div class="monitor-panel-label"><Grip :size="10" />{{ item.monitor.name }}</div>
            </div>
          </div>
        </div>
        <div class="monitor-canvas-legend"><span><i class="legend-display"></i> {{ t('monitors.legendActive') }}</span><span><i class="legend-selected"></i> {{ t('monitors.legendSelected') }}</span><span class="coordinate-tip">{{ t('monitors.coordTip') }}</span></div>
      </section>

      <section class="card monitor-detail-card">
        <div class="card-header"><div><div class="section-kicker">{{ t('monitors.outputDetails') }}</div><h3 class="card-title">{{ t('monitors.tuneMonitor') }}</h3></div><span v-if="selectedMonitor" class="tag lime">{{ selectedMonitor.name }}</span></div>
        <template v-if="selectedMonitor">
          <div class="monitor-picker">
            <label class="form-label">{{ t('monitors.selectedOutput') }}<select v-model="selected" class="form-select"><option v-for="monitor in activeMonitors" :key="monitor.name" :value="monitor.name">{{ monitor.name }} · {{ monitor.make }} {{ monitor.model }}</option></select></label>
          </div>
          <div class="monitor-facts">
            <div><span>{{ t('monitors.currentMode') }}</span><strong>{{ selectedMonitor.width }} × {{ selectedMonitor.height }} @ {{ selectedMonitor.refreshRate.toFixed(2) }} Hz</strong></div>
            <div><span>{{ t('monitors.physicalPosition') }}</span><strong>{{ positionFor(selectedMonitor).x }} × {{ positionFor(selectedMonitor).y }}</strong></div>
            <div><span>{{ t('monitors.activeWorkspace') }}</span><strong>{{ selectedMonitor.activeWorkspace?.name || '--' }}</strong></div>
            <div><span>{{ t('monitors.vrrStatus') }}</span><strong>{{ selectedMonitor.vrr ? t('common.enabled') : t('common.disabled') }}</strong></div>
          </div>
          <div class="monitor-fields">
            <label class="form-label">{{ t('monitors.mode') }}<select v-model="modeDraft" class="form-select mono"><option v-for="mode in modeOptions" :key="mode" :value="mode">{{ mode }}</option></select><span class="field-hint">{{ t('monitors.modesAvailable', { count: modeOptions.length }) }}</span></label>
            <div class="monitor-field-row">
              <label class="form-label">{{ t('monitors.scale') }}<input v-model.number="scaleDraft" class="form-input mono" type="number" min="0.5" max="4" step="0.05" /></label>
              <label class="vrr-toggle"><span>{{ t('monitors.vrr') }}</span><input v-model="vrrDraft" type="checkbox" /><i></i></label>
            </div>
            <button class="button-primary monitor-save" :disabled="saving" @click="saveDetails"><Save :size="13" /> {{ saving ? t('monitors.applying') : t('monitors.applySettings') }}</button>
          </div>
        </template>
        <div v-else class="empty-state">{{ t('monitors.noSelection') }}</div>
      </section>
    </div>

    <section class="card configured-list">
      <div class="card-header"><div><div class="section-kicker">{{ t('monitors.declarations') }}</div><h3 class="card-title">{{ t('monitors.configuredRules') }}</h3></div><span class="tag">{{ t('monitors.specs', { count: configured.length }) }}</span></div>
      <div v-if="!configured.length" class="empty-state">{{ t('monitors.noDeclarations') }}</div>
      <div v-for="(monitor, index) in configured" :key="`${monitor.file}:${monitor.line}:${index}`" class="configured-monitor-row">
        <span class="configured-index">{{ String(index + 1).padStart(2, '0') }}</span><code>{{ monitor.output || t('monitors.defaultOutput') }}</code><span>{{ monitor.mode || 'preferred' }}</span><span>{{ monitor.position || 'auto' }}</span><span>scale {{ monitor.scale ?? 'auto' }}</span><span class="monitor-origin">{{ monitor.file.split('/').slice(-2).join('/') }}:{{ monitor.line }}</span>
      </div>
    </section>
  </div>
</template>

<style scoped>
.monitors-layout { display: flex; flex-direction: column; gap: 13px; }
.monitor-summary { min-height: 91px; display: flex; align-items: center; gap: 13px; padding: 15px 18px; border-color: #39444b; background: linear-gradient(105deg, #20282b, #1b1e26 68%); }
.monitor-summary-icon { width: 38px; height: 38px; display: grid; place-items: center; border: 1px solid #465753; border-radius: 9px; background: #293431; color: #a5d2bc; }
.monitor-summary-copy { flex: 1; }
.monitor-summary-copy h2 { margin: 4px 0 3px; color: #e7eaed; font-size: 12px; }
.monitor-summary-copy p { margin: 0; color: #859096; font-size: 8.5px; line-height: 1.45; }
.monitor-summary-meta { display: flex; align-items: center; gap: 8px; }
.monitor-summary-meta .icon-button { border-color: #343a44; background: #22262e; }
.monitor-warning { display: flex; align-items: center; gap: 7px; padding: 10px 13px; color: #dfbf86; font-size: 8.5px; }
.monitor-warning span { color: #8f8777; }
.monitor-workspace { display: grid; grid-template-columns: minmax(0, 1.25fr) minmax(270px, .75fr); gap: 13px; align-items: start; }
.monitor-canvas-card, .monitor-detail-card, .configured-list { overflow: hidden; }
.monitor-stage-wrap { min-height: 300px; display: grid; place-items: center; overflow: auto; padding: 17px 18px; background-color: #171a21; background-image: radial-gradient(#363b45 1px, transparent 1px); background-size: 16px 16px; }
.monitor-stage { position: relative; flex: none; }
.monitor-panel { position: absolute; display: flex; flex-direction: column; gap: 4px; cursor: grab; touch-action: none; user-select: none; }
.monitor-panel:active { cursor: grabbing; }
.monitor-screen { position: relative; flex: 1; display: grid; place-items: center; overflow: hidden; border: 1px solid #536944; border-radius: 6px; background: linear-gradient(137deg, #3b4b2d, #222d25 63%, #24343c); box-shadow: 0 7px 22px #0006; }
.monitor-panel.selected .monitor-screen { border-color: #c4f274; box-shadow: 0 0 0 2px #c4f27430, 0 8px 24px #0007; }
.monitor-screen-lines { position: absolute; inset: 0; background: repeating-linear-gradient(0deg, transparent, transparent 12px, #ffffff05 13px); }
.monitor-index { position: absolute; top: 8px; left: 9px; color: #d3edab99; font: 8px var(--mono); }
.monitor-resolution { color: #d8e4ca; font: 9px var(--mono); }
.monitor-panel-label { display: flex; align-items: center; gap: 4px; color: #b7c1b0; font: 7px var(--mono); }
.monitor-canvas-legend { display: flex; align-items: center; gap: 14px; padding: 10px 17px; border-top: 1px solid #2b2f38; color: #737b89; font-size: 7.5px; }
.monitor-canvas-legend span { display: flex; align-items: center; gap: 5px; }
.monitor-canvas-legend i { width: 7px; height: 7px; border: 1px solid #799755; border-radius: 2px; background: #39472d; }
.monitor-canvas-legend .legend-selected { border-color: var(--lime); background: #586f38; }
.monitor-canvas-legend .coordinate-tip { margin-left: auto; color: #828a98; font: 7px var(--mono); }
.monitor-picker, .monitor-fields { padding: 0 16px; }
.monitor-picker { padding-bottom: 12px; }
.monitor-facts { display: grid; grid-template-columns: 1fr 1fr; gap: 1px; border-top: 1px solid #2b2f39; border-bottom: 1px solid #2b2f39; background: #2b2f39; }
.monitor-facts > div { min-width: 0; display: flex; flex-direction: column; gap: 5px; padding: 9px 10px; background: #1b1e26; }
.monitor-facts span { color: #737b88; font: 6.5px var(--mono); letter-spacing: .08em; }
.monitor-facts strong { overflow: hidden; color: #ced2da; font: 8px var(--mono); text-overflow: ellipsis; white-space: nowrap; }
.monitor-fields { display: flex; flex-direction: column; gap: 10px; padding-top: 13px; padding-bottom: 15px; }
.monitor-fields .form-input { min-height: 31px; }
.field-hint { color: #6d7481; font: 7.5px var(--mono); font-weight: 400; }
.monitor-field-row { display: grid; grid-template-columns: .8fr 1.2fr; align-items: end; gap: 9px; }
.vrr-toggle { min-height: 34px; display: flex; align-items: center; gap: 7px; color: #8e95a2; font-size: 8px; cursor: pointer; }
.vrr-toggle input { position: absolute; opacity: 0; }
.vrr-toggle i { width: 29px; height: 16px; position: relative; order: -1; border: 1px solid #464b56; border-radius: 10px; background: #292d36; }
.vrr-toggle i::after { content: ''; position: absolute; top: 2px; left: 2px; width: 10px; height: 10px; border-radius: 50%; background: #9299a6; transition: .16s; }
.vrr-toggle input:checked + i { border-color: #8aa958; background: #526a35; }
.vrr-toggle input:checked + i::after { left: 15px; background: var(--lime); }
.monitor-save { width: 100%; min-height: 32px; }
.configured-monitor-row { min-height: 39px; display: grid; grid-template-columns: 23px minmax(100px, 1fr) minmax(140px, 1fr) 70px 75px minmax(120px, .8fr); align-items: center; gap: 8px; padding: 0 16px; border-top: 1px solid #2b2f38; }
.configured-index { color: #838c6f; font: 7px var(--mono); }
.configured-monitor-row code, .configured-monitor-row > span { overflow: hidden; color: #bbc1cb; font: 7.5px var(--mono); text-overflow: ellipsis; white-space: nowrap; }
.configured-monitor-row code { color: #d8e3c7; }
.configured-monitor-row .monitor-origin { color: #707785; text-align: right; }
@media (max-width: 1000px) { .monitor-workspace { grid-template-columns: 1fr; } }
@media (max-width: 750px) { .configured-monitor-row { grid-template-columns: 20px minmax(70px, 1fr) 1fr 60px; } .configured-monitor-row > span:nth-of-type(3), .configured-monitor-row .monitor-origin { display: none; } .monitor-summary-meta .tag { display: none; } }
</style>
