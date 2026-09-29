<script setup lang="ts">
import { computed, ref } from 'vue'
import { useI18n } from '../i18n'
import { Activity, Plus, Sparkles } from '@lucide/vue'
import * as Api from '../../bindings/github.com/juniorgs/hyprmod/internal/api/service'
import type { Document, Animation, Curve } from '../../bindings/github.com/juniorgs/hyprmod/internal/config/models'

const { t } = useI18n()

const props = defineProps<{ document: Document | null }>()
const emit = defineEmits<{ saved: [message?: string]; error: [message: string] }>()
const curveName = ref('hyprmodEase')
const curveType = ref<'bezier' | 'spring'>('bezier')
const curveJSON = ref('{ "type": "bezier", "points": [[0.22, 1], [0.36, 1]] }')
const animationJSON = ref('{ "leaf": "windows", "enabled": true, "speed": 4, "bezier": "hyprmodEase" }')
const customCurves = computed(() => (props.document?.curves ?? []))
const animations = computed(() => (props.document?.animations ?? []))
const curvePath = computed(() => {
  const curve = customCurves.value.find((item: Curve) => item.name === curveName.value)
  const points = curve?.spec?.points
  if (Array.isArray(points) && points.length >= 2) {
    const p1 = points[0] as number[]
    const p2 = points[1] as number[]
    if (p1.length >= 2 && p2.length >= 2) return `M 0 86 C ${p1[0] * 100} ${86 - p1[1] * 72}, ${p2[0] * 100} ${86 - p2[1] * 72}, 100 14`
  }
  return 'M 0 86 C 22 18, 64 14, 100 14'
})

async function addCurve() {
  try {
    const spec = JSON.parse(curveJSON.value)
    spec.type = curveType.value
    await Api.AddCurve(curveName.value, spec)
    emit('saved', t('animations.curveAdded', { name: curveName.value }))
  } catch (err) {
    emit('error', `Could not add curve: ${String(err)}`)
  }
}

async function addAnimation() {
  try {
    const spec = JSON.parse(animationJSON.value)
    await Api.AddAnimation(spec)
    emit('saved', t('animations.animationAdded', { leaf: spec.leaf }))
  } catch (err) {
    emit('error', `Could not add animation: ${String(err)}`)
  }
}
</script>

