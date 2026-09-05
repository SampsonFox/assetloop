(() => {
  const items = [...document.querySelectorAll('[data-timeline-item]')];
  let active = null, pinned = false, timer;
  const close = () => {
    clearTimeout(timer);
    if (active) {
      active.querySelector('.timeline-trigger').setAttribute('aria-expanded', 'false');
      active.querySelector('.timeline-popover').hidden = true;
    }
    active = null;
    pinned = false;
  };
  const open = (item) => {
    if (active !== item) close();
    active = item;
    item.querySelector('.timeline-popover').hidden = false;
    item.querySelector('.timeline-trigger').setAttribute('aria-expanded', 'true');
  };
  for (const item of items) {
    const trigger = item.querySelector('.timeline-trigger');
    item.addEventListener('pointerenter', (event) => {
      if (event.pointerType !== 'mouse' || pinned) return;
      clearTimeout(timer);
      timer = setTimeout(() => open(item), 150);
    });
    item.addEventListener('pointerleave', () => {
      clearTimeout(timer);
      if (!pinned && active === item && !item.contains(document.activeElement)) close();
    });
    item.addEventListener('focusin', () => { if (!pinned) open(item); });
    item.addEventListener('focusout', (event) => {
      if (!pinned && !item.contains(event.relatedTarget)) close();
    });
    trigger.addEventListener('click', () => {
      clearTimeout(timer);
      if (active === item && pinned) close();
      else { open(item); pinned = true; }
    });
  }
  document.addEventListener('click', (event) => { if (active && !active.contains(event.target)) close(); });
  document.addEventListener('keydown', (event) => {
    if (event.key !== 'Escape' || !active) return;
    const trigger = active.querySelector('.timeline-trigger');
    trigger.focus();
    close();
  });
})();
