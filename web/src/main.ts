import { createApp } from 'vue'
import LoginGate from './components/LoginGate.vue'
import './styles.css'

const app = createApp(LoginGate)
app.config.errorHandler = (error) => {
  console.error('[Clash for fnOS]', error)
}
app.mount('#app')
