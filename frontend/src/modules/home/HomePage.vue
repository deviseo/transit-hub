<script setup lang="ts">
import { ref, onMounted, computed } from 'vue'
import { Button } from '@/components/ui/button'
import { usePortalAnimations } from './composables/usePortalAnimations'
import { t } from '@/locales'
// Refs for animations
const badgeRef = ref<HTMLElement | null>(null)
const titleRef = ref<HTMLElement | null>(null)
const subtitleRef = ref<HTMLElement | null>(null)
const actionsRef = ref<HTMLElement | null>(null)
const visualRef = ref<HTMLElement | null>(null)

const featuresContainer = ref<HTMLElement | null>(null)
const featureCards = ref<HTMLElement[]>([])

const ctaContainer = ref<HTMLElement | null>(null)

const { playHeroEntrance, setupScrollAnimations } = usePortalAnimations()

const features = computed(() => [
  {
    icon: '⚡',
    title: t('features.items.sync.title'),
    description: t('features.items.sync.desc'),
    color: 'from-primary/20 to-transparent'
  },
  {
    icon: '🛡️',
    title: t('features.items.fallback.title'),
    description: t('features.items.fallback.desc'),
    color: 'from-accent/20 to-transparent'
  },
  {
    icon: '📊',
    title: t('features.items.observe.title'),
    description: t('features.items.observe.desc'),
    color: 'from-signal/20 to-transparent'
  },
  {
    icon: '🚀',
    title: t('features.items.selfhost.title'),
    description: t('features.items.selfhost.desc'),
    color: 'from-warning/20 to-transparent'
  }
])

onMounted(() => {
  // Play Hero Entrance
  playHeroEntrance({
    badge: badgeRef,
    title: titleRef,
    subtitle: subtitleRef,
    actions: actionsRef,
    visual: visualRef
  })

  // Setup Scroll Animations
  setupScrollAnimations(featuresContainer, featureCards, ctaContainer)
})
</script>

