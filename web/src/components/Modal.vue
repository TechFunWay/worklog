<template>
  <Teleport to="body">
    <Transition name="modal">
      <div v-if="modelValue" class="fixed inset-0 z-50 flex items-end sm:items-center sm:justify-center" @click.self="handleClose">
        <div class="absolute inset-0 bg-black bg-opacity-50" @click="handleClose"></div>
        <!-- Mobile: bottom sheet docked above the keyboard; desktop: centered dialog -->
        <div
          class="relative bg-surface text-foreground shadow-xl w-full sm:max-w-lg sm:mx-4 rounded-t-2xl sm:rounded-2xl overflow-y-auto px-5 pt-3 pb-[calc(20px+env(safe-area-inset-bottom))] sm:p-6 max-h-[92dvh]"
          :style="sheetStyle"
        >
          <div class="sm:hidden mx-auto mb-3 h-1 w-10 rounded-full bg-muted-foreground/30 shrink-0" aria-hidden="true"></div>
          <div class="flex items-center justify-between mb-3 sm:mb-4">
            <h3 class="text-base sm:text-lg font-bold text-foreground">{{ title }}</h3>
            <button @click="handleClose" class="p-1 -mr-1 text-muted-foreground hover:text-foreground transition-colors">
              <svg class="w-5 h-5" fill="none" stroke="currentColor" viewBox="0 0 24 24"><path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M6 18L18 6M6 6l12 12" /></svg>
            </button>
          </div>
          <div>
            <slot />
          </div>
        </div>
      </div>
    </Transition>
  </Teleport>
</template>

<script setup lang="ts">
import { computed, onBeforeUnmount, onMounted, ref, watch } from 'vue'

const props = defineProps<{
  modelValue: boolean
  title?: string
  closable?: boolean
}>()

const emit = defineEmits<{
  'update:modelValue': [value: boolean]
}>()

function handleClose() {
  if (props.closable !== false) {
    emit('update:modelValue', false)
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
    } else {
      kbInset.value = 0
    }
  },
)

onMounted(() => {
  window.visualViewport?.addEventListener('resize', measureKeyboard)
  window.visualViewport?.addEventListener('scroll', measureKeyboard)
  document.addEventListener('focusin', onFocusIn)
})

onBeforeUnmount(() => {
  window.visualViewport?.removeEventListener('resize', measureKeyboard)
  window.visualViewport?.removeEventListener('scroll', measureKeyboard)
  document.removeEventListener('focusin', onFocusIn)
  document.body.style.overflow = ''
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
