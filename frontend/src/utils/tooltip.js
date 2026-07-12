// macOS 风格全局 tooltip
//
// 用法：
//   1. 元素加 data-tip="文案" —— 显示纯文本气泡
//   2. 元素加 data-tip="文案|⌘F" —— "|" 后面的部分渲染为键帽样式的快捷键
//   3. 存量元素的 title 属性会被自动接管（首次 hover 时转为 data-tip，避免原生 tooltip 重复弹出）
//
// 通过 document 级事件委托实现，无需每个组件单独引入。
// 在 main.js 中调用 initTooltip() 一次即可。

let tipEl = null
let showTimer = null
let currentTarget = null

const SHOW_DELAY = 350

function ensureTipEl() {
  if (tipEl) return tipEl
  tipEl = document.createElement('div')
  tipEl.className = 'mac-tooltip'
  tipEl.setAttribute('role', 'tooltip')
  document.body.appendChild(tipEl)
  return tipEl
}

function renderContent(el, raw) {
  el.innerHTML = ''
  const [label, keys] = raw.split('|')
  const labelSpan = document.createElement('span')
  labelSpan.className = 'mac-tooltip-label'
  labelSpan.textContent = label.trim()
  el.appendChild(labelSpan)
  if (keys && keys.trim()) {
    const kbd = document.createElement('span')
    kbd.className = 'mac-tooltip-keys'
    kbd.textContent = keys.trim()
    el.appendChild(kbd)
  }
}

function positionTip(target) {
  const el = ensureTipEl()
  const rect = target.getBoundingClientRect()
  const tw = el.offsetWidth
  const th = el.offsetHeight
  const margin = 6

  // 默认显示在元素下方居中，越界时翻转/收缩
  let left = rect.left + rect.width / 2 - tw / 2
  let top = rect.bottom + margin

  if (left < 4) left = 4
  if (left + tw > window.innerWidth - 4) left = window.innerWidth - tw - 4
  if (top + th > window.innerHeight - 4) top = rect.top - th - margin // 翻转到上方

  el.style.left = `${Math.round(left)}px`
  el.style.top = `${Math.round(top)}px`
}

function show(target, text) {
  const el = ensureTipEl()
  renderContent(el, text)
  el.classList.add('visible')
  positionTip(target)
}

function hide() {
  clearTimeout(showTimer)
  showTimer = null
  currentTarget = null
  if (tipEl) tipEl.classList.remove('visible')
}

function findTipTarget(node) {
  if (!(node instanceof Element)) return null
  return node.closest('[data-tip], [title]')
}

function onMouseOver(e) {
  const target = findTipTarget(e.target)
  if (!target || target === currentTarget) return

  // 接管原生 title，避免双重 tooltip
  if (target.hasAttribute('title')) {
    const t = target.getAttribute('title')
    if (t && !target.hasAttribute('data-tip')) target.setAttribute('data-tip', t)
    target.removeAttribute('title')
  }

  const text = target.getAttribute('data-tip')
  if (!text) return

  hide()
  currentTarget = target
  showTimer = setTimeout(() => {
    // 延迟结束时可能已经移开
    if (currentTarget === target && document.contains(target)) {
      // data-tip 可能是动态绑定，显示前重新读取
      const latest = target.getAttribute('data-tip')
      if (latest) show(target, latest)
    }
  }, SHOW_DELAY)
}

function onMouseOut(e) {
  if (!currentTarget) return
  const to = e.relatedTarget
  if (to instanceof Element && currentTarget.contains(to)) return
  hide()
}

export function initTooltip() {
  document.addEventListener('mouseover', onMouseOver, true)
  document.addEventListener('mouseout', onMouseOut, true)
  // 点击/滚动/按键时立即隐藏
  document.addEventListener('mousedown', hide, true)
  document.addEventListener('wheel', hide, { capture: true, passive: true })
  window.addEventListener('blur', hide)
  document.addEventListener('keydown', hide, true)
}
