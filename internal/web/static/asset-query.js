(() => {
  const root = document.querySelector('#asset-browser');
  if (!root || !window.fetch || !window.AbortController) return;
  const form = root.querySelector('.asset-filters');
  const search = form.elements.namedItem('q');
  const disclosure = form.querySelector('details');
  const summary = disclosure.querySelector('summary');
  const clearLink = form.querySelector('[data-asset-clear]');
  const feedback = root.querySelector('[data-asset-feedback]');
  const status = feedback.querySelector('[role="status"]');
  const retry = feedback.querySelector('button');
  const login = feedback.querySelector('a');
  const fields = ['status', 'sort', 'direction'];
  const keys = ['q', ...fields, 'page'];
  let applied = readFilters(), sequence = 0, controller, debounce, composing = false, lastRequest;

  function readFilters() {
    const values = new URLSearchParams(new FormData(form));
    return Object.fromEntries(fields.map(key => [key, values.get(key) || '']));
  }
  function urlFor(query, filters, page = '') {
    const url = new URL(location.href);
    for (const key of keys) url.searchParams.delete(key);
    url.searchParams.set('view', form.elements.namedItem('view').value);
    if (query) url.searchParams.set('q', query);
    for (const key of fields) if (filters[key]) url.searchParams.set(key, filters[key]);
    if (page) url.searchParams.set('page', page);
    return url;
  }
  function invalidate() {
    clearTimeout(debounce);
    controller?.abort();
    sequence++;
    root.querySelector('[data-asset-results]').removeAttribute('aria-busy');
    feedback.hidden = true;
  }
  function setDraft(filters) {
    for (const key of fields) {
      const input = form.elements.namedItem(key);
      input.value = filters[key];
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
    const oldResults = root.querySelector('[data-asset-results]');
    oldResults.setAttribute('aria-busy', 'true');
    retry.hidden = login.hidden = true;

    try {
      const response = await fetch(url, {signal: controller.signal, credentials:'same-origin'});
      if (revision !== sequence) return;
      const destination = new URL(response.url || url);
      if (response.status === 401 || destination.pathname === '/login') throw new Error('login');
      if (!response.ok || destination.origin !== location.origin || destination.pathname !== location.pathname) throw new Error('response');
      const doc = new DOMParser().parseFromString(await response.text(), 'text/html');
      if (revision !== sequence) return;
      const replacement = doc.querySelector('[data-asset-results]');
      const nextForm = doc.querySelector('.asset-filters');
      const nextCount = doc.querySelector('[data-asset-count]');
      const nextClear = nextForm?.querySelector('[data-asset-clear]');
      const nextSummary = nextForm?.querySelector('summary');
      const nextViews = nextForm?.querySelectorAll('.view-switcher a');
      if (!replacement || !nextCount || !nextClear || !nextSummary || !nextForm.elements.namedItem('view') || nextViews.length !== form.querySelectorAll('.view-switcher a').length) throw new Error('fragment');
      replacement.querySelectorAll('script').forEach(script => script.remove());
      const hadFocus = oldResults.contains(document.activeElement);
      const x = window.scrollX, y = window.scrollY;
      oldResults.replaceWith(replacement);
      applied = {...filters};
      if (resetDraft) { search.value = url.searchParams.get('q') || ''; setDraft(filters); }
      clearLink.hidden = nextClear.hidden;
      clearLink.href = nextClear.href;
      root.querySelector('[data-asset-count]').textContent = nextCount.textContent;
      form.elements.namedItem('view').value = nextForm.elements.namedItem('view').value;
      form.querySelectorAll('.view-switcher a').forEach((link, index) => {
        link.href = nextViews[index].href;
        link.className = nextViews[index].className;
        if (nextViews[index].hasAttribute('aria-current')) link.setAttribute('aria-current', 'page');
        else link.removeAttribute('aria-current');
      });
      summary.replaceChildren(...nextSummary.childNodes);
      summary.setAttribute('aria-label', nextSummary.getAttribute('aria-label'));
      disclosure.classList.toggle('has-active', nextForm.querySelector('details').classList.contains('has-active'));
      history.replaceState(history.state, '', destination.pathname + destination.search + destination.hash);
      if (closeOnSuccess) closeFilters(document.activeElement && disclosure.contains(document.activeElement));
      if (hadFocus) replacement.focus({preventScroll:true});
      window.scrollTo(x, y);
      feedback.hidden = true;
      status.textContent = '';
    } catch (error) {
      if (revision !== sequence || error.name === 'AbortError') return;
      feedback.hidden = false;
      status.textContent = error.message === 'login' ? root.dataset.session : root.dataset.failed;
      login.hidden = error.message !== 'login';
      retry.hidden = error.message === 'login';
    } finally {
      if (revision === sequence) root.querySelector('[data-asset-results]').removeAttribute('aria-busy');
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
    const apply = event.submitter?.hasAttribute('data-asset-apply');
    const filters = apply ? readFilters() : applied;
    request(urlFor(search.value, filters), filters, apply);
  });
  root.addEventListener('click', event => {
    const link = event.target.closest('[data-asset-clear], [data-asset-results] .pagination a, [data-asset-results] .table-sort, .view-switcher a');
    if (!link || event.button !== 0 || event.ctrlKey || event.metaKey || event.shiftKey || event.altKey) return;
    event.preventDefault();
    const next = new URL(link.href);
    const filters = Object.fromEntries(fields.map(key => [key, next.searchParams.get(key) || ({sort:'created', direction:'desc'})[key] || '']));
    request(next, filters, true, true);
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
