<template>
  <Teleport to="body">
    <Transition name="modal">
      <div v-if="modelValue" class="fixed inset-0 z-50 flex items-end sm:items-center sm:justify-center" @click.self="handleClose">
        <div class="absolute inset-0 bg-black bg-opacity-50" @click="handleClose"></div>
        <!-- Mobile: bottom sheet docked above the keyboard; desktop: centered dialog -->
        <div
          class="relative bg-surface text-foreground shadow-xl w-full sm:max-w-lg sm:mx-4 rounded-t-2xl sm:rounded-2xl overflow-y-auto px-5 pb-[calc(20px_+_var(--app-bottom-inset))] sm:px-6 sm:pb-6 max-h-[92dvh]"
          :style="sheetStyle"
        >
          <!-- 标题栏 sticky：长表单滚动时右上角关闭按钮始终可见可点。
               背景用不透明的主题表面色遮住下方滚动内容，z-10 保证盖住表单控件。 -->
          <div class="sticky top-0 z-10 -mx-5 sm:-mx-6 border-b border-border/60 bg-surface px-5 pt-3 pb-2.5 sm:px-6 sm:pt-6 sm:pb-3.5">
            <div class="sm:hidden mx-auto mb-2.5 h-1 w-10 rounded-full bg-muted-foreground/30 shrink-0" aria-hidden="true"></div>
            <div class="flex items-center justify-between gap-3">
              <h3 class="min-w-0 flex-1 truncate text-base sm:text-lg font-bold text-foreground">{{ title }}</h3>
              <button
                type="button"
                aria-label="关闭"
                :disabled="closable === false"
                class="-mr-2 inline-flex shrink-0 items-center justify-center min-w-11 min-h-11 rounded-xl text-muted-foreground transition-colors hover:bg-muted hover:text-foreground disabled:opacity-40 disabled:pointer-events-none"
                @click="handleClose"
              >
                <svg class="w-5 h-5" fill="none" stroke="currentColor" viewBox="0 0 24 24"><path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M6 18L18 6M6 6l12 12" /></svg>
              </button>
            </div>
          </div>
          <div class="pt-2.5 sm:pt-3.5">
            <slot />
          </div>
        </div>
      </div>
    </Transition>
  </Teleport>
</template>

<script setup lang="ts">
import { computed, onBeforeUnmount, onMounted, ref, watch } from 'vue'
import { isTopOverlay, popOverlay, pushOverlay } from '../utils/overlay-stack'

// closable 必须给默认 true：仅声明为可选 Boolean 时，Vue 运行时会把
// 未传的值转换成 false 而不是 undefined，导致默认的 X/遮罩关闭全部失效
const props = withDefaults(defineProps<{
  modelValue: boolean
  title?: string
  closable?: boolean
}>(), {
  closable: true,
})

const emit = defineEmits<{
  'update:modelValue': [value: boolean]
}>()

function handleClose() {
  if (props.closable !== false) {
    emit('update:modelValue', false)
  }
}

// ESC 关闭：与 ConfirmDialog.vue 保持一致的键盘出口。
// 浮层可以叠加，ESC 只关最上面那一层，否则一次按键会连带关掉下面的弹窗。
const overlayToken = ref<number | null>(null)

function onKeydown(e: KeyboardEvent) {
  if (e.key === 'Escape' && props.modelValue && overlayToken.value !== null && isTopOverlay(overlayToken.value)) {
    handleClose()
  }
}

// Lift the sheet above the on-screen keyboard: track how much of the layout
// viewport the keyboard covers and shrink/lift accordingly, then keep the
// focused input visible inside the sheet.
const kbInset = ref(0)

function measureKeyboard() {
  const vv = window.visualViewport
  if (!vv) {
    kbInset.value = 0
    return
  }
  kbInset.value = Math.max(0, Math.round(window.innerHeight - vv.height - vv.offsetTop))
}

function onFocusIn(e: FocusEvent) {
  const el = e.target as HTMLElement | null
  if (!el) return
  const tag = el.tagName
  if (tag === 'INPUT' || tag === 'SELECT' || tag === 'TEXTAREA' || el.isContentEditable) {
    // Wait a tick so the keyboard resize event lands first.
    setTimeout(() => el.scrollIntoView({ block: 'center', behavior: 'smooth' }), 60)
  }
}

const sheetStyle = computed(() =>
  kbInset.value > 0
    ? { marginBottom: `${kbInset.value}px`, maxHeight: `calc(92dvh - ${kbInset.value}px)` }
    : {},
)

watch(
  () => props.modelValue,
  (open) => {
    document.body.style.overflow = open ? 'hidden' : ''
    if (open) {
      measureKeyboard()
      // 打开时才入栈：组件常驻在模板里，创建时就入栈会把没打开的浮层也混进栈
      if (overlayToken.value === null) {
        overlayToken.value = pushOverlay()
      }
    } else {
      kbInset.value = 0
      if (overlayToken.value !== null) {
        popOverlay(overlayToken.value)
        overlayToken.value = null
      }
    }
  },
  { immediate: true },
)

onMounted(() => {
  window.visualViewport?.addEventListener('resize', measureKeyboard)
  window.visualViewport?.addEventListener('scroll', measureKeyboard)
  document.addEventListener('focusin', onFocusIn)
  document.addEventListener('keydown', onKeydown)
})

onBeforeUnmount(() => {
  window.visualViewport?.removeEventListener('resize', measureKeyboard)
  window.visualViewport?.removeEventListener('scroll', measureKeyboard)
  document.removeEventListener('focusin', onFocusIn)
  document.removeEventListener('keydown', onKeydown)
  document.body.style.overflow = ''
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
.modal-enter-from .relative,
.modal-leave-to .relative {
  transform: translateY(24px) scale(0.98);
}
@media (min-width: 640px) {
  .modal-enter-from .relative,
  .modal-leave-to .relative {
    transform: scale(0.95);
  }
}
</style>
