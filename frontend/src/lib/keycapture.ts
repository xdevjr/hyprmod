import { ref, onBeforeUnmount } from 'vue'

// Hyprland spells modifiers in a fixed vocabulary, and the browser reports
// layout-dependent key names, so both are normalised here.
const MODIFIER_ALIASES: Record<string, string> = {
  Control: 'CTRL',
  ControlLeft: 'CTRL',
  ControlRight: 'CTRL',
  Shift: 'SHIFT',
  ShiftLeft: 'SHIFT',
  ShiftRight: 'SHIFT',
  Alt: 'ALT',
  AltLeft: 'ALT',
  AltRight: 'ALT',
  AltGraph: 'ALT',
  Meta: 'SUPER',
  MetaLeft: 'SUPER',
  MetaRight: 'SUPER',
  OS: 'SUPER',
  // Not standard KeyboardEvent.key values, but some environments report the
  // X11-style names, and mapping them costs nothing.
  Super: 'SUPER',
  SuperLeft: 'SUPER',
  SuperRight: 'SUPER',
  Hyper: 'SUPER',
}

// Read in this order so the produced combo string is stable across repeats.
const MODIFIER_ORDER = ['SUPER', 'SHIFT', 'CTRL', 'ALT']

const KEY_ALIASES: Record<string, string> = {
  ' ': 'SPACE',
  ArrowUp: 'UP',
  ArrowDown: 'DOWN',
  ArrowLeft: 'LEFT',
  ArrowRight: 'RIGHT',
  Escape: 'ESC',
  Enter: 'ENTER',
  NumpadEnter: 'KP_ENTER',
  Backspace: 'BACKSPACE',
  Delete: 'DELETE',
  Tab: 'TAB',
  CapsLock: 'CAPS',
  Home: 'HOME',
  End: 'END',
  PageUp: 'PGUP',
  PageDown: 'PGDN',
  Insert: 'INS',
  ContextMenu: 'MENU',
  PrintScreen: 'PRTSC',
  ScrollLock: 'SCRLK',
  Pause: 'PAUSE',
  NumLock: 'NUMLK',
}

/**
 * Reference of the libinput button codes Hyprland understands inside a key
 * string. The browser DOM cannot report the physical side buttons, so mouse
 * binds are added by picking a code from this list instead of capturing.
 */
export const MOUSE_CODE_REFERENCE = [
  { code: 'mouse:272', label: 'left' },
  { code: 'mouse:273', label: 'right' },
  { code: 'mouse:274', label: 'middle' },
  { code: 'mouse:275', label: 'side' },
  { code: 'mouse:276', label: 'extra' },
  { code: 'mouse:277', label: 'forward' },
  { code: 'mouse:278', label: 'back' },
  { code: 'mouse:279', label: 'task' },
]

/** Shared so global shortcuts (e.g. the command palette) stay inert while capturing. */
export const keyCaptureActive = ref(false)

const preview = ref('')

function baseKeyName(event: KeyboardEvent): string {
  const aliased = KEY_ALIASES[event.key]
  if (aliased) return aliased
  if (event.key.length === 1) return event.key.toUpperCase()
  return event.key.toUpperCase()
}

/** Splits a combo into ordered tokens so capture can continue where it left off. */
export function comboTokens(combo: string): string[] {
  return combo
    .split('+')
    .map(token => token.trim().toUpperCase())
    .filter(Boolean)
}

function orderTokens(tokens: string[]): string[] {
  const modifiers = MODIFIER_ORDER.filter(modifier => tokens.includes(modifier))
  const rest = tokens.filter(token => !MODIFIERS.has(token))
  return [...modifiers, ...rest]
}

const MODIFIERS = new Set(MODIFIER_ORDER)

/**
 * Builds a shortcut one keystroke at a time: every press appends to the
 * combination and the capture stays open until the caller finishes it, so the
 * user can assemble longer sequences incrementally. Mouse buttons are not
 * captured: the DOM cannot report the physical side buttons, so they are added
 * from MOUSE_CODE_REFERENCE instead.
 */
export function useKeyCapture(onChange: (combo: string) => void) {
  let tokens: string[] = []

  function emit() {
    const combo = orderTokens(tokens).join(' + ')
    preview.value = combo
    onChange(combo)
  }

  function stop() {
    keyCaptureActive.value = false
    preview.value = ''
    window.removeEventListener('keydown', onKeyDown, true)
  }

  function onKeyDown(event: KeyboardEvent) {
    if (!keyCaptureActive.value) return
    event.preventDefault()
    event.stopPropagation()

    if (event.key === 'Enter') {
      stop()
      return
    }
    if (event.key === 'Backspace') {
      tokens.pop()
      emit()
      return
    }
    if (event.key === 'Escape') {
      // Escape clears the composition instead of ending it, so a mis-pressed
      // key can be removed without leaving capture mode.
      tokens = []
      emit()
      return
    }
    const modifier = MODIFIER_ALIASES[event.key]
    if (modifier) {
      if (!tokens.includes(modifier)) tokens.push(modifier)
      emit()
      return
    }
    const key = baseKeyName(event)
    if (!tokens.includes(key)) tokens.push(key)
    emit()
  }

  function start(seed = '') {
    stop()
    tokens = comboTokens(seed)
    keyCaptureActive.value = true
    window.addEventListener('keydown', onKeyDown, true)
    emit()
  }

  onBeforeUnmount(stop)

  return { start, stop, active: keyCaptureActive, preview }
}
