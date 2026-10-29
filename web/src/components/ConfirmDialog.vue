<template>
  <Teleport to="body">
    <Transition name="modal">
      <div v-if="modelValue" class="fixed inset-0 z-50 flex items-end sm:items-center sm:justify-center" @click.self="handleCancel">
        <!-- 遮罩层铺满整个视口，根节点的 @click.self 永远不会触发，
             所以关闭必须由遮罩自己处理，否则「点暗处关闭」形同虚设。 -->
        <div class="absolute inset-0 bg-black bg-opacity-50" @click="handleCancel"></div>
        <div class="relative bg-surface text-foreground shadow-xl w-full sm:max-w-sm sm:mx-4 rounded-t-2xl sm:rounded-2xl px-5 pt-4 pb-[calc(20px_+_var(--app-bottom-inset))] sm:p-6">
          <div class="sm:hidden mx-auto mb-3 h-1 w-10 rounded-full bg-muted-foreground/30" aria-hidden="true"></div>
          <!-- 标题栏：与 Modal.vue 保持一致，右上角固定 X 关闭按钮（触控区 44px） -->
          <div class="flex items-start justify-between gap-3 mb-1.5 sm:mb-2">
            <h3 class="min-w-0 flex-1 text-base sm:text-lg font-bold text-foreground">{{ title }}</h3>
            <button
              type="button"
              aria-label="关闭"
              class="-mr-2 -mt-1 inline-flex shrink-0 items-center justify-center min-w-11 min-h-11 rounded-xl text-muted-foreground transition-colors hover:bg-muted hover:text-foreground"
              @click="handleCancel"
            >
              <svg class="w-5 h-5" fill="none" stroke="currentColor" viewBox="0 0 24 24"><path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M6 18L18 6M6 6l12 12" /></svg>
            </button>
          </div>
          <p class="text-sm text-muted-foreground mb-5 sm:mb-6 whitespace-pre-line">{{ message }}</p>
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
import { onBeforeUnmount, onMounted, ref, watch } from 'vue'
import { isTopOverlay, popOverlay, pushOverlay } from '../utils/overlay-stack'

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

// ESC 关闭：与 Modal.vue 的交互口径保持一致。
// 浮层可以叠加，ESC 只关最上面那一层，否则一次按键会连带关掉下面的弹窗。
const overlayToken = ref<number | null>(null)

function onKeydown(e: KeyboardEvent) {
  if (e.key === 'Escape' && props.modelValue && overlayToken.value !== null && isTopOverlay(overlayToken.value)) {
    handleCancel()
  }
}

watch(
  () => props.modelValue,
  (open) => {
    // 打开时才入栈：组件常驻在模板里，创建时就入栈会把没打开的浮层也混进栈
    if (open) {
      if (overlayToken.value === null) {
        overlayToken.value = pushOverlay()
      }
    } else if (overlayToken.value !== null) {
      popOverlay(overlayToken.value)
      overlayToken.value = null
    }
  },
  { immediate: true },
)

onMounted(() => {
  document.addEventListener('keydown', onKeydown)
})

onBeforeUnmount(() => {
  document.removeEventListener('keydown', onKeydown)
  if (overlayToken.value !== null) {
    popOverlay(overlayToken.value)
    overlayToken.value = null
  }
})
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
