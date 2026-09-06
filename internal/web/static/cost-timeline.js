(() => {
  const initialized = new WeakSet();
  let active = null, pinned = false, timer;
  const close = () => {
    clearTimeout(timer);
    if (active) {
      active.querySelector('.timeline-trigger').setAttribute('aria-expanded', 'false');
      const details = active.querySelector('.timeline-details');
      details.hidden = true;
      details.inert = true;
    }
    active = null;
    pinned = false;
  };
  const open = (item) => {
    if (active !== item) close();
    active = item;
    const details = item.querySelector('.timeline-details');
    details.hidden = false;
    details.inert = false;
    item.querySelector('.timeline-trigger').setAttribute('aria-expanded', 'true');
  };
  const initialize = () => { for (const item of document.querySelectorAll('[data-timeline-item]')) {
    if (initialized.has(item)) continue;
    initialized.add(item);
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
  } };
  initialize();
  document.addEventListener('timeline:updated', () => { close(); initialize(); });
  document.addEventListener('click', (event) => { if (active && !active.contains(event.target)) close(); });
  document.addEventListener('keydown', (event) => {
    if (event.key !== 'Escape' || !active) return;
    const trigger = active.querySelector('.timeline-trigger');
    trigger.focus();
    close();
  });
})();
