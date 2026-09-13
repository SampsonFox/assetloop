package web

import (
	"context"
	"database/sql"
	"errors"
	"net/http"
	"net/url"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/SampsonFox/assetloop/internal/application"
	"github.com/SampsonFox/assetloop/internal/domain"
)

// Trade-in Web transport. Every mutation goes through the shared application
// service: this file only parses the form, builds the documented command and
// renders the shared result. Direction semantics are the application contract:
// "source" means the current asset is the NEW asset carrying the purchase, and
// "destination" means the current asset is the OLD asset carrying the sale.
const (
	tradeInDirectionSource      = "source"
	tradeInDirectionDestination = "destination"

	tradeInModeSelect = "select"
	tradeInModeReview = "review"
	tradeInModeLink   = "link"

	tradeInCandidatesPerPage = 10
	tradeInChoicePageSize    = 200
)

type tradeInMoneyForm struct {
	Amount       string
	Currency     string
	OccurredAt   string
	FXRate       string
	FXRateDate   string
	FXRateSource string
	Reference    string
	Notes        string
}

// tradeInEconomicView is one asset's economic side of the command. A side whose
// record already exists is shown read-only with its original evidence; a side
// that still needs a record exposes the money fields. Stale marks an explicitly
// submitted record that no longer qualifies: the submission is kept verbatim and
// the group offers reselection instead of recording new money.
type tradeInEconomicView struct {
	AssetID    string
	AssetName  string
	AssetSpec  string
	Role       string
	RoleLabel  string
	Selection  string
	ExistingID string
	Event      *domain.AssetEvent
	Editing    bool
	Stale      bool
	Form       tradeInMoneyForm
}

// tradeInPairView is one relation that will be created, shown in the review
// summary. A zero-value neutral relation never appears as money here.
type tradeInPairView struct {
	NewAssetID   string
	NewAssetName string
	NewAssetSpec string
	OldAssetID   string
	OldAssetName string
	OldAssetSpec string
	NewSelection string
	OldSelection string
}

type tradeInCandidate struct {
	AssetID  string
	Name     string
	Spec     string
	Status   string
	Selected bool
}

// tradeInAction is the expected-event pair of one rendered relation. The
// dedicated correction and cancellation actions carry both expected event IDs so
// a stale page can never rewrite a different pair.
type tradeInAction struct {
	LinkID             string
	SourceEventID      string
	DestinationEventID string
	Editable           bool
}

type tradeInLinkEdit struct {
	LinkID             string
	SourceEventID      string
	DestinationEventID string
	Direction          string
	DirectionLabel     string
	NewAssetID         string
	NewAssetName       string
	NewAssetSpec       string
	OldAssetID         string
	OldAssetName       string
	OldAssetSpec       string
}

type tradeInPageData struct {
	Mode             string
	Direction        string
	DirectionLabel   string
	SourceLabel      string
	DestinationLabel string
	CurrentAssetID   string
	CurrentName      string
	CurrentSpec      string
	Candidates       []tradeInCandidate
	Query            string
	Page             int
	PreviousPage     int
	NextPage         int
	SelectedIDs      []string
	SelectionCount   int
	SelectionLabel   string
	Choices          []tradeInCandidate
	Links            []tradeInPairView
	Economics        []tradeInEconomicView
	PreviewReady     bool
	AssociationDate  string
	AssociationRef   string
	AssociationNotes string
	RequestKey       string
	LinkEdit         *tradeInLinkEdit
}

// tradeInAttempt is the submitted state of one trade-in command. It is retained
// across validation errors so a rejected command re-renders the same selection
// and the same edited economic values.
type tradeInAttempt struct {
	Direction  string
	Query      string
	Page       int
	Selected   []string
	Money      map[string]tradeInMoneyForm
	Existing   map[string]string
	Date       string
	Reference  string
	Notes      string
	RequestKey string
	// Initial marks the selection->review transition performed by the preview
	// endpoint. Only that transition resolves an implicit effective record and
	// defaults a blank association date or money group; a rejected final POST
	// always re-renders exactly what was submitted, blanks included.
	Initial bool
}

func normalizeTradeInDirection(value string) string {
	if strings.TrimSpace(value) == tradeInDirectionDestination {
		return tradeInDirectionDestination
	}
	return tradeInDirectionSource
}

func (s *Server) requireManagement(w http.ResponseWriter, r *http.Request) (*application.ManagementService, bool) {
	if s.options.Management == nil {
		s.renderError(w, r, http.StatusServiceUnavailable, errors.New("trade-in service is not configured"))
		return nil, false
	}
	return s.options.Management, true
}

func (s *Server) tradeInDirectionLabel(locale application.Locale, direction string) string {
	if direction == tradeInDirectionDestination {
		return textFor(locale, "trade_in.destination")
	}
	return textFor(locale, "trade_in.source")
}

// tradeInRole returns the economic side the asset plays for one direction.
func tradeInRole(direction, assetID, currentAssetID string) string {
	currentIsNew := direction == tradeInDirectionSource
	if assetID == currentAssetID {
		if currentIsNew {
			return "new"
		}
		return "old"
	}
	if currentIsNew {
		return "old"
	}
	return "new"
}

func tradeInKind(role string) domain.AssetEventType {
	if role == "new" {
		return domain.AssetEventPurchase
	}
	return domain.AssetEventSale
}

func (s *Server) tradeInRoleLabel(locale application.Locale, role string) string {
	if role == "new" {
		return textFor(locale, "trade_in.role_new")
	}
	return textFor(locale, "trade_in.role_old")
}

func (s *Server) tradeInSelectionLabel(locale application.Locale, selection string) string {
	switch selection {
	case application.TradeInSelectionReuse, application.TradeInSelectionLinked:
		return textFor(locale, "trade_in.reuse")
	case application.TradeInSelectionCreate:
		return textFor(locale, "trade_in.create")
	case application.TradeInSelectionMissing:
		return textFor(locale, "trade_in.missing")
	default:
		return textFor(locale, "trade_in.unknown")
	}
}

