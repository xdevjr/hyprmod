import { createApp } from 'vue'
import App from './App.vue'
import './style.css'

// setLocale normalises the stored/browser value and marks the document language.
import { locale } from './i18n'
document.documentElement.lang = locale.value

createApp(App).mount('#app')
