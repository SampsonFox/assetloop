(() => {
  const footer = document.querySelector('[data-delete-reveal]');
  if (!footer) return;
  const root = document.scrollingElement;
  footer.hidden = false;
  footer.inert = true;
  // The footer always occupies space. The first stop is its top edge, not
  // the document end, so pulling moves real content and uncovers the footer.
  const anchor = () => Math.max(0, footer.getBoundingClientRect().top + root.scrollTop - root.clientHeight);
  const end = () => Math.max(0, root.scrollHeight - root.clientHeight);
  const atAnchor = () => root.scrollTop >= anchor() - 2;
  const motion = () => matchMedia('(prefers-reduced-motion: reduce)').matches ? 'instant' : 'smooth';
  const eligible = target => {
    if (document.querySelector('dialog[open]')) return false;
    if (target?.closest?.('input, textarea, select, [contenteditable="true"], .account-menu[open]')) return false;
    for (let node = target; node && node !== root && node !== document.body; node = node.parentElement) {
      if (node.scrollHeight > node.clientHeight + 2 && /auto|scroll/.test(getComputedStyle(node).overflowY)) return false;
    }
    return true;
  };
  let revealed = false, preview = 0, wheelPull = 0, releaseTimer;
  let lastWheel = -Infinity, wheelStartedAtBottom = false;
  let bottomSince = atAnchor() ? performance.now() : Infinity;
  function release() {
    clearTimeout(releaseTimer);
    const wasRevealed = revealed;
    const hadPull = revealed || preview > 0;
    revealed = false;
    footer.inert = true;
    wheelPull = 0;
    wheelStartedAtBottom = false;
    // Keep the current travel available while the return animation runs.
    preview = Math.max(0, root.scrollTop - anchor());
    if (hadPull) root.scrollTo({top:Math.min(root.scrollTop, anchor()), behavior:motion()});
    if (!atAnchor()) bottomSince = Infinity;
    else if (wasRevealed || !Number.isFinite(bottomSince)) bottomSince = performance.now();
  }
  function reveal(keyboard = false) {
    if (revealed) return;
    clearTimeout(releaseTimer);
    revealed = true;
    preview = 0;
    footer.inert = false;
    root.scrollTo({top:end(), behavior:motion()});
    if (keyboard) footer.querySelector('a').focus({preventScroll:true});
  }
  function pull(ratio) {
    if (ratio >= 1) { reveal(); return; }
    // Resistance: a light pull reveals a hint; even near the threshold the
    // button remains partly below the viewport and inert until latched open.
    preview = Math.min(72, footer.offsetHeight * .65) * Math.sqrt(Math.max(0, ratio));
    root.scrollTo({top:anchor() + preview, behavior:'instant'});
  }
  function constrainScroll() {
    if (!revealed && root.scrollTop > anchor() + preview + 1) {
      root.scrollTo({top:anchor() + preview, behavior:'instant'});
    }
    if (!atAnchor()) bottomSince = Infinity;
    else if (!Number.isFinite(bottomSince)) bottomSince = performance.now();
    if (!revealed && root.scrollTop <= anchor() + 1) preview = 0;
  }
  window.addEventListener('scroll', constrainScroll, {passive:true});
  window.addEventListener('resize', constrainScroll, {passive:true});
  constrainScroll();
  window.addEventListener('wheel', event => {
    if (event.ctrlKey || Math.abs(event.deltaX) > Math.abs(event.deltaY) || !eligible(event.target)) return;
    const now = performance.now();
    if (now - lastWheel > 260) {
      wheelStartedAtBottom = atAnchor() && now - bottomSince >= 260;
      wheelPull = 0;
    }
    lastWheel = now;
    if (event.deltaY < 0) {
      if (revealed || preview > 0) event.preventDefault();
      release();
      return;
    }
    if (event.deltaY <= 0) return;
    if (revealed) { event.preventDefault(); return; }
    const delta = event.deltaY * (event.deltaMode === 1 ? 16 : event.deltaMode === 2 ? root.clientHeight : 1);
    if (wheelStartedAtBottom && atAnchor()) {
      event.preventDefault();
      clearTimeout(releaseTimer);
      wheelPull += delta;
      pull(wheelPull / 240);
      if (!revealed) releaseTimer = setTimeout(release, 220);
    } else if (root.scrollTop + delta >= anchor()) {
      // The gesture that arrives at the first stop cannot consume the second.
      event.preventDefault();
      root.scrollTo({top:anchor(), behavior:'instant'});
      constrainScroll();
    }
  }, {passive:false});
  let touchY = null, lastTouchY = null;
  window.addEventListener('touchstart', event => {
    clearTimeout(releaseTimer);
    touchY = event.touches.length === 1 && (revealed || atAnchor()) && eligible(event.target) ? event.touches[0].clientY : null;
    lastTouchY = touchY;
  }, {passive:true});
  window.addEventListener('touchmove', event => {
    if (touchY === null) return;
    if (event.touches.length !== 1 || !eligible(event.target)) { if (!revealed) release(); touchY = null; return; }
    const y = event.touches[0].clientY;
    if (revealed && y - lastTouchY > 8) { event.preventDefault(); release(); touchY = null; return; }
    lastTouchY = y;
    const distance = touchY - y;
    if (distance < -8) { release(); touchY = null; }
    else if (revealed || atAnchor()) {
      event.preventDefault();
      if (!revealed) pull(distance / 96);
    }
  }, {passive:false});
  for (const type of ['touchend', 'touchcancel']) window.addEventListener(type, () => {
    touchY = null;
    if (!revealed) release();
  }, {passive:true});
  window.addEventListener('keydown', event => {
    if (eligible(event.target) && (['ArrowUp', 'PageUp', 'Home', 'Escape'].includes(event.key) || (event.key === ' ' && event.shiftKey))) release();
    if (revealed || event.repeat || event.altKey || event.ctrlKey || event.metaKey || event.shiftKey || !atAnchor() || !eligible(event.target)) return;
    if (event.key === 'Tab') reveal();
    else if (['ArrowDown', 'PageDown', 'End', ' '].includes(event.key) && !event.target?.closest?.('button, a, summary')) {
      event.preventDefault();
      reveal(true);
    }
  });
})();