// tradeInForm renders the selection step. The direction names which role the
// current asset plays; it is never inferred from an event order or a label.
func (s *Server) tradeInForm(w http.ResponseWriter, r *http.Request) {
	principal, ok := s.requirePrincipal(w, r)
	if !ok {
		return
	}
	if !principal.Can(application.CapabilityManageLifecycle) {
		s.renderForbidden(w, principal, "error.forbidden_lifecycle")
		return
	}
	if _, ok := s.requireManagement(w, r); !ok {
		return
	}
	current, err := s.getAsset(r.Context(), principal, r.PathValue("id"))
	if err != nil {
		s.renderNotFound(w, principal, "error.not_found_asset")
		return
	}
	attempt := tradeInAttempt{
		Direction: normalizeTradeInDirection(r.URL.Query().Get("direction")),
		Query:     strings.TrimSpace(r.URL.Query().Get("q")),
		Page:      queryPage(r),
		Selected:  tradeInQuerySelection(r.URL.Query()),
	}
	s.renderTradeInSelect(w, r, http.StatusOK, principal, current, attempt, "")
}

func tradeInQuerySelection(query url.Values) []string {
	values := url.Values{"selected": query["selected"], "retain": query["retain"]}
	return tradeInSelection(nil, values)
}

// tradeInSelection merges the checked page rows with the retained selection. The
// rows the user actually saw are supplied by the submitted form, never by
// re-running a changed query: a row that was displayed and is now unchecked must
// be dropped, while a row that was not displayed keeps its retained selection.
func tradeInSelection(displayed []string, submitted url.Values) []string {
	visible := make(map[string]bool, len(displayed))
	for _, id := range displayed {
		visible[strings.TrimSpace(id)] = true
	}
	selected := make([]string, 0, len(submitted["selected"])+len(submitted["retain"]))
	seen := map[string]bool{}
	add := func(id string) {
		id = strings.TrimSpace(id)
		if id == "" || seen[id] {
			return
		}
		seen[id] = true
		selected = append(selected, id)
	}
	for _, id := range submitted["selected"] {
		add(id)
	}
	for _, id := range submitted["retain"] {
		if visible[strings.TrimSpace(id)] {
			continue
		}
		add(id)
	}
	sort.Strings(selected)
	return selected
}

func (s *Server) renderTradeInSelect(w http.ResponseWriter, r *http.Request, status int, principal application.Principal, current domain.Asset, attempt tradeInAttempt, message string) {
	if attempt.Page < 1 {
		attempt.Page = 1
	}
	candidates, total, err := s.tradeInCandidatePage(r.Context(), principal, current.ID, attempt.Query, attempt.Page)
	if err != nil {
		s.renderError(w, r, http.StatusInternalServerError, err)
		return
	}
	selected := attempt.Selected
	selectedSet := map[string]bool{}
	for _, id := range selected {
		selectedSet[id] = true
	}
	for index := range candidates {
		candidates[index].Selected = selectedSet[candidates[index].AssetID]
	}
	// The candidate count is paged over the tenant's assets; the current asset is
	// removed from display, so its page slot simply stays empty.
	totalPages := 0
	if total > 0 {
		totalPages = (total + tradeInCandidatesPerPage - 1) / tradeInCandidatesPerPage
	}
	if attempt.Page > 1 && totalPages > 0 && attempt.Page > totalPages {
		attempt.Page = totalPages
	}
	previousPage, nextPage := 0, 0
	if attempt.Page > 1 {
		previousPage = attempt.Page - 1
	}
	if attempt.Page < totalPages {
		nextPage = attempt.Page + 1
	}
	data := tradeInPageData{
		Mode:             tradeInModeSelect,
		Direction:        attempt.Direction,
		DirectionLabel:   s.tradeInDirectionLabel(principal.Locale, attempt.Direction),
		SourceLabel:      textFor(principal.Locale, "trade_in.source_short"),
		DestinationLabel: textFor(principal.Locale, "trade_in.destination_short"),
		CurrentAssetID:   current.ID,
		CurrentName:      assetTitle(current),
		CurrentSpec:      current.TagSummary,
		Candidates:       candidates,
		Query:            attempt.Query,
		Page:             attempt.Page,
		PreviousPage:     previousPage,
		NextPage:         nextPage,
		SelectedIDs:      selected,
		SelectionCount:   len(selected),
		SelectionLabel:   textFor(principal.Locale, "trade_in.count", len(selected)),
		RequestKey:       randomToken(),
		AssociationDate:  attempt.Date,
	}
	if data.AssociationDate == "" {
		data.AssociationDate = time.Now().Local().Format("2006-01-02T15:04")
	}
	s.renderTradeIn(w, status, principal, current, data, message, r)
}

func (s *Server) renderTradeIn(w http.ResponseWriter, status int, principal application.Principal, current domain.Asset, data tradeInPageData, message string, r *http.Request) {
	s.render(w, status, "trade_in", pageData{
		Title:     textFor(principal.Locale, "trade_in.heading"),
		CSRFToken: s.ensureCSRF(w, r),
		Principal: &principal,
		Error:     message,
		ReturnTo:  "/assets/" + current.ID,
		Asset:     &current,
		BaseCurrency: func() string {
			currency, _, err := s.lifecycle.BaseCurrency(r.Context(), principal)
			if err != nil {
				return ""
			}
			return currency
		}(),
		TradeIn: data,
	})
}

