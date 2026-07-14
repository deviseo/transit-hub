import gsap from 'gsap'
import { nextTick, onBeforeUnmount, ref } from 'vue'

export type LotteryDrawRevealOutcome = 'won' | 'lost' | 'spectator'
export type LotteryDrawRevealPhase = 'countdown' | 'drawing' | 'result'

export function useLotteryDrawReveal() {
  const isVisible = ref(false)
  const phase = ref<LotteryDrawRevealPhase>('countdown')
  const countdown = ref(3)
  const outcome = ref<LotteryDrawRevealOutcome>('spectator')
  const overlayRef = ref<HTMLElement | null>(null)
  const resultActionRef = ref<HTMLButtonElement | null>(null)

  let timeline: gsap.core.Timeline | null = null

  const animateResult = async () => {
    await nextTick()
    const overlay = overlayRef.value
    if (!overlay) return

    const result = overlay.querySelector<HTMLElement>('[data-draw-result]')
    const reducedMotion = window.matchMedia('(prefers-reduced-motion: reduce)').matches
    if (result) {
      if (reducedMotion) {
        gsap.set(result, { autoAlpha: 1, y: 0, scale: 1 })
      } else {
        gsap.fromTo(
          result,
          { autoAlpha: 0, y: 18, scale: 0.94 },
          { autoAlpha: 1, y: 0, scale: 1, duration: 0.65, ease: 'back.out(1.35)' },
        )
      }
    }

    if (outcome.value === 'won' && !reducedMotion) {
      const particles = Array.from(overlay.querySelectorAll<HTMLElement>('[data-draw-particle]'))
      particles.forEach((particle, index) => {
        const angle = (Math.PI * 2 * index) / Math.max(particles.length, 1) - Math.PI / 2
        const distance = 116 + (index % 5) * 24
        gsap.fromTo(
          particle,
          { autoAlpha: 0, x: 0, y: 0, rotation: 0, scale: 0.2 },
          {
            autoAlpha: 1,
            x: Math.cos(angle) * distance,
            y: Math.sin(angle) * distance + 34,
            rotation: 180 + index * 31,
            scale: 1,
            duration: 1.15 + (index % 4) * 0.08,
            ease: 'power3.out',
          },
        )
        gsap.to(particle, {
          autoAlpha: 0,
          y: `+=${60 + (index % 3) * 18}`,
          duration: 0.65,
          delay: 0.85 + (index % 4) * 0.04,
          ease: 'power1.in',
        })
      })
    }

    resultActionRef.value?.focus({ preventScroll: true })
  }

  const reveal = async (onReveal: () => void | Promise<void>) => {
    await onReveal()
    phase.value = 'result'
    void animateResult()
  }

  const play = async (nextOutcome: LotteryDrawRevealOutcome, onReveal: () => void | Promise<void>) => {
    timeline?.kill()
    outcome.value = nextOutcome
    countdown.value = 3
    phase.value = 'countdown'
    isVisible.value = true

    await nextTick()
    const overlay = overlayRef.value
    if (!overlay) {
      await reveal(onReveal)
      return
    }

    const panel = overlay.querySelector<HTMLElement>('[data-draw-panel]')
    const wheel = overlay.querySelector<HTMLElement>('[data-draw-wheel]')
    const reducedMotion = window.matchMedia('(prefers-reduced-motion: reduce)').matches
    panel?.focus({ preventScroll: true })

    if (reducedMotion) {
      timeline = gsap.timeline()
        .fromTo(overlay, { autoAlpha: 0 }, { autoAlpha: 1, duration: 0.15 })
        .call(() => { countdown.value = 2 }, [], 0.2)
        .call(() => { countdown.value = 1 }, [], 0.45)
        .call(() => { phase.value = 'drawing' }, [], 0.7)
        .call(() => { void reveal(onReveal) }, [], 1.05)
      return
    }

    timeline = gsap.timeline()
    timeline.fromTo(overlay, { autoAlpha: 0 }, { autoAlpha: 1, duration: 0.25, ease: 'power1.out' })
    if (panel) {
      timeline.fromTo(panel, { y: 20, scale: 0.97 }, { y: 0, scale: 1, duration: 0.45, ease: 'power3.out' }, 0)
    }
    if (wheel) {
      timeline
        .fromTo(wheel, { rotation: 0, scale: 0.82 }, { rotation: 360, scale: 1, duration: 0.9, ease: 'power2.out' }, 0.15)
        .to(wheel, { rotation: 1260, scale: 1.08, duration: 2.05, ease: 'power2.inOut' }, 0.85)
        .to(wheel, { rotation: 1620, scale: 1, duration: 0.55, ease: 'power4.out' }, 2.9)
    }
    timeline
      .call(() => { countdown.value = 2 }, [], 0.85)
      .call(() => { countdown.value = 1 }, [], 1.55)
      .call(() => { phase.value = 'drawing' }, [], 2.2)
      .call(() => { void reveal(onReveal) }, [], 3.5)
  }

  const close = () => {
    if (phase.value !== 'result') return
    timeline?.kill()
    timeline = null
    const overlay = overlayRef.value
    if (overlay) gsap.killTweensOf(overlay.querySelectorAll('*'))
    isVisible.value = false
  }

  onBeforeUnmount(() => {
    timeline?.kill()
    const overlay = overlayRef.value
    if (overlay) gsap.killTweensOf(overlay.querySelectorAll('*'))
  })

  return {
    close,
    countdown,
    isVisible,
    outcome,
    overlayRef,
    phase,
    play,
    resultActionRef,
  }
}
