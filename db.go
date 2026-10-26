package main

import (
	"database/sql"
	"encoding/json"
	"html/template"
	"strconv"

	_ "modernc.org/sqlite"
)

type app struct {
	db        *sql.DB
	templates *template.Template
}
type vehicle struct {
	ID, KM                                                                                 int64
	Name, Plate, Make, Model, VIN, Registered, ITV, Fuel, Label, Insurer, Insurance, Notes string
}

func (v vehicle) Title() string {
	if v.Name != "" {
		return v.Name
	}
	return v.Make + " " + v.Model
}
func (v vehicle) URL() string { return "/vehicles/" + integer(v.ID) }

type entry struct {
	ID, VehicleID, KM, Cents int64
	Date, Workshop, Notes    string
	Tasks                    []string
}

func (e entry) URL() string { return "/vehicles/" + integer(e.VehicleID) + "/entries/" + integer(e.ID) }

type task struct{ Key, Label string }

var tasks = []task{
	{"oil", "Aceite de motor"}, {"oil_filter", "Filtro de aceite"}, {"air_filter", "Filtro de aire"},
	{"cabin_filter", "Filtro de habitáculo"}, {"fuel_filter", "Filtro de combustible"}, {"tyres", "Neumáticos"},
	{"brake_pads", "Pastillas de freno"}, {"brake_discs", "Discos de freno"}, {"brake_fluid", "Líquido de frenos"},
	{"coolant", "Refrigerante"}, {"battery", "Batería"}, {"timing", "Distribución"},
	{"spark_plugs", "Bujías / calentadores"}, {"wipers", "Limpiaparabrisas"}, {"itv", "ITV realizada"},
}
var fuels = []string{"Gasolina", "Diésel", "Híbrido", "Híbrido enchufable", "Eléctrico", "GLP", "GNC", "Otro"}
var labels = []string{"Sin distintivo", "B", "C", "ECO", "0 emisiones"}

type page struct {
	View, Error, Today, Price, DeleteAction string
	Vehicle                                 vehicle
	Entry                                   entry
	Vehicles                                []vehicle
	Entries                                 []entry
	Tasks                                   []task
	Fuels, Labels                           []string
	Total                                   int64
	DeleteEntry                             bool
}

func integer(n int64) string { return strconv.FormatInt(n, 10) }
func amount(n int64) string {
	return integer(n/100) + "." + string([]byte{'0' + byte(n%100/10), '0' + byte(n%10)})
}

func openDB(path string) (*sql.DB, error) {
	db, err := sql.Open("sqlite", path)
	if err != nil {
		return nil, err
	}
	db.SetMaxOpenConns(1)
	_, err = db.Exec(`PRAGMA foreign_keys = ON; PRAGMA journal_mode = WAL; PRAGMA busy_timeout = 5000;
CREATE TABLE IF NOT EXISTS vehicles (
 id INTEGER PRIMARY KEY, name TEXT NOT NULL, plate TEXT NOT NULL UNIQUE COLLATE NOCASE,
 make TEXT NOT NULL, model TEXT NOT NULL, km INTEGER NOT NULL CHECK(km >= 0),
 vin TEXT NOT NULL, registered TEXT NOT NULL, itv TEXT NOT NULL, fuel TEXT NOT NULL,
 label TEXT NOT NULL, insurer TEXT NOT NULL, insurance TEXT NOT NULL, notes TEXT NOT NULL
);
CREATE TABLE IF NOT EXISTS entries (
 id INTEGER PRIMARY KEY, vehicle_id INTEGER NOT NULL REFERENCES vehicles(id) ON DELETE CASCADE,
 date TEXT NOT NULL, km INTEGER NOT NULL CHECK(km >= 0), cents INTEGER NOT NULL CHECK(cents >= 0),
 tasks TEXT NOT NULL, workshop TEXT NOT NULL, notes TEXT NOT NULL
);
CREATE INDEX IF NOT EXISTS entries_vehicle_date ON entries(vehicle_id, date DESC, id DESC);`)
	if err != nil {
		db.Close()
		return nil, err
	}
	return db, nil
}

const vehicleColumns = `id, name, plate, make, model, km, vin, registered, itv, fuel, label, insurer, insurance, notes`

type scanner interface{ Scan(...any) error }

func scanVehicle(row scanner) (v vehicle, err error) {
	err = row.Scan(&v.ID, &v.Name, &v.Plate, &v.Make, &v.Model, &v.KM, &v.VIN, &v.Registered, &v.ITV, &v.Fuel, &v.Label, &v.Insurer, &v.Insurance, &v.Notes)
	return
}
func (a *app) vehicles() ([]vehicle, error) {
	rows, err := a.db.Query("SELECT " + vehicleColumns + " FROM vehicles ORDER BY id")
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var list []vehicle
	for rows.Next() {
		v, err := scanVehicle(rows)
		if err != nil {
			return nil, err
		}
		list = append(list, v)
	}
	return list, rows.Err()
}
func (a *app) getVehicle(id string) (vehicle, error) {
	return scanVehicle(a.db.QueryRow("SELECT "+vehicleColumns+" FROM vehicles WHERE id = ?", id))
}

const entryColumns = `id, vehicle_id, date, km, cents, tasks, workshop, notes`

func scanEntry(row scanner) (e entry, err error) {
	var raw string
	err = row.Scan(&e.ID, &e.VehicleID, &e.Date, &e.KM, &e.Cents, &raw, &e.Workshop, &e.Notes)
	if err == nil {
		err = json.Unmarshal([]byte(raw), &e.Tasks)
	}
	return
}
func (a *app) getEntry(id string, vehicleID int64) (entry, error) {
	return scanEntry(a.db.QueryRow("SELECT "+entryColumns+" FROM entries WHERE id = ? AND vehicle_id = ?", id, vehicleID))
}
func (a *app) entries(id int64) ([]entry, error) {
	rows, err := a.db.Query("SELECT "+entryColumns+" FROM entries WHERE vehicle_id = ? ORDER BY date DESC, id DESC", id)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var list []entry
	for rows.Next() {
		e, err := scanEntry(rows)
		if err != nil {
			return nil, err
		}
		list = append(list, e)
	}
	return list, rows.Err()
}