<template>
  <div class="animation-layout">
    <section class="animation-hero card">
      <div class="animation-hero-copy"><div class="section-kicker">{{ t('animations.motionSystem') }}</div><h2>{{ t('animations.headline') }}</h2><p>{{ t('animations.subtitle') }}</p></div>
      <div class="curve-preview">
        <div class="curve-preview-top"><span>{{ curveName }}</span><span>0 ms <i></i> 100%</span></div>
        <svg viewBox="0 0 100 100" preserveAspectRatio="none" aria-label="Bezier curve preview"><path class="curve-axis" d="M 0 86 L 100 14" /><path class="curve-line" :d="curvePath" /></svg>
        <div class="curve-preview-foot"><span>{{ t('animations.start') }}</span><span>{{ t('animations.response') }}</span><span>{{ t('animations.end') }}</span></div>
      </div>
      <div class="motion-orbit"><Sparkles :size="22" /></div>
    </section>

    <div class="animation-columns">
      <section class="card animation-list-card">
        <div class="card-header"><div><div class="section-kicker">{{ t('animations.customCurves') }}</div><h3 class="card-title">{{ t('animations.curveCatalog') }}</h3></div><span class="tag">{{ t('animations.curves', { count: customCurves.length }) }}</span></div>
        <div v-if="!customCurves.length" class="empty-state">{{ t('animations.noCurves') }}</div>
        <article v-for="(curve, index) in customCurves" :key="`${curve.file}:${curve.line}:${curve.name}`" class="curve-row" :class="{ selected: curve.name === curveName }" @click="curveName = curve.name">
          <span class="curve-row-index">{{ String(index + 1).padStart(2, '0') }}</span>
          <div class="curve-row-main"><strong>{{ curve.name }}</strong><span>{{ curve.spec?.type || 'curve' }} · {{ curve.file.split('/').slice(-2).join('/') }}:{{ curve.line }}</span></div>
          <span class="curve-type-pill">{{ curve.spec?.type || 'custom' }}</span>
        </article>
      </section>

      <section class="card animation-list-card">
        <div class="card-header"><div><div class="section-kicker">{{ t('animations.leaves') }}</div><h3 class="card-title">{{ t('animations.resolvedAnimations') }}</h3></div><span class="tag">{{ t('animations.leavesCount', { count: animations.length }) }}</span></div>
        <div v-if="!animations.length" class="empty-state">{{ t('animations.noAnimations') }}</div>
        <article v-for="(animation, index) in animations" :key="`${animation.file}:${animation.line}:${animation.leaf}:${index}`" class="animation-row">
          <span class="animation-index">{{ String(index + 1).padStart(2, '0') }}</span>
          <div class="animation-row-main"><strong>{{ animation.leaf }}</strong><span>{{ animation.file.split('/').slice(-2).join('/') }}:{{ animation.line }}</span></div>
          <span class="animation-speed">{{ animation.spec?.speed ?? '--' }}<small>SPD</small></span>
          <span class="animation-switch" :class="{ on: animation.spec?.enabled !== false }"></span>
        </article>
      </section>
    </div>

    <div class="animation-editors">
      <section class="card animation-editor-card">
        <div class="card-header"><div><div class="section-kicker">{{ t('animations.defineCurve') }}</div><h3 class="card-title">{{ t('animations.newCurve') }}</h3></div><Activity :size="15" class="editor-icon" /></div>
        <div class="animation-form">
          <div class="animation-form-row"><label class="form-label">{{ t('animations.curveName') }}<input v-model="curveName" class="form-input mono" /></label><label class="form-label">{{ t('animations.curveModel') }}<select v-model="curveType" class="form-select"><option value="bezier">{{ t('animations.bezier') }}</option><option value="spring">{{ t('animations.spring') }}</option></select></label></div>
          <label class="form-label">{{ t('animations.curveParams') }}<textarea v-model="curveJSON" class="form-textarea" spellcheck="false"></textarea></label>
          <button class="button-primary" @click="addCurve"><Plus :size="13" /> {{ t('animations.addingCurve') }}</button>
        </div>
      </section>
      <section class="card animation-editor-card">
        <div class="card-header"><div><div class="section-kicker">{{ t('animations.defineLeaf') }}</div><h3 class="card-title">{{ t('animations.newAnimation') }}</h3></div><Activity :size="15" class="editor-icon" /></div>
        <div class="animation-form">
          <label class="form-label">{{ t('animations.animationSpec') }} <span class="label-hint">{{ t('animations.luaFields') }}</span><textarea v-model="animationJSON" class="form-textarea animation-json" spellcheck="false"></textarea></label>
          <button class="button-primary" @click="addAnimation"><Plus :size="13" /> {{ t('animations.addingAnimation') }}</button>
        </div>
      </section>
    </div>
  </div>
</template>

