<script setup lang="ts">
import { onBeforeUnmount } from 'vue'

// Animate a padding-free wrapper so the last frame matches its natural height.
const running = new Map<HTMLElement, Animation>()
const interrupted = new WeakMap<HTMLElement, number>()

function reset(el: HTMLElement) {
  el.style.removeProperty('height')
  el.style.removeProperty('overflow')
}
function cancel(element: Element) {
  const el = element as HTMLElement
  interrupted.set(el, el.getBoundingClientRect().height)
  running.get(el)?.cancel()
  running.delete(el)
  reset(el)
}
function enter(element: Element, done: () => void) { animate(element, true, done) }
function leave(element: Element, done: () => void) { animate(element, false, done) }
function animate(element: Element, expanding: boolean, done: () => void) {
  const el = element as HTMLElement
  const from = interrupted.get(el) ?? (expanding ? 0 : el.getBoundingClientRect().height)
  interrupted.delete(el)
  running.get(el)?.cancel()
  if (window.matchMedia('(prefers-reduced-motion: reduce)').matches) {
    reset(el)
    done()
    return
  }
  el.style.height = 'auto'
  const to = expanding ? el.getBoundingClientRect().height : 0
  el.style.overflow = 'hidden'
  el.style.height = `${from}px`
  const animation = el.animate([{ height: `${from}px` }, { height: `${to}px` }], {
    duration: 280, easing: 'cubic-bezier(.25, .8, .25, 1)', fill: 'both',
  })
  running.set(el, animation)
  animation.onfinish = () => {
    if (running.get(el) !== animation) return
    running.delete(el)
    reset(el)
    animation.cancel()
    done()
  }
}
onBeforeUnmount(() => {
  for (const [el, animation] of running) { animation.cancel(); reset(el) }
  running.clear()
})
</script>

<template>
  <Transition :css="false" @enter="enter" @leave="leave" @enter-cancelled="cancel" @leave-cancelled="cancel">
    <slot />
  </Transition>
</template>
