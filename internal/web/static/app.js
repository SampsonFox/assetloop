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
	model3dSourceUrl: "model_3d_source_url",
	model3dAuthor: "model_3d_author",
	model3dLicense: "model_3d_license",
  };

  const resetForm = (form) => {
    const hidden = [...form.querySelectorAll('input[type="hidden"]')].map((input) => [input, input.value]);
    form.reset();
    for (const [input, value] of hidden) input.value = value;
  };

  document.addEventListener("click", (event) => {
    const opener = event.target.closest("[data-dialog-open]");
    if (opener) {
      const dialog = document.getElementById(opener.dataset.dialogOpen);
	  const form = dialog && dialog.querySelector("form");
      if (!dialog || !form) return;
      const mediaForm = dialog.querySelector("[data-model-media-form]");
      resetForm(form);
      if (mediaForm) resetForm(mediaForm);
      form.action = opener.dataset.action;
      const title = dialog.querySelector("[data-dialog-title]");
      if (title) title.textContent = opener.dataset.title;
      for (const [dataKey, fieldName] of Object.entries(fields)) {
        const field = form.elements.namedItem(fieldName);
        if (field) field.value = opener.dataset[dataKey] || "";
      }
	  if (mediaForm) {
		const editing = Boolean(opener.dataset.modelId);
		mediaForm.hidden = !editing;
		if (editing) {
		  mediaForm.action = `/admin/catalog/models/${opener.dataset.modelId}/3d`;
		  for (const [dataKey, fieldName] of Object.entries(fields)) {
			const field = mediaForm.elements.namedItem(fieldName);
			if (field) field.value = opener.dataset[dataKey] || "";
		  }
		  const state = mediaForm.querySelector("[data-model-media-state]");
		  if (state) state.textContent = opener.dataset.hasModel3d ? "当前已绑定 GLB" : "尚未上传 GLB";
		}
	  }
      dialog.showModal();
      return;
    }

    const closer = event.target.closest("[data-dialog-close]");
    if (closer) closer.closest("dialog")?.close();
  });
})();
