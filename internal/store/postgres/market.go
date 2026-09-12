package postgres

import (
	"context"
	"database/sql"
	"github.com/SampsonFox/assetloop/internal/application"
	"github.com/SampsonFox/assetloop/internal/domain"
	dbq "github.com/SampsonFox/assetloop/internal/store/postgres/postgresdb"
	"github.com/google/uuid"
	"time"
)

func (s *Store) WithMarketWrite(ctx context.Context, tenant string, fn func(application.MarketStore) error) error {
	if s.tx != nil && s.managementTenant != "" {
		if tenant != s.managementTenant {
			return application.ErrForbidden
		}
		return fn(s)
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	scoped := &Store{db: s.db, tx: tx}
	tenantID, err := uuid.Parse(tenant)
	if err != nil {
		return err
	}
	n, err := scoped.queries().LockLifecycleTenant(ctx, tenantID)
	if err != nil {
		return err
	}
	if n != 1 {
		return sql.ErrNoRows
	}
	if err = fn(scoped); err != nil {
		return err
	}
	return tx.Commit()
}
func (s *Store) LockMarketBaseCurrency(ctx context.Context, tenant, currency string) error {
	tenantID, err := uuid.Parse(tenant)
	if err != nil {
		return err
	}
	return s.queries().LockTenantBaseCurrency(ctx, dbq.LockTenantBaseCurrencyParams{ID: tenantID, BaseCurrency: currency})
}
func marketTime(v int64) time.Time {
	if v == 0 {
		return time.Time{}
	}
	return time.Unix(v, 0).UTC()
}
func marketInt(v *int64) sql.NullInt64 {
	if v == nil {
		return sql.NullInt64{}
	}
	return sql.NullInt64{Int64: *v, Valid: true}
}
func marketString(v *string) sql.NullString {
	if v == nil {
		return sql.NullString{}
	}
	return sql.NullString{String: *v, Valid: true}
}
func marketIntPtr(v sql.NullInt64) *int64 {
	if !v.Valid {
		return nil
	}
	return &v.Int64
}
func marketStringPtr(v sql.NullString) *string {
	if !v.Valid {
		return nil
	}
	return &v.String
}
func marketItem(r dbq.GetMarketItemRow) domain.MarketItem {
	return domain.MarketItem{SelectionJSON: r.SelectionJson.String, ID: r.ID, TenantID: r.TenantID, Name: r.Name, Provider: r.Provider, Keyword: r.Keyword, FilterCriteria: r.FilterCriteria, ModelDesc: r.ModelDesc, Region: r.Region, ExternalID: r.ExternalID.String, Enabled: r.Enabled == 1, CreatedAt: marketTime(r.CreatedAt), LastAttempt: marketTime(r.LastAttempt), LastSuccess: marketTime(r.LastSuccess), LastError: r.LastError, LeaseToken: r.LeaseToken, LeaseUntil: marketTime(r.LeaseUntil)}
}
func (s *Store) GetMarketItem(ctx context.Context, tenant, id string) (domain.MarketItem, error) {
	r, e := s.queries().GetMarketItem(ctx, dbq.GetMarketItemParams{Tenant: tenant, ID: id})
	return marketItem(r), e
}
func (s *Store) ListMarketItems(ctx context.Context, tenant string) ([]domain.MarketItem, error) {
	rows, e := s.queries().ListMarketItems(ctx, tenant)
	if e != nil {
		return nil, e
	}
	out := make([]domain.MarketItem, 0, len(rows))
	for _, r := range rows {
		out = append(out, marketItem(dbq.GetMarketItemRow(r)))
	}
	return out, nil
}
func (s *Store) ListMarketTenants(ctx context.Context) ([]string, error) {
	return s.queries().ListMarketTenants(ctx)
}
func (s *Store) CreateMarketItem(ctx context.Context, m domain.MarketItem) error {
	var enabled int64
	if m.Enabled {
		enabled = 1
	}
	return s.queries().CreateMarketItem(ctx, dbq.CreateMarketItemParams{SelectionJson: sql.NullString{String: m.SelectionJSON, Valid: m.SelectionJSON != ""}, ID: m.ID, Tenant: m.TenantID, Name: m.Name, Provider: m.Provider, Keyword: m.Keyword, FilterCriteria: m.FilterCriteria, ModelDesc: m.ModelDesc, Region: m.Region, ExternalID: sql.NullString{String: m.ExternalID, Valid: m.ExternalID != ""}, Enabled: enabled, CreatedAt: m.CreatedAt.Unix(), LastSuccess: m.LastSuccess.Unix()})
}
func (s *Store) UpdateMarketItem(ctx context.Context, m domain.MarketItem) error {
	var enabled int64
	if m.Enabled {
		enabled = 1
	}
	n, e := s.queries().UpdateMarketItem(ctx, dbq.UpdateMarketItemParams{Tenant: m.TenantID, ID: m.ID, Name: m.Name, Enabled: enabled})
	if e == nil && n != 1 {
		return sql.ErrNoRows
	}
	return e
}
func (s *Store) BindAssetMarket(ctx context.Context, tenant, asset, id string) error {
	if id == "" {
		return s.queries().UnbindAssetMarket(ctx, dbq.UnbindAssetMarketParams{Tenant: tenant, AssetID: asset})
	}
	return s.queries().BindAssetMarket(ctx, dbq.BindAssetMarketParams{Tenant: tenant, AssetID: asset, MarketItemID: id})
}
func (s *Store) MarketAssetIDs(ctx context.Context, tenant, id string) ([]string, error) {
	return s.queries().MarketAssetIDs(ctx, dbq.MarketAssetIDsParams{Tenant: tenant, ID: id})
}
func (s *Store) PutMarketPrice(ctx context.Context, p domain.MarketPrice) error {
	arg := dbq.PutMarketPriceParams{Tenant: p.TenantID, MarketItemID: p.MarketItemID, ObservationDate: p.ObservationDate, ObservedAt: p.ObservedAt.Unix(), MaxMinor: p.MaxMinor, MinMinor: marketInt(p.MinMinor), Currency: p.Currency, BaseCurrency: p.BaseCurrency, BaseMinor: marketInt(p.BaseMinor), Provider: p.Provider, ProviderVersion: p.ProviderVersion, Provenance: p.Provenance, Evidence: p.Evidence, SourceDate: marketString(p.SourceDate), SampleCount: marketInt(p.SampleCount)}
	if p.FX != nil {
		arg.RateScaled = sql.NullInt64{Int64: p.FX.RateScaled, Valid: true}
		arg.RateDate = sql.NullString{String: p.FX.RateDate.Format("2006-01-02"), Valid: true}
		arg.RateSource = sql.NullString{String: p.FX.RateSource, Valid: true}
	}
	return s.queries().PutMarketPrice(ctx, arg)
}
func (s *Store) ListMarketPrices(ctx context.Context, tenant, id string) ([]domain.MarketPrice, error) {
	rows, e := s.queries().ListMarketPrices(ctx, dbq.ListMarketPricesParams{Tenant: tenant, ID: id})
	if e != nil {
		return nil, e
	}
	out := make([]domain.MarketPrice, 0, len(rows))
	for _, r := range rows {
		p := domain.MarketPrice{TenantID: r.TenantID, MarketItemID: r.MarketItemID, ObservationDate: r.ObservationDate, ObservedAt: marketTime(r.ObservedAt), MaxMinor: r.MaxMinor, MinMinor: marketIntPtr(r.MinMinor), Currency: r.Currency, BaseCurrency: r.BaseCurrency, BaseMinor: marketIntPtr(r.BaseMinor), Provider: r.Provider, ProviderVersion: r.ProviderVersion, Provenance: r.Provenance, Evidence: r.Evidence, SourceDate: marketStringPtr(r.SourceDate), SampleCount: marketIntPtr(r.SampleCount)}
		if r.RateScaled.Valid {
			date, err := time.Parse("2006-01-02", r.RateDate.String)
			if err != nil {
				return nil, err
			}
			p.FX = &domain.FXEvidence{OriginalAmountMinor: p.MaxMinor, OriginalCurrency: p.Currency, RateScaled: r.RateScaled.Int64, RateDate: date, RateSource: r.RateSource.String}
		}
		out = append(out, p)
	}
	return out, nil
}
func (s *Store) ClaimMarketLease(ctx context.Context, tenant, id, token string, now, until time.Time) (bool, error) {
	n, e := s.queries().ClaimMarketLease(ctx, dbq.ClaimMarketLeaseParams{Tenant: tenant, ID: id, Token: token, NowTime: now.Unix(), UntilTime: until.Unix()})
	return n == 1, e
}
func (s *Store) FinishMarketLease(ctx context.Context, tenant, id, token string, success time.Time, message string) error {
	n, e := s.queries().FinishMarketLease(ctx, dbq.FinishMarketLeaseParams{Tenant: tenant, ID: id, Token: token, LastError: message, SuccessTime: success.Unix()})
	if e == nil && n != 1 {
		return sql.ErrNoRows
	}
	return e
}
