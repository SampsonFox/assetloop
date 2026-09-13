(() => {
  if (window.assetloopAssetTags) {
    window.assetloopAssetTags.initialize(document);
    return;
  }
  const initialized = new WeakSet();
  const refreshers = new WeakMap();
  function initialize(root = document) {
    for (const select of root.querySelectorAll('[data-resource-choice]')) {
      if (initialized.has(select)) continue;
      initialized.add(select);
      initializeResource(select);
    }
    for (const model of root.querySelectorAll('[data-asset-model]')) {
      const form = model.closest('form');
      const fields = form?.querySelector('[data-asset-tag-fields]');
      if (!fields || initialized.has(model)) continue;
      initialized.add(model);
      initializeModel(model, fields, form);
    }
  }
  function initializeResource(select) {
    const input = document.createElement('input');
    const list = document.createElement('datalist');
    const preview = select.closest('[data-asset-resource]').querySelector('[data-resource-preview]');
    input.id = select.id; select.removeAttribute('id');
    list.id = input.id + '-choices'; input.setAttribute('list', list.id);
    input.autocomplete = 'off'; input.required = true; input.name = 'resource_search';
    const options = [...select.options];
    const choices = options.map(option => ({id: option.value, label: options.filter(other => other.text === option.text).length > 1 ? `${option.text} (${option.value})` : option.text}));
    for (const choice of choices) { const option = document.createElement('option'); option.value = choice.label; list.append(option); }
    select.before(input, list); select.hidden = true; preview.hidden = false;
    const sync = () => { input.value = choices.find(choice => choice.id === select.value)?.label || ''; input.setCustomValidity(''); preview.disabled = !select.value; };
    sync();
    input.addEventListener('input', () => {
      const choice = choices.find(choice => choice.label === input.value);
      input.setCustomValidity(choice ? '' : select.dataset.invalidMessage);
      preview.disabled = !choice?.id;
      if (choice) { select.value = choice.id; select.dispatchEvent(new Event('change', {bubbles:true})); }
    });
    preview.addEventListener('click', () => {
      if (!select.value || !input.validity.valid) return;
      window.assetloopDrawers?.open({href:'/resources/' + encodeURIComponent(select.value), dataset:{drawerTarget:'resource-editor'}, get isConnected() { return preview.isConnected; }, focus: options => preview.focus(options)});
    });
    select.form.addEventListener('reset', () => queueMicrotask(sync));
    window.assetloopDialog?.initialize(select.closest('dialog') || select.closest('section'));
  }
  function initializeModel(model, fields, form) {
    let previous = model.value;
    let revision = 0;
    let refreshRevision = 0;
    const applySelections = (fragment, selected) => {
      for (const node of fragment.querySelectorAll('[data-tag-name]')) {
        if (node.tagName === 'OPTION') node.selected = selected.has(node.value);
        else node.checked = selected.has(node.value);
      }
      for (const select of fragment.querySelectorAll('select')) {
        if (![...select.options].some(option => option.value && option.selected)) select.value = '';
      }
    };
    refreshers.set(model, async () => {
      const id = model.value, requestRevision = ++refreshRevision, selectionRevision = revision;
      const status = form.querySelector('[data-tag-change-status]');
      const english = document.documentElement?.lang.startsWith('en');
      const stillCurrent = () => model.isConnected && model.value === id && requestRevision === refreshRevision && selectionRevision === revision;
      try {
        const response = await fetch(`/assets/new?model_id=${encodeURIComponent(id)}`, {credentials: 'same-origin', headers: {'X-Assetloop-Drawer': 'asset-editor'}});
        if (!response.ok) throw Error('refresh');
        const page = new DOMParser().parseFromString(await response.text(), 'text/html');
        const fresh = [...page.querySelectorAll('[data-asset-tag-template]')].find(node => node.dataset.assetTagTemplate === id);
        if (!fresh) throw Error('template');
        if (!stillCurrent()) return;
        const next = fresh.content.cloneNode(true);
        const current = [...fields.querySelectorAll('option:checked, input:checked')].filter(node => node.value);
        const allowed = new Set([...next.querySelectorAll('[data-tag-name]')].map(node => node.value));
        const invalid = current.filter(node => !allowed.has(node.value));
        applySelections(next, new Set(current.map(node => node.value)));
        if (invalid.length) {
          const retained = document.createElement('fieldset');
          retained.dataset.invalidAssetTags = '';
          const legend = document.createElement('legend');
          legend.textContent = english ? 'Selected tags no longer allowed' : '已选标签已不再允许';
          retained.append(legend);
          for (const node of invalid) {
            const label = document.createElement('label');
            label.className = 'checkbox';
            const input = document.createElement('input');
            input.type = 'checkbox'; input.name = 'tag_ids'; input.value = node.value; input.checked = true;
            input.dataset.tagName = node.dataset.tagName || node.value;
            label.append(input, document.createTextNode(input.dataset.tagName));
            retained.append(label);
          }
          next.append(retained);
        }
        const old = [...form.querySelectorAll('[data-asset-tag-template]')].find(node => node.dataset.assetTagTemplate === id);
        if (old) old.replaceWith(fresh);
        else form.append(fresh);
        fields.replaceChildren(next);
        if (status) status.textContent = invalid.length
          ? (english ? 'Allowed tags updated. Uncheck the unavailable selections before saving; your draft is preserved.' : '允许标签已更新。请取消不再允许的选择后保存；当前草稿已保留。')
          : (english ? 'Allowed tags updated; your selections are preserved.' : '允许标签已更新，当前选择已保留。');
      } catch {
        if (stillCurrent() && status) status.textContent = english ? 'Unable to refresh allowed tags. Your draft is preserved; save the model again to retry.' : '无法刷新允许标签。当前草稿已保留，可再次保存型号重试。';
      }
    });
    model.addEventListener('change', async () => {
      const currentRevision = ++revision;
      if (model.value === previous) return;
      const selectedModel = model.value;
      const template = [...form.querySelectorAll('[data-asset-tag-template]')].find(node => node.dataset.assetTagTemplate === selectedModel);
      const current = [...fields.querySelectorAll('option:checked, input:checked')].filter(node => node.value);
      const next = template?.content.cloneNode(true);
      const choices = next ? [...next.querySelectorAll('[data-tag-name]')] : [];
      const allowed = new Set(choices.map(node => node.value));
      const removed = current.filter(node => !allowed.has(node.value));
      if (removed.length) {
        const accepted = await window.assetloopDialog.confirm(`${model.dataset.removeTagsConfirm}\n${removed.map(node => node.dataset.tagName).join(' · ')}`);
        if (revision !== currentRevision || model.value !== selectedModel) return;
        if (!accepted) {
          model.value = previous;
          model.dispatchEvent(new Event('change', {bubbles: true}));
          return;
        }
      }
      const selected = new Set(current.map(node => node.value));
      if (next) {
        applySelections(next, selected);
        fields.replaceChildren(next);
      } else fields.replaceChildren();
      previous = model.value;
    });
  }
  window.assetloopAssetTags = {initialize};
  initialize();
  document.addEventListener('drawer:loaded', event => initialize(event.target));
  document.addEventListener('drawer:saved', event => {
    if (event.detail?.kind !== 'model') return;
    const pending = [];
    for (const model of document.querySelectorAll('[data-asset-model]')) {
      if (model.value === event.detail.id) pending.push(refreshers.get(model)?.());
    }
    return Promise.all(pending);
  });
})();