<style scoped>
.animation-layout { display: flex; flex-direction: column; gap: 13px; }
.animation-hero { position: relative; min-height: 180px; overflow: hidden; display: flex; align-items: center; justify-content: space-between; gap: 25px; padding: 20px 25px; border-color: #373a46; background: radial-gradient(circle at 84% 55%, #353d4d66, transparent 34%), linear-gradient(115deg, #20232d, #1b1e27); }
.animation-hero-copy { max-width: 380px; position: relative; z-index: 1; }
.animation-hero-copy .section-kicker { color: #a4b988; }
.animation-hero-copy h2 { max-width: 350px; margin: 8px 0 7px; color: #eff0f3; font-size: 17px; line-height: 1.25; letter-spacing: -.6px; }
.animation-hero-copy p { margin: 0; color: #9198a6; font-size: 9px; line-height: 1.65; }
.curve-preview { position: relative; z-index: 1; width: 270px; height: 121px; padding: 10px 11px 8px; border: 1px solid #424752; border-radius: 8px; background: #151820c9; }
.curve-preview-top, .curve-preview-foot { display: flex; justify-content: space-between; color: #868e9d; font: 7px var(--mono); }
.curve-preview-top span:first-child { color: #c3dc9a; }
.curve-preview-top i { display: inline-block; width: 4px; height: 4px; margin: 0 5px 1px; border-radius: 50%; background: var(--lime); }
.curve-preview svg { width: 100%; height: 68px; margin: 4px 0 1px; overflow: visible; }
.curve-axis { fill: none; stroke: #4c5360; stroke-width: .65; stroke-dasharray: 2 3; }
.curve-line { fill: none; stroke: var(--lime); stroke-width: 1.7; vector-effect: non-scaling-stroke; filter: drop-shadow(0 0 3px #c4f27488); }
.curve-preview-foot { color: #626a78; font-size: 6px; }
.motion-orbit { position: absolute; right: -22px; top: -28px; width: 125px; height: 125px; display: grid; place-items: center; border: 1px solid #d1bcff12; border-radius: 50%; color: #bca3f35c; }
.motion-orbit::after { content: ''; position: absolute; inset: 17px; border: 1px solid #d1bcff12; border-radius: 50%; }
.animation-columns, .animation-editors { display: grid; grid-template-columns: 1fr 1fr; align-items: start; gap: 13px; }
.animation-list-card { overflow: hidden; }
.curve-row, .animation-row { min-height: 43px; display: flex; align-items: center; gap: 10px; padding: 0 14px; border-top: 1px solid #2a2e37; cursor: pointer; }
.curve-row:hover, .curve-row.selected { background: #242932; }
.curve-row-index, .animation-index { color: #78806a; font: 8px var(--mono); }
.curve-row-main, .animation-row-main { min-width: 0; display: flex; flex: 1; flex-direction: column; gap: 3px; }
.curve-row-main strong, .animation-row-main strong { overflow: hidden; color: #d8dce3; font: 9px var(--mono); text-overflow: ellipsis; white-space: nowrap; }
.curve-row-main span, .animation-row-main span { overflow: hidden; color: #78808e; font: 7px var(--mono); text-overflow: ellipsis; white-space: nowrap; }
.curve-type-pill { padding: 4px 6px; border: 1px solid #383d47; border-radius: 4px; color: #9fa6b3; font: 7px var(--mono); text-transform: uppercase; }
.animation-speed { display: flex; align-items: baseline; gap: 4px; color: #c3d99f; font: 9px var(--mono); }
.animation-speed small { color: #707887; font-size: 6px; }
.animation-switch { width: 8px; height: 8px; border: 1px solid #545a67; border-radius: 50%; background: #393d47; }
.animation-switch.on { border-color: #819d53; background: var(--lime); box-shadow: 0 0 8px #c4f27455; }
.animation-editor-card { overflow: hidden; }
.editor-icon { color: #8e9f71; }
.animation-form { display: flex; flex-direction: column; gap: 11px; padding: 0 18px 17px; }
.animation-form-row { display: grid; grid-template-columns: 1fr 1fr; gap: 9px; }
.animation-form .form-textarea { min-height: 82px; }
.animation-form .animation-json { min-height: 127px; }
.animation-form > button { align-self: flex-start; }
.label-hint { color: #69717f; font: 8px var(--mono); font-weight: 400; }
@media (max-width: 900px) { .animation-hero { align-items: flex-start; flex-direction: column; } .curve-preview { width: min(100%, 400px); } .animation-columns, .animation-editors { grid-template-columns: 1fr; } }
</style>
