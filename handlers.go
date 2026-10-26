package main

import (
	"database/sql"
	"encoding/json"
	"errors"
	"net/http"
	"regexp"
	"strconv"
	"strings"
	"time"
)

func (a *app) home(w http.ResponseWriter, r *http.Request) {
	list, err := a.vehicles()
	if err != nil {
		serverError(w, err)
		return
	}
	if len(list) > 0 {
		http.Redirect(w, r, list[0].URL(), http.StatusSeeOther)
		return
	}
	a.render(w, page{View: "empty"}, 200)
}
func (a *app) findVehicle(w http.ResponseWriter, r *http.Request) (vehicle, bool) {
	v, err := a.getVehicle(r.PathValue("id"))
	if errors.Is(err, sql.ErrNoRows) {
		http.NotFound(w, r)
		return v, false
	}
	if err != nil {
		serverError(w, err)
		return v, false
	}
	return v, true
}
func (a *app) findEntry(w http.ResponseWriter, r *http.Request, v vehicle) (entry, bool) {
	e, err := a.getEntry(r.PathValue("entry"), v.ID)
	if errors.Is(err, sql.ErrNoRows) {
		http.NotFound(w, r)
		return e, false
	}
	if err != nil {
		serverError(w, err)
		return e, false
	}
	return e, true
}
func (a *app) showVehicle(w http.ResponseWriter, r *http.Request) {
	v, ok := a.findVehicle(w, r)
	if !ok {
		return
	}
	entries, err := a.entries(v.ID)
	if err != nil {
		serverError(w, err)
		return
	}
	var total int64
	for _, e := range entries {
		total += e.Cents
	}
	a.render(w, page{View: "vehicle", Vehicle: v, Entries: entries, Total: total}, 200)
}
func (a *app) vehicleForm(w http.ResponseWriter, r *http.Request) {
	var v vehicle
	if r.PathValue("id") != "" {
		var ok bool
		v, ok = a.findVehicle(w, r)
		if !ok {
			return
		}
	}
	a.render(w, page{View: "vehicle-form", Vehicle: v}, 200)
}
func parseForm(w http.ResponseWriter, r *http.Request) bool {
	if err := r.ParseForm(); err != nil {
		http.Error(w, "Formulario inválido o demasiado grande.", 400)
		return false
	}
	return true
}
func field(r *http.Request, name string) string { return strings.TrimSpace(r.PostForm.Get(name)) }
func dateValid(s string) bool {
	t, err := time.Parse("2006-01-02", s)
	return err == nil && t.Year() >= 1900 && t.Year() <= 2200
}
func optionValid(value string, options []string) bool {
	if value == "" {
		return true
	}
	for _, s := range options {
		if s == value {
			return true
		}
	}
	return false
}
func parseKM(value string) (int64, error) {
	n, err := strconv.ParseInt(value, 10, 64)
	if err != nil || n < 0 || n > 10000000 {
		return 0, errors.New("Introduce un kilometraje entre 0 y 10.000.000.")
	}
	return n, nil
}

var platePattern = regexp.MustCompile(`^[A-Z0-9-]{2,15}$`)
var pricePattern = regexp.MustCompile(`^\d{1,7}([.,]\d{1,2})?$`)

