<script setup lang="ts">
import { computed, ref } from 'vue'
import { useI18n } from '../i18n'
import { KeyRound, LockKeyhole, Plus, ShieldAlert } from '@lucide/vue'
import * as Api from '../../bindings/github.com/juniorgs/hyprmod/internal/api/service'
import type { Document } from '../../bindings/github.com/juniorgs/hyprmod/internal/config/models'

const { t } = useI18n()

const props = defineProps<{ document: Document | null }>()
const emit = defineEmits<{ saved: [message?: string]; error: [message: string] }>()
const permissions = computed(() => props.document?.permissions ?? [])
const binary = ref('')
const kind = ref('screencopy')
const mode = ref('allow')
const saving = ref(false)

async function addPermission() {
  saving.value = true
  try {
    await Api.AddPermission(binary.value, kind.value, mode.value)
    emit('saved', t('permissions.adding'))
    binary.value = ''
  } catch (err) {
    emit('error', String(err))
  } finally {
    saving.value = false
  }
}
</script>

<template>
  <div class="permissions-layout">
    <section class="permission-warning card">
      <div class="warning-icon"><ShieldAlert :size="20" /></div>
      <div><div class="section-kicker">{{ t('permissions.securityBoundary') }}</div><h2>{{ t('permissions.headline') }}</h2><p>{{ t('permissions.subtitle') }}</p></div>
      <span class="tag amber">{{ t('permissions.restart') }}</span>
    </section>

    <section class="card permission-create">
      <div class="card-header"><div><div class="section-kicker">{{ t('permissions.policy') }}</div><h3 class="card-title">{{ t('permissions.addRule') }}</h3><p class="card-subtitle">{{ t('permissions.addRuleSubtitle') }}</p></div><span class="permission-glyph"><LockKeyhole :size="17" /></span></div>
      <form class="permission-form" @submit.prevent="addPermission">
        <label class="form-label">{{ t('permissions.binary') }}<input v-model="binary" class="form-input mono" placeholder="/usr/bin/grim" /></label>
        <label class="form-label">{{ t('permissions.type') }}<select v-model="kind" class="form-select"><option value="screencopy">screencopy</option><option value="plugin">plugin</option><option value="clipboard">clipboard</option><option value="global_shortcuts">global_shortcuts</option></select></label>
        <label class="form-label">{{ t('permissions.policyLabel') }}<select v-model="mode" class="form-select"><option value="allow">allow</option><option value="deny">deny</option></select></label>
        <button class="button-primary" type="submit" :disabled="saving || !binary.trim()"><Plus :size="13" /> {{ t('permissions.adding') }}</button>
      </form>
    </section>

    <section class="card permission-list">
      <div class="card-header"><div><div class="section-kicker">{{ t('permissions.configured') }}</div><h3 class="card-title">{{ t('permissions.rules') }}</h3></div><span class="tag"><KeyRound :size="11" /> {{ t('permissions.rulesCount', { count: permissions.length }) }}</span></div>
      <div v-if="!permissions.length" class="empty-state">{{ t('permissions.noRules') }}</div>
      <div v-for="(permission, index) in permissions" :key="`${permission.file}:${permission.line}:${index}`" class="permission-row">
        <span class="permission-index">{{ String(index + 1).padStart(2, '0') }}</span>
        <code class="permission-binary">{{ permission.binary }}</code>
        <span class="permission-type">{{ permission.type }}</span>
        <span class="permission-mode" :class="permission.mode === 'allow' ? 'allowed' : 'denied'">{{ permission.mode }}</span>
        <small>{{ permission.file.split('/').slice(-2).join('/') }}:{{ permission.line }}</small>
      </div>
    </section>
  </div>
</template>

<style scoped>
.permissions-layout { display: flex; flex-direction: column; gap: 13px; }
.permission-warning { min-height: 100px; display: flex; align-items: center; gap: 13px; padding: 16px 19px; border-color: #5c4930; background: linear-gradient(105deg, #2c251b, #211e1e); }
.warning-icon { width: 39px; height: 39px; display: grid; place-items: center; border: 1px solid #6a5738; border-radius: 9px; background: #3a2f1e; color: var(--amber); }
.permission-warning > div:nth-child(2) { flex: 1; }
.permission-warning h2 { margin: 4px 0 3px; color: #efe5d3; font-size: 12px; }
.permission-warning p { max-width: 650px; margin: 0; color: #ad9c80; font-size: 8.5px; line-height: 1.55; }
.permission-glyph { width: 31px; height: 31px; display: grid; place-items: center; border: 1px solid #3b4234; border-radius: 7px; background: #272c23; color: #bcd892; }
.permission-form { display: grid; grid-template-columns: 1.5fr 1fr .8fr auto; align-items: end; gap: 10px; padding: 0 18px 17px; }
.permission-form .button-primary { min-height: 34px; white-space: nowrap; }
.permission-list { overflow: hidden; }
.permission-row { min-height: 42px; display: grid; grid-template-columns: 25px minmax(170px, 1fr) 110px 65px minmax(120px, .8fr); align-items: center; gap: 10px; padding: 0 17px; border-top: 1px solid #2a2e37; }
.permission-index { color: #7e866d; font: 8px var(--mono); }
.permission-binary { overflow: hidden; color: #d4d7df; font: 8.5px var(--mono); text-overflow: ellipsis; white-space: nowrap; }
.permission-type, .permission-mode { font: 8px var(--mono); }
.permission-type { color: #9ea6b2; }
.permission-mode { justify-self: start; padding: 4px 6px; border: 1px solid #363c46; border-radius: 4px; }
.permission-mode.allowed { border-color: #455535; background: #27301f; color: #c2e295; }
.permission-mode.denied { border-color: #56363a; background: #2b1e22; color: #ef9da1; }
.permission-row small { overflow: hidden; color: #707785; font: 7px var(--mono); text-overflow: ellipsis; white-space: nowrap; }
@media (max-width: 900px) { .permission-form { grid-template-columns: 1fr 1fr; } .permission-warning > .tag { display: none; } }
@media (max-width: 620px) { .permission-row { grid-template-columns: 20px 1fr 55px; } .permission-row .permission-type, .permission-row small { display: none; } }
</style>
