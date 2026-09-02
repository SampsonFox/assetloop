(() => {
  const fields = {
    name: "name",
    iconKey: "icon_key",
    categoryId: "category_id",
    modelId: "model_id",
    variantId: "variant_id",
    displayName: "display_name",
    serialNumber: "serial_number",
    color: "color",
    purchaseChannel: "purchase_channel",
    notes: "notes",
  };

  document.addEventListener("click", (event) => {
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
      dialog.showModal();
      return;
    }

    const closer = event.target.closest("[data-dialog-close]");
    if (closer) closer.closest("dialog")?.close();
  });

  const params = new URLSearchParams(window.location.search);
  const initialDialog = params.get("dialog");
  const opener = [...document.querySelectorAll("[data-dialog-open]")].find(
    (candidate) => candidate.dataset.dialogOpen === initialDialog,
  );
  if (opener) {
    for (const [dataKey, fieldName] of Object.entries(fields)) {
      const value = params.get(fieldName);
      if (value) opener.dataset[dataKey] = value;
    }
    opener.click();
  }
})();
