import { ref, type Ref } from 'vue'
import en from './locales/en'
import { ptBR } from './locales/pt-BR'

export const availableLocales = [
  { code: 'en', label: 'English' },
  { code: 'pt-BR', label: 'Português (BR)' },
] as const

export type LocaleCode = (typeof availableLocales)[number]['code']

const STORAGE_KEY = 'hyprmod.locale'
const FALLBACK: LocaleCode = 'en'

const messages: Record<LocaleCode, unknown> = {
  en,
  'pt-BR': ptBR,
}

function initialLocale(): LocaleCode {
  const stored = localStorage.getItem(STORAGE_KEY)
  if (stored && availableLocales.some(item => item.code === stored)) return stored as LocaleCode
  return navigator.language.toLowerCase().startsWith('pt') ? 'pt-BR' : FALLBACK
}

export const locale: Ref<LocaleCode> = ref(initialLocale())

function lookup(source: unknown, path: string): string | undefined {
  let node = source
  for (const segment of path.split('.')) {
    if (typeof node !== 'object' || node === null) return undefined
    node = (node as Record<string, unknown>)[segment]
  }
  return typeof node === 'string' ? node : undefined
}

function interpolate(template: string, params?: Record<string, unknown>) {
  if (!params) return template
  return template.replace(/\{(\w+)\}/g, (match, key: string) =>
    Object.prototype.hasOwnProperty.call(params, key) ? String(params[key]) : match,
  )
}

/** Resolves a dotted message key in the active locale, falling back to English. */
export function t(path: string, params?: Record<string, unknown>): string {
  const resolved = lookup(messages[locale.value], path) ?? lookup(messages[FALLBACK], path)
  if (resolved === undefined) return path
  return interpolate(resolved, params)
}

export function setLocale(next: LocaleCode) {
  locale.value = next
  localStorage.setItem(STORAGE_KEY, next)
  document.documentElement.lang = next
}

/** Minimal composable mirroring the vue-i18n API surface the app relies on. */
export function useI18n() {
  return { t, locale, availableLocales, setLocale }
}
