<template>
  <div ref="wrap" class="relative inline-block text-left">
    <button
      ref="trigger"
      type="button"
      :disabled="disabled"
      class="input-field !w-auto !py-2 flex items-center justify-between gap-2 pr-3 text-left cursor-pointer disabled:cursor-not-allowed disabled:opacity-50"
      :aria-haspopup="'listbox'"
      :aria-expanded="open"
      @click="toggle"
      @keydown.escape.prevent="close"
      @keydown.down.prevent="open ? move(1) : openMenu()"
      @keydown.up.prevent="open ? move(-1) : openMenu()"
      @keydown.enter.prevent="open ? pickHighlighted() : openMenu()"
    >
      <span class="truncate" :class="selected ? 'text-foreground' : 'text-muted-foreground'">
        {{ selectedLabel || placeholder || '请选择' }}
      </span>
      <svg
        class="w-4 h-4 shrink-0 text-muted-foreground transition-transform duration-200"
        :class="open ? 'rotate-180' : ''"
        fill="none" stroke="currentColor" viewBox="0 0 24 24"
      >
        <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M19 9l-7 7-7-7" />
      </svg>
    </button>

    <Teleport to="body">
      <transition
        enter-active-class="transition duration-150 ease-out"
        enter-from-class="opacity-0 scale-95 -translate-y-1"
        leave-active-class="transition duration-100 ease-in"
        leave-to-class="opacity-0 scale-95"
      >
        <div
          v-if="open"
          ref="pop"
          role="listbox"
          class="fixed z-[90] rounded-xl border border-border bg-surface/95 dark:bg-[#191a28]/95 backdrop-blur-xl shadow-2xl p-1.5 max-h-72 overflow-y-auto overscroll-contain"
          :style="popStyle"
          @pointerdown.stop
        >
          <button
            v-for="(opt, i) in options"
            :key="opt.value"
            type="button"
            role="option"
            :aria-selected="opt.value === modelValue"
            class="w-full flex items-center justify-between gap-3 px-3 py-2 rounded-lg text-sm text-left transition-colors"
            :class="i === hi
              ? 'bg-brand-500/15 text-foreground'
              : 'text-muted-foreground hover:bg-brand-500/10 hover:text-foreground'"
            @click="pick(opt)"
            @pointerenter="hi = i"
          >
            <span class="truncate">{{ opt.label }}</span>
            <svg
              v-if="opt.value === modelValue"
              class="w-4 h-4 shrink-0 text-brand-400"
              fill="none" stroke="currentColor" viewBox="0 0 24 24"
            >
              <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2.5" d="M5 13l4 4L19 7" />
            </svg>
          </button>
        </div>
      </transition>
    </Teleport>
  </div>
</template>

<script setup lang="ts">
import { ref, computed, nextTick, onBeforeUnmount } from 'vue'

export interface SelectOption {
  value: string | number
  label: string
}

const props = withDefaults(defineProps<{
  // 允许 undefined：表单里「未选择」的可选 id 字段（如计件项目、工人）直接绑定
  modelValue: string | number | undefined
  options: SelectOption[]
  placeholder?: string
  disabled?: boolean
}>(), { placeholder: '', disabled: false })

const emit = defineEmits<{
  (e: 'update:modelValue', value: string | number | undefined): void
  (e: 'change', value: string | number | undefined): void
}>()

const wrap = ref<HTMLElement | null>(null)
const trigger = ref<HTMLElement | null>(null)
const pop = ref<HTMLElement | null>(null)
const open = ref(false)
const hi = ref(0)
const popStyle = ref<Record<string, string>>({})

const selected = computed(() => props.options.find(o => o.value === props.modelValue))
const selectedLabel = computed(() => selected.value?.label || '')

function position() {
  const el = trigger.value
  if (!el) return
  const r = el.getBoundingClientRect()
  const maxH = 288 // max-h-72 + padding
  const spaceBelow = window.innerHeight - r.bottom
  const openUp = spaceBelow < maxH && r.top > spaceBelow
  popStyle.value = {
    left: `${Math.max(8, Math.min(r.left, window.innerWidth - 8))}px`,
    top: openUp ? `${Math.max(8, r.top - maxH - 6)}px` : `${r.bottom + 6}px`,
    minWidth: `${r.width}px`,
    maxWidth: `${Math.max(r.width, 320)}px`,
  }
}

function openMenu() {
  if (props.disabled || props.options.length === 0) return
  hi.value = Math.max(0, props.options.findIndex(o => o.value === props.modelValue))
  open.value = true
  nextTick(() => {
    position()
    // 键盘导航时保证高亮项可见
    const active = pop.value?.children[hi.value] as HTMLElement | undefined
    active?.scrollIntoView({ block: 'nearest' })
  })
}

function close() {
  if (!open.value) return
  open.value = false
}

function toggle() {
  open.value ? close() : openMenu()
}

function move(delta: number) {
  if (props.options.length === 0) return
  hi.value = (hi.value + delta + props.options.length) % props.options.length
  const active = pop.value?.children[hi.value] as HTMLElement | undefined
  active?.scrollIntoView({ block: 'nearest' })
}

function pickHighlighted() {
  const opt = props.options[hi.value]
  if (opt) pick(opt)
}

function pick(opt: SelectOption) {
  close()
  if (opt.value === props.modelValue) return
  emit('update:modelValue', opt.value)
  emit('change', opt.value)
}

function onPointerDown(e: PointerEvent) {
  const t = e.target as Node
  if (wrap.value?.contains(t)) return
  close()
}

function onReflow() {
  if (open.value) position()
}

function onKeydown(e: KeyboardEvent) {
  if (e.key === 'Escape' && open.value) {
    e.stopPropagation()
    close()
  }
}

window.addEventListener('pointerdown', onPointerDown, true)
window.addEventListener('scroll', onReflow, true)
window.addEventListener('resize', onReflow)
window.addEventListener('keydown', onKeydown)
onBeforeUnmount(() => {
  window.removeEventListener('pointerdown', onPointerDown, true)
  window.removeEventListener('scroll', onReflow, true)
  window.removeEventListener('resize', onReflow)
  window.removeEventListener('keydown', onKeydown)
})
</script>