// tradeInCandidatePage reads one page of selectable counterpart assets and
// hydrates their specification summary through the existing asset list service.
func (s *Server) tradeInCandidatePage(ctx context.Context, principal application.Principal, currentID, query string, page int) ([]tradeInCandidate, int, error) {
	result, err := s.catalog.ListAssetsWithSummary(ctx, principal, application.AssetListOptions{
		Query: query, Sort: "name", Direction: "asc", Page: page, PageSize: tradeInCandidatesPerPage,
	})
	if err != nil {
		return nil, 0, err
	}
	assets := make([]domain.Asset, 0, len(result.Assets))
	statuses := make(map[string]string, len(result.Assets))
	for _, row := range result.Assets {
		assets = append(assets, row.Asset)
		statuses[row.Asset.ID] = row.Summary.Status
	}
	assets, err = s.describeTradeInAssets(ctx, principal, assets)
	if err != nil {
		return nil, 0, err
	}
	candidates := make([]tradeInCandidate, 0, len(assets))
	for _, asset := range assets {
		if asset.ID == currentID {
			continue
		}
		candidates = append(candidates, tradeInCandidate{
			AssetID: asset.ID, Name: assetTitle(asset), Spec: asset.TagSummary, Status: statuses[asset.ID],
		})
	}
	return candidates, result.Total, nil
}

// tradeInChoices reads every tenant asset once for the searchable target picker
// of the dedicated link editor.
func (s *Server) tradeInChoices(ctx context.Context, principal application.Principal) ([]tradeInCandidate, error) {
	var assets []domain.Asset
	for page := 1; ; page++ {
		result, err := s.catalog.ListAssetsWithSummary(ctx, principal, application.AssetListOptions{
			Sort: "name", Direction: "asc", Page: page, PageSize: tradeInChoicePageSize,
		})
		if err != nil {
			return nil, err
		}
		for _, row := range result.Assets {
			assets = append(assets, row.Asset)
		}
		if len(result.Assets) == 0 || page*tradeInChoicePageSize >= result.Total {
			break
		}
	}
	assets, err := s.describeTradeInAssets(ctx, principal, assets)
	if err != nil {
		return nil, err
	}
	choices := make([]tradeInCandidate, 0, len(assets))
	for _, asset := range assets {
		choices = append(choices, tradeInCandidate{AssetID: asset.ID, Name: assetTitle(asset), Spec: asset.TagSummary})
	}
	return choices, nil
}

func (s *Server) describeTradeInAssets(ctx context.Context, principal application.Principal, assets []domain.Asset) ([]domain.Asset, error) {
	if s.options.Specifications == nil || len(assets) == 0 {
		return assets, nil
	}
	return s.options.Specifications.DescribeAssets(ctx, principal, assets)
}

// relatedAssetOptions lists the tenant's live assets for the optional neutral
// relation picker of the ordinary create and correct forms. The current asset is
// never offered, so a self relation cannot be assembled in the browser.
func (s *Server) relatedAssetOptions(ctx context.Context, principal application.Principal, excludeID string) ([]relatedAssetOption, error) {
	choices, err := s.tradeInChoices(ctx, principal)
	if err != nil {
		return nil, err
	}
	options := make([]relatedAssetOption, 0, len(choices))
	for _, choice := range choices {
		if choice.AssetID == excludeID {
			continue
		}
		options = append(options, relatedAssetOption{AssetID: choice.AssetID, Label: choice.Name, Spec: choice.Spec})
	}
	return options, nil
}

// tradeInPreview handles both steps of the create flow. Search and review are
// read-only POSTs; only the final commit writes.
func (s *Server) tradeInPreview(w http.ResponseWriter, r *http.Request) {
	if !s.verifyCSRF(w, r) {
		return
	}
	principal, ok := s.requirePrincipal(w, r)
	if !ok {
		return
	}
	if !principal.Can(application.CapabilityManageLifecycle) {
		s.renderForbidden(w, principal, "error.forbidden_lifecycle")
		return
	}
	if _, ok := s.requireManagement(w, r); !ok {
		return
	}
	current, err := s.getAsset(r.Context(), principal, r.PathValue("id"))
	if err != nil {
		s.renderNotFound(w, principal, "error.not_found_asset")
		return
	}
	attempt := s.tradeInAttemptFromForm(r)
	attempt.Direction = normalizeTradeInDirection(r.FormValue("direction"))
	if attempt.RequestKey == "" {
		attempt.RequestKey = randomToken()
	}
	step := strings.TrimSpace(r.FormValue("step"))
	gotoPage := strings.TrimSpace(r.FormValue("goto"))
	// Deselection is decided by the rows the user actually saw: the rendered page
	// submits its own displayed IDs, so changing the search cannot resurrect a row
	// that was unchecked before the new result set was known.
	attempt.Selected = tradeInSelection(r.PostForm["displayed"], r.PostForm)
	if step == "search" || gotoPage != "" {
		if gotoPage != "" {
			if page, err := strconv.Atoi(gotoPage); err == nil && page > 0 {
				attempt.Page = page
			}
		} else {
			attempt.Page = 1
		}
		s.renderTradeInSelect(w, r, http.StatusOK, principal, current, attempt, "")
		return
	}
	attempt.Page = 1
	if len(attempt.Selected) == 0 {
		s.renderTradeInSelect(w, r, http.StatusUnprocessableEntity, principal, current, attempt, textFor(principal.Locale, "trade_in.no_selection"))
		return
	}
	// This request is the selection->review transition, and the only place that
	// resolves what the reviewer left implicit before the shared preview runs.
	// The generic error renderer is never allowed to do it: a refused final POST
	// must re-render its own blanks instead of inventing a record or a date.
	attempt.Initial = true
	attempt, err = s.resolveInitialTradeInReview(r.Context(), principal, current.ID, attempt)
	if err != nil {
		s.renderError(w, r, http.StatusInternalServerError, err)
		return
	}
	s.renderTradeInReview(w, r, http.StatusOK, principal, current, attempt, "")
}

