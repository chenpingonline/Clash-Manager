import { createApp } from 'vue'
import LoginGate from './components/LoginGate.vue'
import './tokens.css'
import './styles.css'
import './ui-polish.css'

const app = createApp(LoginGate)
app.config.errorHandler = (error) => {
  console.error(`[${document.title}]`, error)
}
app.mount('#app')
document.documentElement.removeAttribute('data-booting')
