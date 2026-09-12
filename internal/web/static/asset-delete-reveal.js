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
    if (!atBottom()) bottomSince = Infinity;
    else if (!Number.isFinite(bottomSince)) bottomSince = performance.now();
  }, {passive:true});
  let lastWheel = -Infinity, wheelStartedAtBottom = false;
  window.addEventListener('wheel', event => {
    const now = performance.now();
    if (now - lastWheel > 260) wheelStartedAtBottom = atBottom() && now - bottomSince >= 260 && eligible(event.target);
    lastWheel = now;
    if (event.ctrlKey || Math.abs(event.deltaX) > Math.abs(event.deltaY)) return;
    if (event.deltaY < 0) wheelStartedAtBottom = false;
    if (event.deltaY > 0 && wheelStartedAtBottom && atBottom() && eligible(event.target)) reveal();
  }, {passive:true});
  let touchY = null;
  window.addEventListener('touchstart', event => {
    touchY = event.touches.length === 1 && atBottom() && eligible(event.target) ? event.touches[0].clientY : null;
  }, {passive:true});
  window.addEventListener('touchmove', event => {
    if (touchY === null || event.touches.length !== 1) return;
    const distance = touchY - event.touches[0].clientY;
    if (distance < -8) touchY = null;
    else if (distance >= 48 && atBottom() && eligible(event.target)) reveal();
  }, {passive:true});
  for (const type of ['touchend', 'touchcancel']) window.addEventListener(type, () => { touchY = null; }, {passive:true});
  window.addEventListener('keydown', event => {
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
