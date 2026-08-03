import { createApp } from 'vue'
import App from './App.vue'
import './styles/globals.css'
import { router } from './router'

// 固定语言标识，供浏览器和 Intl 使用
document.documentElement.lang = 'zh-CN'

const app = createApp(App)

const themeColor = document.querySelector<HTMLMetaElement>('meta[name="theme-color"]')
const syncThemeColor = () => {
  themeColor?.setAttribute('content', document.documentElement.classList.contains('dark') ? '#121212' : '#fafafa')
}
const themeObserver = new MutationObserver(syncThemeColor)
themeObserver.observe(document.documentElement, { attributes: true, attributeFilter: ['class'] })
syncThemeColor()

app.use(router)
app.mount('#app')

