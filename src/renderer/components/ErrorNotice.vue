<script setup lang="ts">
defineProps<{ message: string }>()
</script>

<template><p v-if="message" class="error-notice" role="alert">{{ message }}</p></template>

<style scoped>
.error-notice { padding: 12px 14px; color: #80281e; border-left: 3px solid var(--accent); background: #f4e5e1; font-size: 13px; line-height: 1.6; }
</style>
