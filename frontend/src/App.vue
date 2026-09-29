<script setup lang="ts">
import { computed, onMounted, onUnmounted, ref, shallowRef } from 'vue'
import {
  Activity,
  Aperture,
  ChevronDown,
  Command,
  Copy,
  FileCode2,
  FolderOpen,
  Grid2X2,
  History,
  Keyboard,
  LayoutDashboard,
  Languages,
  LockKeyhole,
  Monitor,
  MousePointer2,
  PanelTop,
  Search,
  Settings2,
  Sparkles,
  SlidersHorizontal,
  Zap,
} from '@lucide/vue'
import { Clipboard, Dialogs } from '@wailsio/runtime'
import * as Api from '../bindings/github.com/juniorgs/hyprmod/internal/api/service'
import type { Dashboard } from '../bindings/github.com/juniorgs/hyprmod/internal/api/models'
import type { Document } from '../bindings/github.com/juniorgs/hyprmod/internal/config/models'
import type { Schema } from '../bindings/github.com/juniorgs/hyprmod/internal/schema/models'
import { useI18n, setLocale, type LocaleCode } from './i18n'
import { keyCaptureActive } from './lib/keycapture'
import OverviewPage from './components/OverviewPage.vue'
import MonitorsPage from './components/MonitorsPage.vue'
import VariablesPage from './components/VariablesPage.vue'
import KeybindsPage from './components/KeybindsPage.vue'
import RulesPage from './components/RulesPage.vue'
import AnimationsPage from './components/AnimationsPage.vue'
import StartupPage from './components/StartupPage.vue'
import PermissionsPage from './components/PermissionsPage.vue'
import SnapshotsPage from './components/SnapshotsPage.vue'
import LuaEditorPage from './components/LuaEditorPage.vue'

const { t, locale, availableLocales } = useI18n()
type PageID = 'overview' | 'monitors' | 'variables' | 'keybinds' | 'window-rules' | 'workspace-rules' | 'layer-rules' | 'animations' | 'input' | 'startup' | 'permissions' | 'snapshots' | 'source'

const dashboard = shallowRef<Dashboard | null>(null)
const document = shallowRef<Document | null>(null)
const schema = shallowRef<Schema | null>(null)
const activePage = ref<PageID>('overview')
const variableSection = ref('general')
const loading = ref(true)
const refreshError = ref('')
const searchOpen = ref(false)
const configMenuOpen = ref(false)
const searchQuery = ref('')
const toastMessage = ref('')
const toastKind = ref<'success' | 'error'>('success')
let toastTimer: ReturnType<typeof setTimeout> | undefined

const navGroups = computed(() => [
  {
    name: t('app.group.workspace'),
    items: [
      { id: 'overview' as PageID, label: t('app.nav.overview'), icon: LayoutDashboard },
      { id: 'monitors' as PageID, label: t('app.nav.monitors'), icon: Monitor },
    ],
  },
  {
    name: t('app.group.configuration'),
    items: [
      { id: 'variables' as PageID, label: t('app.nav.variables'), icon: SlidersHorizontal },
      { id: 'input' as PageID, label: t('app.nav.input'), icon: MousePointer2 },
      { id: 'keybinds' as PageID, label: t('app.nav.keybinds'), icon: Keyboard },
      { id: 'window-rules' as PageID, label: t('app.nav.windowRules'), icon: PanelTop },
      { id: 'workspace-rules' as PageID, label: t('app.nav.workspaceRules'), icon: Grid2X2 },
      { id: 'layer-rules' as PageID, label: t('app.nav.layerRules'), icon: Aperture },
      { id: 'animations' as PageID, label: t('app.nav.animations'), icon: Sparkles },
      { id: 'startup' as PageID, label: t('app.nav.startup'), icon: Zap },
      { id: 'permissions' as PageID, label: t('app.nav.permissions'), icon: LockKeyhole },
    ],
  },
  {
    name: t('app.group.system'),
    items: [
      { id: 'snapshots' as PageID, label: t('app.nav.snapshots'), icon: History },
      { id: 'source' as PageID, label: t('app.nav.source'), icon: FileCode2 },
    ],
  },
])

const pageKey: Record<PageID, string> = {
  overview: 'overview', monitors: 'monitors', variables: 'variables', input: 'input',
  keybinds: 'keybinds', 'window-rules': 'windowRules', 'workspace-rules': 'workspaceRules',
  'layer-rules': 'layerRules', animations: 'animations', startup: 'startup',
  permissions: 'permissions', snapshots: 'snapshots', source: 'source',
}

const currentMeta = computed(() => {
  const key = pageKey[activePage.value]
  return { eyebrow: t(`app.page.${key}.eyebrow`), title: t(`app.page.${key}.title`), description: t(`app.page.${key}.description`) }
})

const filteredNav = computed(() => {
  const q = searchQuery.value.trim().toLowerCase()
  if (!q) return navGroups.value
  return navGroups.value.map(group => ({
    ...group,
    items: group.items.filter(item => item.label.toLowerCase().includes(q)),
  })).filter(group => group.items.length > 0)
})

