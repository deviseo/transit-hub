<script setup lang="ts">
import { Button } from '@/components/ui/button'
import { onMounted, ref } from 'vue'
import gsap from 'gsap'
import { useDark, useToggle } from '@vueuse/core'
import { useI18n } from 'vue-i18n'
import { Sun, Moon, Globe } from 'lucide-vue-next'
import logoUrl from '@/assets/logo.png'

const headerRef = ref<HTMLElement | null>(null)

const isDark = useDark({
  selector: 'html',
  attribute: 'class',
  valueDark: 'dark',
  valueLight: '',
})
const toggleDark = useToggle(isDark)

const { t, locale } = useI18n()

const toggleLocale = () => {
  locale.value = locale.value === 'zh-CN' ? 'en-US' : 'zh-CN'
}

onMounted(() => {
  if (headerRef.value) {
    gsap.from(headerRef.value, {
      y: -50,
      opacity: 0,
      duration: 1,
      ease: 'power3.out',
    })
  }
})
</script>

<template>
  <div class="min-h-screen text-foreground flex flex-col selection:bg-primary/30">
    <header ref="headerRef" class="fixed top-0 left-0 right-0 z-50 border-b border-border/40 bg-background/60 backdrop-blur-xl">
      <div class="mx-auto flex h-16 max-w-7xl items-center justify-between px-4 sm:px-6 lg:px-8">
        <div class="flex items-center gap-2">
          <img :src="logoUrl" :alt="t('brand.logoAlt')" class="h-8 w-8 shrink-0 object-contain" />
          <span class="text-xl font-bold tracking-tight text-foreground">{{ t('brand.name') }}</span>
        </div>
        
        <nav class="hidden md:flex items-center gap-8 text-sm font-medium text-muted-foreground">
          <a href="#" class="hover:text-foreground transition-colors">{{ t('nav.features') }}</a>
          <a href="#" class="hover:text-foreground transition-colors">{{ t('nav.integrations') }}</a>
          <a href="#" class="hover:text-foreground transition-colors">{{ t('nav.documentation') }}</a>
          <a href="#" class="hover:text-foreground transition-colors">{{ t('nav.pricing') }}</a>
        </nav>

        <div class="flex items-center gap-4">
          <!-- Theme & Language Toggles -->
          <div class="flex items-center gap-2 border-r border-border/40 pr-4 mr-1">
            <button @click="toggleLocale" class="flex h-9 w-9 items-center justify-center rounded-full hover:bg-surface-elevated text-muted-foreground hover:text-foreground transition-colors" title="Toggle Language">
              <Globe class="h-4 w-4" />
            </button>
            <button @click="toggleDark()" class="flex h-9 w-9 items-center justify-center rounded-full hover:bg-surface-elevated text-muted-foreground hover:text-foreground transition-colors" title="Toggle Theme">
              <Moon v-if="!isDark" class="h-4 w-4" />
              <Sun v-else class="h-4 w-4" />
            </button>
          </div>

          <Button @click="$router.push('/login')" variant="secondary" class="hidden sm:inline-flex rounded-full bg-surface-elevated/50 border border-border/50 hover:bg-surface-line/50">
            {{ t('nav.signIn') }}
          </Button>
          <Button class="rounded-full bg-primary text-primary-foreground shadow-glow hover:bg-accent hover:text-accent-foreground transition-all">
            {{ t('nav.getStarted') }}
          </Button>
        </div>
      </div>
    </header>

    <main class="flex-1 w-full">
      <router-view />
    </main>
    
    <footer class="border-t border-border/30 bg-surface/30 py-12 backdrop-blur-sm">
      <div class="mx-auto max-w-7xl px-4 sm:px-6 lg:px-8 flex flex-col md:flex-row justify-between items-center gap-6">
        <div class="flex items-center gap-2">
          <span class="text-xl font-bold tracking-tight text-muted-foreground">TransitHub</span>
        </div>
        <p class="text-sm text-muted-foreground">
          &copy; {{ new Date().getFullYear() }} {{ t('footer.rights') }}
        </p>
      </div>
    </footer>
  </div>
</template>
