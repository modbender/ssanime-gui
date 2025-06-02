<template>
  <Card>
    <template #content>
      <div class="p-4 flex items-center gap-4">
        <img
          src="/logo.svg"
          alt="Logo"
          style="
            height: 40px;
            width: 40px;
            border-radius: 50%;
            box-shadow: 0 2px 8px rgba(0, 0, 0, 0.1);
          "
        />
        <div>
          <h1 class="text-3xl font-bold">SSAnime GUI</h1>
          <p class="text-secondary">Anime Encoding Tool</p>
        </div>
        <div class="ml-auto flex gap-2">
          <Button icon="pi pi-cog" text rounded aria-label="Customize" />
        </div>
      </div>
    </template>
  </Card>
</template>

<script setup>
// No manual imports needed, using Nuxt auto-imports
</script>

<style scoped>
.text-secondary {
  color: #6b7280;
}
</style>
