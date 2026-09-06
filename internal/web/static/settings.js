(() => {
  const roots = new Set(['/admin/catalog', '/admin/tags', '/admin/3d', '/admin/event-types']);
  const tabs = document.querySelector('.settings-tabs');
  const feedback = document.querySelector('[data-settings-feedback]');
  if (!tabs || !feedback) return;
  const retry = feedback.querySelector('[data-settings-retry]');
  const login = feedback.querySelector('[data-settings-login]');
  const message = feedback.querySelector('span');
  const key = 'assetloop.settings.urls';
  let saved = {};
  try {
    const value = JSON.parse(sessionStorage.getItem(key) || '{}');
    if (value && typeof value === 'object' && !Array.isArray(value)) saved = value;
  } catch {}
  let controller, serial = 0, failedURL, renderedURL = location.href;
  const content = () => document.getElementById('settings-content');
  const localRoot = (url) => url.origin === location.origin && roots.has(url.pathname);
  const remember = () => {
    const url = new URL(renderedURL);
    if (!localRoot(url)) return;
    // Keep list state and explicit binding context, never reopen an old editor.
    for (const param of [...url.searchParams.keys()]) {
      if (!['q', 'sort', 'direction', 'page', 'status', 'view', 'type_id', 'category', 'kind', 'target', 'name'].includes(param)) url.searchParams.delete(param);
    }
    url.hash = '';
    saved[url.pathname] = url.pathname + url.search;
    try { sessionStorage.setItem(key, JSON.stringify(saved)); } catch {}
  };
  const prepare = () => {
    for (const form of content().querySelectorAll('form[method="post"]')) {
      form.dataset.guardDirty = '';
      form.dataset.discardConfirm ||= feedback.dataset.discard;
    }
  };
  const canLeave = () => {
    if (content().querySelector('form[data-submitting="true"]')) return false;
    const dirty = content().querySelector('form[data-dirty="true"]');
    return !dirty || window.confirm(dirty.dataset.discardConfirm || feedback.dataset.discard);
  };
  const show = (text, failed = false, expired = false) => {
    message.textContent = text;
    retry.hidden = !failed || expired;
    login.hidden = !expired;
    feedback.hidden = false;
  };
  const navigate = async (url, fromHistory = false) => {
    if (!canLeave()) {
      if (fromHistory) history.pushState(history.state, '', renderedURL);
      return;
    }
    remember();
    controller?.abort();
    controller = new AbortController();
    const requestID = ++serial;
    content().setAttribute('aria-busy', 'true');
    feedback.hidden = true;
    const slow = setTimeout(() => {
      if (requestID === serial) show(feedback.dataset.loading);
    }, 250);
    try {
      const response = await fetch(url, {signal: controller.signal, credentials: 'same-origin', headers: {Accept: 'text/html'}});
      if (requestID !== serial) return;
      if (response.status === 401 || new URL(response.url).pathname === '/login') {
        throw Object.assign(new Error('login'), {expired: true});
      }
      if (!response.ok) throw new Error('http');
      const page = new DOMParser().parseFromString(await response.text(), 'text/html');
      if (requestID !== serial) return;
      const next = page.getElementById('settings-content');
      const nextTabs = page.querySelector('.settings-tabs');
      if (!next || !nextTabs || !localRoot(new URL(response.url))) throw new Error('content');
      // Only root management pages are enhanced. Preview modules use native navigation.
      for (const script of next.querySelectorAll('script')) script.remove();
      for (const dialog of content().querySelectorAll('dialog[open]')) dialog.close();
      content().replaceWith(next);
      tabs.replaceChildren(...nextTabs.childNodes);
      document.title = page.title;
      if (!fromHistory) history.pushState({}, '', response.url);
      else history.replaceState(history.state, '', response.url);
      renderedURL = location.href;
      const returnTo = document.querySelector('.preference-form [name="return_to"]');
      if (returnTo) returnTo.value = location.pathname + location.search + location.hash;
      prepare();
      document.dispatchEvent(new Event('settings:loaded'));
      renderedURL = location.href; // drawer initialization can consume deep-link parameters
      remember();
      if (!next.querySelector('dialog[open]')) next.focus({preventScroll: true});
      feedback.hidden = true;
      failedURL = undefined;
    } catch (error) {
      if (error.name === 'AbortError' || requestID !== serial) return;
      failedURL = url;
      if (fromHistory) history.replaceState(history.state, '', renderedURL);
      show(error.expired ? feedback.dataset.login : feedback.dataset.failure, true, error.expired);
    } finally {
      clearTimeout(slow);
      if (requestID === serial) content().removeAttribute('aria-busy');
    }
  };
  document.addEventListener('click', (event) => {
    if (event.defaultPrevented || event.button !== 0 || event.metaKey || event.ctrlKey || event.shiftKey || event.altKey) return;
    const link = event.target.closest('a[href]');
    if (!link || link.target || link.hasAttribute('download') || !(tabs.contains(link) || content().contains(link))) return;
    let url = new URL(link.href);
    if (!localRoot(url) || !roots.has(location.pathname)) return;
    if (link.hasAttribute('data-settings-tab') && saved[url.pathname]) {
      try {
        const candidate = new URL(saved[url.pathname], location.origin);
        if (localRoot(candidate) && candidate.pathname === url.pathname) url = candidate;
      } catch {}
    }
    // Preserve native in-page reference navigation.
    if (url.pathname === location.pathname && url.search === location.search && url.hash) return;
    event.preventDefault();
    navigate(url.href);
  });
  // Capture first: app.js must not mark a locally handled GET form as submitting.
  document.addEventListener('submit', (event) => {
    const form = event.target;
    if (event.defaultPrevented || form.method !== 'get' || !content().contains(form)) return;
    const url = new URL(form.action);
    if (!localRoot(url) || !roots.has(location.pathname)) return;
    event.preventDefault();
    url.search = new URLSearchParams(new FormData(form)).toString();
    navigate(url.href);
  }, true);
  const retainNewInput = (event) => {
    if (!content().contains(event.target) || !content().hasAttribute('aria-busy')) return;
    controller?.abort();
    ++serial;
    content().removeAttribute('aria-busy');
    feedback.hidden = true;
  };
  document.addEventListener('input', retainNewInput);
  document.addEventListener('change', retainNewInput);
  retry.addEventListener('click', () => { if (failedURL) navigate(failedURL); });
  window.addEventListener('popstate', () => {
    if (!roots.has(location.pathname) || !roots.has(new URL(renderedURL).pathname)) {
      location.reload();
      return;
    }
    navigate(location.href, true);
  });
  window.addEventListener('pagehide', remember);
  prepare();
  remember();
})();