function changeLocale(event: Event) {
  setLocale((event.target as HTMLSelectElement).value as LocaleCode)
}

function showToast(message: string, kind: 'success' | 'error' = 'success') {
  toastMessage.value = message
  toastKind.value = kind
  if (toastTimer) clearTimeout(toastTimer)
  toastTimer = setTimeout(() => { toastMessage.value = '' }, 3400)
}

async function refreshAll() {
  loading.value = true
  refreshError.value = ''
  try {
    const [nextDashboard, nextSchema, nextDocument] = await Promise.all([
      Api.GetDashboard(),
      Api.GetSchema(),
      Api.LoadDocument(),
    ])
    dashboard.value = nextDashboard
    schema.value = nextSchema
    document.value = nextDocument
  } catch (err) {
    refreshError.value = String(err)
    dashboard.value = await Api.GetDashboard().catch(() => null)
    schema.value = await Api.GetSchema().catch(() => null)
    document.value = null
  } finally {
    loading.value = false
  }
}

async function copyConfigPath() {
  const path = dashboard.value?.configPath
  if (!path) return showToast(t('app.configNotFound'), 'error')
  try {
    await Clipboard.SetText(path)
    showToast(t('app.pathCopied'))
  } catch {
    showToast(path)
  }
}

// Lets users point HyprMod at a non-default entry file, e.g. when the session
// was started with a custom config path. The backend validates the selection.
async function pickConfigFile() {
  configMenuOpen.value = false
  try {
    const selected = await Dialogs.OpenFile({ Title: t('app.changeConfig'), Filters: [{ DisplayName: 'Lua', Pattern: '*.lua' }] })
    const path = Array.isArray(selected) ? selected[0] : selected
    if (!path) return
    await Api.SelectConfigPath(path)
    await refreshAll()
    showToast(t('app.configChanged', { path }))
  } catch (err) {
    showToast(String(err), 'error')
  }
}

function goTo(id: PageID) {
  activePage.value = id
  if (id === 'input') {
    variableSection.value = 'input'
  } else if (id === 'variables') {
    // Both entries render the same component, so the section has to be reset
    // explicitly or "Variables" would keep showing the input section.
    variableSection.value = 'general'
  }
  searchOpen.value = false
  searchQuery.value = ''
}

function handleKeydown(event: KeyboardEvent) {
  // While a shortcut is being captured, keystrokes belong to the capture flow
  // and must not trigger global shortcuts.
  if (keyCaptureActive.value) return
  if ((event.ctrlKey || event.metaKey) && event.key.toLowerCase() === 'k') {
    event.preventDefault()
    searchOpen.value = !searchOpen.value
    searchQuery.value = ''
  }
  if (event.key === 'Escape') searchOpen.value = false
}

function notifySaved(message = 'Configuration applied and validated') {  showToast(message)
  void refreshAll()
}

onMounted(() => {
  window.addEventListener('keydown', handleKeydown)
  void refreshAll()
})
onUnmounted(() => {
  window.removeEventListener('keydown', handleKeydown)
  if (toastTimer) clearTimeout(toastTimer)
})
</script>