// tradeInAttemptFromForm reads the submitted economic values. It never decides
// business policy: a blank side stays an empty selection and the shared service
// reports what is still required.
func (s *Server) tradeInAttemptFromForm(r *http.Request) tradeInAttempt {
	date := strings.TrimSpace(r.FormValue("association_date"))
	attempt := tradeInAttempt{
		Direction: normalizeTradeInDirection(r.FormValue("direction")),
		Query:     strings.TrimSpace(r.FormValue("q")),
		Date:      date,
		Reference: strings.TrimSpace(r.FormValue("association_reference")),
		Notes:     strings.TrimSpace(r.FormValue("association_notes")),
		// The submitted key is retained so a rejected command replays instead of
		// writing a second relation when the user retries.
		RequestKey: strings.TrimSpace(r.FormValue("request_key")),
	}
	if page, err := strconv.Atoi(strings.TrimSpace(r.FormValue("page"))); err == nil && page > 0 {
		attempt.Page = page
	}
	if attempt.Page == 0 {
		attempt.Page = 1
	}
	ids := make([]string, 0, len(r.PostForm["counterpart_id"])+1)
	ids = append(ids, r.PathValue("id"))
	ids = append(ids, r.PostForm["counterpart_id"]...)
	attempt.Money = make(map[string]tradeInMoneyForm, len(ids))
	attempt.Existing = make(map[string]string, len(ids))
	for _, id := range ids {
		id = strings.TrimSpace(id)
		if id == "" {
			continue
		}
		attempt.Money[id] = tradeInMoneyForm{
			Amount:       r.FormValue("amount_" + id),
			Currency:     strings.TrimSpace(r.FormValue("currency_" + id)),
			OccurredAt:   r.FormValue("occurred_" + id),
			FXRate:       r.FormValue("fx_rate_" + id),
			FXRateDate:   r.FormValue("fx_date_" + id),
			FXRateSource: strings.TrimSpace(r.FormValue("fx_source_" + id)),
			Reference:    strings.TrimSpace(r.FormValue("reference_" + id)),
			Notes:        strings.TrimSpace(r.FormValue("notes_" + id)),
		}
		attempt.Existing[id] = strings.TrimSpace(r.FormValue("existing_" + id))
	}
	return attempt
}

// tradeInCommand builds the documented command from the retained form state.
func (s *Server) tradeInCommand(ctx context.Context, principal application.Principal, currentID string, attempt tradeInAttempt) (application.TradeInCommand, error) {
	id := strings.TrimSpace(attempt.RequestKey)
	if id == "" {
		return application.TradeInCommand{}, application.NewInputError("validation.request_key")
	}
	cmd := application.TradeInCommand{
		CurrentAssetID:    currentID,
		Direction:         domain.TradeInDirection(attempt.Direction),
		ExternalReference: attempt.Reference,
		Notes:             attempt.Notes,
	}
	if strings.TrimSpace(attempt.Date) != "" {
		occurredAt, err := parseFormTime(attempt.Date)
		if err != nil {
			return application.TradeInCommand{}, err
		}
		cmd.OccurredAt = occurredAt
	}
	selection, err := s.tradeInEconomicSelection(ctx, principal, currentID, tradeInRole(attempt.Direction, currentID, currentID), attempt.Money[currentID], attempt.Existing[currentID])
	if err != nil {
		return application.TradeInCommand{}, err
	}
	cmd.Current = selection
	for _, counterpartID := range attempt.Selected {
		selection, err := s.tradeInEconomicSelection(ctx, principal, counterpartID, tradeInRole(attempt.Direction, counterpartID, currentID), attempt.Money[counterpartID], attempt.Existing[counterpartID])
		if err != nil {
			return application.TradeInCommand{}, err
		}
		cmd.Counterparts = append(cmd.Counterparts, selection)
	}
	return cmd, nil
}

func (s *Server) tradeInEconomicSelection(ctx context.Context, principal application.Principal, assetID, role string, form tradeInMoneyForm, existingID string) (application.EconomicSelection, error) {
	selection := application.EconomicSelection{AssetID: assetID, ExistingEventID: strings.TrimSpace(existingID)}
	if selection.ExistingEventID != "" {
		return selection, nil
	}
	if strings.TrimSpace(form.Amount) == "" && strings.TrimSpace(form.Currency) == "" && strings.TrimSpace(form.OccurredAt) == "" {
		return selection, nil
	}
	record, err := s.tradeInRecordEvent(ctx, principal, tradeInKind(role), form)
	if err != nil {
		return application.EconomicSelection{}, err
	}
	selection.NewEvent = &record
	return selection, nil
}

// tradeInRecordEvent mirrors the ordinary event form: the amount is parsed from
// major units, the rate evidence is parsed only for a foreign currency, and the
// base-currency lock stays the application's.
func (s *Server) tradeInRecordEvent(ctx context.Context, principal application.Principal, kind domain.AssetEventType, form tradeInMoneyForm) (application.RecordEvent, error) {
	currency, err := domain.NormalizeCurrency(form.Currency)
	if err != nil {
		return application.RecordEvent{}, err
	}
	amount, err := domain.ParseMajorAmount(form.Amount, currency)
	if err != nil {
		return application.RecordEvent{}, tradeInAmountError(err)
	}
	occurredAt, err := parseFormTime(form.OccurredAt)
	if err != nil {
		return application.RecordEvent{}, err
	}
	command := application.RecordEvent{
		Type: kind, AmountMinor: amount, Currency: currency, OccurredAt: occurredAt,
		Source: "manual", ExternalReference: form.Reference, Notes: form.Notes,
	}
	baseCurrency, _, err := s.lifecycle.BaseCurrency(ctx, principal)
	if err != nil {
		return application.RecordEvent{}, err
	}
	if currency != baseCurrency {
		command.FXRateScaled, err = domain.ParseFXRate(form.FXRate)
		if err != nil {
			return application.RecordEvent{}, err
		}
		command.FXRateDate, err = parseFormDate(form.FXRateDate)
		if err != nil {
			return application.RecordEvent{}, err
		}
		command.FXRateSource = form.FXRateSource
		command.FXConfirmed = true
	}
	return command, nil
}

