(() => {
  const root = document.querySelector('#lifecycle-timeline');
  if (!root || !window.fetch || !window.AbortController) return;
  const form = root.querySelector('.timeline-filters');
  const search = form.elements.namedItem('q');
  const disclosure = form.querySelector('details');
  const summary = disclosure.querySelector('summary');
  const clearLink = form.querySelector('[data-timeline-clear]');
  const feedback = root.querySelector('[data-timeline-feedback]');
  const status = feedback.querySelector('[role="status"]');
  const retry = feedback.querySelector('button');
  const login = feedback.querySelector('a');
  const fields = ['event_type', 'sort', 'direction', 'show_voided'];
  const keys = ['q', ...fields, 'page'];
  let applied = readFilters(), sequence = 0, controller, debounce, composing = false, lastRequest;

  function readFilters() {
    const values = new URLSearchParams(new FormData(form));
    return Object.fromEntries(fields.map(key => [key, values.get(key) || '']));
  }
  function urlFor(query, filters, page = '') {
    const url = new URL(location.href);
    for (const key of keys) url.searchParams.delete(key);
    if (query) url.searchParams.set('q', query);
    for (const key of fields) if (filters[key]) url.searchParams.set(key, filters[key]);
    if (page) url.searchParams.set('page', page);
    return url;
  }
  function invalidate() {
    clearTimeout(debounce);
    controller?.abort();
    sequence++;
    root.querySelector('[data-timeline-results]').removeAttribute('aria-busy');
    feedback.hidden = true;
  }
  function setDraft(filters) {
    for (const key of fields) {
      const input = form.elements.namedItem(key);
      if (input.type === 'checkbox') input.checked = filters[key] === '1';
      else input.value = filters[key];
    }
  }
  function closeFilters(focus = false) {
    disclosure.open = false;
    if (focus) summary.focus();
  }
  async function request(url, filters, closeOnSuccess = false, resetDraft = false) {
    invalidate();
    const revision = sequence;
    controller = new AbortController();
    lastRequest = {url, filters, closeOnSuccess, resetDraft};
    const oldResults = root.querySelector('[data-timeline-results]');
    oldResults.setAttribute('aria-busy', 'true');
    retry.hidden = login.hidden = true;
    const slow = setTimeout(() => {
      if (revision !== sequence) return;
      feedback.hidden = false;
      status.textContent = root.dataset.loading;
    }, 250);
    try {
      const response = await fetch(url, {signal: controller.signal, credentials:'same-origin'});
      if (revision !== sequence) return;
      const destination = new URL(response.url || url);
      if (response.status === 401 || destination.pathname === '/login') throw new Error('login');
      if (!response.ok || destination.origin !== location.origin || destination.pathname !== location.pathname) throw new Error('response');
      const doc = new DOMParser().parseFromString(await response.text(), 'text/html');
      if (revision !== sequence) return;
      const replacement = doc.querySelector('[data-timeline-results]');
      if (!replacement) throw new Error('fragment');
      replacement.querySelectorAll('script').forEach(script => script.remove());
      const hadFocus = oldResults.contains(document.activeElement);
      const x = window.scrollX, y = window.scrollY;
      oldResults.replaceWith(replacement);
      applied = {...filters};
      if (resetDraft) { search.value = ''; setDraft(filters); }
      const nextForm = doc.querySelector('.timeline-filters');
      const nextClear = nextForm.querySelector('[data-timeline-clear]');
      clearLink.hidden = nextClear.hidden;
      clearLink.href = nextClear.href;
      const nextSummary = nextForm.querySelector('summary');
      summary.setAttribute('aria-label', nextSummary.getAttribute('aria-label'));
      disclosure.classList.toggle('has-active', nextForm.querySelector('details').classList.contains('has-active'));
      history.replaceState(history.state, '', url.pathname + url.search + url.hash);
      document.dispatchEvent(new Event('timeline:updated'));
      if (closeOnSuccess) closeFilters(document.activeElement && disclosure.contains(document.activeElement));
      if (hadFocus) replacement.focus({preventScroll:true});
      window.scrollTo(x, y);
      feedback.hidden = false;
      status.textContent = root.dataset.updated;
    } catch (error) {
      if (revision !== sequence || error.name === 'AbortError') return;
      feedback.hidden = false;
      status.textContent = error.message === 'login' ? root.dataset.session : root.dataset.failed;
      login.hidden = error.message !== 'login';
      retry.hidden = error.message === 'login';
    } finally {
      clearTimeout(slow);
      if (revision === sequence) root.querySelector('[data-timeline-results]').removeAttribute('aria-busy');
    }
  }
  function searchNow() { request(urlFor(search.value, applied), applied); }
  function schedule() { invalidate(); if (!composing) debounce = setTimeout(searchNow, 300); }
  search.addEventListener('compositionstart', () => { composing = true; invalidate(); });
  search.addEventListener('compositionend', () => { composing = false; schedule(); });
  search.addEventListener('input', schedule);
  search.addEventListener('keydown', event => {
    if (event.key !== 'Enter' || event.isComposing || composing) return;
    event.preventDefault(); searchNow();
  });
  form.addEventListener('submit', event => {
    event.preventDefault();
    const apply = event.submitter?.hasAttribute('data-timeline-apply');
    const filters = apply ? readFilters() : applied;
    request(urlFor(search.value, filters), filters, apply);
  });
  root.addEventListener('click', event => {
    const link = event.target.closest('[data-timeline-clear], [data-timeline-results] .pagination a');
    if (!link || event.button !== 0 || event.ctrlKey || event.metaKey || event.shiftKey || event.altKey) return;
    event.preventDefault();
    if (link.hasAttribute('data-timeline-clear')) {
      const defaults = {event_type:'', sort:'occurred', direction:'asc', show_voided:''};
      request(urlFor('', defaults), defaults, true, true);
    } else {
      const next = new URL(link.href);
      const currentQuery = new URL(location.href).searchParams.get('q') || '';
      request(urlFor(currentQuery, applied, next.searchParams.get('page')), applied);
    }
  });
  retry.addEventListener('click', () => {
    const {url, filters, closeOnSuccess, resetDraft} = lastRequest;
    request(url, filters, closeOnSuccess, resetDraft);
  });
  document.addEventListener('click', event => { if (!disclosure.contains(event.target)) closeFilters(); });
  document.addEventListener('keydown', event => {
    if (event.key === 'Escape' && disclosure.open) { event.preventDefault(); closeFilters(true); }
  });
})();
