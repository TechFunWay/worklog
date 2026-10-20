<template>
  <div class="flex flex-wrap items-start justify-between gap-3">
    <div class="min-w-0 flex items-start gap-2">
      <!-- 返回入口：二级页面（如工资条详情）用，避免只能靠侧边栏或浏览器返回 -->
      <slot name="back" />
      <div class="min-w-0">
        <h1 class="text-lg sm:text-xl font-display font-bold text-foreground leading-tight">{{ title }}</h1>
        <p v-if="description" class="text-xs sm:text-sm text-muted-foreground mt-0.5 sm:mt-1">{{ description }}</p>
      </div>
    </div>
    <div v-if="$slots.actions" class="flex items-center gap-2 shrink-0">
      <slot name="actions" />
    </div>
  </div>
</template>

<script setup lang="ts">
withDefaults(
  defineProps<{
    title: string
    description?: string
  }>(),
  { description: '' },
)
</script>