<template>
  <div class="app-shell">
    <aside class="sidebar">
      <div class="brand-lockup">
        <div class="brand-mark"><span></span><span></span><span></span></div>
        <div class="brand-copy">
          <strong>hyprmod</strong>
          <span>{{ t('app.brandSubtitle') }}</span>
        </div>
      </div>

      <button class="search-trigger" @click="searchOpen = true">
        <Search :size="15" />
        <span>{{ t('app.search') }}</span>
        <kbd>Ctrl K</kbd>
      </button>

      <nav class="nav-scroll">
        <section v-for="group in navGroups" :key="group.name" class="nav-group">
          <h2>{{ group.name }}</h2>
          <button
            v-for="item in group.items"
            :key="item.id"
            class="nav-item"
            :class="{ active: activePage === item.id }"
            @click="goTo(item.id)"
          >
            <component :is="item.icon" :size="17" :stroke-width="1.8" />
            <span>{{ item.label }}</span>
            <span v-if="item.id === 'overview' && dashboard?.online" class="nav-live-dot"></span>
          </button>
        </section>
      </nav>

      <div class="sidebar-footer">
        <div class="session-chip" :class="{ offline: !dashboard?.online }">
          <span class="session-dot"></span>
          <span>{{ dashboard?.online ? t('app.connected') : t('app.offline') }}</span>
          <Activity :size="14" />
        </div>
        <label class="locale-picker">
          <Languages :size="13" />
          <select :value="locale" :aria-label="t('app.language')" @change="changeLocale">
            <option v-for="item in availableLocales" :key="item.code" :value="item.code">{{ item.label }}</option>
          </select>
        </label>
        <div class="sidebar-version">HyprMod <span>0.1.0</span></div>
      </div>
    </aside>

    <main class="main-frame">
      <header class="topbar">
        <div class="breadcrumbs">
          <span>{{ currentMeta.eyebrow }}</span>
          <span class="crumb-slash">/</span>
          <strong>{{ currentMeta.title }}</strong>
        </div>
        <div class="topbar-actions">
          <label class="locale-picker topbar-locale">
            <Languages :size="14" />
            <select :value="locale" :aria-label="t('app.language')" @change="changeLocale">
              <option v-for="item in availableLocales" :key="item.code" :value="item.code">{{ item.label }}</option>
            </select>
          </label>
          <button class="icon-button" :title="t('app.refreshState')" @click="refreshAll"><Activity :size="16" /></button>
          <button class="config-path" :title="t('app.configPathHint')" @click="configMenuOpen = !configMenuOpen">
            <span class="path-led"></span>
            <span>{{ dashboard?.configPath || t('app.configNotFound') }}</span>
            <ChevronDown :size="14" />
          </button>
          <div v-if="configMenuOpen" class="config-menu">
            <div class="config-menu-head">{{ t('app.activeConfig') }}</div>
            <code class="config-menu-path">{{ dashboard?.configPath || t('app.configNotFound') }}</code>
            <div class="config-menu-actions">
              <button class="button-quiet" @click="copyConfigPath"><Copy :size="13" /> {{ t('app.copyPath') }}</button>
              <button class="button-secondary" @click="pickConfigFile"><FolderOpen :size="13" /> {{ t('app.changeConfig') }}</button>
            </div>
            <p class="config-menu-note">{{ t('app.configMenuNote') }}</p>
          </div>
        </div>
      </header>

      <div class="page-heading">
        <div>
          <div class="eyebrow">{{ currentMeta.eyebrow }}</div>
          <h1>{{ currentMeta.title }}</h1>
          <p>{{ currentMeta.description }}</p>
        </div>
        <div class="heading-version" v-if="dashboard?.online">
          <span class="version-label">RUNNING</span>
          <span>Hyprland {{ dashboard.version.version }}</span>
        </div>
      </div>

      <div v-if="refreshError" class="global-warning">
        <Settings2 :size="16" />
        <span>{{ refreshError }}</span>
        <button @click="refreshAll">Retry</button>
      </div>

      <div class="content-scroll">
        <OverviewPage v-if="activePage === 'overview'" :dashboard="dashboard" :document="document" :loading="loading" @navigate="goTo" @refresh="refreshAll" />
        <MonitorsPage v-else-if="activePage === 'monitors'" :document="document" @saved="notifySaved" @error="(m) => showToast(m, 'error')" />
        <VariablesPage v-else-if="activePage === 'variables' || activePage === 'input'" :schema="schema" :initial-section="variableSection" @saved="notifySaved" @error="(m) => showToast(m, 'error')" />
        <KeybindsPage v-else-if="activePage === 'keybinds'" :schema="schema" :document="document" @saved="notifySaved" @error="(m) => showToast(m, 'error')" />
        <RulesPage v-else-if="activePage === 'window-rules' || activePage === 'workspace-rules' || activePage === 'layer-rules'" :kind="activePage" :document="document" @saved="notifySaved" @error="(m) => showToast(m, 'error')" />
        <AnimationsPage v-else-if="activePage === 'animations'" :document="document" @saved="notifySaved" @error="(m) => showToast(m, 'error')" />
        <StartupPage v-else-if="activePage === 'startup'" :document="document" @saved="notifySaved" @error="(m) => showToast(m, 'error')" />
        <PermissionsPage v-else-if="activePage === 'permissions'" :document="document" @saved="notifySaved" @error="(m) => showToast(m, 'error')" />
        <SnapshotsPage v-else-if="activePage === 'snapshots'" @saved="notifySaved" @error="(m) => showToast(m, 'error')" />
        <LuaEditorPage v-else :document="document" @saved="notifySaved('Lua source validated and saved')" @error="(m) => showToast(m, 'error')" />
      </div>
    </main>

    <div v-if="searchOpen" class="command-backdrop" @click.self="searchOpen = false">
      <div class="command-panel">
        <div class="command-input">
          <Search :size="18" />
          <input v-model="searchQuery" autofocus :placeholder="t('app.command.placeholder')" />
          <kbd>ESC</kbd>
        </div>
        <div class="command-results">
          <template v-for="group in filteredNav" :key="group.name">
            <div class="command-group-label">{{ group.name }}</div>
            <button v-for="item in group.items" :key="item.id" @click="goTo(item.id)">
              <component :is="item.icon" :size="16" />
              <span>{{ item.label }}</span>
              <ChevronDown :size="14" class="command-enter" />
            </button>
          </template>
          <div v-if="!filteredNav.length" class="command-empty">{{ t('app.command.empty') }}</div>
        </div>
        <div class="command-hint"><Command :size="13" /> {{ t('app.command.hint') }}</div>
      </div>
    </div>

    <Transition name="toast">
      <div v-if="toastMessage" class="toast" :class="toastKind">
        <span class="toast-mark">{{ toastKind === 'success' ? '✓' : '!' }}</span>
        <span>{{ toastMessage }}</span>
      </div>
    </Transition>
  </div>
</template>
