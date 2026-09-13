package web

import (
	"net/http"

	"github.com/SampsonFox/assetloop/internal/application"
)

// saveDrawerAsset serves both POST /assets and POST /assets/{id}. Normal
// submissions retain their redirect; negotiated drawers return the saved entity.
func (s *Server) saveDrawerAsset(w http.ResponseWriter, r *http.Request) {
	if !s.verifyCSRF(w, r) {
		return
	}
	actor, ok := s.requirePrincipal(w, r)
	if !ok {
		return
	}
	if !actor.Can(application.CapabilityManageAssets) {
		s.renderForbidden(w, actor, "error.forbidden_asset")
		return
	}
	id := r.PathValue("id")
	if err := r.ParseForm(); err != nil {
		s.renderError(w, r, http.StatusBadRequest, err)
		return
	}
	if r.PostForm.Has("variant_id") {
		s.renderAssetMutationError(w, r, actor, assetFromForm(r, id), application.NewInputError("validation.specification_retired"))
		return
	}
	cmd := application.SaveSpecificationAsset{ID: id, ModelID: r.PostForm.Get("model_id"), DisplayName: r.PostForm.Get("display_name"), SerialNumber: r.PostForm.Get("serial_number"), PurchaseChannel: r.PostForm.Get("purchase_channel"), Notes: r.PostForm.Get("notes")}
	if r.PostForm.Has("market_item_id") {
		value := r.PostForm.Get("market_item_id")
		cmd.MarketItemID = &value
	}
	if r.PostForm.Has("resource_id") {
		value := r.PostForm.Get("resource_id")
		cmd.ResourceID = &value
	}
	if values, present := r.PostForm["tag_ids"]; present {
		cmd.TagIDs = nonemptyTagIDs(values)
	}
	asset, err := s.options.Specifications.SaveAsset(r.Context(), actor, cmd)
	if err != nil {
		s.renderAssetMutationError(w, r, actor, assetFromForm(r, id), err)
		return
	}
	if drawerSaved(w, "asset", asset.ID, assetTitle(asset), true) {
		return
	}
	http.Redirect(w, r, "/assets/"+asset.ID, http.StatusSeeOther)
}
