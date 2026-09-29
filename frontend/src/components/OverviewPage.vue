<script setup lang="ts">
import { computed } from 'vue'
import { useI18n } from '../i18n'
import { ArrowRight, CircleAlert, FileCode2, Keyboard, Monitor, ShieldCheck, Sparkles } from '@lucide/vue'
import type { Dashboard } from '../../bindings/github.com/juniorgs/hyprmod/internal/api/models'
import type { Document } from '../../bindings/github.com/juniorgs/hyprmod/internal/config/models'

const { t } = useI18n()

const props = defineProps<{
  dashboard: Dashboard | null
  document: Document | null
  loading: boolean
}>()
const emit = defineEmits<{
  navigate: [id: 'variables' | 'monitors' | 'snapshots']
  refresh: []
}>()

const diagnostics = computed(() => props.document?.diagnostics ?? [])
const optionCount = computed(() => props.document?.values?.length ?? 0)
const bindCount = computed(() => props.document?.binds?.length ?? 0)
const monitorCount = computed(() => props.document?.monitors?.length ?? 0)
const healthLabel = computed(() => {
  if (!props.dashboard?.online) return t('app.offline')
  if (!props.dashboard.supported) return t('overview.needsAttention')
  if (diagnostics.value.some(item => item.severity === 'error')) return t('overview.configErrors')
  return t('overview.healthy')
})
</script>

<template>
  <div class="overview-layout">
    <section class="overview-hero card">
      <div class="hero-copy">
        <div class="hero-orbit"><span></span><span></span><span></span></div>
        <div class="section-kicker">{{ t('overview.sessionStatus') }}</div>
        <h2>{{ healthLabel }}</h2>
        <p>{{ dashboard?.message || t('overview.needsAttention') }}</p>
        <div class="hero-actions">
          <button class="button-primary" @click="emit('navigate', 'variables')">
            {{ t('overview.explore') }} <ArrowRight :size="14" />
          </button>
          <button class="button-quiet" @click="emit('refresh')">{{ t('app.refreshState') }}</button>
        </div>
      </div>
      <div class="hero-status-panel">
        <div class="status-ring" :class="{ offline: !dashboard?.online, warning: dashboard?.online && !dashboard?.supported }">
          <ShieldCheck v-if="dashboard?.online && dashboard?.supported" :size="27" :stroke-width="1.5" />
          <CircleAlert v-else :size="27" :stroke-width="1.5" />
        </div>
        <div class="status-indicator"><span></span>{{ dashboard?.online ? t('overview.socketReachable') : t('overview.noSocket') }}</div>
        <div class="status-provider">{{ dashboard?.configProvider || '--' }}</div>
        <div class="status-backend">{{ dashboard?.backend || '--' }}</div>
      </div>
      <div class="hero-index">01 <span>/</span> {{ t('overview.controlPlane') }}</div>
    </section>

    <section class="overview-stats">
      <article class="stat-card card">
        <div class="stat-top"><span class="stat-icon lime"><FileCode2 :size="15" /></span><span class="section-kicker">{{ t('overview.configuredValues') }}</span></div>
        <strong>{{ loading ? '...' : optionCount }}</strong>
        <span class="stat-caption">{{ t('overview.valuesCaption') }}</span>
      </article>
      <article class="stat-card card">
        <div class="stat-top"><span class="stat-icon blue"><Keyboard :size="15" /></span><span class="section-kicker">{{ t('overview.keybinds') }}</span></div>
        <strong>{{ loading ? '...' : bindCount }}</strong>
        <span class="stat-caption">{{ t('overview.bindsCaption') }}</span>
      </article>
      <article class="stat-card card">
        <div class="stat-top"><span class="stat-icon amber"><Monitor :size="15" /></span><span class="section-kicker">{{ t('overview.outputs') }}</span></div>
        <strong>{{ loading ? '...' : monitorCount }}</strong>
        <span class="stat-caption">{{ t('overview.outputsCaption') }}</span>
      </article>
    </section>

    <div class="overview-columns">
      <section class="card overview-files">
        <div class="card-header">
          <div>
            <div class="section-kicker">{{ t('overview.sourceGraph') }}</div>
            <h3 class="card-title">{{ t('overview.loadedModules') }}</h3>
            <p class="card-subtitle">{{ t('overview.sourceGraphSubtitle') }}</p>
          </div>
          <span class="tag">{{ t('overview.fileCount', { count: document?.files?.length ?? 0 }) }}</span>
        </div>
        <div v-if="document?.files?.length" class="file-list">
          <div v-for="file in document.files.slice(0, 7)" :key="file.path" class="file-row">
            <span class="file-type">LUA</span>
            <div class="file-name"><strong>{{ file.rel }}</strong><span>{{ file.path }}</span></div>
            <span class="file-bytes">{{ file.bytes.toLocaleString() }} B</span>
          </div>
          <div v-if="document.files.length > 7" class="file-more">{{ t('overview.moreModules', { count: document.files.length - 7 }) }}</div>
        </div>
        <div v-else class="empty-state">{{ loading ? t('overview.reading') : t('overview.noFiles') }}</div>
      </section>

      <section class="card overview-safety">
        <div class="card-header">
          <div>
            <div class="section-kicker">{{ t('overview.beforeYouEdit') }}</div>
            <h3 class="card-title">{{ t('overview.guardRails') }}</h3>
            <p class="card-subtitle">{{ t('overview.guardRailsSubtitle') }}</p>
          </div>
          <span class="safety-glyph"><ShieldCheck :size="19" /></span>
        </div>
        <div class="safety-steps">
          <div><span>01</span><strong>{{ t('overview.step1') }}</strong><small>{{ t('overview.step1Sub') }}</small></div>
          <div><span>02</span><strong>{{ t('overview.step2') }}</strong><small>{{ t('overview.step2Sub') }}</small></div>
          <div><span>03</span><strong>{{ t('overview.step3') }}</strong><small>{{ t('overview.step3Sub') }}</small></div>
        </div>
        <button class="safety-link" @click="emit('navigate', 'snapshots')">{{ t('overview.openSnapshots') }} <ArrowRight :size="13" /></button>
      </section>
    </div>

    <section v-if="diagnostics.length" class="diagnostic-panel card">
      <div class="card-header">
        <div>
          <div class="section-kicker">{{ t('overview.validation') }}</div>
          <h3 class="card-title">{{ t('overview.diagnostics') }}</h3>
        </div>
        <span class="tag red">{{ t('overview.issues', { count: diagnostics.length }) }}</span>
      </div>
      <div v-for="(item, index) in diagnostics" :key="`${item.file}-${item.line}-${index}`" class="diagnostic-row">
        <CircleAlert :size="14" />
        <span>{{ item.message }}</span>
        <small v-if="item.file">{{ item.file }}{{ item.line ? `:${item.line}` : '' }}</small>
      </div>
    </section>

    <section class="quick-start-strip">
      <div class="quick-start-mark"><Sparkles :size="16" /></div>
      <div><strong>{{ t('overview.sourceAware') }}</strong><span>{{ t('overview.sourceAwareBody') }}</span></div>
      <button @click="emit('navigate', 'monitors')">{{ t('overview.arrangeOutputs') }} <ArrowRight :size="13" /></button>
    </section>
  </div>
