export const WORKSPACE_DELETE_CONFIRMATION = 'DELETE WORKSPACE' as const

export type WorkspaceDeleteConfirmation = typeof WORKSPACE_DELETE_CONFIRMATION

export interface AdminAccount {
  id: string
  platform: string
  baseUrl: string
  identity: string
  displayName: string
  authMethod: string
  current: boolean
  lastUsedAt: string | null
  createdAt: string
  updatedAt: string
}

export interface DeleteAdminAccountResponse {
  deletedId: string
  hasCurrent: boolean
  currentAdminAccountId: string | null
  cleanupComplete: boolean
  cleanupPending: boolean
}
