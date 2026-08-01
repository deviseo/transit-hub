export interface GroupAssociationsPreferences {
  version: 1
  groupOrder: string[]
  hiddenGroupIds: string[]
}

export const createDefaultGroupAssociationsPreferences = (): GroupAssociationsPreferences => ({
  version: 1,
  groupOrder: [],
  hiddenGroupIds: [],
})

export const groupAssociationsPreferencesStorageKey = (scope: string): string =>
  `transithub.group-associations.preferences.v1:${scope || 'anonymous'}`

const normalizeIds = (value: unknown): string[] => {
  if (!Array.isArray(value)) return []
  return Array.from(new Set(
    value
      .filter((item): item is string => typeof item === 'string')
      .map(item => item.trim())
      .filter(Boolean),
  ))
}

const isRecord = (value: unknown): value is Record<string, unknown> =>
  typeof value === 'object' && value !== null

const normalizePreferences = (value: unknown): GroupAssociationsPreferences => {
  if (!isRecord(value) || value.version !== 1) return createDefaultGroupAssociationsPreferences()
  return {
    version: 1,
    groupOrder: normalizeIds(value.groupOrder),
    hiddenGroupIds: normalizeIds(value.hiddenGroupIds),
  }
}

export const readGroupAssociationsPreferences = (scope: string): GroupAssociationsPreferences => {
  try {
    if (typeof window === 'undefined') return createDefaultGroupAssociationsPreferences()
    const raw = window.localStorage.getItem(groupAssociationsPreferencesStorageKey(scope))
    if (!raw) return createDefaultGroupAssociationsPreferences()
    return normalizePreferences(JSON.parse(raw) as unknown)
  } catch {
    return createDefaultGroupAssociationsPreferences()
  }
}

export const writeGroupAssociationsPreferences = (
  scope: string,
  preferences: GroupAssociationsPreferences,
): void => {
  try {
    if (typeof window === 'undefined') return
    window.localStorage.setItem(
      groupAssociationsPreferencesStorageKey(scope),
      JSON.stringify(normalizePreferences(preferences)),
    )
  } catch {
    // Local display preferences are optional and must not block the page.
  }
}

export const mergeGroupAssociationsGroupOrder = (
  storedOrder: string[],
  currentIds: string[],
): string[] => {
  const currentSet = new Set(currentIds)
  const result: string[] = []
  const seen = new Set<string>()

  for (const id of storedOrder) {
    if (currentSet.has(id) && !seen.has(id)) {
      result.push(id)
      seen.add(id)
    }
  }

  for (const id of currentIds) {
    if (!seen.has(id)) {
      result.push(id)
      seen.add(id)
    }
  }

  return result
}
