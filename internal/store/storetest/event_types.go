package storetest

import (
	"context"
	"database/sql"
	"fmt"
	"github.com/SampsonFox/assetloop/internal/application"
	"github.com/SampsonFox/assetloop/internal/domain"
	"sync"
	"testing"
	"time"
)

func RunEventTypeManagement(t *testing.T, first, second Store, db *sql.DB, driver string) {
	t.Helper()
	ctx := context.Background()
	owner, err := first.FirstPrincipal(ctx)
	if err != nil {
		t.Fatal(err)
	}
	catalog := application.NewCatalogService(first)
	snapshot, err := catalog.Snapshot(ctx, owner)
	if err != nil {
		t.Fatal(err)
	}
	asset, err := catalog.CreateAsset(ctx, owner, application.CreateCatalogAsset{VariantID: snapshot.Variants[0].ID, DisplayName: "Type management"})
	if err != nil {
		t.Fatal(err)
	}
	service := application.NewLifecycleService(first)
	item, err := service.CreateEventType(ctx, owner, application.CreateAssetEventType{Name: "Type original", Cashflow: domain.AssetEventNeutral})
	if err != nil {
		t.Fatal(err)
	}
	item, err = service.UpdateEventType(ctx, owner, item.ID, application.UpdateEventType{Name: "Type original", Cashflow: domain.AssetEventExpense})
	if err != nil {
		t.Fatal(err)
	}
	cmd := application.RecordEvent{AssetID: asset.ID, TypeID: item.ID, AmountMinor: 1000, Currency: "CNY", OccurredAt: time.Now().Add(-time.Hour), Source: "type-test", RequestKey: "type-management-retry"}
	event, err := service.Record(ctx, owner, cmd)
	if err != nil {
		t.Fatal(err)
	}
	var marker string
	placeholder := "?"
	if driver == "postgres" {
		placeholder = "$1"
	}
	if err := db.QueryRow("SELECT event_type FROM asset_events WHERE id = "+placeholder, event.ID).Scan(&marker); err != nil {
		t.Fatal(err)
	}
	if marker != "custom" {
		t.Fatalf("stored a display name: %s", marker)
	}
	if _, err = service.UpdateEventType(ctx, owner, item.ID, application.UpdateEventType{Name: "Type renamed", Cashflow: domain.AssetEventIncome}); err == nil {
		t.Fatal("used direction changed")
	}
	if _, err = service.UpdateEventType(ctx, owner, item.ID, application.UpdateEventType{Name: "Type renamed", Cashflow: domain.AssetEventExpense}); err != nil {
		t.Fatal(err)
	}
	got, err := service.GetEvent(ctx, owner, event.ID)
	if err != nil || got.Type != "Type renamed" || got.TypeID != item.ID || got.BaseAmountMinor != -1000 {
		t.Fatalf("renamed event: %+v %v", got, err)
	}
	page, err := service.TimelinePage(ctx, owner, asset.ID, application.EventListOptions{Type: item.ID})
	if err != nil || page.Total != 1 {
		t.Fatalf("ID filter: %+v %v", page, err)
	}
	if _, err = service.SetEventTypeEnabled(ctx, owner, item.ID, false); err != nil {
		t.Fatal(err)
	}
	retried, err := service.Record(ctx, owner, cmd)
	if err != nil || retried.ID != event.ID {
		t.Fatalf("disabled retry: %+v %v", retried, err)
	}
	cmd.RequestKey = ""
	if _, err = service.Record(ctx, owner, cmd); err == nil {
		t.Fatal("disabled type accepted new event")
	}
	corrected, err := service.Correct(ctx, owner, event.ID, cmd)
	if err != nil || corrected.TypeID != item.ID {
		t.Fatalf("disabled correction: %+v %v", corrected, err)
	}
	if _, err = service.UpdateEventType(ctx, owner, item.ID, application.UpdateEventType{Name: "Type renamed", Cashflow: domain.AssetEventNeutral}); err == nil {
		t.Fatal("voided history did not lock direction")
	}
	if _, err = service.SetEventTypeEnabled(ctx, owner, item.ID, true); err != nil {
		t.Fatal(err)
	}
	if _, err = service.Record(ctx, owner, cmd); err != nil {
		t.Fatal(err)
	}
	cost, err := service.CostDashboard(ctx, owner, asset.ID)
	if err != nil || cost.NetMinor != 2000 || len(cost.Categories) != 1 || cost.Categories[0].TypeID != item.ID {
		t.Fatalf("cost category by ID: %+v %v", cost, err)
	}
	types, err := service.EventTypes(ctx, owner)
	if err != nil {
		t.Fatal(err)
	}
	for _, builtin := range types {
		if builtin.BuiltIn {
			if builtin.ID == "" {
				t.Fatal("builtin lacks ID")
			}
			if _, err := service.SetEventTypeEnabled(ctx, owner, builtin.ID, false); err == nil {
				t.Fatal("builtin disabled")
			}
		}
	}
	viewer := owner
	viewer.Role = application.RoleViewer
	if _, err := service.UpdateEventType(ctx, viewer, item.ID, application.UpdateEventType{Name: "Denied", Cashflow: domain.AssetEventExpense}); err == nil {
		t.Fatal("viewer wrote")
	}
	foreign := owner
	foreign.TenantID = "99999999-9999-4999-8999-999999999999"
	if _, err := service.SetEventTypeEnabled(ctx, foreign, item.ID, false); err == nil {
		t.Fatal("cross-space write accepted")
	}
	listed, err := service.EventTypePage(ctx, viewer, application.EventTypeListOptions{Query: "Type renamed", Status: "enabled", Page: 1, PageSize: 1})
	if err != nil || listed.Total != 1 || listed.Types[0].ReferenceCount != 3 {
		t.Fatalf("management list: %+v %v", listed, err)
	}
	editor := owner
	editor.Role = application.RoleEditor
	if _, err := service.UpdateEventType(ctx, editor, item.ID, application.UpdateEventType{Name: "Type renamed", Cashflow: domain.AssetEventExpense}); err != nil {
		t.Fatal(err)
	}

	for n := 0; n < 4; n++ {
		t.Run(fmt.Sprintf("concurrent-use-and-direction-%d", n), func(t *testing.T) {
			item, err := service.CreateEventType(ctx, owner, application.CreateAssetEventType{Name: fmt.Sprintf("Race type %d", n), Cashflow: domain.AssetEventExpense})
			if err != nil {
				t.Fatal(err)
			}
			command := cmd
			command.TypeID = item.ID
			start := make(chan struct{})
			var wg sync.WaitGroup
			wg.Add(2)
			var event domain.AssetEvent
			var writeErr, changeErr error
			go func() { defer wg.Done(); <-start; event, writeErr = service.Record(ctx, owner, command) }()
			go func() {
				defer wg.Done()
				<-start
				_, changeErr = application.NewLifecycleService(second).UpdateEventType(ctx, owner, item.ID, application.UpdateEventType{Name: item.Name, Cashflow: domain.AssetEventIncome})
			}()
			close(start)
			wg.Wait()
			if writeErr != nil {
				t.Fatal(writeErr)
			}
			if changeErr == nil && event.BaseAmountMinor <= 0 {
				t.Fatal("direction updated after expense was committed")
			}
		})
	}
	t.Run("concurrent-stop-and-use", func(t *testing.T) {
		item, err := service.CreateEventType(ctx, owner, application.CreateAssetEventType{Name: "Stop race", Cashflow: domain.AssetEventExpense})
		if err != nil {
			t.Fatal(err)
		}
		command := cmd
		command.TypeID = item.ID
		start := make(chan struct{})
		var wg sync.WaitGroup
		wg.Add(2)
		var stopErr error
		go func() { defer wg.Done(); <-start; service.Record(ctx, owner, command) }()
		go func() {
			defer wg.Done()
			<-start
			_, stopErr = application.NewLifecycleService(second).SetEventTypeEnabled(ctx, owner, item.ID, false)
		}()
		close(start)
		wg.Wait()
		if stopErr != nil {
			t.Fatal(stopErr)
		}
		if _, err := service.Record(ctx, owner, command); err == nil {
			t.Fatal("recorded after stop completed")
		}
	})
}
