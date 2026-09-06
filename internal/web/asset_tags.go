package web

import (
	"context"
	"net/http"

	"github.com/SampsonFox/assetloop/internal/application"
	"github.com/SampsonFox/assetloop/internal/domain"
)

func (s *Server) getAsset(ctx context.Context, actor application.Principal, id string) (domain.Asset, error) {
	if s.options.Specifications != nil {
		return s.options.Specifications.Asset(ctx, actor, id)
	}
	return s.catalog.GetAsset(ctx, actor, id)
}

func (s *Server) saveTaggedAsset(w http.ResponseWriter, r *http.Request, actor application.Principal, id string) {
	_ = r.ParseForm()
	if r.PostForm.Has("variant_id") {
		s.renderAssetMutationError(w, r, actor, assetFromForm(r, id), application.NewInputError("validation.specification_retired"))
		return
	}
	cmd := application.SaveSpecificationAsset{ID: id, ModelID: r.PostForm.Get("model_id"), DisplayName: r.PostForm.Get("display_name"), SerialNumber: r.PostForm.Get("serial_number"), PurchaseChannel: r.PostForm.Get("purchase_channel"), Notes: r.PostForm.Get("notes")}
	if values, present := r.PostForm["tag_ids"]; present {
		cmd.TagIDs = []string{}
		for _, value := range values {
			if value != "" {
				cmd.TagIDs = append(cmd.TagIDs, value)
			}
		}
	}
	asset, err := s.options.Specifications.SaveAsset(r.Context(), actor, cmd)
	if err != nil {
		s.renderAssetMutationError(w, r, actor, assetFromForm(r, id), err)
		return
	}
	http.Redirect(w, r, "/assets/"+asset.ID, http.StatusSeeOther)
}

func assetTagEditors(state application.SpecificationSnapshot, tenant string, models []domain.ProductModel, asset domain.Asset) []modelTagEditor {
	var result []modelTagEditor
	selected := map[string]bool{}
	for _, tag := range asset.Tags {
		selected[tag.ID] = true
	}
	for _, model := range models {
		definition := state.Model(tenant, model.ID)
		allowed := map[string]bool{}
		for _, id := range definition.AllowedTagIDs {
			allowed[id] = true
		}
		editor := modelTagEditor{ModelID: model.ID}
		for _, kind := range state.Types {
			dim := tagDimension{ID: kind.ID, Name: kind.Name, Multiple: kind.Multiple, Appearance: kind.AffectsAppearance}
			for _, tag := range state.Tags {
				if tag.TypeID != kind.ID || !allowed[tag.ID] {
					continue
				}
				enabled := tag.Enabled && kind.Enabled
				retained := selected[tag.ID] && model.ID == asset.ModelID
				if enabled || retained {
					dim.Choices = append(dim.Choices, tagChoice{ID: tag.ID, Name: tag.Name, Selected: retained, Enabled: enabled})
				}
			}
			if len(dim.Choices) > 0 {
				editor.Dimensions = append(editor.Dimensions, dim)
			}
		}
		result = append(result, editor)
	}
	return result
}