// tradeInAmountError keeps the shared wording for the ordinary amount rejections
// and reports a magnitude the supported range cannot represent as invalid input.
// The review form then shows an actionable localized error instead of a generic
// failure message while it preserves the submitted value.
func tradeInAmountError(err error) error {
	if strings.Contains(err.Error(), "supported range") {
		return application.NewInputError("validation.amount_invalid")
	}
	return err
}

// renderTradeInReview renders the money/review step and its final single POST.
// The submitted economic values are retained verbatim so a rejected command can
// be corrected without re-entering anything.
func (s *Server) renderTradeInReview(w http.ResponseWriter, r *http.Request, status int, principal application.Principal, current domain.Asset, attempt tradeInAttempt, message string) {
	management, ok := s.requireManagement(w, r)
	if !ok {
		return
	}
	counterparts := make([]domain.Asset, 0, len(attempt.Selected))
	for _, assetID := range attempt.Selected {
		counterpart, err := s.getAsset(r.Context(), principal, assetID)
		if err != nil {
			s.renderTradeInSelect(w, r, http.StatusUnprocessableEntity, principal, current, attempt, s.userError(principal.Locale, application.NewInputError("validation.trade_in_asset_unavailable")))
			return
		}
		counterparts = append(counterparts, counterpart)
	}
	baseCurrency, _, err := s.lifecycle.BaseCurrency(r.Context(), principal)
	if err != nil {
		s.renderError(w, r, http.StatusInternalServerError, err)
		return
	}
	pairs, involved := tradeInPairs(current, attempt.Direction, counterparts)
	// The shared preview is authoritative for the selection kinds and the still
	// missing fields; an unmappable submitted value or a preview refusal is
	// reported without losing the form or the retained selection.
	preview := application.TradeInPreview{}
	var previewErr error
	if command, commandErr := s.tradeInCommand(r.Context(), principal, current.ID, attempt); commandErr != nil {
		previewErr = commandErr
	} else {
		preview, previewErr = management.PreviewTradeIn(r.Context(), principal, command)
	}
	selections := map[string]string{}
	if previewErr == nil {
		for _, pair := range preview.Pairs {
			selections[pair.NewAssetID] = pair.NewSelection
			selections[pair.OldAssetID] = pair.OldSelection
		}
	} else if message == "" {
		message = s.userError(principal.Locale, previewErr)
		status = http.StatusUnprocessableEntity
	}
	economics, err := s.tradeInEconomics(r.Context(), principal, involved, attempt, selections, baseCurrency)
	if err != nil {
		s.renderError(w, r, http.StatusInternalServerError, err)
		return
	}
	// A retained request key is echoed back so a retry after a validation error
	// replays the original command instead of writing twice.
	requestKey := strings.TrimSpace(attempt.RequestKey)
	if requestKey == "" {
		requestKey = randomToken()
	}
	data := tradeInPageData{
		Mode:             tradeInModeReview,
		Direction:        attempt.Direction,
		DirectionLabel:   s.tradeInDirectionLabel(principal.Locale, attempt.Direction),
		SourceLabel:      textFor(principal.Locale, "trade_in.source_short"),
		DestinationLabel: textFor(principal.Locale, "trade_in.destination_short"),
		CurrentAssetID:   current.ID,
		CurrentName:      assetTitle(current),
		CurrentSpec:      current.TagSummary,
		SelectedIDs:      attempt.Selected,
		SelectionCount:   len(attempt.Selected),
		SelectionLabel:   textFor(principal.Locale, "trade_in.count", len(attempt.Selected)),
		Links:            pairs,
		Economics:        economics,
		PreviewReady:     previewErr == nil && preview.Ready,
		AssociationDate:  attempt.Date,
		AssociationRef:   attempt.Reference,
		AssociationNotes: attempt.Notes,
		RequestKey:       requestKey,
	}
	s.renderTradeIn(w, status, principal, current, data, message, r)
}

// tradeInPairs builds the display order of the selected relations. The order
// follows the application's own pair ordering (old asset, then new asset).
func tradeInPairs(current domain.Asset, direction string, counterparts []domain.Asset) ([]tradeInPairView, []tradeInInvolved) {
	involved := []tradeInInvolved{{Asset: current, Role: tradeInRole(direction, current.ID, current.ID)}}
	pairs := make([]tradeInPairView, 0, len(counterparts))
	for _, counterpart := range counterparts {
		role := tradeInRole(direction, counterpart.ID, current.ID)
		involved = append(involved, tradeInInvolved{Asset: counterpart, Role: role})
		pair := tradeInPairView{}
		if role == "new" {
			pair.NewAssetID, pair.NewAssetName, pair.NewAssetSpec = counterpart.ID, assetTitle(counterpart), counterpart.TagSummary
			pair.OldAssetID, pair.OldAssetName, pair.OldAssetSpec = current.ID, assetTitle(current), current.TagSummary
		} else {
			pair.NewAssetID, pair.NewAssetName, pair.NewAssetSpec = current.ID, assetTitle(current), current.TagSummary
			pair.OldAssetID, pair.OldAssetName, pair.OldAssetSpec = counterpart.ID, assetTitle(counterpart), counterpart.TagSummary
		}
		pairs = append(pairs, pair)
	}
	sort.Slice(pairs, func(i, j int) bool {
		if pairs[i].OldAssetID != pairs[j].OldAssetID {
			return pairs[i].OldAssetID < pairs[j].OldAssetID
		}
		return pairs[i].NewAssetID < pairs[j].NewAssetID
	})
	return pairs, involved
}

// tradeInInvolved is one asset's economic side of the command under
// construction.
type tradeInInvolved struct {
	Asset domain.Asset
	Role  string
}