<template>
  <div class="flex flex-col w-full overflow-hidden">
    
    <!-- Hero Section -->
    <section class="relative flex min-h-dvh items-center justify-center pt-20">
      <!-- Background Abstract Elements -->
      <div class="absolute inset-0 -z-10 overflow-hidden">
        <div class="absolute top-1/4 left-1/4 h-[500px] w-[500px] -translate-x-1/2 -translate-y-1/2 rounded-full bg-primary/20 blur-[120px]" />
        <div class="absolute bottom-1/4 right-1/4 h-[600px] w-[600px] translate-x-1/3 translate-y-1/3 rounded-full bg-accent/15 blur-[150px]" />
        <div class="absolute inset-0 bg-[linear-gradient(rgba(var(--foreground),0.05)_1px,transparent_1px),linear-gradient(90deg,rgba(var(--foreground),0.05)_1px,transparent_1px)] bg-[size:64px_64px] [mask-image:radial-gradient(ellipse_80%_80%_at_50%_50%,#000_20%,transparent_100%)]" />
      </div>

      <div class="container mx-auto grid gap-16 px-4 lg:grid-cols-2 lg:gap-8 lg:px-8 xl:gap-24 items-center">
        <!-- Hero Text -->
        <div class="flex flex-col items-center text-center lg:items-start lg:text-left">
          <div ref="badgeRef" class="mb-8 inline-flex items-center gap-2 rounded-full border border-primary/30 bg-primary/10 px-4 py-2 text-sm font-medium text-primary backdrop-blur-md">
            <span class="relative flex h-2.5 w-2.5">
              <span class="absolute inline-flex h-full w-full animate-ping rounded-full bg-primary opacity-75"></span>
              <span class="relative inline-flex h-2.5 w-2.5 rounded-full bg-primary"></span>
            </span>
            {{ t('hero.badge') }}
          </div>
          
          <h1 ref="titleRef" class="mb-6 text-5xl font-black leading-tight tracking-tighter sm:text-6xl md:text-7xl lg:text-[5.5rem] lg:leading-[1.1]">
            {{ t('hero.title') }} <br/>
            <span class="bg-gradient-to-r from-primary via-accent to-primary bg-clip-text text-transparent">{{ t('hero.highlight') }}</span>
          </h1>
          
          <p ref="subtitleRef" class="mb-10 max-w-2xl text-lg text-muted-foreground sm:text-xl lg:text-2xl leading-relaxed">
            {{ t('hero.subtitle') }}
          </p>
          
          <div ref="actionsRef" class="flex flex-col sm:flex-row items-center gap-4 w-full justify-center lg:justify-start">
            <Button size="lg" class="w-full sm:w-auto h-14 rounded-full px-8 text-lg font-bold bg-primary text-primary-foreground shadow-[0_0_40px_rgba(var(--primary),0.4)] hover:shadow-[0_0_60px_rgba(var(--primary),0.6)] hover:bg-primary/90 transition-all duration-300">
              {{ t('hero.startBtn') }}
            </Button>
            <Button variant="secondary" size="lg" class="w-full sm:w-auto h-14 rounded-full px-8 text-lg font-medium border-border/50 bg-background/50 backdrop-blur-md hover:bg-surface-elevated transition-all duration-300">
              {{ t('hero.docBtn') }}
            </Button>
          </div>
        </div>

        <!-- Hero Visual -->
        <div ref="visualRef" class="relative mx-auto w-full max-w-lg lg:max-w-none perspective-1000">
          <div class="relative w-full aspect-square rounded-[3rem] border border-border/50 bg-surface/40 p-8 backdrop-blur-2xl shadow-2xl rotate-y-[-10deg] rotate-x-[5deg] transform-style-3d">
            <!-- Glass Panes inside visual -->
            <div class="absolute inset-0 rounded-[3rem] bg-gradient-to-tr from-primary/10 via-transparent to-accent/10 opacity-50"></div>
            
            <div class="flex h-full flex-col gap-4 relative z-10">
              <!-- Mock UI Header -->
              <div class="flex items-center justify-between border-b border-border/40 pb-4">
                <div class="flex gap-2">
                  <div class="h-3 w-3 rounded-full bg-red-500/80"></div>
                  <div class="h-3 w-3 rounded-full bg-yellow-500/80"></div>
                  <div class="h-3 w-3 rounded-full bg-green-500/80"></div>
                </div>
                <div class="rounded-full bg-surface-elevated px-3 py-1 text-xs font-mono text-muted-foreground">global-router-1</div>
              </div>
              
              <!-- Mock UI Body -->
              <div class="flex-1 space-y-4 pt-4">
                <div class="flex items-center justify-between rounded-2xl bg-surface-elevated/50 p-4 border border-border/30">
                  <div class="flex items-center gap-3">
                    <div class="h-10 w-10 rounded-xl bg-primary/20 flex items-center justify-center text-primary font-bold">N</div>
                    <div>
                      <div class="text-sm font-bold text-foreground">US-East Node</div>
                      <div class="text-xs text-muted-foreground">NewAPI • 99.9% Uptime</div>
                    </div>
                  </div>
                  <div class="text-right">
                    <div class="text-sm font-bold text-signal">Active</div>
                    <div class="text-xs text-muted-foreground">1.2k req/s</div>
                  </div>
                </div>

                <div class="flex items-center justify-between rounded-2xl bg-surface-elevated/50 p-4 border border-border/30">
                  <div class="flex items-center gap-3">
                    <div class="h-10 w-10 rounded-xl bg-accent/20 flex items-center justify-center text-accent font-bold">S</div>
                    <div>
                      <div class="text-sm font-bold text-foreground">EU-Central Sub</div>
                      <div class="text-xs text-muted-foreground">Sub2API • 98.2% Uptime</div>
                    </div>
                  </div>
                  <div class="text-right">
                    <div class="text-sm font-bold text-warning">Syncing</div>
                    <div class="text-xs text-muted-foreground">840 req/s</div>
                  </div>
                </div>

                <div class="flex-1 rounded-2xl bg-surface-elevated/30 border border-border/20 p-4 relative overflow-hidden mt-4">
                  <div class="absolute bottom-0 left-0 right-0 h-1/2 bg-gradient-to-t from-primary/10 to-transparent"></div>
                  <!-- Abstract Chart Lines -->
                  <svg class="absolute bottom-0 left-0 h-full w-full" preserveAspectRatio="none" viewBox="0 0 100 100">
                    <path d="M0,100 L0,80 Q25,60 50,80 T100,40 L100,100 Z" fill="rgba(var(--primary), 0.1)" />
                    <path d="M0,80 Q25,60 50,80 T100,40" fill="none" stroke="hsl(var(--primary))" stroke-width="2" />
                  </svg>
                </div>
              </div>
            </div>
          </div>
        </div>
      </div>
    </section>

    <!-- Features Section -->
    <section ref="featuresContainer" class="relative py-32 bg-surface/30 border-y border-border/30">
      <div class="container mx-auto px-4 sm:px-6 lg:px-8">
        <div class="text-center mb-20">
          <h2 class="text-3xl font-bold tracking-tight sm:text-5xl mb-6">{{ t('features.title') }}</h2>
          <p class="text-lg text-muted-foreground max-w-2xl mx-auto">
            {{ t('features.subtitle') }}
          </p>
        </div>

        <div class="grid gap-8 md:grid-cols-2 lg:grid-cols-4">
          <div
            v-for="(feature, idx) in features"
            :key="idx"
            ref="featureCards"
            class="group relative overflow-hidden rounded-[2rem] border border-border/50 bg-card p-8 transition-all hover:-translate-y-2 hover:shadow-2xl hover:shadow-primary/10"
          >
            <div class="absolute inset-0 bg-gradient-to-br opacity-0 transition-opacity duration-500 group-hover:opacity-100" :class="feature.color" />
            <div class="relative z-10">
              <div class="mb-6 flex h-14 w-14 items-center justify-center rounded-2xl bg-surface-elevated border border-border/50 text-2xl shadow-inner">
                {{ feature.icon }}
              </div>
              <h3 class="mb-3 text-xl font-bold text-foreground">{{ feature.title }}</h3>
              <p class="text-muted-foreground leading-relaxed">{{ feature.description }}</p>
            </div>
          </div>
        </div>
      </div>
    </section>

    <!-- CTA Section -->
    <section ref="ctaContainer" class="relative py-32 overflow-hidden">
      <div class="absolute inset-0 bg-[radial-gradient(circle_at_center,hsl(var(--primary)/0.1),transparent_50%)]" />
      <div class="container mx-auto px-4 text-center relative z-10">
        <h2 class="text-4xl md:text-6xl font-black tracking-tight mb-8">{{ t('cta.title') }}</h2>
        <p class="text-xl text-muted-foreground mb-12 max-w-2xl mx-auto">
          {{ t('cta.subtitle') }}
        </p>
        <div class="flex flex-col sm:flex-row justify-center gap-4">
          <Button size="lg" class="h-14 rounded-full px-10 text-lg bg-foreground text-background hover:bg-foreground/90 transition-all">
            {{ t('cta.deployBtn') }}
          </Button>
          <Button variant="secondary" size="lg" class="h-14 rounded-full px-10 text-lg border-border/50 bg-surface/50 backdrop-blur-md hover:bg-surface-elevated">
            {{ t('cta.salesBtn') }}
          </Button>
        </div>
      </div>
    </section>

  </div>
</template>

<style scoped>
.perspective-1000 {
  perspective: 1000px;
}
.transform-style-3d {
  transform-style: preserve-3d;
}
.rotate-y-\[-10deg\] {
  transform: rotateY(-10deg);
}
.rotate-x-\[5deg\] {
  transform: rotateX(5deg);
}
</style>
