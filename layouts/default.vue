<template>
  <div class="native-layout">
    <!-- Native desktop layout - content only -->
    <slot />
  </div>
</template>

<script setup>
// No header/footer needed for native desktop layout
</script>

<style scoped>
.native-layout {
  height: 100%;
  background: var(--bg-primary);
  color: var(--text-primary);
}
</style>