// resolveInitialTradeInReview fills in what the selection->review transition
// left implicit. A side submitted without an explicit record and without new
// money is bound to its current effective record, and a blank association date
// is defaulted, so the rendered review form and the shared preview describe the
// same command. An explicit submitted ID is never replaced and submitted money
// is never touched; only this transition calls the helper.
func (s *Server) resolveInitialTradeInReview(ctx context.Context, principal application.Principal, currentID string, attempt tradeInAttempt) (tradeInAttempt, error) {
	if strings.TrimSpace(attempt.Date) == "" {
		attempt.Date = time.Now().Local().Format("2006-01-02T15:04")
	}
	if attempt.Existing == nil {
		attempt.Existing = make(map[string]string, len(attempt.Selected)+1)
	}
	for _, assetID := range append([]string{currentID}, attempt.Selected...) {
		if strings.TrimSpace(attempt.Existing[assetID]) != "" {
			continue
		}
		if tradeInNewMoneySubmitted(attempt.Money[assetID]) {
			continue
		}
		event, err := s.effectiveTradeInEvent(ctx, principal, assetID, tradeInKind(tradeInRole(attempt.Direction, assetID, currentID)))
		if err != nil {
			return attempt, err
		}
		if event != nil {
			attempt.Existing[assetID] = event.ID
		}
	}
	return attempt, nil
}

// tradeInNewMoneySubmitted reports whether the reviewer supplied any new-money
// field for one side. Such a submission must never be replaced by an inferred
// reuse of an existing record.
func tradeInNewMoneySubmitted(form tradeInMoneyForm) bool {
	return strings.TrimSpace(form.Amount) != "" || strings.TrimSpace(form.Currency) != "" || strings.TrimSpace(form.OccurredAt) != ""
}

// tradeInEconomics projects every involved asset: an explicitly submitted record
// is shown read-only with its original evidence, a side without one exposes the
// money fields a write still needs, and a submitted record that no longer
// qualifies stays visible as stale instead of being replaced. Nothing is
// inferred here: the selection->review transition has already bound every record
// the form is allowed to carry.
func (s *Server) tradeInEconomics(ctx context.Context, principal application.Principal, involved []tradeInInvolved, attempt tradeInAttempt, selections map[string]string, baseCurrency string) ([]tradeInEconomicView, error) {
	views := make([]tradeInEconomicView, 0, len(involved))
	for _, item := range involved {
		role := item.Role
		kind := tradeInKind(role)
		submitted := tradeInNewMoneySubmitted(attempt.Money[item.Asset.ID])
		view := tradeInEconomicView{
			AssetID: item.Asset.ID, AssetName: assetTitle(item.Asset), AssetSpec: item.Asset.TagSummary,
			Role: role, RoleLabel: s.tradeInRoleLabel(principal.Locale, role),
			Form: attempt.Money[item.Asset.ID],
		}
		if attempt.Initial {
			// Only the selection->review transition closes an empty money group;
			// a rejected final POST re-renders the submitted blanks unchanged.
			if strings.TrimSpace(view.Form.Currency) == "" {
				view.Form.Currency = baseCurrency
			}
			if strings.TrimSpace(view.Form.OccurredAt) == "" {
				view.Form.OccurredAt = time.Now().Local().Format("2006-01-02T15:04")
			}
		}
		existingID := strings.TrimSpace(attempt.Existing[item.Asset.ID])
		if existingID != "" {
			if err := s.resolveSubmittedTradeInEvent(ctx, principal, item.Asset.ID, kind, existingID, &view); err != nil {
				return nil, err
			}
		}
		selection := selections[item.Asset.ID]
		if selection == application.TradeInSelectionLinked {
			selection = application.TradeInSelectionReuse
		}
		if existingID == "" && !submitted {
			// This side was submitted with neither a record nor new money, so the
			// form must ask for it again instead of adopting an inferred reuse the
			// submitted command never carried.
			selection = application.TradeInSelectionMissing
		} else if selection == "" {
			if view.Event != nil {
				selection = application.TradeInSelectionReuse
			} else {
				selection = application.TradeInSelectionMissing
			}
		}
		if view.Stale {
			// A stale selection never becomes new-money inputs: the submitted ID
			// stays in the form, so a retry conflicts again, and the group points
			// at the explicit reselection path instead.
			selection = application.TradeInSelectionMissing
		}
		view.Selection = selection
		view.Editing = !view.Stale && selection != application.TradeInSelectionReuse
		views = append(views, view)
	}
	return views, nil
}

// resolveSubmittedTradeInEvent projects an explicitly submitted record. A record
// that no longer qualifies is kept as submitted and flagged stale, never swapped
// for a different effective event: silently binding another ID would substitute
// money the reviewer never chose. The application service stays the authority and
// refuses the retry, so the conflict boundary is unchanged.
func (s *Server) resolveSubmittedTradeInEvent(ctx context.Context, principal application.Principal, assetID string, kind domain.AssetEventType, existingID string, view *tradeInEconomicView) error {
	view.ExistingID = existingID
	event, err := s.lifecycle.GetEvent(ctx, principal, existingID)
	if err != nil {
		var input application.InputError
		if !errors.As(err, &input) && !errors.Is(err, sql.ErrNoRows) {
			return err
		}
		view.Stale = true
		return nil
	}
	if event.AssetID == assetID {
		// The submitted evidence is shown even when the record was voided or
		// changed, so the reviewer sees exactly what was selected.
		copied := event
		view.Event = &copied
	}
	view.Stale = event.IsVoided || event.Kind() != kind || event.AssetID != assetID
	return nil
}

// effectiveTradeInEvent reads the one current purchase or sale of an asset for
// display only; the application service remains the authority for what a write
// accepts.
func (s *Server) effectiveTradeInEvent(ctx context.Context, principal application.Principal, assetID string, kind domain.AssetEventType) (*domain.AssetEvent, error) {
	events, _, err := s.lifecycle.Timeline(ctx, principal, assetID)
	if err != nil {
		return nil, err
	}
	for index := range events {
		if events[index].IsVoided || events[index].Kind() != kind {
			continue
		}
		return &events[index], nil
	}
	return nil, nil
}

