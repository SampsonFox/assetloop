(() => {
  const fields = {
    name: "name",
    color: "color",
    iconKey: "icon_key",
    categoryId: "category_id",
    modelId: "model_id",
    returnModelId: "return_model_id",
    eventType: "event_type",
    model3dSourceUrl: "model_3d_source_url",
    model3dAuthor: "model_3d_author",
    model3dLicense: "model_3d_license",
  };
  const dialogOpeners = new WeakMap();

  const resetForm = (form) => {
    const hidden = [...form.querySelectorAll('input[type="hidden"]')].map((input) => [input, input.value]);
    form.reset();
    for (const [input, value] of hidden) input.value = value;
  };

  const decimalProduct = (left, right, exponent) => {
    const parse = (value) => {
      const match = String(value).trim().match(/^([+-]?)(\d+)(?:\.(\d+))?$/);
      if (!match) return null;
      const fraction = match[3] || "";
      return { value: BigInt(`${match[1] === "-" ? "-" : ""}${match[2]}${fraction}`), scale: fraction.length };
    };
    const amount = parse(left);
    const rate = parse(right);
    if (!amount || !rate || rate.value <= 0n) return null;
    let value = amount.value * rate.value;
    const scale = amount.scale + rate.scale;
    if (scale > exponent) {
      const divisor = 10n ** BigInt(scale - exponent);
      const negative = value < 0n;
      let magnitude = negative ? -value : value;
      let rounded = magnitude / divisor;
      if ((magnitude % divisor) * 2n >= divisor) rounded += 1n;
      value = negative ? -rounded : rounded;
    } else {
      value *= 10n ** BigInt(exponent - scale);
    }
    const negative = value < 0n;
    let digits = (negative ? -value : value).toString().padStart(exponent + 1, "0");
    if (exponent) digits = `${digits.slice(0, -exponent)}.${digits.slice(-exponent)}`;
    return `${negative ? "-" : ""}${digits}`;
  };

  const syncFXPreview = (form) => {
    const output = form.querySelector("[data-fx-result]");
    if (!output) return;
    const amount = form.elements.namedItem("amount")?.value;
    const rate = form.elements.namedItem("fx_rate")?.value;
    const converted = decimalProduct(amount, rate, Number(output.dataset.baseMinorUnits));
    output.hidden = converted === null;
    const value = output.querySelector("[data-fx-result-value]");
    if (value && converted !== null) value.textContent = `${converted} ${output.dataset.baseCurrency}`;
  };

  const syncFXFields = (select) => {
    const neutral = select.form.querySelector("[data-event-type-select] option:checked")?.dataset.cashflow === "neutral";
    const foreign = !neutral && select.value !== select.dataset.baseCurrency;
    for (const field of select.form.querySelectorAll("[data-fx-field]")) field.hidden = !foreign;
    for (const input of select.form.querySelectorAll("[data-fx-required]")) input.required = foreign;
    for (const unit of select.form.querySelectorAll("[data-fx-rate-from]")) unit.textContent = select.value.toUpperCase();
    syncFXPreview(select.form);
  };

  const chooseEventType = (select) => {
    if (!select.selectedOptions[0]?.hasAttribute('data-event-type-create')) return false;
    select.value = select.dataset.selectedType || '';
    window.assetloopDrawers?.open({
      href: '/admin/event-types?dialog=event-type-manage',
      dataset: {drawerTarget: 'event-type-manage'},
      get isConnected() { return select.isConnected; },
      focus: (options) => select.focus(options),
    });
    return true;
  };

  const syncEventTypeFields = (select) => {
    select.dataset.selectedType = select.value;
    const create = select.querySelector('[data-event-type-create]');
    if (create) { create.hidden = false; create.disabled = false; }
    const neutral = select.selectedOptions[0]?.dataset.cashflow === "neutral";
    const amount = select.form.elements.namedItem("amount");
    for (const field of select.form.querySelectorAll("[data-money-field]")) field.hidden = neutral;
    if (amount) {
      if (neutral) {
        if (amount.value !== "0") amount.dataset.previousValue = amount.value;
        amount.value = "0";
        amount.required = false;
        amount.removeAttribute("pattern");
      } else {
        if (amount.value === "0") amount.value = amount.dataset.previousValue || "";
        amount.required = true;
        amount.pattern = amount.dataset.positivePattern;
      }
    }
    const currency = select.form.querySelector("[data-currency-select]");
    if (currency) syncFXFields(currency);
  };

  const formBaselines = new WeakMap();
  const formValues = (form) => JSON.stringify([...form.elements]
    .filter((field) => field.name && !['submit', 'button', 'reset'].includes(field.type))
    .map((field) => [field.name, field.type === 'file'
      ? [...field.files].map((file) => [file.name, file.size, file.lastModified])
      : ['checkbox', 'radio'].includes(field.type) ? field.checked
      : field.multiple ? [...field.selectedOptions].map((option) => option.value) : field.value]));
  const rememberDialogForms = (dialog) => {
    for (const form of dialog.querySelectorAll('form[data-guard-dirty]')) {
      formBaselines.set(form, formValues(form));
      form.dataset.dirty = 'false';
    }
  };
  const updateDirty = (form) => {
    if (form) form.dataset.dirty = String(formBaselines.has(form)
      ? formValues(form) !== formBaselines.get(form) : true);
  };
  const dirtyForm = (dialog) => {
    const forms = [...dialog.querySelectorAll('form[data-guard-dirty]')];
    for (const form of forms) {
      if (formBaselines.has(form)) updateDirty(form);
    }
    return forms.find((form) => form.dataset.dirty === 'true');
  };
  // Use an in-page modal: embedded browsers may suppress native confirm UI.
  const confirmDiscard = (message, rename = false) => new Promise((resolve) => {
    const english = document.documentElement.lang.startsWith('en');
    const prompt = document.createElement('dialog');
    prompt.className = 'discard-dialog';
    prompt.setAttribute('aria-labelledby', 'discard-dialog-title');
    prompt.setAttribute('aria-describedby', 'discard-dialog-message');
    const title = document.createElement('h2');
    title.id = 'discard-dialog-title';
    title.textContent = english ? 'Discard changes?' : '放弃修改？';
    if (rename) title.textContent = english ? 'Rename shared label?' : '确认修改共享名称？';
    const description = document.createElement('p');
    description.id = 'discard-dialog-message';
    description.textContent = message;
    const actions = document.createElement('div');
    actions.className = 'discard-actions';
    const stay = document.createElement('button');
    stay.type = 'button'; stay.className = 'secondary auto';
    stay.textContent = english ? 'Keep editing' : '继续编辑';
    const discard = document.createElement('button');
    discard.type = 'button'; discard.className = 'auto';
    discard.textContent = english ? 'Discard changes' : '放弃修改';
    if (rename) discard.textContent = english ? 'Confirm and save' : '确认并保存';
    const finish = (accepted) => { prompt.close(); prompt.remove(); resolve(accepted); };
    stay.addEventListener('click', () => finish(false));
    discard.addEventListener('click', () => finish(true));
    prompt.addEventListener('cancel', (event) => { event.preventDefault(); finish(false); });
    prompt.addEventListener('click', (event) => { if (event.target === prompt) finish(false); });
    actions.append(stay, discard); prompt.append(title, description, actions);
    document.body.append(prompt); prompt.showModal(); stay.focus();
  });
  // Deep links open a drawer once; refreshing after dismissal must not reopen it.
  const consumeDialogURL = (dialog) => {
    if (!dialog) return;
    const url = new URL(window.location.href);
    const matched = url.searchParams.get("dialog") === dialog.id;
    const eventHash = dialog.id === "event-drawer" && url.hash === "#add-event";
    if (!matched && !eventHash) return;
    if (matched) {
      url.searchParams.delete("dialog");
      if (dialog.id === "event-drawer") url.searchParams.delete("event_type");
      if (dialog.id === "model-drawer") url.searchParams.delete("edit_model_id");
    }
    if (eventHash) url.hash = "lifecycle-timeline";
    window.history.replaceState(window.history.state, "", url.pathname + url.search + url.hash);
  };
  const discardDialogForms = (dialog) => {
    for (const form of dialog.querySelectorAll("form[data-guard-dirty]")) {
      if (form.dataset.dirty === "true") resetForm(form);
      form.dataset.dirty = "false";
    }
  };
  const closingDialogs = new WeakSet();
  const closeDialog = async (dialog) => {
    if (dialog?.querySelector('form[data-submitting="true"]')) return false;
    if (!dialog?.open || closingDialogs.has(dialog)) return false;
    closingDialogs.add(dialog);
    try {
      const form = dirtyForm(dialog);
      if (form && !await confirmDiscard(form.dataset.discardConfirm)) return false;
      await window.assetloopDrawers?.beforeClose(dialog);
      dialog.close();
      discardDialogForms(dialog);
      return true;
    } finally { closingDialogs.delete(dialog); }
  };
  // Standalone editors navigate away rather than closing a drawer.
  const pageForms = () => [...document.querySelectorAll('form[data-guard-dirty]')]
    .filter((form) => !form.closest('dialog, #settings-content'));
  let leavingPage = false, checkingPageLeave = false;
  const leaveEditor = async (url) => {
    if (checkingPageLeave || leavingPage) return;
    checkingPageLeave = true;
    try {
      const forms = pageForms();
      for (const form of forms) updateDirty(form);
      const dirty = forms.find((form) => form.dataset.dirty === 'true');
      if (dirty && !await confirmDiscard(dirty.dataset.discardConfirm)) return;
      // Avoid a second, browser-native beforeunload prompt after confirmation.
      leavingPage = true;
      window.location.assign(url);
    } finally { checkingPageLeave = false; }
  };
  document.addEventListener('click', (event) => {
    if (event.defaultPrevented || event.button !== 0 || event.ctrlKey || event.metaKey || event.shiftKey || event.altKey) return;
    const link = event.target.closest('a[href]');
    if (!link || link.target || link.hasAttribute('download') || link.closest('dialog') || !pageForms().length) return;
    const url = new URL(link.href, window.location.href);
    if (url.origin !== window.location.origin || (url.pathname === window.location.pathname && url.search === window.location.search && url.hash)) return;
    event.preventDefault();
    leaveEditor(url.href);
  });
  const focusDialog = (dialog) => {
    const target = dialog.querySelector("[data-error-summary]")
      || dialog.querySelector("[data-dialog-initial-focus]")
      || dialog.querySelector("input:not([type='hidden']), select, textarea, button");
    target?.focus();
  };

  const initializedDialogs = new WeakSet();
  const initDialogs = () => {
  for (const dialog of document.querySelectorAll("dialog.drawer")) {
    if (initializedDialogs.has(dialog)) continue;
    initializedDialogs.add(dialog);
    dialog.addEventListener("cancel", (event) => {
      event.preventDefault();
      closeDialog(dialog);
    });
    dialog.addEventListener("close", () => {
      consumeDialogURL(dialog);
      dialogOpeners.get(dialog)?.focus();
    });
  }

  };

  document.addEventListener("click", (event) => {
    for (const menu of document.querySelectorAll(".account-menu[open]")) {
      if (!menu.contains(event.target)) menu.removeAttribute("open");
    }
    if (event.target.matches("dialog.drawer")) {
      closeDialog(event.target);
      return;
    }

    const opener = event.target.closest("[data-dialog-open]");
    if (opener) {
      const dialog = document.getElementById(opener.dataset.dialogOpen);
      const form = dialog?.querySelector("form");
      if (!dialog || !form) return;
      dialogOpeners.set(dialog, opener);
      resetForm(form);
      form.dataset.dirty = "false";
      const mediaForm = dialog.querySelector("[data-model-media-form]");
      if (mediaForm) {
        resetForm(mediaForm);
        mediaForm.dataset.dirty = "false";
      }
      if (opener.dataset.action) form.action = opener.dataset.action;
      const title = dialog.querySelector("[data-dialog-title]");
      if (title) title.textContent = opener.dataset.title;
      for (const [dataKey, fieldName] of Object.entries(fields)) {
        if (form.hasAttribute?.("data-drawer-step")) continue;
        if (fieldName === "event_type" && !opener.dataset[dataKey]) continue;
        const field = form.elements.namedItem(fieldName);
        if (field) field.value = opener.dataset[dataKey] || "";
      }
      if (dialog.id === "model-drawer") {
        const modelId = opener.dataset.editModelId || "";
        const image = dialog.querySelector("[data-model-image]");
        if (image) {
          image.hidden = !modelId;
          image.querySelector("[data-model-image-link]").href = `/admin/catalog/models/${modelId}/image`;
        }
        const library = dialog.querySelector("[data-model-library]");
        if (library) {
          library.hidden = !modelId;
          library.querySelector("[data-model-library-link]").href = `/admin/catalog/models/${modelId}/binding`;
        }
        const tags = dialog.querySelector("[data-model-tags]");
        if (tags) tags.hidden = !modelId;
        for (const group of dialog.querySelectorAll("[data-model-tag-group]")) {
          group.hidden = group.dataset.modelTagGroup !== modelId;
          if (group.tagName === "FIELDSET") group.disabled = group.hidden;
        }
        if (mediaForm) {
          mediaForm.hidden = !modelId;
          if (modelId) {
            mediaForm.action = `/admin/catalog/models/${modelId}/3d`;
            for (const [dataKey, fieldName] of Object.entries(fields)) {
              const field = mediaForm.elements.namedItem(fieldName);
              if (field) field.value = opener.dataset[dataKey] || "";
            }
            const state = mediaForm.querySelector("[data-model-media-state]");
            if (state) state.textContent = opener.dataset.hasModel3d ? mediaForm.dataset.boundLabel : mediaForm.dataset.emptyLabel;
          }
        }
      }
      const currency = form.querySelector("[data-currency-select]");
      if (currency) syncFXFields(currency);
      const eventType = form.querySelector("[data-event-type-select]");
      if (eventType) syncEventTypeFields(eventType);
      rememberDialogForms(dialog);
      dialog.showModal();
      window.assetloopDrawers?.opened(dialog);
      queueMicrotask(() => focusDialog(dialog));
      return;
    }

    const closer = event.target.closest("[data-dialog-close]");
    if (closer) {
      event.preventDefault();
      closeDialog(closer.closest("dialog"));
    }
  });

  document.addEventListener("keydown", (event) => {
    if (event.key !== "Escape") return;
    for (const menu of document.querySelectorAll(".account-menu[open]")) menu.removeAttribute("open");
  });

  document.addEventListener("input", (event) => {
    if (event.target.matches('[data-event-type-select]') && event.target.selectedOptions[0]?.hasAttribute('data-event-type-create')) return;
    if (event.target.closest("[data-transfer-ui]")) return;
    const form = event.target.closest("form[data-guard-dirty]");
    updateDirty(form);
    if (form && event.target.matches("[name='amount'], [name='fx_rate']")) syncFXPreview(form);
  });

  document.addEventListener("change", (event) => {
    if (event.target.closest("[data-transfer-ui]")) return;
    const eventType = event.target.closest("[data-event-type-select]");
    if (eventType && chooseEventType(eventType)) return;
    const dirty = event.target.closest("form[data-guard-dirty]");
    updateDirty(dirty);
    const currency = event.target.closest("[data-currency-select]");
    if (currency) syncFXFields(currency);
    if (eventType) syncEventTypeFields(eventType);
    updateDirty(dirty);
    const autoSubmit = event.target.closest("[data-auto-submit]");
    if (!autoSubmit) return;
    const form = autoSubmit.matches("form") ? autoSubmit : autoSubmit.form;
    if (!form) return;
    if (event.target.name === "theme") document.documentElement.dataset.theme = event.target.value;
    if (event.target.name === "accent") document.documentElement.dataset.accent = event.target.value;
    form.requestSubmit();
  });

  document.addEventListener("submit", async (event) => {
    if (event.defaultPrevented) return;
    const form = event.target;
    if (form.hasAttribute('data-shared-name')) {
      const name = form.elements.namedItem('name').value.trim();
      const confirmation = form.elements.namedItem('confirm_rename');
      if (name !== form.dataset.sharedName && confirmation.value !== '1') {
        event.preventDefault();
        if (form.dataset.confirming === 'true') return;
        form.dataset.confirming = 'true';
        const accepted = await confirmDiscard(form.dataset.renameMessage, true);
        delete form.dataset.confirming;
        if (accepted && name === form.elements.namedItem('name').value.trim()) {
          confirmation.value = '1';
          form.requestSubmit(event.submitter);
        }
        return;
      }
    }
    const message = form.dataset.confirm;
    if (message && !form.dataset.confirmAccepted) {
      event.preventDefault();
      if(form.dataset.confirming === 'true') return;
      form.dataset.confirming='true';
      const accepted=await confirmDiscard(message);
      delete form.dataset.confirming;
      if(accepted){form.dataset.confirmAccepted='true';form.requestSubmit(event.submitter);}
      return;
    }
    delete form.dataset.confirmAccepted;
    if (window.assetloopDrawers?.submit(event)) return;
    if (form.dataset.submitting === "true") {
      event.preventDefault();
      return;
    }
    form.dataset.submitting = "true";
    form.dataset.dirty = "false";
    if (event.submitter) {
      // Keep the submitter successful so native POST includes its name/value.
      // The form-level guard above already rejects repeated submissions.
      event.submitter.setAttribute("aria-disabled", "true");
      event.submitter.setAttribute("aria-busy", "true");
    }
  });

  window.addEventListener("beforeunload", (event) => {
    if (leavingPage) return;
    if (!document.querySelector("form[data-guard-dirty][data-dirty='true']")) return;
    event.preventDefault();
    event.returnValue = "";
  });

  const initPage = () => {
  initDialogs();
  for (const field of document.querySelectorAll('form[data-shared-name] input[name="confirm_rename"]')) field.disabled = false;
  const resourceEditor = document.querySelector('dialog[data-resource-editor]');
  if (resourceEditor && !resourceEditor.open) {
    rememberDialogForms(resourceEditor);
    resourceEditor.showModal();
    queueMicrotask(() => focusDialog(resourceEditor));
  }
  for (const select of document.querySelectorAll("[data-currency-select]")) syncFXFields(select);
  for (const select of document.querySelectorAll("[data-event-type-select]")) syncEventTypeFields(select);
  for (const form of pageForms()) {
    if (!formBaselines.has(form)) {
      formBaselines.set(form, formValues(form));
      form.dataset.dirty = 'false';
    }
  }

  const params = new URLSearchParams(window.location.search);
  const initialDialog = params.get("dialog");
  const initialModelId = params.get("edit_model_id");
    let opener = document.querySelector("[data-dialog-initial-open]") || [...document.querySelectorAll("[data-dialog-open]")].find(
    (candidate) => candidate.dataset.dialogOpen === initialDialog
      && (!initialModelId || candidate.dataset.editModelId === initialModelId),
  );
  if (!opener && (window.location.hash === "#add-event" || document.querySelector("#event-drawer .error"))) {
    opener = document.querySelector('[data-dialog-open="event-drawer"]');
  }
  if (opener) {
    for (const [dataKey, fieldName] of Object.entries(fields)) {
      const value = params.get(fieldName);
      if (value) opener.dataset[dataKey] = fieldName === "event_type" && initialDialog === "event-drawer"
        ? document.querySelector('#event-type-select')?.value || value : value;
    }
    opener.click();
    consumeDialogURL(document.getElementById(opener.dataset.dialogOpen));
  } else {
    const erroredDialog = [...document.querySelectorAll("dialog.drawer")].find((dialog) => dialog.querySelector("[data-error-summary]"));
    if (erroredDialog) {
      rememberDialogForms(erroredDialog);
      erroredDialog.showModal();
      queueMicrotask(() => focusDialog(erroredDialog));
    } else {
      document.querySelector("[data-error-summary]")?.focus();
    }
  }
  };
  document.addEventListener("settings:loaded", initPage);
  window.assetloopDialog = {close:closeDialog, confirm:confirmDiscard, initialize(dialog) {
    initDialogs();
    for (const field of dialog.querySelectorAll('input[name="confirm_rename"]')) field.disabled = false;
    rememberDialogForms(dialog);
  }};
  document.addEventListener('input', (event) => {
    const form = event.target.closest('form[data-shared-name]');
    if (form && event.target.name === 'name') form.elements.namedItem('confirm_rename').value = '';
  });
  initPage();
})();
