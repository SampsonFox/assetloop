(() => {
  const fields = {
    name: "name",
    iconKey: "icon_key",
    categoryId: "category_id",
    modelId: "model_id",
    returnModelId: "return_model_id",
    variantId: "variant_id",
    displayName: "display_name",
    serialNumber: "serial_number",
    color: "color",
    purchaseChannel: "purchase_channel",
    notes: "notes",
  };

  const syncFXFields = (select) => {
    const foreign = select.value !== select.dataset.baseCurrency;
    for (const field of select.form.querySelectorAll("[data-fx-field]")) field.hidden = !foreign;
    for (const input of select.form.querySelectorAll("[data-fx-required]")) input.required = foreign;
  };

  document.addEventListener("click", (event) => {
	for (const menu of document.querySelectorAll(".account-menu[open]")) {
	  if (!menu.contains(event.target)) menu.removeAttribute("open");
	}
    const opener = event.target.closest("[data-dialog-open]");
    if (opener) {
      const dialog = document.getElementById(opener.dataset.dialogOpen);
      const form = dialog && dialog.querySelector("form");
      if (!dialog || !form) return;
      const currentDialog = opener.closest("dialog");
      if (currentDialog?.open && currentDialog !== dialog) currentDialog.close();
      form.reset();
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
      dialog.showModal();
      return;
    }

    const closer = event.target.closest("[data-dialog-close]");
    if (closer) closer.closest("dialog")?.close();
  });

  document.addEventListener("keydown", (event) => {
	if (event.key !== "Escape") return;
	for (const menu of document.querySelectorAll(".account-menu[open]")) menu.removeAttribute("open");
  });

  document.addEventListener("change", (event) => {
    const currency = event.target.closest("[data-currency-select]");
    if (currency) syncFXFields(currency);
    const form = event.target.closest("[data-auto-submit]");
    if (!form) return;
    if (event.target.name === "theme") document.documentElement.dataset.theme = event.target.value;
    form.requestSubmit();
  });

  document.addEventListener("submit", (event) => {
    const message = event.target.dataset.confirm;
    if (message && !window.confirm(message)) event.preventDefault();
  });

  for (const select of document.querySelectorAll("[data-currency-select]")) syncFXFields(select);

  const params = new URLSearchParams(window.location.search);
  const initialDialog = params.get("dialog");
  const initialModelId = params.get("edit_model_id");
  const opener = [...document.querySelectorAll("[data-dialog-open]")].find(
    (candidate) => candidate.dataset.dialogOpen === initialDialog
      && (!initialModelId || candidate.dataset.editModelId === initialModelId),
  );
  if (opener) {
    for (const [dataKey, fieldName] of Object.entries(fields)) {
      const value = params.get(fieldName);
      if (value) opener.dataset[dataKey] = value;
    }
    opener.click();
  }
})();
