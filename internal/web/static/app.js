(() => {
  const fields = {
    name: "name",
    iconKey: "icon_key",
    categoryId: "category_id",
    modelId: "model_id",
    returnModelId: "return_model_id",
    eventType: "event_type",
  };
  const dialogOpeners = new WeakMap();

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

  const syncEventTypeFields = (select) => {
    const neutral = select.selectedOptions[0]?.dataset.cashflow === "neutral";
    const amount = select.form.elements.namedItem("amount");
    for (const field of select.form.querySelectorAll("[data-money-field]")) field.hidden = neutral;
    if (amount) {
      if (neutral) {
        if (amount.value !== "0") amount.dataset.previousValue = amount.value;
        amount.value = "0";
        amount.required = false;
      } else {
        if (amount.value === "0") amount.value = amount.dataset.previousValue || "";
        amount.required = true;
      }
    }
    const currency = select.form.querySelector("[data-currency-select]");
    if (currency) syncFXFields(currency);
  };

  const dirtyForm = (dialog) => dialog?.querySelector("form[data-guard-dirty][data-dirty='true']");
  const canDiscardDialog = (dialog) => {
    const form = dirtyForm(dialog);
    return !form || window.confirm(form.dataset.discardConfirm);
  };
  const closeDialog = (dialog) => {
    if (!dialog || !canDiscardDialog(dialog)) return false;
    const form = dialog.querySelector("form[data-guard-dirty]");
    if (form) form.dataset.dirty = "false";
    dialog.close();
    return true;
  };
  const focusDialog = (dialog) => {
    const target = dialog.querySelector("[data-error-summary]")
      || dialog.querySelector("[data-dialog-initial-focus]")
      || dialog.querySelector("input:not([type='hidden']), select, textarea, button");
    target?.focus();
  };

  for (const dialog of document.querySelectorAll("dialog.drawer")) {
    dialog.addEventListener("cancel", (event) => {
      if (!canDiscardDialog(dialog)) {
        event.preventDefault();
        return;
      }
      const form = dialog.querySelector("form[data-guard-dirty]");
      if (form) form.dataset.dirty = "false";
    });
    dialog.addEventListener("close", () => dialogOpeners.get(dialog)?.focus());
  }

  document.addEventListener("click", (event) => {
    for (const menu of document.querySelectorAll(".account-menu[open]")) {
      if (!menu.contains(event.target)) menu.removeAttribute("open");
    }
    if (event.target.matches("dialog.drawer")) {
      if (canDiscardDialog(event.target)) {
        const form = event.target.querySelector("form[data-guard-dirty]");
        if (form) form.dataset.dirty = "false";
        event.target.close();
      }
      return;
    }

    const opener = event.target.closest("[data-dialog-open]");
    if (opener) {
      const dialog = document.getElementById(opener.dataset.dialogOpen);
      const form = dialog?.querySelector("form");
      if (!dialog || !form) return;
      dialogOpeners.set(dialog, opener);
      form.reset();
      form.dataset.dirty = "false";
      form.action = opener.dataset.action;
      const title = dialog.querySelector("[data-dialog-title]");
      if (title) title.textContent = opener.dataset.title;
      for (const [dataKey, fieldName] of Object.entries(fields)) {
        const field = form.elements.namedItem(fieldName);
        if (field) field.value = opener.dataset[dataKey] || "";
      }
      if (dialog.id === "model-drawer") {
        const modelId = opener.dataset.editModelId || "";
        const manager = dialog.querySelector("[data-model-variants]");
        if (manager) manager.hidden = !modelId;
        for (const group of dialog.querySelectorAll("[data-variant-group]")) {
          group.hidden = group.dataset.variantGroup !== modelId;
        }
      }
      const currency = form.querySelector("[data-currency-select]");
      if (currency) syncFXFields(currency);
      const eventType = form.querySelector("[data-event-type-select]");
      if (eventType) syncEventTypeFields(eventType);
      dialog.showModal();
      queueMicrotask(() => focusDialog(dialog));
      return;
    }

    const closer = event.target.closest("[data-dialog-close]");
    if (closer) closeDialog(closer.closest("dialog"));
  });

  document.addEventListener("keydown", (event) => {
    if (event.key !== "Escape") return;
    for (const menu of document.querySelectorAll(".account-menu[open]")) menu.removeAttribute("open");
  });

  document.addEventListener("input", (event) => {
    const form = event.target.closest("form[data-guard-dirty]");
    if (form) form.dataset.dirty = "true";
    if (form && event.target.matches("[name='amount'], [name='fx_rate']")) syncFXPreview(form);
  });

  document.addEventListener("change", (event) => {
    const dirty = event.target.closest("form[data-guard-dirty]");
    if (dirty) dirty.dataset.dirty = "true";
    const currency = event.target.closest("[data-currency-select]");
    if (currency) syncFXFields(currency);
    const eventType = event.target.closest("[data-event-type-select]");
    if (eventType) syncEventTypeFields(eventType);
    const autoSubmit = event.target.closest("[data-auto-submit]");
    if (!autoSubmit) return;
    const form = autoSubmit.matches("form") ? autoSubmit : autoSubmit.form;
    if (!form) return;
    if (event.target.name === "theme") document.documentElement.dataset.theme = event.target.value;
    form.requestSubmit();
  });

  document.addEventListener("submit", (event) => {
    const form = event.target;
    const message = form.dataset.confirm;
    if (message && !window.confirm(message)) {
      event.preventDefault();
      return;
    }
    if (form.dataset.submitting === "true") {
      event.preventDefault();
      return;
    }
    form.dataset.submitting = "true";
    form.dataset.dirty = "false";
    if (event.submitter) {
      event.submitter.disabled = true;
      event.submitter.setAttribute("aria-busy", "true");
    }
  });

  window.addEventListener("beforeunload", (event) => {
    if (!document.querySelector("form[data-guard-dirty][data-dirty='true']")) return;
    event.preventDefault();
    event.returnValue = "";
  });

  for (const select of document.querySelectorAll("[data-currency-select]")) syncFXFields(select);
  for (const select of document.querySelectorAll("[data-event-type-select]")) syncEventTypeFields(select);

  const params = new URLSearchParams(window.location.search);
  const initialDialog = params.get("dialog");
  const initialModelId = params.get("edit_model_id");
  let opener = [...document.querySelectorAll("[data-dialog-open]")].find(
    (candidate) => candidate.dataset.dialogOpen === initialDialog
      && (!initialModelId || candidate.dataset.editModelId === initialModelId),
  );
  if (!opener && (window.location.hash === "#add-event" || document.querySelector("#event-drawer .error"))) {
    opener = document.querySelector('[data-dialog-open="event-drawer"]');
  }
  if (opener) {
    for (const [dataKey, fieldName] of Object.entries(fields)) {
      const value = params.get(fieldName);
      if (value) opener.dataset[dataKey] = value;
    }
    opener.click();
  } else {
    const erroredDialog = [...document.querySelectorAll("dialog.drawer")].find((dialog) => dialog.querySelector("[data-error-summary]"));
    if (erroredDialog) {
      erroredDialog.showModal();
      queueMicrotask(() => focusDialog(erroredDialog));
    } else {
      document.querySelector("[data-error-summary]")?.focus();
    }
  }
})();
