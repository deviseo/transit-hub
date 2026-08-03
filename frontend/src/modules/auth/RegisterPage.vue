<script setup lang="ts">
import { ref } from 'vue'
import { useRouter } from 'vue-router'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import { ArrowLeft, Mail, KeyRound, ShieldCheck } from 'lucide-vue-next'
import { registerWithEmail, requestEmailCode, storeAccessToken } from './api/auth'

import { t } from '@/locales'
const router = useRouter()

const email = ref('')
const password = ref('')
const code = ref('')
const isLoading = ref(false)
const isSendingCode = ref(false)
const statusKey = ref<string | null>(null)
const statusParams = ref<Record<string, string>>({})
const errorKey = ref<string | null>(null)

const handleSendCode = async () => {
  if (!email.value) return
  isSendingCode.value = true
  statusKey.value = null
  statusParams.value = {}
  errorKey.value = null

  try {
    const response = await requestEmailCode({ email: email.value })
    statusKey.value = 'auth.register.codeSentSuccess'
    statusParams.value = { code: response.code }
  } catch (error) {
    errorKey.value = error instanceof Error ? error.message : 'auth.errors.unknown'
  } finally {
    isSendingCode.value = false
  }
}

const handleRegister = async () => {
  isLoading.value = true
  statusKey.value = null
  statusParams.value = {}
  errorKey.value = null

  try {
    const response = await registerWithEmail({
      email: email.value,
      password: password.value,
      code: code.value,
    })
    storeAccessToken(response.accessToken)
    statusKey.value = 'auth.register.success'
    await router.push('/admin')
  } catch (error) {
    errorKey.value = error instanceof Error ? error.message : 'auth.errors.unknown'
  } finally {
    isLoading.value = false
  }
}
</script>

<template>
  <div class="relative flex min-h-dvh items-center justify-center overflow-hidden bg-background p-4">
    <!-- Background abstract -->
    <div class="absolute inset-0 -z-10 overflow-hidden">
      <div class="absolute top-0 left-1/2 -translate-x-1/2 w-[800px] h-[400px] bg-primary/10 blur-[100px] rounded-full" />
    </div>

    <!-- Back Button -->
    <button 
      @click="router.push('/')" 
      class="absolute top-8 left-8 flex items-center gap-2 text-muted-foreground hover:text-foreground transition-colors"
    >
      <ArrowLeft class="w-4 h-4" />
      {{ t('auth.backToHome') }}
    </button>

    <div class="w-full max-w-md">
      <div class="bg-surface-elevated border border-border/50 rounded-[2rem] p-8 shadow-2xl backdrop-blur-xl relative overflow-hidden">
        <div class="absolute top-0 left-0 right-0 h-1 bg-gradient-to-r from-primary via-accent to-primary" />
        
        <div class="text-center mb-8">
          <div class="inline-flex h-12 w-12 items-center justify-center rounded-xl bg-primary/10 border border-primary/20 mb-4">
            <span class="text-2xl font-black text-primary leading-none">T</span>
          </div>
          <h2 class="text-2xl font-bold tracking-tight text-foreground">{{ t('auth.register.title') }}</h2>
          <p class="text-sm text-muted-foreground mt-2">{{ t('auth.register.subtitle') }}</p>
        </div>

        <form @submit.prevent="handleRegister" class="space-y-5">
          <div class="space-y-2">
            <label class="text-sm font-medium text-foreground">{{ t('auth.register.email') }}</label>
            <div class="relative">
              <Mail class="absolute left-3 top-1/2 -translate-y-1/2 w-4 h-4 text-muted-foreground" />
              <Input 
                v-model="email"
                type="email" 
                :placeholder="t('auth.register.emailPlaceholder')" 
                class="pl-10 h-12 bg-surface border-border/50 focus:border-primary"
                autocomplete="email"
                :disabled="isLoading"
                required
              />
            </div>
          </div>

          <div class="space-y-2">
            <label class="text-sm font-medium text-foreground">{{ t('auth.register.password') }}</label>
            <div class="relative">
              <KeyRound class="absolute left-3 top-1/2 -translate-y-1/2 w-4 h-4 text-muted-foreground" />
              <Input 
                v-model="password"
                type="password" 
                :placeholder="t('auth.register.passwordPlaceholder')" 
                class="pl-10 h-12 bg-surface border-border/50 focus:border-primary"
                autocomplete="new-password"
                :disabled="isLoading"
                required
              />
            </div>
          </div>

          <div class="space-y-2">
            <label class="text-sm font-medium text-foreground">{{ t('auth.register.code') }}</label>
            <div class="relative">
              <ShieldCheck class="absolute left-3 top-1/2 -translate-y-1/2 w-4 h-4 text-muted-foreground" />
              <Input 
                v-model="code"
                type="text" 
                :placeholder="t('auth.register.codePlaceholder')" 
                class="pl-10 h-12 bg-surface border-border/50 focus:border-primary pr-28"
                autocomplete="one-time-code"
                :disabled="isLoading"
                required
              />
              <button 
                type="button" 
                @click="handleSendCode"
                :disabled="isSendingCode || isLoading || !email"
                class="absolute right-2 top-1/2 -translate-y-1/2 text-xs font-medium text-primary hover:text-primary/80 px-2 py-1.5 rounded-md hover:bg-primary/10 transition-colors disabled:opacity-50 disabled:cursor-not-allowed"
              >
                {{ isSendingCode ? t('auth.register.sendingCode') : t('auth.register.sendCode') }}
              </button>
            </div>
          </div>

          <p
            v-if="statusKey"
            class="rounded-xl border border-signal/20 bg-signal/10 px-4 py-3 text-sm font-medium text-signal"
          >
            {{ t(statusKey, statusParams) }}
          </p>

          <p
            v-if="errorKey"
            class="rounded-xl border border-warning/20 bg-warning/10 px-4 py-3 text-sm font-medium text-warning"
          >
            {{ t(errorKey) }}
          </p>

          <Button 
            type="submit" 
            class="w-full h-12 text-base font-bold mt-2 shadow-glow"
            :disabled="isLoading"
          >
            {{ isLoading ? t('auth.register.submitting') : t('auth.register.submit') }}
          </Button>

          <div class="text-center mt-6 text-sm">
            <span class="text-muted-foreground">{{ t('auth.register.hasAccount') }}</span>
            <router-link to="/login" class="text-primary hover:text-primary/80 font-medium ml-1 transition-colors">
              {{ t('auth.register.loginLink') }}
            </router-link>
          </div>
        </form>
      </div>
    </div>
  </div>
</template>
