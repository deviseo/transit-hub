export interface ConnectionHealthPreferences {
  version: 1
  groupOrder: string[]
  hiddenGroupIds: string[]
  hideUnmonitoredAccounts: boolean
}

export const createDefaultConnectionHealthPreferences = (): ConnectionHealthPreferences => ({
  version: 1,
  groupOrder: [],
  hiddenGroupIds: [],
  hideUnmonitoredAccounts: false,
})

export const connectionHealthPreferencesStorageKey = (scope: string): string =>
  `transithub.connection-health.preferences.v1:${scope || 'anonymous'}`

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

const normalizePreferences = (value: unknown): ConnectionHealthPreferences => {
  if (!isRecord(value) || value.version !== 1) return createDefaultConnectionHealthPreferences()
  return {
    version: 1,
    groupOrder: normalizeIds(value.groupOrder),
    hiddenGroupIds: normalizeIds(value.hiddenGroupIds),
    hideUnmonitoredAccounts: value.hideUnmonitoredAccounts === true,
  }
}

export const readConnectionHealthPreferences = (scope: string): ConnectionHealthPreferences => {
  try {
    if (typeof window === 'undefined') return createDefaultConnectionHealthPreferences()
    const raw = window.localStorage.getItem(connectionHealthPreferencesStorageKey(scope))
    if (!raw) return createDefaultConnectionHealthPreferences()
    return normalizePreferences(JSON.parse(raw) as unknown)
  } catch {
    return createDefaultConnectionHealthPreferences()
  }
}

export const writeConnectionHealthPreferences = (
  scope: string,
  preferences: ConnectionHealthPreferences,
): void => {
  try {
    if (typeof window === 'undefined') return
    window.localStorage.setItem(
      connectionHealthPreferencesStorageKey(scope),
      JSON.stringify(normalizePreferences(preferences)),
    )
  } catch {
    // Local display preferences are optional and must not block the health page.
  }
}

export const mergeConnectionHealthGroupOrder = (
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
