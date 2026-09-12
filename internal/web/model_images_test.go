package web

import (
	"bytes"
	"image"
	"image/png"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
)

func testModelPNG(t *testing.T) []byte {
	t.Helper()
	var b bytes.Buffer
	if err := png.Encode(&b, image.NewRGBA(image.Rect(0, 0, 2, 2))); err != nil {
		t.Fatal(err)
	}
	return b.Bytes()
}
func uploadTestModelImage(t *testing.T, h http.Handler, model string, session, csrf *http.Cookie, data []byte, action string) *httptest.ResponseRecorder {
	t.Helper()
	var b bytes.Buffer
	writer := multipart.NewWriter(&b)
	if csrf != nil {
		_ = writer.WriteField("csrf_token", csrf.Value)
	}
	_ = writer.WriteField("action", action)
	if data != nil {
		f, e := writer.CreateFormFile("image", "product.png")
		if e != nil {
			t.Fatal(e)
		}
		_, _ = f.Write(data)
	}
	if e := writer.Close(); e != nil {
		t.Fatal(e)
	}
	r := httptest.NewRequest(http.MethodPost, "/admin/catalog/models/"+model+"/image", &b)
	r.Header.Set("Content-Type", writer.FormDataContentType())
	if session != nil {
		r.AddCookie(session)
	}
	if csrf != nil {
		r.AddCookie(csrf)
	}
	out := httptest.NewRecorder()
	h.ServeHTTP(out, r)
	return out
}
func TestModelImagesUploadReadReplaceClear(t *testing.T) {
	h := newTestHandler(t)
	page := request(t, h, "GET", "/setup", nil, nil)
	csrf := responseCookie(t, page, csrfCookie)
	setup := request(t, h, "POST", "/setup", url.Values{"csrf_token": {csrf.Value}, "tenant_name": {"Images"}, "base_currency": {"CNY"}, "username": {"owner"}, "password": {"owner secure password"}}, []*http.Cookie{csrf})
	session := responseCookie(t, setup, sessionCookie)
	cookies := []*http.Cookie{session, csrf}
	request(t, h, "POST", "/admin/catalog/categories", url.Values{"csrf_token": {csrf.Value}, "name": {"Phones"}, "icon_key": {"smartphone"}}, cookies)
	catalog := request(t, h, "GET", "/admin/catalog", nil, cookies)
	category := optionID(t, catalog.Body.String(), "Phones")
	request(t, h, "POST", "/admin/catalog/models", url.Values{"csrf_token": {csrf.Value}, "name": {"Image Phone"}, "category_id": {category}}, cookies)
	catalog = request(t, h, "GET", "/assets/new", nil, cookies)
	model := optionID(t, catalog.Body.String(), "Phones / Image Phone")
	for _, data := range [][]byte{[]byte("<svg/>"), nil} {
		if got := uploadTestModelImage(t, h, model, session, csrf, data, ""); got.Code != 422 {
			t.Fatalf("bad image accepted: %d", got.Code)
		}
	}
	if got := uploadTestModelImage(t, h, model, session, nil, testModelPNG(t), ""); got.Code != 403 {
		t.Fatalf("CSRF: %d", got.Code)
	}
	for i := 0; i < 2; i++ {
		if got := uploadTestModelImage(t, h, model, session, csrf, testModelPNG(t), ""); got.Code != 303 {
			t.Fatalf("upload %d: %d %s", i, got.Code, got.Body.String())
		}
	}
	got := request(t, h, "GET", "/models/"+model+"/image", nil, cookies)
	if got.Code != 200 || got.Header().Get("Content-Type") != "image/png" || !bytes.Equal(got.Body.Bytes(), testModelPNG(t)) {
		t.Fatalf("image response: %d", got.Code)
	}
	if !strings.Contains(got.Header().Get("Cache-Control"), "private") {
		t.Fatal("public cache")
	}
	editor := request(t, h, "GET", "/admin/catalog/models/"+model+"/image", nil, cookies)
	if editor.Code != 200 || !strings.Contains(editor.Body.String(), "multipart/form-data") || !strings.Contains(editor.Body.String(), "移除图片") {
		t.Fatalf("editor: %d", editor.Code)
	}
	if got := uploadTestModelImage(t, h, model, session, csrf, nil, "clear"); got.Code != 303 {
		t.Fatalf("clear: %d", got.Code)
	}
	if got := request(t, h, "GET", "/models/"+model+"/image", nil, cookies); got.Code != 404 {
		t.Fatalf("cleared image still visible: %d", got.Code)
	}
}
