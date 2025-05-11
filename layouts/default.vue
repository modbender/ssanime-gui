<template>
  <div class="min-h-screen bg-background text-foreground flex flex-col">
    <Header />
    <main class="container mx-auto py-6 flex-grow">
      <slot />
    </main>
    <Footer />
  </div>
</template>

<script lang="ts" setup>
import Header from '~/components/Header.vue';
import Footer from '~/components/Footer.vue';
</script>

<style></style>
