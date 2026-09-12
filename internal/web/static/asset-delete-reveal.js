(() => {
  const footer = document.querySelector('[data-delete-reveal]');
  if (!footer) return;
  const root = document.scrollingElement;
  const atBottom = () => root.scrollTop + root.clientHeight >= root.scrollHeight - 2;
  const eligible = target => {
    if (document.querySelector('dialog[open]')) return false;
    if (target?.closest?.('input, textarea, select, [contenteditable="true"], .account-menu[open]')) return false;
    for (let node = target; node && node !== root && node !== document.body; node = node.parentElement) {
      if (node.scrollHeight > node.clientHeight + 2 && /auto|scroll/.test(getComputedStyle(node).overflowY)) return false;
    }
    return true;
  };
  let revealed = false;
  let wheelPull = 0;
  function hide() {
    if (!revealed) return;
    revealed = false;
    footer.hidden = true;
    wheelPull = 0;
    wheelStartedAtBottom = false;
    bottomSince = atBottom() ? performance.now() : Infinity;
  }
  function reveal(keyboard = false) {
    if (revealed) return;
    revealed = true;
    footer.hidden = false;
    footer.scrollIntoView({block:'end', behavior:matchMedia('(prefers-reduced-motion: reduce)').matches ? 'auto' : 'smooth'});
    if (keyboard) footer.querySelector('a').focus({preventScroll:true});
  }
  // Wheel momentum remains one gesture until events have been quiet for 260ms.
  // Reaching the bottom cannot turn that same gesture into a reveal command.
  let bottomSince = atBottom() ? performance.now() : Infinity;
  window.addEventListener('scroll', () => {
    // Scroll position also changes during smooth reveal, layout and browser
    // anchoring. Only explicit upward input below may release the open latch.
    if (!atBottom()) bottomSince = Infinity;
    else if (!Number.isFinite(bottomSince)) bottomSince = performance.now();
  }, {passive:true});
  let lastWheel = -Infinity, wheelStartedAtBottom = false;
  window.addEventListener('wheel', event => {
    const now = performance.now();
    if (now - lastWheel > 260) {
      wheelStartedAtBottom = atBottom() && now - bottomSince >= 260 && eligible(event.target);
      wheelPull = 0;
    }
    lastWheel = now;
    if (event.ctrlKey || Math.abs(event.deltaX) > Math.abs(event.deltaY) || !eligible(event.target)) return;
    if (event.deltaY < 0) {
      hide();
      wheelStartedAtBottom = false;
      wheelPull = 0;
    }
    if (event.deltaY > 0 && wheelStartedAtBottom && atBottom()) {
      // Normalize line/page wheels; a light nudge must not cross the anchor.
      wheelPull += event.deltaY * (event.deltaMode === 1 ? 16 : event.deltaMode === 2 ? root.clientHeight : 1);
      if (wheelPull >= 240) reveal();
    }
  }, {passive:true});
  let touchY = null, lastTouchY = null;
  window.addEventListener('touchstart', event => {
    touchY = event.touches.length === 1 && (revealed || atBottom()) && eligible(event.target) ? event.touches[0].clientY : null;
    lastTouchY = touchY;
  }, {passive:true});
  window.addEventListener('touchmove', event => {
    if (touchY === null) return;
    if (event.touches.length !== 1 || !eligible(event.target)) { touchY = null; return; }
    if (revealed && event.touches[0].clientY - lastTouchY > 8) { hide(); touchY = null; return; }
    lastTouchY = event.touches[0].clientY;
    const distance = touchY - event.touches[0].clientY;
    if (distance < -8) { hide(); touchY = null; }
    else if (distance >= 96 && atBottom()) reveal();
  }, {passive:true});
  for (const type of ['touchend', 'touchcancel']) window.addEventListener(type, () => { touchY = null; }, {passive:true});
  window.addEventListener('keydown', event => {
    if (eligible(event.target) && (['ArrowUp', 'PageUp', 'Home', 'Escape'].includes(event.key) || (event.key === ' ' && event.shiftKey))) hide();
    if (revealed || event.repeat || event.altKey || event.ctrlKey || event.metaKey || event.shiftKey || !atBottom() || !eligible(event.target)) return;
    // A second navigation key at the bottom reveals the action. Tab preserves
    // normal focus order, including for screen-reader and keyboard users.
    if (event.key === 'Tab') reveal();
    else if (['ArrowDown', 'PageDown', 'End', ' '].includes(event.key) && !event.target?.closest?.('button, a, summary')) {
      event.preventDefault();
      reveal(true);
    }
  });
})();
