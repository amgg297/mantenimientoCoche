package main

import (
	"net/http"
	"net/http/httptest"
	"net/url"
	"path/filepath"
	"strings"
	"testing"
)

func testApp(t *testing.T) (*app, http.Handler, string) {
	t.Helper()
	path := filepath.Join(t.TempDir(), "test.db")
	db, err := openDB(path)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	a := &app{db: db, templates: loadTemplates()}
	return a, a.routes(), path
}
func request(t *testing.T, h http.Handler, method, path string, values url.Values, status int) *httptest.ResponseRecorder {
	t.Helper()
	r := httptest.NewRequest(method, path, strings.NewReader(values.Encode()))
	r.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	w := httptest.NewRecorder()
	h.ServeHTTP(w, r)
	if w.Code != status {
		t.Fatalf("%s %s: got %d want %d: %s", method, path, w.Code, status, w.Body.String())
	}
	return w
}
func car(plate string) url.Values {
	return url.Values{"make": {"SEAT"}, "model": {"León"}, "plate": {plate}, "km": {"100000"}, "registered": {"2018-04-20"}, "fuel": {"Gasolina"}, "label": {"C"}}
}
func maintenance() url.Values {
	return url.Values{"date": {"2025-06-10"}, "km": {"105000"}, "price": {"123,45"}, "oil": {"on"}, "oil_filter": {"on"}, "notes": {"Aceite 5W30"}, "workshop": {"Mi taller"}}
}
func TestCRUDAndPersistence(t *testing.T) {
	a, h, path := testApp(t)
	request(t, h, "GET", "/", nil, 200)
	request(t, h, "GET", "/vehicles/new", nil, 200)
	request(t, h, "POST", "/vehicles", car("1234 abc"), 303)
	v, err := a.getVehicle("1")
	if err != nil || v.Plate != "1234ABC" {
		t.Fatalf("vehicle: %+v %v", v, err)
	}
	request(t, h, "POST", "/vehicles/1/entries", maintenance(), 303)
	e, err := a.getEntry("1", 1)
	if err != nil || e.Cents != 12345 || len(e.Tasks) != 2 {
		t.Fatalf("entry: %+v %v", e, err)
	}
	v, _ = a.getVehicle("1")
	if v.KM != 105000 {
		t.Fatal("odometer not updated")
	}
	w := request(t, h, "GET", "/vehicles/1", nil, 200)
	for _, text := range []string{"123,45 €", "Aceite de motor", "Filtro de aceite", "Mi taller", "Aceite 5W30"} {
		if !strings.Contains(w.Body.String(), text) {
			t.Errorf("missing %q", text)
		}
	}
	for _, path := range []string{"/vehicles/1/edit", "/vehicles/1/entries/new", "/vehicles/1/entries/1/edit", "/vehicles/1/delete", "/vehicles/1/entries/1/delete"} {
		request(t, h, "GET", path, nil, 200)
	}
	updated := maintenance()
	updated.Set("price", "0.01")
	updated.Set("km", "99000")
	updated.Del("oil")
	updated.Del("oil_filter")
	request(t, h, "POST", "/vehicles/1/entries/1", updated, 303)
	e, _ = a.getEntry("1", 1)
	if e.Cents != 1 || len(e.Tasks) != 0 {
		t.Fatal("entry update failed")
	}
	v, _ = a.getVehicle("1")
	if v.KM != 105000 {
		t.Fatal("historical edit reduced odometer")
	}
	form := car("1234ABC")
	form.Set("name", "Coche de casa")
	form.Set("km", "105001")
	request(t, h, "POST", "/vehicles/1", form, 303)
	if err := a.db.Close(); err != nil {
		t.Fatal(err)
	}
	db, err := openDB(path)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	a.db = db
	v, _ = a.getVehicle("1")
	if v.Name != "Coche de casa" || v.KM != 105001 {
		t.Fatal("data not persisted")
	}
	request(t, h, "POST", "/vehicles/1/entries/1/delete", url.Values{"confirm": {"yes"}}, 303)
	entries, _ := a.entries(1)
	if len(entries) != 0 {
		t.Fatal("entry not deleted")
	}
	request(t, h, "POST", "/vehicles/1/entries", maintenance(), 303)
	request(t, h, "POST", "/vehicles/1/delete", url.Values{"confirm": {"yes"}}, 303)
	var count int
	if err := db.QueryRow("SELECT COUNT(*) FROM entries").Scan(&count); err != nil || count != 0 {
		t.Fatal("cascade failed", err)
	}
	request(t, h, "GET", "/", nil, 200)
}
func TestValidationAndIsolation(t *testing.T) {
	a, h, _ := testApp(t)
	request(t, h, "POST", "/vehicles", car("1234ABC"), 303)
	request(t, h, "POST", "/vehicles", car("1234 abc"), 422)
	request(t, h, "POST", "/vehicles", car("5678DEF"), 303)
	request(t, h, "POST", "/vehicles/1/entries", maintenance(), 303)
	for _, path := range []string{"/vehicles/2/entries/1/edit", "/vehicles/2/entries/1/delete"} {
		request(t, h, "GET", path, nil, 404)
	}
	request(t, h, "POST", "/vehicles/2/entries/1", maintenance(), 404)
	request(t, h, "POST", "/vehicles/2/entries/1/delete", url.Values{"confirm": {"yes"}}, 404)
	request(t, h, "POST", "/vehicles/1/delete", nil, 400)
	request(t, h, "GET", "/vehicles/999", nil, 404)
	request(t, h, "GET", "/vehicles/1/entries/999/edit", nil, 404)
	request(t, h, "GET", "/static/app.css", nil, 200)
	request(t, h, "GET", "/static/htmx.min.js", nil, 200)
	request(t, h, "GET", "/static/pico.min.css", nil, 200)
	for _, invalid := range []struct{ key, value string }{{"price", "-1"}, {"price", "12.345"}, {"price", "NaN"}, {"date", "2025-02-30"}, {"date", "2199-01-01"}, {"km", "-5"}, {"km", "1.5"}} {
		form := maintenance()
		form.Set(invalid.key, invalid.value)
		request(t, h, "POST", "/vehicles/1/entries", form, 422)
	}
	empty := maintenance()
	empty.Del("oil")
	empty.Del("oil_filter")
	empty.Set("notes", "")
	validation := request(t, h, "POST", "/vehicles/1/entries", empty, 422)
	if validation.Header().Get("HX-Push-Url") != "false" {
		t.Fatal("validation must preserve the form URL")
	}
	for _, invalid := range []struct{ key, value string }{{"km", "100"}, {"make", ""}, {"registered", "2199-01-01"}, {"itv", "nonsense"}, {"fuel", "invalid"}, {"label", "invalid"}} {
		form := car("1234ABC")
		form.Set("km", "105000")
		form.Set(invalid.key, invalid.value)
		request(t, h, "POST", "/vehicles/1", form, 422)
	}
	list, _ := a.entries(1)
	if len(list) != 1 {
		t.Fatal("invalid submissions were saved")
	}
	request(t, h, "POST", "/vehicles/1/delete", url.Values{"confirm": {"yes"}}, 303)
	request(t, h, "GET", "/vehicles/2", nil, 200)
}
func TestEscapingAndCrossOrigin(t *testing.T) {
	_, h, _ := testApp(t)
	form := car("1234ABC")
	form.Set("name", `<script>alert(1)</script>`)
	request(t, h, "POST", "/vehicles", form, 303)
	w := request(t, h, "GET", "/vehicles/1", nil, 200)
	if strings.Contains(w.Body.String(), `<script>alert(1)</script>`) || !strings.Contains(w.Body.String(), "&lt;script&gt;") {
		t.Fatal("unsafe HTML")
	}
	r := httptest.NewRequest("POST", "http://garage.local/vehicles", strings.NewReader(car("5678DEF").Encode()))
	r.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	r.Header.Set("Origin", "https://evil.example")
	w = httptest.NewRecorder()
	h.ServeHTTP(w, r)
	if w.Code != 403 {
		t.Fatal("cross-origin write accepted", w.Code)
	}
	r.Header.Set("Origin", "http://garage.local")
	w = httptest.NewRecorder()
	h.ServeHTTP(w, r)
	if w.Code != 303 {
		t.Fatal("same-origin write rejected", w.Code)
	}
	r = httptest.NewRequest("POST", "/vehicles", strings.NewReader("name="+strings.Repeat("a", 70<<10)))
	r.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	w = httptest.NewRecorder()
	h.ServeHTTP(w, r)
	if w.Code != 400 {
		t.Fatal("oversized body accepted", w.Code)
	}
}
func TestMoney(t *testing.T) {
	for value, want := range map[string]int64{"0": 0, "0,1": 10, "12.34": 1234, "0001,05": 105, "9999999.99": 999999999} {
		got, err := parsePrice(value)
		if err != nil || got != want {
			t.Errorf("%s: got %d %v", value, got, err)
		}
		back, err := parsePrice(amount(got))
		if err != nil || back != got {
			t.Error("money roundtrip failed")
		}
	}
}
