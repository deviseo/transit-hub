<script setup lang="ts">
import { computed, nextTick, ref, watch } from 'vue'
import { Check, ChevronDown, ChevronUp, Link2, Loader2, Search, Server, Settings2, X } from 'lucide-vue-next'
import { t } from '@/locales'
import type { MySiteGroupRef, MySiteUpstreamTargetOption } from '../../types/mySites'
import {
  createDefaultGroupAssociationsPreferences,
  mergeGroupAssociationsGroupOrder,
  readGroupAssociationsPreferences,
  type GroupAssociationsPreferences,
  writeGroupAssociationsPreferences,
} from '../../utils/groupAssociationsPreferences'

interface SiteSection {
  siteId: string
  siteName: string
  platform: string
  options: MySiteUpstreamTargetOption[]
}

const props = defineProps<{
  open: boolean
  ownGroup: string
  options: MySiteUpstreamTargetOption[]
  selected: MySiteGroupRef[]
  preferenceScope: string
  saving?: boolean
}>()

const emit = defineEmits<{
  (event: 'close'): void
  (event: 'save', targets: MySiteGroupRef[]): void
}>()

const search = ref('')
const selectedKeys = ref<string[]>([])
const openedSelectedKeys = ref<string[]>([])
const openedSelectedSiteIds = ref<string[]>([])
const siteManagerOpen = ref(false)
const optionsList = ref<HTMLElement | null>(null)
const preferences = ref<GroupAssociationsPreferences>(createDefaultGroupAssociationsPreferences())
const prefix = 'admin.groupAssociations.targetsDrawer'
const targetKey = (target: MySiteGroupRef): string => `${target.siteId}\u0000${target.groupName}`
const drawerPreferenceScope = computed(() => `drawer:${props.preferenceScope || 'anonymous'}`)

const updatePreferences = (updater: (current: GroupAssociationsPreferences) => GroupAssociationsPreferences) => {
  const next = updater(preferences.value)
  preferences.value = next
  writeGroupAssociationsPreferences(drawerPreferenceScope.value, next)
}

watch(drawerPreferenceScope, (scope) => {
  preferences.value = readGroupAssociationsPreferences(scope)
}, { immediate: true })

watch(() => props.open, async (open) => {
  if (!open) return
  search.value = ''
  selectedKeys.value = props.selected.map(targetKey)
  openedSelectedKeys.value = [...selectedKeys.value]
  openedSelectedSiteIds.value = Array.from(new Set(props.selected.map(target => target.siteId)))
  siteManagerOpen.value = false
  preferences.value = readGroupAssociationsPreferences(drawerPreferenceScope.value)

  await nextTick()
  const list = optionsList.value
  if (!list) return
  list.scrollTop = 0
  const openedSelectedSites = new Set(openedSelectedSiteIds.value)
  const firstSelectedSiteId = displaySiteSections.value.find(section => openedSelectedSites.has(section.siteId))?.siteId
  if (!firstSelectedSiteId) return
  const siteElement = Array.from(list.querySelectorAll<HTMLElement>('[data-site-id]'))
    .find(element => element.dataset.siteId === firstSelectedSiteId)
  siteElement?.scrollIntoView({ block: 'start' })
})

const siteSections = computed<SiteSection[]>(() => {
  const sections = new Map<string, SiteSection>()
  for (const option of props.options) {
    const existing = sections.get(option.siteId)
    if (existing) {
      existing.options.push(option)
      continue
    }
    sections.set(option.siteId, {
      siteId: option.siteId,
      siteName: option.siteName,
      platform: option.platform,
      options: [option],
    })
  }
  return [...sections.values()]
})

const orderedSiteSections = computed(() => {
  const currentIds = siteSections.value.map(section => section.siteId)
  const order = mergeGroupAssociationsGroupOrder(preferences.value.groupOrder, currentIds)
  const sectionsById = new Map(siteSections.value.map(section => [section.siteId, section]))
  return order.flatMap((siteId) => {
    const section = sectionsById.get(siteId)
    return section ? [section] : []
  })
})

const displaySiteSections = computed(() => {
  const selectedSites = new Set(openedSelectedSiteIds.value)
  return [
    ...orderedSiteSections.value.filter(section => selectedSites.has(section.siteId)),
    ...orderedSiteSections.value.filter(section => !selectedSites.has(section.siteId)),
  ]
})

