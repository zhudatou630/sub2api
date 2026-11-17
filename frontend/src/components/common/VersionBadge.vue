<template>
  <!-- Local build metadata only: no upstream update, rollback or restart UI. -->
  <span
    class="inline-flex items-center rounded-lg bg-gray-100 px-2 py-1 text-xs font-medium text-gray-600 dark:bg-dark-800 dark:text-dark-400"
    :title="t('version.currentVersion')"
  >
    {{ currentVersion ? `v${currentVersion}` : '--' }}
  </span>
</template>

<script setup lang="ts">
import { computed, onMounted } from 'vue'
import { useI18n } from 'vue-i18n'
import { useAuthStore, useAppStore } from '@/stores'

const props = defineProps<{ version?: string }>()
const { t } = useI18n()
const authStore = useAuthStore()
const appStore = useAppStore()
const currentVersion = computed(() => appStore.currentVersion || props.version || appStore.siteVersion || '')

onMounted(() => {
  if (authStore.isAdmin) {
    // fetchVersion only reads the running build; it never checks releases.
    void appStore.fetchVersion()
  }
})
</script>
