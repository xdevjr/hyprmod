<script setup lang="ts">
import { computed, ref, watch } from 'vue'
import { useI18n } from '../i18n'
import { Check, Code2, Play, Save } from '@lucide/vue'
import type { Option } from '../../bindings/github.com/juniorgs/hyprmod/internal/schema/models'
import type { Value } from '../../bindings/github.com/juniorgs/hyprmod/internal/config/models'

const { t } = useI18n()

const props = defineProps<{ option: Option; value?: Value }>()
const emit = defineEmits<{ save: [path: string, value: unknown]; preview: [path: string, value: unknown]; error: [message: string] }>()
const text = ref('')
const boolValue = ref(false)
const saving = ref(false)

const kind = computed(() => props.option.kind as string)
const isBool = computed(() => kind.value === 'bool')
const isNumber = computed(() => kind.value === 'int' || kind.value === 'float')
const isStructured = computed(() => ['gradient', 'vec2', 'css_gaps'].includes(kind.value))
const hasValue = computed(() => props.value?.resolved !== undefined && props.value?.resolved !== null)
const formatted = (value: unknown) => {
  if (typeof value === 'string') return value
  if (value === undefined || value === null) return ''
  if (typeof value === 'object') return JSON.stringify(value, null, 2)
  return String(value)
}

watch(() => props.value?.resolved, (value) => {
  text.value = formatted(value)
  boolValue.value = value === true
}, { immediate: true, deep: true })

const changed = computed(() => {
  if (isBool.value) return boolValue.value !== (props.value?.resolved === true)
  return text.value !== formatted(props.value?.resolved)
})

function parseValue(): unknown {
  if (isBool.value) return boolValue.value
  if (kind.value === 'int') return Number.parseInt(text.value, 10)
  if (kind.value === 'float') return Number(text.value)
  if (kind.value === 'css_gaps') {
    const trimmed = text.value.trim()
    if (trimmed.startsWith('{')) return JSON.parse(trimmed)
    return Number(trimmed)
  }
  if (kind.value === 'vec2' || kind.value === 'gradient') {
    const trimmed = text.value.trim()
    if (trimmed.startsWith('{') || trimmed.startsWith('[')) return JSON.parse(trimmed)
    return text.value
  }
  if (kind.value === 'font_weight') {
    const number = Number(text.value)
    return text.value.trim() !== '' && Number.isFinite(number) ? number : text.value
  }
  return text.value
}

async function save() {
  saving.value = true
  try {
    emit('save', props.option.path, parseValue())
  } catch (err) {
    emit('error', String(err))
  } finally {
    saving.value = false
  }
}

function preview() {
  try {
    emit('preview', props.option.path, parseValue())
  } catch (err) {
    emit('error', String(err))
  }
}
</script>

<template>
  <article class="field-card">
    <div class="field-meta">
      <div class="field-name-line">
        <code>{{ option.name }}</code>
        <span v-if="value?.dynamic" class="tag amber"><Code2 :size="10" /> {{ t('common.dynamic') }}</span>
      </div>
      <div class="field-type-line"><span>{{ kind || 'unknown' }}</span><span v-if="option.raw">{{ option.raw }}</span></div>
      <div v-if="value?.file" class="field-origin" :title="`${value.file}:${value.line}`">{{ value.file.split('/').slice(-2).join('/') }}:{{ value.line }}</div>
    </div>
    <div class="field-control">
      <label v-if="isBool" class="toggle-control">
        <input v-model="boolValue" type="checkbox" />
        <span class="toggle-track"></span>
        <span class="toggle-label">{{ boolValue ? t('common.enabled') : t('common.disabled') }}</span>
      </label>
      <input
        v-else-if="isNumber"
        v-model="text"
        class="form-input field-input mono"
        type="number"
        :step="kind === 'int' ? 1 : 'any'"
        :placeholder="hasValue ? '' : t('common.notSet')"
      />
      <textarea
        v-else-if="isStructured"
        v-model="text"
        class="form-textarea field-textarea"
        :placeholder="kind === 'gradient' ? 'Color string or Lua-like JSON table' : 'Enter JSON or a supported string'"
        spellcheck="false"
      ></textarea>
      <div v-else-if="kind === 'color'" class="color-control">
        <span class="color-swatch" :style="{ background: text || '#343943' }"></span>
        <input v-model="text" class="form-input field-input mono" type="text" :placeholder="hasValue ? '' : 'rgba(...)'" />
      </div>
      <input v-else v-model="text" class="form-input field-input" type="text" :placeholder="hasValue ? '' : t('common.notSet')" />
      <button v-if="changed" class="field-preview" :disabled="saving" :title="t('common.preview')" @click="preview"><Play :size="12" /> {{ t('common.preview') }}</button>
      <button class="field-save" :class="{ dirty: changed }" :disabled="!changed || saving" @click="save">
        <Save v-if="changed" :size="13" /><Check v-else :size="13" />
        {{ changed ? t('common.apply') : t('common.saved') }}
      </button>
    </div>
  </article>
