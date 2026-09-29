<script setup lang="ts">
import { onMounted, ref } from 'vue'
import { useI18n } from '../i18n'
import { ArchiveRestore, Clock3, FileCode2, Plus, RotateCcw, Trash2 } from '@lucide/vue'
import * as Api from '../../bindings/github.com/juniorgs/hyprmod/internal/api/service'
import type { Snapshot } from '../../bindings/github.com/juniorgs/hyprmod/internal/snapshots/models'

const { t } = useI18n()

const emit = defineEmits<{ saved: [message?: string]; error: [message: string] }>()
const snapshots = ref<Snapshot[]>([])
const loading = ref(true)
const busy = ref('')
const loadError = ref('')

async function load() {
  loading.value = true
  loadError.value = ''
  try {
    snapshots.value = await Api.ListSnapshots() ?? []
  } catch (err) {
    loadError.value = String(err)
  } finally {
    loading.value = false
  }
}

async function create() {
  busy.value = 'create'
  try {
    const result = await Api.CreateSnapshot()
    emit('saved', `${t('snapshots.create')}: ${result.id}`)
    await load()
  } catch (err) { emit('error', String(err)) } finally { busy.value = '' }
}

async function restore(item: Snapshot) {
  if (!window.confirm(t('snapshots.confirmRestore', { id: item.id }))) return
  busy.value = item.id
  try {
    const result = await Api.RestoreSnapshot(item.id)
    emit('saved', result.reloadError ? t('snapshots.restoredNoReload', { error: result.reloadError }) : t('snapshots.restored', { id: result.preRestoreSnapshotId }))
    await load()
  } catch (err) { emit('error', String(err)) } finally { busy.value = '' }
}

async function remove(item: Snapshot) {
  if (!window.confirm(t('snapshots.confirmDelete', { id: item.id }))) return
  busy.value = item.id
  try {
    await Api.DeleteSnapshot(item.id)
    await load()
  } catch (err) { emit('error', String(err)) } finally { busy.value = '' }
}

function formatDate(value: string) {
  const date = new Date(value)
  return Number.isNaN(date.getTime()) ? value : new Intl.DateTimeFormat(undefined, { dateStyle: 'medium', timeStyle: 'short' }).format(date)
}

onMounted(load)
</script>

<template>
  <div class="snapshots-layout">
    <section class="snapshot-hero card">
      <div class="snapshot-hero-icon"><ArchiveRestore :size="21" /></div>
      <div><div class="section-kicker">{{ t('snapshots.recoveryPoints') }}</div><h2>{{ t('snapshots.headline') }}</h2><p>{{ t('snapshots.subtitle') }}</p></div>
      <button class="button-primary" :disabled="busy === 'create'" @click="create"><Plus :size="14" /> {{ busy === 'create' ? t('snapshots.creating') : t('snapshots.create') }}</button>
    </section>

    <div v-if="loadError" class="snapshot-error card">{{ loadError }}</div>
    <section class="card snapshot-list-card">
      <div class="card-header"><div><div class="section-kicker">{{ t('snapshots.localHistory') }}</div><h3 class="card-title">{{ t('snapshots.saved') }}</h3><p class="card-subtitle">{{ t('snapshots.savedSubtitle', { path: '~/.local/share/hyprmod/snapshots' }) }}</p></div><span class="tag">{{ t('snapshots.points', { count: snapshots.length }) }}</span></div>
      <div v-if="loading" class="empty-state">{{ t('snapshots.loading') }}</div>
      <div v-else-if="!snapshots.length" class="snapshot-empty"><div class="snapshot-empty-icon"><Clock3 :size="19" /></div><strong>{{ t('snapshots.none') }}</strong><span>{{ t('snapshots.noneBody') }}</span><button class="button-secondary" @click="create">{{ t('snapshots.createFirst') }}</button></div>
      <article v-for="(item, index) in snapshots" :key="item.id" class="snapshot-row">
        <div class="snapshot-index">{{ String(index + 1).padStart(2, '0') }}</div>
        <div class="snapshot-main">
          <div class="snapshot-title-line"><strong>{{ item.id }}</strong><span v-if="index === 0" class="tag lime">{{ t('snapshots.latest') }}</span></div>
          <div class="snapshot-meta"><span><Clock3 :size="11" /> {{ formatDate(item.created) }}</span><span><FileCode2 :size="11" /> {{ item.files?.length ?? 0 }} {{ t('common.files') }}</span></div>
          <div class="snapshot-files">{{ (item.files ?? []).map(file => file.split('/').slice(-2).join('/')).join(' · ') }}</div>
        </div>
        <div class="snapshot-actions">
          <button class="button-secondary" :disabled="busy === item.id" @click="restore(item)"><RotateCcw :size="12" /> {{ t('snapshots.restore') }}</button>
          <button class="row-icon-button" :title="t('snapshots.confirmDelete', { id: item.id })" @click="remove(item)"><Trash2 :size="13" /></button>
        </div>
      </article>
    </section>
  </div>