// recordTradeIn commits the confirmed command through the shared service. The
// request key is retained across validation errors, so a retry replays instead
// of writing twice.
func (s *Server) recordTradeIn(w http.ResponseWriter, r *http.Request) {
	if !s.verifyCSRF(w, r) {
		return
	}
	principal, ok := s.requirePrincipal(w, r)
	if !ok {
		return
	}
	if !principal.Can(application.CapabilityManageLifecycle) {
		s.renderForbidden(w, principal, "error.forbidden_lifecycle")
		return
	}
	management, ok := s.requireManagement(w, r)
	if !ok {
		return
	}
	current, err := s.getAsset(r.Context(), principal, r.PathValue("id"))
	if err != nil {
		s.renderNotFound(w, principal, "error.not_found_asset")
		return
	}
	attempt := s.tradeInAttemptFromForm(r)
	attempt.Selected = tradeInRequestedCounterparts(r)
	command, err := s.tradeInCommand(r.Context(), principal, current.ID, attempt)
	if err != nil {
		s.renderTradeInReview(w, r, http.StatusUnprocessableEntity, principal, current, attempt, s.userError(principal.Locale, err))
		return
	}
	if _, err := management.RecordTradeIn(r.Context(), principal, attempt.RequestKey, command); err != nil {
		if errors.Is(err, application.ErrForbidden) || errors.Is(err, application.ErrUnauthorized) {
			s.renderForbidden(w, principal, "error.forbidden_lifecycle")
			return
		}
		s.renderTradeInReview(w, r, http.StatusUnprocessableEntity, principal, current, attempt, s.userError(principal.Locale, err))
		return
	}
	http.Redirect(w, r, "/assets/"+current.ID, http.StatusSeeOther)
}

func tradeInRequestedCounterparts(r *http.Request) []string {
	ids := make([]string, 0, len(r.PostForm["counterpart_id"]))
	seen := map[string]bool{}
	for _, id := range r.PostForm["counterpart_id"] {
		id = strings.TrimSpace(id)
		if id == "" || seen[id] {
			continue
		}
		seen[id] = true
		ids = append(ids, id)
	}
	sort.Strings(ids)
	return ids
}

// tradeInLinkForm renders the dedicated relation editor for an active pair. A
// cancelled, voided or broken pair is never editable.
func (s *Server) tradeInLinkForm(w http.ResponseWriter, r *http.Request) {
	principal, ok := s.requirePrincipal(w, r)
	if !ok {
		return
	}
	if !principal.Can(application.CapabilityManageLifecycle) {
		s.renderForbidden(w, principal, "error.forbidden_lifecycle")
		return
	}
	management, ok := s.requireManagement(w, r)
	if !ok {
		return
	}
	current, err := s.getAsset(r.Context(), principal, r.PathValue("id"))
	if err != nil {
		s.renderNotFound(w, principal, "error.not_found_asset")
		return
	}
	link, err := management.TradeInLink(r.Context(), principal, application.TradeInLinkQuery{OwningAssetID: current.ID, LinkID: r.PathValue("link")})
	if err != nil {
		s.renderTradeInLinkError(w, principal, err)
		return
	}
	if !tradeInLinkEditable(link) {
		s.renderTradeInLinkError(w, principal, application.NewInputError("validation.trade_in_link_stale"))
		return
	}
	choices, err := s.tradeInChoices(r.Context(), principal)
	if err != nil {
		s.renderError(w, r, http.StatusInternalServerError, err)
		return
	}
	direction := tradeInDirectionDestination
	if link.NewAssetID == current.ID {
		direction = tradeInDirectionSource
	}
	edit := &tradeInLinkEdit{
		LinkID: link.LinkID, SourceEventID: link.SourceEventID, DestinationEventID: link.DestinationEventID,
		Direction: direction, DirectionLabel: s.tradeInDirectionLabel(principal.Locale, direction),
		NewAssetID: link.NewAssetID, NewAssetName: link.NewAssetName, NewAssetSpec: link.NewAssetSpec,
		OldAssetID: link.OldAssetID, OldAssetName: link.OldAssetName, OldAssetSpec: link.OldAssetSpec,
	}
	date := time.Now().Local().Format("2006-01-02T15:04")
	reference, notes := "", ""
	if source, err := s.lifecycle.GetEvent(r.Context(), principal, link.SourceEventID); err == nil {
		date = source.OccurredAt.Local().Format("2006-01-02T15:04")
		reference, notes = source.ExternalReference, source.Notes
	}
	data := tradeInPageData{
		Mode: tradeInModeLink, Direction: direction, DirectionLabel: edit.DirectionLabel,
		SourceLabel: textFor(principal.Locale, "trade_in.source_short"), DestinationLabel: textFor(principal.Locale, "trade_in.destination_short"),
		CurrentAssetID: current.ID, CurrentName: assetTitle(current), CurrentSpec: current.TagSummary,
		Choices: choices, LinkEdit: edit, AssociationDate: date, AssociationRef: reference, AssociationNotes: notes,
		RequestKey: randomToken(),
	}
	s.renderTradeIn(w, http.StatusOK, principal, current, data, "", r)
}

func tradeInLinkEditable(link application.TradeInLinkView) bool {
	return link.State == string(domain.TradeInStateActive) && !link.NewAssetDeleted && !link.OldAssetDeleted &&
		link.SourceEventID != "" && link.DestinationEventID != ""
}

