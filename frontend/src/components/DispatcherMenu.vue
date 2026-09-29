<script setup lang="ts">
import { computed, ref, watch } from 'vue'
import { Check, ChevronDown, Search } from '@lucide/vue'

const props = defineProps<{ modelValue: string; options: string[]; placeholder?: string }>()
const emit = defineEmits<{ 'update:modelValue': [value: string] }>()

const open = ref(false)
const query = ref('')
const root = ref<HTMLElement | null>(null)

const selected = computed({
  get: () => props.modelValue,
  set: value => emit('update:modelValue', value),
})

// Dispatchers are namespaced (exec_cmd, window.close, workspace.toggle_special),
// so they are grouped by their first segment to keep the list scannable.
const groups = computed(() => {
  const buckets = new Map<string, string[]>()
  for (const name of props.options) {
    const group = name.split('.')[0]
    buckets.set(group, [...(buckets.get(group) ?? []), name])
  }
  return [...buckets.entries()].map(([name, items]) => ({ name, items: items.sort() }))
})

const filtered = computed(() => {
  const q = query.value.trim().toLowerCase()
  if (!q) return groups.value
  return groups.value
    .map(group => ({ name: group.name, items: group.items.filter(item => item.toLowerCase().includes(q)) }))
    .filter(group => group.items.length > 0)
})

function pick(name: string) {
  selected.value = name
  open.value = false
  query.value = ''
}

function onDocumentClick(event: MouseEvent) {
  if (root.value && !root.value.contains(event.target as Node)) open.value = false
}

watch(open, isOpen => {
  document.addEventListener('click', onDocumentClick, true)
  if (!isOpen) document.removeEventListener('click', onDocumentClick, true)
})
</script>

<template>
  <div ref="root" class="dispatcher-menu">
    <button type="button" class="dispatcher-trigger" :class="{ open }" @click="open = !open">
      <span class="dispatcher-value" :class="{ placeholder: !modelValue }">{{ modelValue || placeholder || '—' }}</span>
      <ChevronDown :size="14" />
    </button>
    <div v-if="open" class="dispatcher-panel">
      <div class="dispatcher-search"><Search :size="13" /><input v-model="query" autofocus placeholder="Filtrar dispatchers" /></div>
      <div class="dispatcher-list">
        <div v-for="group in filtered" :key="group.name" class="dispatcher-group">
          <div class="dispatcher-group-label">{{ group.name }}</div>
          <button v-for="name in group.items" :key="name" type="button" class="dispatcher-item" :class="{ active: name === modelValue }" @click="pick(name)">
            <span>{{ name }}</span>
            <Check v-if="name === modelValue" :size="12" />
          </button>
        </div>
        <div v-if="!filtered.length" class="dispatcher-empty">Nenhum dispatcher encontrado.</div>
      </div>
    </div>
  </div>
</template>

<style scoped>
.dispatcher-menu { position: relative; }
.dispatcher-trigger { width: 100%; min-height: 34px; display: flex; align-items: center; gap: 8px; padding: 0 9px; border: 1px solid #383d49; border-radius: 6px; background: #14171e; color: #e4e6eb; cursor: pointer; font: 10px var(--mono); }
.dispatcher-trigger:hover { border-color: #4d535f; }
.dispatcher-trigger.open { border-color: #728852; box-shadow: 0 0 0 2px #c4f27417; }
.dispatcher-trigger > svg { flex: 0 0 auto; color: #6f7683; }
.dispatcher-value { flex: 1; overflow: hidden; text-align: left; text-overflow: ellipsis; white-space: nowrap; }
.dispatcher-value.placeholder { color: #646b79; }
.dispatcher-panel { position: absolute; z-index: 25; top: calc(100% + 4px); left: 0; width: max(100%, 260px); overflow: hidden; border: 1px solid #3a3f4b; border-radius: 8px; background: #1c1f27; box-shadow: 0 16px 44px #000a; }
.dispatcher-search { height: 34px; display: flex; align-items: center; gap: 7px; padding: 0 10px; border-bottom: 1px solid #30343e; color: #79808d; }
.dispatcher-search input { width: 100%; border: 0; outline: 0; background: transparent; color: #e4e6eb; font-size: 9.5px; }
.dispatcher-list { max-height: 260px; overflow: auto; padding: 5px; scrollbar-width: thin; }
.dispatcher-group-label { padding: 8px 8px 4px; color: #7e8a63; font: 7.5px var(--mono); letter-spacing: .1em; text-transform: uppercase; }
.dispatcher-item { width: 100%; min-height: 27px; display: flex; align-items: center; gap: 8px; padding: 0 8px; border: 0; border-radius: 5px; background: transparent; color: #c2c6cf; text-align: left; cursor: pointer; font: 9px var(--mono); }
.dispatcher-item:hover { background: #2a2e38; color: #e6e9ef; }
.dispatcher-item.active { background: #2c3a22; color: #cbe79c; }
.dispatcher-item span { flex: 1; }
.dispatcher-empty { padding: 18px; color: #7f8694; text-align: center; font-size: 9px; }
</style>