</template>

<style scoped>
.snapshots-layout { display: flex; flex-direction: column; gap: 13px; }
.snapshot-hero { min-height: 105px; display: flex; align-items: center; gap: 14px; padding: 17px 19px; border-color: #424638; background: linear-gradient(110deg, #252b20, #1b1e26 68%); }
.snapshot-hero-icon { width: 41px; height: 41px; display: grid; place-items: center; border: 1px solid #596744; border-radius: 10px; background: #303a27; color: var(--lime); }
.snapshot-hero > div:nth-child(2) { flex: 1; }
.snapshot-hero h2 { margin: 5px 0 4px; color: #e9ede4; font-size: 14px; }
.snapshot-hero p { max-width: 610px; margin: 0; color: #89917e; font-size: 8.5px; line-height: 1.6; }
.snapshot-list-card { overflow: hidden; }
.snapshot-empty { min-height: 190px; display: flex; flex-direction: column; align-items: center; justify-content: center; gap: 9px; color: #858c99; }
.snapshot-empty-icon { width: 40px; height: 40px; display: grid; place-items: center; border: 1px solid #3b4135; border-radius: 50%; background: #252b21; color: #aecb82; }
.snapshot-empty strong { color: #d4d7df; font-size: 10px; }
.snapshot-empty span { color: #7c8391; font-size: 8.5px; }
.snapshot-empty .button-secondary { margin-top: 4px; }
.snapshot-row { min-height: 79px; display: flex; align-items: center; gap: 13px; padding: 12px 18px; border-top: 1px solid #2b2f39; }
.snapshot-index { color: #8d966f; font: 9px var(--mono); }
.snapshot-main { min-width: 0; flex: 1; }
.snapshot-title-line { display: flex; align-items: center; gap: 8px; }
.snapshot-title-line strong { overflow: hidden; color: #d9dce3; font: 9px var(--mono); text-overflow: ellipsis; white-space: nowrap; }
.snapshot-meta { display: flex; gap: 12px; margin-top: 6px; color: #7f8694; font: 7.5px var(--mono); }
.snapshot-meta span { display: flex; align-items: center; gap: 4px; }
.snapshot-files { overflow: hidden; margin-top: 5px; color: #626a78; font: 7px var(--mono); text-overflow: ellipsis; white-space: nowrap; }
.snapshot-actions { display: flex; align-items: center; gap: 5px; }
.snapshot-actions .button-secondary { min-height: 29px; padding-inline: 9px; font-size: 8px; }
.snapshot-error { padding: 13px; color: #ed9ca1; font-size: 9px; }
@media (max-width: 700px) { .snapshot-hero { align-items: flex-start; flex-wrap: wrap; } .snapshot-hero > div:nth-child(2) { min-width: calc(100% - 60px); } .snapshot-hero > button { margin-left: 54px; } .snapshot-row { align-items: flex-start; } .snapshot-actions { flex-direction: column; } }
</style>
