<template>
  <Teleport to="body">
    <Transition name="modal">
      <div v-if="modelValue" class="fixed inset-0 z-50 flex items-end sm:items-center sm:justify-center" @click.self="handleCancel">
        <div class="absolute inset-0 bg-black bg-opacity-50"></div>
        <div class="relative bg-surface text-foreground shadow-xl w-full sm:max-w-sm sm:mx-4 rounded-t-2xl sm:rounded-2xl px-5 pt-4 pb-[calc(20px+env(safe-area-inset-bottom))] sm:p-6">
          <div class="sm:hidden mx-auto mb-3 h-1 w-10 rounded-full bg-muted-foreground/30" aria-hidden="true"></div>
          <h3 class="text-base sm:text-lg font-bold text-foreground mb-1.5 sm:mb-2">{{ title }}</h3>
          <p class="text-sm text-muted-foreground mb-5 sm:mb-6">{{ message }}</p>
          <div class="flex space-x-3">
            <button
              @click="handleCancel"
              class="flex-1 px-4 py-2.5 rounded-lg border border-border text-foreground hover:bg-muted transition-colors text-sm font-medium"
            >
              取消
            </button>
            <button
              @click="handleConfirm"
              :class="[
                'flex-1 px-4 py-2.5 rounded-lg text-white text-sm font-medium transition-colors',
                confirmType === 'danger' ? 'bg-destructive hover:brightness-110' : 'bg-primary hover:brightness-110'
              ]"
            >
              {{ confirmText }}
            </button>
          </div>
        </div>
      </div>
    </Transition>
  </Teleport>
</template>

<script setup lang="ts">
const props = withDefaults(defineProps<{
  modelValue: boolean
  title?: string
  message?: string
  confirmText?: string
  confirmType?: 'primary' | 'danger'
}>(), {
  title: '确认',
  message: '确定要执行此操作吗？',
  confirmText: '确认',
  confirmType: 'primary',
})

const emit = defineEmits<{
  'update:modelValue': [value: boolean]
  'confirm': []
  'cancel': []
}>()

function handleConfirm() {
  emit('update:modelValue', false)
  emit('confirm')
}

function handleCancel() {
  emit('update:modelValue', false)
  emit('cancel')
}
</script>

<style scoped>
.modal-enter-active,
.modal-leave-active {
  transition: all 0.3s ease;
}
.modal-enter-from,
.modal-leave-to {
  opacity: 0;
}
</style>
