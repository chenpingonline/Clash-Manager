import { createApp } from 'vue'
import LoginGate from './components/LoginGate.vue'
import './styles.css'

const app = createApp(LoginGate)
app.config.errorHandler = (error) => {
  console.error(`[${document.title}]`, error)
}
app.mount('#app')