const visibleSiteSections = computed(() => {
  const hiddenSites = new Set(preferences.value.hiddenGroupIds)
  const protectedKeys = new Set(openedSelectedKeys.value)
  return displaySiteSections.value.flatMap((section) => {
    if (!hiddenSites.has(section.siteId)) return [section]
    const protectedOptions = section.options.filter(option => protectedKeys.has(targetKey(option)))
    return protectedOptions.length > 0 ? [{ ...section, options: protectedOptions }] : []
  })
})

const filteredSiteSections = computed(() => {
  const query = search.value.trim().toLocaleLowerCase()
  if (!query) return visibleSiteSections.value
  return visibleSiteSections.value.flatMap((section) => {
    const siteMatches = section.siteName.toLocaleLowerCase().includes(query)
      || section.platform.toLocaleLowerCase().includes(query)
    const options = siteMatches
      ? section.options
      : section.options.filter(option => option.groupName.toLocaleLowerCase().includes(query))
    return options.length > 0 ? [{ ...section, options }] : []
  })
})

const emptyState = computed(() => {
  if (props.options.length === 0) {
    return { title: `${prefix}.noOptionsTitle`, description: `${prefix}.noOptionsDescription` }
  }
  if (visibleSiteSections.value.length === 0) {
    return { title: `${prefix}.noVisibleSitesTitle`, description: `${prefix}.noVisibleSitesDescription` }
  }
  if (filteredSiteSections.value.length === 0) {
    return { title: `${prefix}.emptyTitle`, description: `${prefix}.emptyDescription` }
  }
  return null
})

const isSelected = (option: MySiteGroupRef): boolean => selectedKeys.value.includes(targetKey(option))

const toggle = (option: MySiteUpstreamTargetOption) => {
  const key = targetKey(option)
  const index = selectedKeys.value.indexOf(key)
  if (index >= 0) selectedKeys.value.splice(index, 1)
  else selectedKeys.value.push(key)
}

const moveSite = (siteId: string, offset: -1 | 1) => {
  const ids = orderedSiteSections.value.map(section => section.siteId)
  const index = ids.indexOf(siteId)
  const targetIndex = index + offset
  if (index < 0 || targetIndex < 0 || targetIndex >= ids.length) return
  const nextOrder = [...ids]
  ;[nextOrder[index], nextOrder[targetIndex]] = [nextOrder[targetIndex], nextOrder[index]]
  updatePreferences(current => ({ ...current, groupOrder: nextOrder }))
}

const toggleSiteVisibility = (siteId: string) => {
  updatePreferences((current) => {
    const hiddenSites = new Set(current.hiddenGroupIds)
    if (hiddenSites.has(siteId)) hiddenSites.delete(siteId)
    else hiddenSites.add(siteId)
    return { ...current, hiddenGroupIds: [...hiddenSites] }
  })
}

const submit = () => {
  const selected = new Set(selectedKeys.value)
  emit('save', props.options
    .filter(option => selected.has(targetKey(option)))
    .map(option => ({ siteId: option.siteId, groupName: option.groupName })))
}

const formatMultiplier = (value: number | null): string => {
  if (value == null || !Number.isFinite(value)) return t(`${prefix}.unknownMultiplier`)
  return t(`${prefix}.multiplier`, { value: Number(value.toFixed(4)).toString() })
}

const formatOptionMultiplier = (option: MySiteUpstreamTargetOption): string => {
  if (option.multiplierMode === 'auto') return t(`${prefix}.autoMultiplier`)
  return formatMultiplier(option.multiplier)
}
</script>

