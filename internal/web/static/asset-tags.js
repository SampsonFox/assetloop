(() => {
  const model = document.querySelector('[data-asset-model]');
  const fields = document.querySelector('[data-asset-tag-fields]');
  if (!model || !fields) return;
  const templates = new Map([...document.querySelectorAll('[data-asset-tag-template]')].map(node => [node.dataset.assetTagTemplate, node]));
  let previous = model.value;
  model.addEventListener('change', () => {
    const current = [...fields.querySelectorAll('option:checked, input:checked')].filter(node => node.value);
    const next = templates.get(model.value)?.content.cloneNode(true);
    const choices = next ? [...next.querySelectorAll('[data-tag-name]')] : [];
    const allowed = new Set(choices.map(node => node.value));
    const removed = current.filter(node => !allowed.has(node.value));
    if (removed.length && !window.confirm(`${model.dataset.removeTagsConfirm}\n${removed.map(node => node.dataset.tagName).join(' · ')}`)) {
      model.value = previous;
      return;
    }
    const selected = new Set(current.map(node => node.value));
    for (const node of choices) {
      if (node.tagName === 'OPTION') node.selected = selected.has(node.value);
      else node.checked = selected.has(node.value);
    }
    if (next) {
      for (const select of next.querySelectorAll('select')) {
        if (![...select.options].some(option => option.value && option.selected)) select.value = '';
      }
      fields.replaceChildren(next);
    } else fields.replaceChildren();
    previous = model.value;
  });
})();
