<template>
  <Teleport to="body">
    <Transition name="modal">
      <div v-if="support.show" class="fixed inset-0 z-50 flex items-end sm:items-center sm:justify-center" @click.self="support.dismiss()">
        <div class="absolute inset-0 bg-black/55" @click="support.dismiss()"></div>
        <!-- 主弹窗：赞赏说明 + 收款码（步骤一）→ 已支持后填金额（步骤二）。
             外层可滚动：内容高于小屏（尤其手机）时弹窗内部滚动；
             点击遮罩或右上角关闭。 -->
        <div
          role="dialog"
          aria-modal="true"
          aria-labelledby="support-modal-title"
          class="relative bg-surface text-foreground shadow-xl w-full sm:max-w-sm sm:mx-4 rounded-t-2xl sm:rounded-2xl overflow-y-auto max-h-[92dvh] px-5 sm:px-6 pt-4 pb-[calc(20px+env(safe-area-inset-bottom))] sm:pb-6 text-center"
        >
          <button
            class="absolute right-3 top-3 z-10 p-1.5 rounded-lg text-muted-foreground hover:text-foreground hover:bg-muted transition-colors"
            aria-label="关闭"
            @click="support.dismiss()"
          >
            <svg class="h-4 w-4" fill="none" stroke="currentColor" viewBox="0 0 24 24"><path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M6 18L18 6M6 6l12 12" /></svg>
          </button>

          <!-- 步骤一：收款码扫码 -->
          <template v-if="!showAmountStep">
            <div class="mx-auto mb-3 mt-1 flex h-14 w-14 items-center justify-center rounded-2xl bg-brand-gradient shadow-glow">
              <span class="text-2xl">☕</span>
            </div>
            <h3 id="support-modal-title" class="mb-2 text-lg font-bold text-foreground">请作者喝杯咖啡</h3>

            <p class="mb-1 text-sm leading-relaxed text-muted-foreground">
              这个应用免费、无广告，数据完全保存在你自己的设备上。
            </p>
            <p class="mb-3 text-sm leading-relaxed text-muted-foreground sm:mb-4">
              如果它帮到了你，欢迎请作者喝杯咖啡——<strong class="text-foreground">金额随意，1 元也是心意</strong>。
              <br />
              <span class="text-xs">不赞赏也完全没有问题，<strong>不支付不影响任何功能</strong>。</span>
            </p>

            <div class="my-3 flex justify-center sm:my-4">
              <div>
                <!-- 明确宽高属性：加载前就占好位，避免图片解码后弹窗高度跳动；
                     max-h 限制高度，矮屏下弹窗不被收款码撑出屏幕 -->
                <img
                  :src="donateQr"
                  alt="微信收款码"
                  width="352"
                  height="480"
                  decoding="async"
                  class="mx-auto h-auto max-h-[32vh] w-[190px] max-w-full rounded-xl border border-border bg-white object-contain p-1"
                  @error="hideQrOnError"
                />
                <span class="mt-2 block text-xs text-muted-foreground">微信扫码赞赏</span>
              </div>
            </div>

            <div class="mt-4 flex justify-center gap-3 border-t border-border pt-4">
              <button class="btn-ghost flex-1" @click="support.dismiss()">暂不支持</button>
              <button class="btn-brand flex-1" @click="showAmountStep = true">❤ 已支持</button>
            </div>
          </template>

          <!-- 步骤二：填金额并确定（不校验支付，金额仅作自愿标注） -->
          <template v-else>
            <div class="mx-auto mb-3 mt-1 flex h-14 w-14 items-center justify-center rounded-2xl bg-rose-400/15 text-rose-500">
              <span class="text-2xl">❤</span>
            </div>
            <h3 id="support-modal-title" class="mb-1 text-lg font-bold text-foreground">感谢你的支持！</h3>
            <p class="mb-4 text-sm leading-relaxed text-muted-foreground">
              你刚刚赞赏了多少钱？（<strong class="text-foreground">选填</strong>，仅用于作者统计赞赏流水，
              <span class="whitespace-nowrap">不涉及</span>你的微信账号与支付记录）
            </p>

            <div class="mb-4">
              <div class="relative mx-auto max-w-[220px]">
                <input
                  ref="amountInput"
                  v-model="amountText"
                  type="number"
                  inputmode="decimal"
                  min="0"
                  step="0.01"
                  placeholder="0.00"
                  class="input-field pr-9 text-center text-2xl font-bold"
                  @keydown.enter="submitSupport"
                />
                <span class="pointer-events-none absolute right-3 top-1/2 -translate-y-1/2 text-lg font-bold text-muted-foreground">元</span>
              </div>
              <!-- 快捷金额 -->
              <div class="mt-3 flex justify-center gap-2">
                <button
                  v-for="v in [5, 10, 20, 50]"
                  :key="v"
                  type="button"
                  class="rounded-full border px-3 py-1 text-xs font-semibold transition-colors"
                  :class="amountText === String(v) ? 'border-brand-500 bg-brand-500/10 text-brand-600 dark:text-brand-300' : 'border-border text-muted-foreground hover:text-foreground'"
                  @click="amountText = String(v)"
                >
                  {{ v }} 元
                </button>
              </div>
            </div>

            <p v-if="support.errorText" class="mb-2 text-xs text-rose-500">{{ support.errorText }}</p>

            <div class="flex justify-center gap-3">
              <button class="btn-ghost flex-1" :disabled="support.sending" @click="backToQr">上一步</button>
              <button class="btn-brand flex-1" :disabled="support.sending" @click="submitSupport">
                {{ support.sending ? '发送中…' : '确定' }}
              </button>
            </div>
          </template>
        </div>
      </div>
    </Transition>
  </Teleport>
</template>

<script setup lang="ts">
import { nextTick, ref, watch } from 'vue'
import { useSupportStore } from '../stores/support'
import donateQr from '../assets/donate-wechat.png'

const support = useSupportStore()

// 弹窗内两步流程：收款码 → 点【已支持】→ 填金额 → 点【确定】上报
const showAmountStep = ref(false)
const amountText = ref('')
const amountInput = ref<HTMLInputElement | null>(null)

watch(showAmountStep, async (show) => {
  if (show) {
    amountText.value = ''
    await nextTick()
    amountInput.value?.focus()
  }
})

// 从金额步骤返回收款码：清掉上次填写的金额
function backToQr() {
  support.errorText = ''
  showAmountStep.value = false
}

async function submitSupport() {
  const amount = parseFloat(amountText.value)
  const ok = await support.confirmSupported(Number.isFinite(amount) && amount > 0 ? amount : undefined)
  if (ok) {
    showAmountStep.value = false
  } else {
    // 失败留在金额步骤，错误文案已由 store 写入 errorText
  }
}

function hideQrOnError(e: Event) {
  ;(e.currentTarget as HTMLImageElement).style.display = 'none'
}
</script>

<style scoped>
.modal-enter-active,
.modal-leave-active {
  transition: opacity 0.25s ease;
}
.modal-enter-from,
.modal-leave-to {
  opacity: 0;
}
.modal-enter-from > div.relative,
.modal-leave-to > div.relative {
  transform: translateY(24px) scale(0.98);
}
@media (min-width: 640px) {
  .modal-enter-from > div.relative,
  .modal-leave-to > div.relative {
    transform: scale(0.95);
  }
}
</style>