</template>

<style scoped>
.field-card { display: grid; grid-template-columns: minmax(170px, .8fr) minmax(250px, 1.2fr); align-items: center; gap: 18px; padding: 12px 14px; border-top: 1px solid #292d36; }
.field-meta { min-width: 0; }
.field-name-line { display: flex; align-items: center; gap: 7px; }
.field-name-line code { overflow: hidden; color: #dde0e6; font: 10px var(--mono); text-overflow: ellipsis; }
.field-type-line { display: flex; flex-wrap: wrap; gap: 6px; margin-top: 4px; color: #a8b785; font: 8px var(--mono); }
.field-type-line span + span { color: #686f7c; }
.field-origin { overflow: hidden; margin-top: 5px; color: #6f7683; font: 7.5px var(--mono); text-overflow: ellipsis; white-space: nowrap; }
.field-control { display: flex; align-items: center; gap: 8px; min-width: 0; }
.field-input { min-width: 0; min-height: 31px; flex: 1; }
.field-textarea { min-height: 55px; max-height: 120px; flex: 1; }
.field-save, .field-preview { min-width: 74px; min-height: 30px; display: inline-flex; align-items: center; justify-content: center; gap: 6px; padding: 0 8px; border: 1px solid #343944; border-radius: 6px; background: #22252e; color: #818795; cursor: default; font-size: 8.5px; }
.field-save.dirty { border-color: #72894c; background: #c4f274; color: #26321a; cursor: pointer; font-weight: 800; }
.field-preview { min-width: 63px; border-color: #44465a; background: #262632; color: #b6b2d5; cursor: pointer; }
.field-preview:hover { border-color: #77719f; background: #302d3f; }
.toggle-control { display: flex; align-items: center; gap: 8px; flex: 1; cursor: pointer; }
.toggle-control input { position: absolute; opacity: 0; pointer-events: none; }
.toggle-track { width: 31px; height: 18px; position: relative; border: 1px solid #444955; border-radius: 12px; background: #2a2e37; transition: .16s; }
.toggle-track::after { content: ''; position: absolute; left: 2px; top: 2px; width: 12px; height: 12px; border-radius: 50%; background: #949aa7; transition: .16s; }
.toggle-control input:checked + .toggle-track { border-color: #92b557; background: #556d36; }
.toggle-control input:checked + .toggle-track::after { left: 15px; background: #d5f8a5; }
.toggle-label { color: #aab0bb; font-size: 9px; }
.color-control { flex: 1; display: flex; gap: 7px; min-width: 0; }
.color-swatch { width: 30px; min-width: 30px; height: 31px; border: 1px solid #4a4e58; border-radius: 6px; }
@media (max-width: 800px) { .field-card { grid-template-columns: 1fr; gap: 9px; } }
</style>
