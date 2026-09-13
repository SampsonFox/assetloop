(() => {
  const initialized = new WeakSet();
  let active = null, closing = null, pinned = false, timer;
  const close = () => {
    clearTimeout(timer);
    if (active) {
      active.querySelector('.timeline-trigger').setAttribute('aria-expanded', 'false');
      const details = active.querySelector('.timeline-details');
      details.hidden = true;
      details.inert = true;
      closing = details;
    }
    active = null;
    pinned = false;
  };
  const open = (item) => {
    const details = item.querySelector('.timeline-details');
    const previous = active ? active.querySelector('.timeline-details') : closing;
    const top = previous && previous !== details ? item.getBoundingClientRect().top : null;
    if (previous && previous !== details) {
      close();
      // A collapsing row above the target must not pull its heading upward.
      // Settle that collapse before measuring; the new details still animate down.
      previous.getAnimations().forEach(animation => animation.finish());
    }
    closing = null;
    active = item;
    details.hidden = false;
    details.inert = false;
    item.querySelector('.timeline-trigger').setAttribute('aria-expanded', 'true');
    if (top !== null) window.scrollBy({top:item.getBoundingClientRect().top - top, behavior:'instant'});
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