func (s *Server) correctTradeInLink(w http.ResponseWriter, r *http.Request) {
	if !s.verifyCSRF(w, r) {
		return
	}
	principal, ok := s.requirePrincipal(w, r)
	if !ok {
		return
	}
	if !principal.Can(application.CapabilityManageLifecycle) {
		s.renderForbidden(w, principal, "error.forbidden_lifecycle")
		return
	}
	management, ok := s.requireManagement(w, r)
	if !ok {
		return
	}
	current, err := s.getAsset(r.Context(), principal, r.PathValue("id"))
	if err != nil {
		s.renderNotFound(w, principal, "error.not_found_asset")
		return
	}
	command := application.CorrectTradeInLinkCommand{
		LinkID:                     r.PathValue("link"),
		ExpectedSourceEventID:      strings.TrimSpace(r.FormValue("source_event_id")),
		ExpectedDestinationEventID: strings.TrimSpace(r.FormValue("destination_event_id")),
		NewAssetID:                 strings.TrimSpace(r.FormValue("new_asset_id")),
		OldAssetID:                 strings.TrimSpace(r.FormValue("old_asset_id")),
		ExternalReference:          strings.TrimSpace(r.FormValue("association_reference")),
		Notes:                      strings.TrimSpace(r.FormValue("association_notes")),
	}
	if value := strings.TrimSpace(r.FormValue("association_date")); value != "" {
		occurredAt, err := parseFormTime(value)
		if err != nil {
			s.renderTradeInLinkMutationError(w, r, principal, current, err)
			return
		}
		command.OccurredAt = occurredAt
	}
	if _, err := management.CorrectTradeInLink(r.Context(), principal, r.FormValue("request_key"), command); err != nil {
		if errors.Is(err, application.ErrForbidden) || errors.Is(err, application.ErrUnauthorized) {
			s.renderForbidden(w, principal, "error.forbidden_lifecycle")
			return
		}
		s.renderTradeInLinkMutationError(w, r, principal, current, err)
		return
	}
	http.Redirect(w, r, "/assets/"+current.ID, http.StatusSeeOther)
}

// cancelTradeInLink voids one pair without touching money.
func (s *Server) cancelTradeInLink(w http.ResponseWriter, r *http.Request) {
	if !s.verifyCSRF(w, r) {
		return
	}
	principal, ok := s.requirePrincipal(w, r)
	if !ok {
		return
	}
	if !principal.Can(application.CapabilityManageLifecycle) {
		s.renderForbidden(w, principal, "error.forbidden_lifecycle")
		return
	}
	management, ok := s.requireManagement(w, r)
	if !ok {
		return
	}
	current, err := s.getAsset(r.Context(), principal, r.PathValue("id"))
	if err != nil {
		s.renderNotFound(w, principal, "error.not_found_asset")
		return
	}
	command := application.CancelTradeInLinkCommand{
		LinkID:                     r.PathValue("link"),
		ExpectedSourceEventID:      strings.TrimSpace(r.FormValue("source_event_id")),
		ExpectedDestinationEventID: strings.TrimSpace(r.FormValue("destination_event_id")),
		Notes:                      strings.TrimSpace(r.FormValue("association_notes")),
	}
	if value := strings.TrimSpace(r.FormValue("association_date")); value != "" {
		if occurredAt, err := parseFormTime(value); err == nil {
			command.OccurredAt = occurredAt
		}
	}
	if _, err := management.CancelTradeInLink(r.Context(), principal, r.FormValue("request_key"), command); err != nil {
		if errors.Is(err, application.ErrForbidden) || errors.Is(err, application.ErrUnauthorized) {
			s.renderForbidden(w, principal, "error.forbidden_lifecycle")
			return
		}
		s.renderAsset(w, r, http.StatusUnprocessableEntity, principal, current.ID, s.userError(principal.Locale, err), "")
		return
	}
	http.Redirect(w, r, "/assets/"+current.ID, http.StatusSeeOther)
}

func (s *Server) renderTradeInLinkMutationError(w http.ResponseWriter, r *http.Request, principal application.Principal, current domain.Asset, err error) {
	message := s.userError(principal.Locale, err)
	direction := normalizeTradeInDirection(r.FormValue("direction"))
	requestKey := strings.TrimSpace(r.FormValue("request_key"))
	if requestKey == "" {
		requestKey = randomToken()
	}
	data := tradeInPageData{
		Mode: tradeInModeLink, Direction: direction, DirectionLabel: s.tradeInDirectionLabel(principal.Locale, direction),
		SourceLabel: textFor(principal.Locale, "trade_in.source_short"), DestinationLabel: textFor(principal.Locale, "trade_in.destination_short"),
		CurrentAssetID: current.ID, CurrentName: assetTitle(current), CurrentSpec: current.TagSummary,
		AssociationDate: r.FormValue("association_date"), AssociationRef: strings.TrimSpace(r.FormValue("association_reference")),
		AssociationNotes: strings.TrimSpace(r.FormValue("association_notes")),
		RequestKey:       requestKey,
	}
	if choices, choiceErr := s.tradeInChoices(r.Context(), principal); choiceErr == nil {
		data.Choices = choices
	}
	data.LinkEdit = &tradeInLinkEdit{
		LinkID: r.PathValue("link"), SourceEventID: r.FormValue("source_event_id"), DestinationEventID: r.FormValue("destination_event_id"),
		Direction: direction, DirectionLabel: data.DirectionLabel,
		NewAssetID: strings.TrimSpace(r.FormValue("new_asset_id")), OldAssetID: strings.TrimSpace(r.FormValue("old_asset_id")),
	}
	if data.AssociationDate == "" {
		data.AssociationDate = time.Now().Local().Format("2006-01-02T15:04")
	}
	s.renderTradeIn(w, http.StatusUnprocessableEntity, principal, current, data, message, r)
}

// renderTradeInLinkError reports a relation that cannot be loaded or edited.
func (s *Server) renderTradeInLinkError(w http.ResponseWriter, principal application.Principal, err error) {
	if errors.Is(err, application.ErrForbidden) || errors.Is(err, application.ErrUnauthorized) {
		s.renderForbidden(w, principal, "error.forbidden_lifecycle")
		return
	}
	var input application.InputError
	if errors.As(err, &input) {
		s.renderNotFound(w, principal, input.Code)
		return
	}
	s.renderNotFound(w, principal, "validation.trade_in_link_stale")
}
