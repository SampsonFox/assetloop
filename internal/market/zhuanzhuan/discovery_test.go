package zhuanzhuan

import (
	"context"
	"encoding/json"
	"github.com/SampsonFox/assetloop/internal/application"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"
)

func TestRecordedDiscovery(t *testing.T) {
	p := New("fixture-secret")
	b, _ := os.ReadFile("testdata/product_search.json")
	page, e := p.parseProducts(b)
	if e != nil || len(page.Items) != 3 || !page.HasNext || page.NextPageToken != "fixture-next-page" || page.Items[0].Reference.Metric != "fixture-metric-0" || *page.Items[0].PriceMinor != 414800 {
		t.Fatal(page, e)
	}
	b, _ = os.ReadFile("testdata/product_detail.json")
	detail, e := p.parseDetail(b, page.Items[0].Reference)
	if e != nil || detail.Selection.ProductID != "100" || detail.Selection.Condition != "95B" {
		t.Fatal(detail, e)
	}
	specs := map[string]string{}
	for _, s := range detail.Selection.Specifications {
		specs[s.Name] = s.Value
	}
	if specs["存储容量"] != "256G" || specs["颜色"] != "蓝色钛金属" || specs["运行内存"] != "" {
		t.Fatal(specs)
	}
	// JSON details preserve options and explicitly missing values without inventing a selection.
	detail, e = p.parseDetail([]byte(`{"structuredContent":{"data":{"title":"Phone","specifications":[{"name":"颜色","valueOptions":[{"value":"黑色","selected":0},{"value":"蓝色","selected":1}],"explanations":["原厂颜色"]}]}}}`), page.Items[0].Reference)
	if e != nil || len(detail.Selection.Specifications) != 1 || len(detail.Selection.Specifications[0].ValueOptions) != 2 || *detail.Selection.Specifications[0].ValueOptions[1].Selected != 1 || detail.Selection.Specifications[0].Value != "" {
		t.Fatal(detail, e)
	}
	if _, e = p.parseDetail([]byte(`{"structuredContent":{"data":null}}`), page.Items[0].Reference); e != application.ErrMarketProductGone {
		t.Fatal(e)
	}
	detail, e = p.parseDetail([]byte(`{"structuredContent":{"data":{"title":"Basic only"}}}`), page.Items[0].Reference)
	if e != nil || len(detail.Selection.Specifications) != 0 {
		t.Fatal(detail, e)
	}
}
func TestQuoteRejectsAmbiguousResults(t *testing.T) {
	p := New("fixture")
	for _, b := range []string{
		`{"content":[{"type":"text","text":"modelDesc=one\ndealMaxPrice=1\nmodelDesc=two\ndealMaxPrice=2"}]}`,
		`{"content":[{"type":"text","text":"modelDesc=one\ndealMaxPrice=1"},{"type":"text","text":"modelDesc=two\ndealMaxPrice=2"}]}`,
		`{"structuredContent":{"data":[{"modelDesc":"one","dealMaxPrice":1},{"modelDesc":"two","dealMaxPrice":2}]}}`,
	} {
		if _, e := p.parseQuote([]byte(b), "1"); e == nil {
			t.Fatal("accepted ambiguous quotes")
		}
	}
}
func TestDiscoveryUsesSameCandidateTupleAndCursor(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var req struct {
			ID     int
			Method string
			Params struct {
				Name      string
				Arguments map[string]json.RawMessage
			}
		}
		json.NewDecoder(r.Body).Decode(&req)
		if req.Method == "initialize" {
			w.Write([]byte(`{"id":1,"result":{"protocolVersion":"2025-03-26"}}`))
			return
		}
		if req.Method == "notifications/initialized" {
			w.WriteHeader(202)
			return
		}
		args := req.Params.Arguments
		switch req.Params.Name {
		case "search":
			if string(args["pageToken"]) != `"next"` {
				t.Error(args)
			}
			w.Write([]byte(`{"id":2,"result":{"structuredContent":{"items":[],"hasNext":false,"pageNo":2}}}`))
		case "product_detail":
			if string(args["infoId"]) != "2091450529925632513" || string(args["metric"]) != `"same-row"` || string(args["businessType"]) != `"CONSUMER_ELECTRONICS"` {
				t.Error(args)
			}
			w.Write([]byte(`{"id":2,"result":{"structuredContent":{"title":"Phone"}}}`))
		}
	}))
	defer server.Close()
	p := New("fixture")
	p.endpoint = server.URL
	if _, e := p.SearchProducts(context.Background(), application.ProductSearchQuery{Keyword: "phone", PageToken: "next"}); e != nil {
		t.Fatal(e)
	}
	if _, e := p.GetProductDetail(context.Background(), application.ProductReference{ID: "2091450529925632513", Metric: "same-row", BusinessType: "CONSUMER_ELECTRONICS"}); e != nil {
		t.Fatal(e)
	}
}
