<script setup lang="ts">
import { LogOut, AlertTriangle } from 'lucide-vue-next'
import { Button } from '@/components/ui/button'

// 退出 admin 账户的二次确认弹窗，防止误触登出。
// 结构参照 AdminLoginModal 的 Teleport 遮罩弹窗，颜色统一使用主题 token + dark: 适配。
defineProps<{
  open: boolean
}>()

const emit = defineEmits<{
  (event: 'confirm'): void
  (event: 'cancel'): void
}>()

import { t } from '@/locales'
</script>

<template>
  <Teleport defer to="body">
    <div v-if="open" class="fixed inset-0 z-[110] flex items-center justify-center p-4">
      <div class="absolute inset-0 bg-background/80 backdrop-blur-sm" @click="emit('cancel')"></div>

      <div
        role="alertdialog"
        aria-modal="true"
        aria-labelledby="admin-logout-confirm-title"
        class="relative w-full max-w-sm overflow-hidden rounded-[1.75rem] border border-border/60 bg-card text-card-foreground shadow-2xl shadow-primary/10 animate-in fade-in zoom-in-95 duration-200"
      >
        <div class="flex flex-col items-center gap-4 px-6 py-7 text-center">
          <div class="flex h-12 w-12 items-center justify-center rounded-full bg-red-500/10 text-red-500 dark:text-red-400">
            <AlertTriangle class="h-6 w-6" />
          </div>
          <div class="space-y-1.5">
            <h2 id="admin-logout-confirm-title" class="text-lg font-semibold text-foreground">{{ t('admin.dashboard.adminAuth.logoutConfirm.title') }}</h2>
            <p class="text-sm text-muted-foreground">{{ t('admin.dashboard.adminAuth.logoutConfirm.description') }}</p>
          </div>

          <div class="mt-2 flex w-full items-center gap-3">
            <Button type="button" variant="secondary" class="flex-1" @click="emit('cancel')">
              {{ t('admin.dashboard.adminAuth.logoutConfirm.cancel') }}
            </Button>
            <Button
              type="button"
              class="flex-1 bg-red-500 text-white hover:bg-red-600 dark:bg-red-500 dark:hover:bg-red-600"
              @click="emit('confirm')"
            >
              <LogOut class="h-4 w-4" />
              {{ t('admin.dashboard.adminAuth.logoutConfirm.confirm') }}
            </Button>
          </div>
        </div>
      </div>
    </div>
  </Teleport>
</template>
