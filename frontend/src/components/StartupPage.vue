<script setup lang="ts">
import { computed, ref } from 'vue'
import { useI18n } from '../i18n'
import { ArrowUpRight, Plus, Terminal, Variable } from '@lucide/vue'
import * as Api from '../../bindings/github.com/juniorgs/hyprmod/internal/api/service'
import type { Document } from '../../bindings/github.com/juniorgs/hyprmod/internal/config/models'

const { t } = useI18n()

const props = defineProps<{ document: Document | null }>()
const emit = defineEmits<{ saved: [message?: string]; error: [message: string] }>()
const envs = computed(() => props.document?.envs ?? [])
const events = computed(() => props.document?.events ?? [])
const envName = ref('')
const envValue = ref('')
const startupCommand = ref('')

async function addEnv() {
  if (!envName.value.trim()) return emit('error', t('common.error'))
  try {
    await Api.AddEnvironment(envName.value, envValue.value)
    emit('saved', `${envName.value} · ${t('startup.add')}`)
    envName.value = ''
    envValue.value = ''
  } catch (err) { emit('error', String(err)) }
}

async function addCommand() {
  if (!startupCommand.value.trim()) return emit('error', t('common.error'))
  try {
    await Api.AddAutostart(startupCommand.value)
    emit('saved', t('startup.addCommand'))
    startupCommand.value = ''
  } catch (err) { emit('error', String(err)) }
}
</script>

<template>
  <div class="startup-layout">
    <section class="startup-banner card">
      <div class="startup-banner-icon"><Terminal :size="19" /></div>
      <div><div class="section-kicker">{{ t('startup.hyprlandEvents') }}</div><h2>{{ t('startup.headline') }}</h2><p>{{ t('startup.subtitle') }}</p></div>
      <span class="tag">{{ t('startup.eventHooks', { count: events.length }) }}</span>
    </section>

    <div class="startup-columns">
      <section class="card startup-list-card">
        <div class="card-header"><div><div class="section-kicker">{{ t('startup.environment') }}</div><h3 class="card-title">{{ t('startup.exportedVars') }}</h3><p class="card-subtitle">{{ t('startup.exportedSubtitle') }}</p></div><span class="tag"><Variable :size="11" /> {{ envs.length }}</span></div>
        <div v-if="!envs.length" class="empty-state">{{ t('startup.noEnvs') }}</div>
        <div v-for="(env, index) in envs" :key="`${env.file}:${env.line}:${index}`" class="env-row">
          <code>{{ env.name }}</code><span class="env-equals">=</span><code class="env-value">{{ env.value }}</code><small>{{ env.file.split('/').slice(-2).join('/') }}:{{ env.line }}</small>
        </div>
        <form class="startup-form" @submit.prevent="addEnv">
          <div class="startup-form-title"><span>{{ t('startup.addVariable') }}</span><a href="https://wiki.hypr.land/Configuring/Advanced-and-Cool/Environment-variables/" target="_blank" rel="noreferrer">{{ t('startup.docs') }} <ArrowUpRight :size="11" /></a></div>
          <div class="startup-form-row"><input v-model="envName" class="form-input mono" placeholder="QT_QPA_PLATFORM" /><input v-model="envValue" class="form-input mono" placeholder="wayland" /><button class="button-primary" type="submit"><Plus :size="13" /> {{ t('startup.add') }}</button></div>
        </form>
      </section>

      <section class="card startup-list-card">
        <div class="card-header"><div><div class="section-kicker">{{ t('startup.events') }}</div><h3 class="card-title">{{ t('startup.startupHooks') }}</h3><p class="card-subtitle">{{ t('startup.hooksSubtitle') }}</p></div><span class="tag">{{ t('startup.hooks', { count: events.length }) }}</span></div>
        <div v-if="!events.length" class="empty-state">{{ t('startup.noEvents') }}</div>
        <div v-for="(event, index) in events" :key="`${event.file}:${event.line}:${index}`" class="event-row">
          <span class="event-index">{{ String(index + 1).padStart(2, '0') }}</span>
          <div><code>{{ event.event }}</code><small>{{ event.file.split('/').slice(-2).join('/') }}:{{ event.line }}</small></div>
          <span class="event-hook-dot"></span>
        </div>
        <form class="startup-form" @submit.prevent="addCommand">
          <div class="startup-form-title"><span>{{ t('startup.addStartup') }}</span><span class="tag amber">hyprland.start</span></div>
          <div class="startup-form-row"><input v-model="startupCommand" class="form-input mono" placeholder="waybar & hyprpaper" /><button class="button-primary" type="submit"><Plus :size="13" /> {{ t('startup.addCommand') }}</button></div>
        </form>
      </section>
    </div>
  </div>
</template>

<style scoped>
.startup-layout { display: flex; flex-direction: column; gap: 13px; }
.startup-banner { min-height: 94px; display: flex; align-items: center; gap: 13px; padding: 15px 18px; border-color: #3c4432; background: linear-gradient(105deg, #22291e, #1b1e26 65%); }
.startup-banner-icon { width: 37px; height: 37px; display: grid; place-items: center; border: 1px solid #4a583a; border-radius: 9px; background: #303a25; color: var(--lime); }
.startup-banner > div:nth-child(2) { flex: 1; }
.startup-banner h2 { margin: 4px 0 3px; color: #e8ebe4; font-size: 12px; }
.startup-banner p { margin: 0; color: #899080; font-size: 8px; line-height: 1.55; }
.startup-columns { display: grid; grid-template-columns: 1fr 1fr; gap: 13px; align-items: start; }
.startup-list-card { overflow: hidden; }
.env-row { min-height: 37px; display: flex; align-items: center; gap: 7px; padding: 0 16px; border-top: 1px solid #2a2e37; }
.env-row code { color: #c9df9b; font: 8px var(--mono); }
.env-row .env-value { overflow: hidden; flex: 1; color: #aeb5c1; text-overflow: ellipsis; white-space: nowrap; }
.env-equals { color: #596170; font: 8px var(--mono); }
.env-row small { color: #6e7582; font: 7px var(--mono); }
.event-row { min-height: 42px; display: flex; align-items: center; gap: 11px; padding: 0 16px; border-top: 1px solid #2a2e37; }
.event-index { color: #819166; font: 8px var(--mono); }
.event-row > div { min-width: 0; display: flex; flex: 1; flex-direction: column; gap: 3px; }
.event-row code { color: #d3d6de; font: 8px var(--mono); }
.event-row small { color: #737a87; font: 7px var(--mono); }
.event-hook-dot { width: 6px; height: 6px; border-radius: 50%; background: #a3bf77; }
.startup-form { padding: 11px 15px 14px; border-top: 1px solid #30343e; background: #1a1d24; }
.startup-form-title { display: flex; align-items: center; justify-content: space-between; margin-bottom: 8px; color: #7d8593; font: 7px var(--mono); letter-spacing: .1em; }
.startup-form-title a { display: flex; align-items: center; gap: 4px; color: #91ab69; text-decoration: none; letter-spacing: 0; }
.startup-form-row { display: flex; gap: 6px; }
.startup-form-row .form-input { min-width: 0; min-height: 31px; flex: 1; padding: 6px 8px; font-size: 8px; }
.startup-form-row .button-primary { min-height: 31px; padding-inline: 9px; font-size: 8px; }
@media (max-width: 840px) { .startup-columns { grid-template-columns: 1fr; } .startup-banner > .tag { display: none; } }
</style>