func parsePrice(value string) (int64, error) {
	if !pricePattern.MatchString(value) {
		return 0, errors.New("Introduce un precio válido, sin separadores de miles y con un máximo de dos decimales.")
	}
	parts := strings.Split(strings.ReplaceAll(value, ",", "."), ".")
	whole, _ := strconv.ParseInt(parts[0], 10, 64)
	var decimals int64
	if len(parts) == 2 {
		decimals, _ = strconv.ParseInt((parts[1] + "0")[:2], 10, 64)
	}
	return whole*100 + decimals, nil
}
func (a *app) saveVehicle(w http.ResponseWriter, r *http.Request) {
	var id int64
	if r.PathValue("id") != "" {
		v, ok := a.findVehicle(w, r)
		if !ok {
			return
		}
		id = v.ID
	}
	if !parseForm(w, r) {
		return
	}
	v := vehicle{ID: id, Name: field(r, "name"), Plate: strings.ToUpper(strings.ReplaceAll(field(r, "plate"), " ", "")), Make: field(r, "make"), Model: field(r, "model"), VIN: strings.ToUpper(field(r, "vin")), Registered: field(r, "registered"), ITV: field(r, "itv"), Fuel: field(r, "fuel"), Label: field(r, "label"), Insurer: field(r, "insurer"), Insurance: field(r, "insurance"), Notes: field(r, "notes")}
	var validation string
	km, err := parseKM(field(r, "km"))
	v.KM = km
	if err != nil {
		validation = err.Error()
	}
	if v.Make == "" || v.Model == "" || !platePattern.MatchString(v.Plate) {
		validation = "Indica marca, modelo y una matrícula válida (letras, números o guiones)."
	}
	for _, s := range []string{v.Registered, v.ITV, v.Insurance} {
		if s != "" && !dateValid(s) {
			validation = "Revisa las fechas indicadas."
		}
	}
	if v.Registered > today() {
		validation = "La matriculación no puede estar en el futuro."
	}
	if !optionValid(v.Fuel, fuels) || !optionValid(v.Label, labels) {
		validation = "Selecciona un combustible y distintivo válidos."
	}
	for _, s := range []string{v.Name, v.Make, v.Model, v.Insurer} {
		if len([]rune(s)) > 100 {
			validation = "Los campos de texto corto admiten hasta 100 caracteres."
		}
	}
	if len(v.VIN) > 30 || len([]rune(v.Notes)) > 5000 {
		validation = "El bastidor admite hasta 30 caracteres y las observaciones hasta 5.000."
	}
	var maxKM int64
	if err := a.db.QueryRow("SELECT COALESCE(MAX(km),0) FROM entries WHERE vehicle_id = ?", id).Scan(&maxKM); err != nil {
		serverError(w, err)
		return
	}
	if km < maxKM {
		validation = "El kilometraje actual no puede ser menor que el de un mantenimiento registrado (" + integer(maxKM) + " km)."
	}
	if validation != "" {
		a.render(w, page{View: "vehicle-form", Vehicle: v, Error: validation}, 422)
		return
	}
	args := []any{v.Name, v.Plate, v.Make, v.Model, v.KM, v.VIN, v.Registered, v.ITV, v.Fuel, v.Label, v.Insurer, v.Insurance, v.Notes}
	if id == 0 {
		var result sql.Result
		result, err = a.db.Exec(`INSERT INTO vehicles (name,plate,make,model,km,vin,registered,itv,fuel,label,insurer,insurance,notes) VALUES (?,?,?,?,?,?,?,?,?,?,?,?,?)`, args...)
		if err == nil {
			id, err = result.LastInsertId()
		}
	} else {
		_, err = a.db.Exec(`UPDATE vehicles SET name=?,plate=?,make=?,model=?,km=?,vin=?,registered=?,itv=?,fuel=?,label=?,insurer=?,insurance=?,notes=? WHERE id=?`, append(args, id)...)
	}
	if err != nil {
		if strings.Contains(err.Error(), "UNIQUE constraint failed: vehicles.plate") {
			a.render(w, page{View: "vehicle-form", Vehicle: v, Error: "Ya existe un vehículo con esa matrícula."}, 422)
			return
		}
		serverError(w, err)
		return
	}
	http.Redirect(w, r, "/vehicles/"+integer(id), http.StatusSeeOther)
}
func (a *app) entryForm(w http.ResponseWriter, r *http.Request) {
	v, ok := a.findVehicle(w, r)
	if !ok {
		return
	}
	e := entry{VehicleID: v.ID, Date: today(), KM: v.KM}
	price := ""
	if r.PathValue("entry") != "" {
		e, ok = a.findEntry(w, r, v)
		if !ok {
			return
		}
		price = amount(e.Cents)
	}
	a.render(w, page{View: "entry-form", Vehicle: v, Entry: e, Price: price}, 200)
}
func (a *app) saveEntry(w http.ResponseWriter, r *http.Request) {
	v, ok := a.findVehicle(w, r)
	if !ok {
		return
	}
	var id int64
	if r.PathValue("entry") != "" {
		old, ok := a.findEntry(w, r, v)
		if !ok {
			return
		}
		id = old.ID
	}
	if !parseForm(w, r) {
		return
	}
	e := entry{ID: id, VehicleID: v.ID, Date: field(r, "date"), Workshop: field(r, "workshop"), Notes: field(r, "notes"), Tasks: []string{}}
	for _, t := range tasks {
		if r.PostForm.Get(t.Key) == "on" {
			e.Tasks = append(e.Tasks, t.Key)
		}
	}
	var validation string
	var err error
	e.KM, err = parseKM(field(r, "km"))
	if err != nil {
		validation = err.Error()
	}
	price := field(r, "price")
	e.Cents, err = parsePrice(price)
	if err != nil {
		validation = err.Error()
	}
	if !dateValid(e.Date) || e.Date > today() {
		validation = "Indica una fecha válida que no esté en el futuro."
	}
	if len(e.Tasks) == 0 && e.Notes == "" {
		validation = "Marca al menos una tarea o describe el trabajo en observaciones."
	}
	if len([]rune(e.Notes)) > 5000 || len([]rune(e.Workshop)) > 100 {
		validation = "El taller admite hasta 100 caracteres y las observaciones hasta 5.000."
	}
	if validation != "" {
		a.render(w, page{View: "entry-form", Vehicle: v, Entry: e, Price: price, Error: validation}, 422)
		return
	}
	raw, _ := json.Marshal(e.Tasks)
	tx, err := a.db.Begin()
	if err != nil {
		serverError(w, err)
		return
	}
	defer tx.Rollback()
	if id == 0 {
		_, err = tx.Exec(`INSERT INTO entries (vehicle_id,date,km,cents,tasks,workshop,notes) VALUES (?,?,?,?,?,?,?)`, v.ID, e.Date, e.KM, e.Cents, string(raw), e.Workshop, e.Notes)
	} else {
		_, err = tx.Exec(`UPDATE entries SET date=?,km=?,cents=?,tasks=?,workshop=?,notes=? WHERE id=? AND vehicle_id=?`, e.Date, e.KM, e.Cents, string(raw), e.Workshop, e.Notes, id, v.ID)
	}
	if err == nil {
		_, err = tx.Exec("UPDATE vehicles SET km = MAX(km,?) WHERE id = ?", e.KM, v.ID)
	}
	if err == nil {
		err = tx.Commit()
	}
	if err != nil {
		serverError(w, err)
		return
	}
	http.Redirect(w, r, v.URL(), http.StatusSeeOther)
}
func (a *app) confirmDelete(w http.ResponseWriter, r *http.Request) {
	v, ok := a.findVehicle(w, r)
	if !ok {
		return
	}
	p := page{View: "delete", Vehicle: v, DeleteAction: v.URL() + "/delete"}
	if r.PathValue("entry") != "" {
		e, ok := a.findEntry(w, r, v)
		if !ok {
			return
		}
		p.Entry = e
		p.DeleteEntry = true
		p.DeleteAction = e.URL() + "/delete"
	}
	a.render(w, p, 200)
}
func (a *app) deleteVehicle(w http.ResponseWriter, r *http.Request) {
	v, ok := a.findVehicle(w, r)
	if !ok {
		return
	}
	if !parseForm(w, r) {
		return
	}
	if field(r, "confirm") != "yes" {
		http.Error(w, "Confirma la eliminación.", 400)
		return
	}
	if _, err := a.db.Exec("DELETE FROM vehicles WHERE id=?", v.ID); err != nil {
		serverError(w, err)
		return
	}
	http.Redirect(w, r, "/", http.StatusSeeOther)
}
func (a *app) deleteEntry(w http.ResponseWriter, r *http.Request) {
	v, ok := a.findVehicle(w, r)
	if !ok {
		return
	}
	e, ok := a.findEntry(w, r, v)
	if !ok {
		return
	}
	if !parseForm(w, r) {
		return
	}
	if field(r, "confirm") != "yes" {
		http.Error(w, "Confirma la eliminación.", 400)
		return
	}
	if _, err := a.db.Exec("DELETE FROM entries WHERE id=? AND vehicle_id=?", e.ID, v.ID); err != nil {
		serverError(w, err)
		return
	}
	http.Redirect(w, r, v.URL(), http.StatusSeeOther)
}