<template>
  <Teleport to="body">
    <Transition
      enter-active-class="transition duration-200 ease-out"
      enter-from-class="opacity-0"
      enter-to-class="opacity-100"
      leave-active-class="transition duration-150 ease-in"
      leave-from-class="opacity-100"
      leave-to-class="opacity-0"
    >
      <div v-if="open" class="fixed inset-0 z-[160]">
        <div class="absolute inset-0 bg-background/70 backdrop-blur-sm" @click="saving ? undefined : emit('close')" />
        <aside
          role="dialog"
          aria-modal="true"
          :aria-label="t(`${prefix}.titleWithGroup`, { group: ownGroup })"
          class="absolute inset-y-0 right-0 flex w-full max-w-xl flex-col border-l border-border/60 bg-card shadow-2xl"
        >
          <header class="flex items-start justify-between gap-4 border-b border-border/60 px-5 py-4">
            <div class="min-w-0">
              <div class="flex items-center gap-2 text-foreground">
                <Link2 class="h-4 w-4 text-primary" />
                <h2 class="truncate text-sm font-semibold">{{ t(`${prefix}.titleWithGroup`, { group: ownGroup }) }}</h2>
              </div>
              <p class="mt-1 text-xs text-muted-foreground">{{ t(`${prefix}.selectedCount`, { count: selectedKeys.length }) }}</p>
            </div>
            <button
              type="button"
              class="inline-flex h-8 w-8 shrink-0 items-center justify-center rounded-md text-muted-foreground transition-colors hover:bg-surface-elevated hover:text-foreground focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-primary"
              :aria-label="t(`${prefix}.close`)"
              :disabled="saving"
              @click="emit('close')"
            >
              <X class="h-4 w-4" />
            </button>
          </header>

          <div class="space-y-3 border-b border-border/50 px-5 py-4">
            <div class="relative">
              <Search class="pointer-events-none absolute left-3 top-1/2 h-4 w-4 -translate-y-1/2 text-muted-foreground" />
              <input
                v-model="search"
                type="search"
                :placeholder="t(`${prefix}.searchPlaceholder`)"
                :aria-label="t(`${prefix}.searchLabel`)"
                class="h-10 w-full rounded-lg border border-border/60 bg-background pl-9 pr-3 text-sm text-foreground outline-none placeholder:text-muted-foreground focus-visible:border-primary focus-visible:ring-2 focus-visible:ring-primary/25"
              >
            </div>
            <button
              type="button"
              class="inline-flex h-9 w-full items-center justify-center gap-2 rounded-lg border border-border/60 bg-background px-3 text-sm text-foreground transition-colors hover:bg-surface focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-primary"
              :aria-expanded="siteManagerOpen"
              @click="siteManagerOpen = !siteManagerOpen"
            >
              <Settings2 class="h-4 w-4" />
              {{ t(`${prefix}.manageSites`) }}
            </button>
            <div v-if="siteManagerOpen" class="max-h-64 space-y-2 overflow-y-auto rounded-lg border border-border/60 bg-background p-2">
              <div class="flex items-center justify-between gap-2 px-1 text-xs text-muted-foreground">
                <span>{{ t(`${prefix}.siteDisplayTitle`) }}</span>
                <span>{{ orderedSiteSections.length }}</span>
              </div>
              <div v-if="orderedSiteSections.length === 0" class="px-1 py-3 text-xs text-muted-foreground">
                {{ t(`${prefix}.siteDisplayEmpty`) }}
              </div>
              <div
                v-for="(section, index) in orderedSiteSections"
                :key="`manage:${section.siteId}`"
                class="flex items-center gap-2 rounded-md px-1 py-1.5 hover:bg-surface/60"
              >
                <label class="flex min-w-0 flex-1 cursor-pointer items-center gap-2">
                  <input
                    type="checkbox"
                    :checked="!preferences.hiddenGroupIds.includes(section.siteId)"
                    class="h-4 w-4 rounded border-border accent-primary"
                    @change="toggleSiteVisibility(section.siteId)"
                  >
                  <span class="min-w-0 truncate text-xs text-foreground">{{ section.siteName }}</span>
                </label>
                <span class="shrink-0 text-[11px] text-muted-foreground">
                  {{ t(`${prefix}.siteGroupCount`, { count: section.options.length }) }}
                </span>
                <button
                  type="button"
                  class="rounded p-1 text-muted-foreground transition-colors hover:bg-surface hover:text-foreground disabled:opacity-30"
                  :aria-label="t(`${prefix}.moveSiteUp`, { site: section.siteName })"
                  :title="t(`${prefix}.moveUp`)"
                  :disabled="index === 0"
                  @click="moveSite(section.siteId, -1)"
                >
                  <ChevronUp class="h-4 w-4" />
                </button>
                <button
                  type="button"
                  class="rounded p-1 text-muted-foreground transition-colors hover:bg-surface hover:text-foreground disabled:opacity-30"
                  :aria-label="t(`${prefix}.moveSiteDown`, { site: section.siteName })"
                  :title="t(`${prefix}.moveDown`)"
                  :disabled="index === orderedSiteSections.length - 1"
                  @click="moveSite(section.siteId, 1)"
                >
                  <ChevronDown class="h-4 w-4" />
                </button>
              </div>
            </div>
          </div>

          <div ref="optionsList" class="min-h-0 flex-1 overflow-y-auto px-3 py-3">
            <div v-if="emptyState" class="flex min-h-56 flex-col items-center justify-center px-6 text-center">
              <Server class="h-8 w-8 text-muted-foreground/50" />
              <p class="mt-3 text-sm font-medium text-foreground">{{ t(emptyState.title) }}</p>
              <p class="mt-1 text-xs text-muted-foreground">{{ t(emptyState.description) }}</p>
            </div>

            <template v-else>
              <section
                v-for="section in filteredSiteSections"
                :key="section.siteId"
                :data-site-id="section.siteId"
                class="border-b border-border/50 pb-3 pt-1 first:pt-0 last:border-b-0 last:pb-0"
              >
                <header class="flex min-w-0 items-center justify-between gap-3 px-3 py-2">
                  <span class="flex min-w-0 items-center gap-2">
                    <Server class="h-4 w-4 shrink-0 text-primary" />
                    <span class="min-w-0">
                      <span class="block truncate text-sm font-semibold text-foreground">{{ section.siteName }}</span>
                      <span v-if="section.platform" class="mt-0.5 block truncate text-[11px] text-muted-foreground">{{ section.platform }}</span>
                    </span>
                  </span>
                  <span class="shrink-0 text-[11px] text-muted-foreground">
                    {{ t(`${prefix}.siteGroupCount`, { count: section.options.length }) }}
                  </span>
                </header>
                <label
                  v-for="option in section.options"
                  :key="targetKey(option)"
                  class="mb-1 flex cursor-pointer items-center gap-3 rounded-lg border px-3 py-3 transition-colors last:mb-0"
                  :class="isSelected(option) ? 'border-primary/35 bg-primary/[0.06]' : 'border-transparent hover:border-border/60 hover:bg-surface/45'"
                >
                  <input
                    type="checkbox"
                    class="sr-only"
                    :checked="isSelected(option)"
                    :aria-label="t(`${prefix}.targetLabel`, { site: section.siteName, group: option.groupName })"
                    @change="toggle(option)"
                  >
                  <span
                    class="flex h-5 w-5 shrink-0 items-center justify-center rounded border"
                    :class="isSelected(option) ? 'border-primary bg-primary text-primary-foreground' : 'border-border bg-background'"
                  >
                    <Check v-if="isSelected(option)" class="h-3.5 w-3.5" />
                  </span>
                  <span class="min-w-0 flex-1">
                    <span class="flex flex-wrap items-center gap-2">
                      <span class="truncate text-sm font-medium text-foreground">{{ option.groupName }}</span>
                      <span v-if="option.stale" class="rounded border border-warning/30 bg-warning/10 px-1.5 py-0.5 text-[10px] font-medium text-warning">
                        {{ t(`${prefix}.stale`) }}
                      </span>
                    </span>
                  </span>
                  <span class="shrink-0 text-xs font-medium tabular-nums text-foreground">{{ formatOptionMultiplier(option) }}</span>
                </label>
              </section>
            </template>
          </div>

          <footer class="flex items-center justify-between gap-3 border-t border-border/60 bg-card px-5 py-4">
            <span class="text-xs text-muted-foreground">{{ t(`${prefix}.selectedCount`, { count: selectedKeys.length }) }}</span>
            <div class="flex items-center gap-2">
              <button
                type="button"
                class="rounded-lg border border-border/60 px-4 py-2 text-sm font-medium text-muted-foreground transition-colors hover:bg-surface-elevated hover:text-foreground disabled:opacity-50"
                :disabled="saving"
                @click="emit('close')"
              >
                {{ t(`${prefix}.cancel`) }}
              </button>
              <button
                type="button"
                class="inline-flex items-center gap-2 rounded-lg bg-primary px-4 py-2 text-sm font-medium text-primary-foreground transition-colors hover:bg-primary/90 disabled:opacity-50"
                :disabled="saving"
                @click="submit"
              >
                <Loader2 v-if="saving" class="h-4 w-4 animate-spin" />
                <Check v-else class="h-4 w-4" />
                {{ saving ? t(`${prefix}.saving`) : t(`${prefix}.save`) }}
              </button>
            </div>
          </footer>
        </aside>
      </div>
    </Transition>
  </Teleport>
</template>