</template>

<style scoped>
.overview-layout { display: flex; flex-direction: column; gap: 15px; }
.overview-hero { position: relative; min-height: 222px; overflow: hidden; display: flex; align-items: center; justify-content: space-between; padding: 27px 31px; border-color: #3b4430; background: radial-gradient(circle at 72% 12%, #38442b77 0, transparent 36%), linear-gradient(118deg, #22291e, #1b1f25 58%, #1b1d26); }
.hero-copy { position: relative; z-index: 1; max-width: 580px; }
.hero-copy .section-kicker { color: #a7c67a; }
.hero-copy h2 { margin: 10px 0 7px; color: #f2f3ee; font-size: 22px; letter-spacing: -.75px; }
.hero-copy p { max-width: 510px; margin: 0; color: #a6ad9e; font-size: 10.5px; line-height: 1.7; }
.hero-actions { display: flex; align-items: center; gap: 9px; margin-top: 20px; }
.hero-status-panel { position: relative; z-index: 1; width: 192px; display: flex; flex-direction: column; align-items: center; padding: 15px 12px 13px; border: 1px solid #434b3a; border-radius: 11px; background: #171b18b3; }
.status-ring { width: 51px; height: 51px; display: grid; place-items: center; margin-bottom: 12px; border: 1px solid #748f4e; border-radius: 50%; background: #303e25; color: var(--lime); box-shadow: 0 0 26px #c4f2741a; }
.status-ring.offline { border-color: #624044; background: #352327; color: #ed9ba0; }
.status-ring.warning { border-color: #6a5636; background: #382e1e; color: var(--amber); }
.status-indicator { display: flex; align-items: center; gap: 6px; color: #aab493; font: 8px var(--mono); letter-spacing: .09em; }
.status-indicator span { width: 5px; height: 5px; border-radius: 50%; background: var(--lime); }
.status-provider { margin-top: 10px; color: #ecefe7; font: 11px var(--mono); }
.status-backend { margin-top: 3px; color: #858d7c; font-size: 9px; }
.hero-index { position: absolute; right: 32px; bottom: 12px; color: #68725b; font: 8px var(--mono); letter-spacing: .1em; }
.hero-index span { margin: 0 4px; color: #46503d; }
.hero-orbit { position: absolute; right: 15%; top: -91px; width: 240px; height: 240px; border: 1px solid #c4f27412; border-radius: 50%; }
.hero-orbit::before, .hero-orbit::after { content: ''; position: absolute; inset: 21px; border: 1px solid #c4f2740d; border-radius: 50%; }
.hero-orbit::after { inset: 52px; }
.hero-orbit span { position: absolute; width: 5px; height: 5px; border-radius: 50%; background: var(--lime); }
.hero-orbit span:nth-child(1) { left: 37px; bottom: 33px; opacity: .7; }
.hero-orbit span:nth-child(2) { right: 26px; top: 70px; opacity: .35; }
.hero-orbit span:nth-child(3) { right: 52px; bottom: 21px; opacity: .5; }
.overview-stats { display: grid; grid-template-columns: repeat(3, minmax(0, 1fr)); gap: 12px; }
.stat-card { min-height: 108px; padding: 14px 16px; }
.stat-top { display: flex; align-items: center; gap: 8px; }
.stat-top .section-kicker { flex: 1; }
.stat-icon { width: 26px; height: 26px; display: grid; place-items: center; border-radius: 7px; }
.stat-icon.lime { background: #2d3924; color: var(--lime); }
.stat-icon.blue { background: #243146; color: var(--blue); }
.stat-icon.amber { background: #3b3020; color: var(--amber); }
.stat-card > strong { display: block; margin-top: 10px; color: #f2f3f5; font: 22px var(--mono); letter-spacing: -1px; }
.stat-caption { display: block; margin-top: 3px; color: #7f8694; font-size: 9px; }
.overview-columns { display: grid; grid-template-columns: 1.12fr .88fr; gap: 13px; }
.overview-files, .overview-safety { min-width: 0; }
.file-list { padding: 0 16px 10px; }
.file-row { min-height: 41px; display: flex; align-items: center; gap: 10px; border-top: 1px solid #282c36; }
.file-type { width: 26px; height: 17px; display: grid; place-items: center; border-radius: 4px; background: #30352b; color: #b6d48a; font: 7px var(--mono); }
.file-name { min-width: 0; display: flex; flex: 1; flex-direction: column; gap: 2px; }
.file-name strong { overflow: hidden; color: #d2d5dd; font: 9.5px var(--mono); text-overflow: ellipsis; white-space: nowrap; }
.file-name span { overflow: hidden; color: #717887; font: 8px var(--mono); text-overflow: ellipsis; white-space: nowrap; }
.file-bytes { color: #6f7684; font: 8px var(--mono); }
.file-more { padding: 8px 0 1px; color: #8d957a; font-size: 9px; }
.safety-glyph { width: 31px; height: 31px; display: grid; place-items: center; border: 1px solid #49583a; border-radius: 8px; background: #293321; color: var(--lime); }
.safety-steps { padding: 0 19px; }
.safety-steps > div { display: grid; grid-template-columns: 26px 1fr; gap: 1px 9px; padding: 9px 0; border-top: 1px solid #292d37; }
.safety-steps > div > span { grid-row: span 2; color: #8ea56d; font: 8px var(--mono); padding-top: 2px; }
.safety-steps strong { color: #d2d5dc; font-size: 9.5px; }
.safety-steps small { color: #7b8290; font-size: 8px; }
.safety-link { width: calc(100% - 38px); min-height: 29px; display: flex; align-items: center; justify-content: space-between; margin: 5px 19px 15px; padding: 0; border: 0; background: transparent; color: #bdd992; text-align: left; cursor: pointer; font-size: 9px; font-weight: 700; }
.diagnostic-panel { border-color: #593c3f; background: #211a1e; }
.diagnostic-row { display: flex; align-items: flex-start; gap: 9px; padding: 10px 19px; border-top: 1px solid #38272c; color: #e3afb0; font-size: 9px; }
.diagnostic-row span { flex: 1; }
.diagnostic-row small { color: #9f777b; font: 8px var(--mono); }
.quick-start-strip { min-height: 57px; display: flex; align-items: center; gap: 11px; padding: 0 15px; border: 1px solid #313641; border-radius: 9px; background: #191c23; }
.quick-start-mark { width: 26px; height: 26px; display: grid; place-items: center; border-radius: 7px; background: #2d3524; color: var(--lime); }
.quick-start-strip > div:nth-child(2) { display: flex; flex: 1; flex-direction: column; gap: 3px; }
.quick-start-strip strong { color: #d5d8df; font-size: 9.5px; }
.quick-start-strip span { color: #7d8491; font-size: 8.5px; }
.quick-start-strip button { display: flex; align-items: center; gap: 7px; border: 0; background: transparent; color: #bfd98c; cursor: pointer; font-size: 9px; font-weight: 700; }
@media (max-width: 980px) { .overview-columns { grid-template-columns: 1fr; } }
@media (max-width: 720px) { .overview-stats { grid-template-columns: 1fr; } .hero-status-panel, .hero-index { display: none; } .overview-hero { padding: 22px; } }
</style>
