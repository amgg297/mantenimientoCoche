package main

import (
	"bytes"
	"context"
	"embed"
	"errors"
	"html/template"
	"io/fs"
	"log"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"time"
	_ "time/tzdata"
)

//go:embed templates/*.html static/*
var assets embed.FS

var madrid, _ = time.LoadLocation("Europe/Madrid")

func today() string { return time.Now().In(madrid).Format("2006-01-02") }

func main() {
	dbPath := env("DB_PATH", "data/mantenimiento.db")
	if err := os.MkdirAll(filepath.Dir(dbPath), 0700); err != nil {
		log.Fatal(err)
	}
	db, err := openDB(dbPath)
	if err != nil {
		log.Fatal(err)
	}
	defer db.Close()
	a := &app{db: db, templates: loadTemplates()}
	srv := &http.Server{Addr: env("ADDR", "127.0.0.1:8080"), Handler: a.routes(), ReadHeaderTimeout: 5 * time.Second, ReadTimeout: 15 * time.Second, WriteTimeout: 30 * time.Second, IdleTimeout: 60 * time.Second}
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, terminationSignal)
	defer stop()
	go func() {
		<-ctx.Done()
		shutdown, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		if err := srv.Shutdown(shutdown); err != nil {
			log.Print(err)
		}
	}()
	log.Printf("Mi garaje: http://%s", srv.Addr)
	if err := srv.ListenAndServe(); !errors.Is(err, http.ErrServerClosed) {
		log.Fatal(err)
	}
}

func env(key, fallback string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return fallback
}

func loadTemplates() *template.Template {
	return template.Must(template.New("").Funcs(template.FuncMap{
		"money": func(c int64) string { return strings.ReplaceAll(amount(c), ".", ",") + " €" },
		"date": func(s string) string {
			if t, err := time.Parse("2006-01-02", s); err == nil {
				return t.Format("02/01/2006")
			}
			return "Sin indicar"
		},
		"number": func(n int64) string {
			s := integer(n)
			for i := len(s) - 3; i > 0; i -= 3 {
				s = s[:i] + "." + s[i:]
			}
			return s
		},
		"due": func(s string) string {
			if s == "" {
				return "Sin fecha"
			}
			if s < today() {
				return "Vencida"
			}
			if s <= time.Now().In(madrid).AddDate(0, 0, 30).Format("2006-01-02") {
				return "Próximamente"
			}
			return "Al día"
		},
		"has": func(values []string, key string) bool {
			for _, v := range values {
				if v == key {
					return true
				}
			}
			return false
		},
	}).ParseFS(assets, "templates/*.html"))
}

func (a *app) routes() http.Handler {
	mux := http.NewServeMux()
	static, _ := fs.Sub(assets, "static")
	mux.Handle("GET /static/", http.StripPrefix("/static/", http.FileServerFS(static)))
	mux.HandleFunc("GET /{$}", a.home)
	mux.HandleFunc("GET /vehicles/new", a.vehicleForm)
	mux.HandleFunc("POST /vehicles", a.saveVehicle)
	mux.HandleFunc("GET /vehicles/{id}", a.showVehicle)
	mux.HandleFunc("GET /vehicles/{id}/edit", a.vehicleForm)
	mux.HandleFunc("POST /vehicles/{id}", a.saveVehicle)
	mux.HandleFunc("GET /vehicles/{id}/delete", a.confirmDelete)
	mux.HandleFunc("POST /vehicles/{id}/delete", a.deleteVehicle)
	mux.HandleFunc("GET /vehicles/{id}/entries/new", a.entryForm)
	mux.HandleFunc("POST /vehicles/{id}/entries", a.saveEntry)
	mux.HandleFunc("GET /vehicles/{id}/entries/{entry}/edit", a.entryForm)
	mux.HandleFunc("POST /vehicles/{id}/entries/{entry}", a.saveEntry)
	mux.HandleFunc("GET /vehicles/{id}/entries/{entry}/delete", a.confirmDelete)
	mux.HandleFunc("POST /vehicles/{id}/entries/{entry}/delete", a.deleteEntry)
	protected := http.NewCrossOriginProtection().Handler(mux)
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("X-Content-Type-Options", "nosniff")
		w.Header().Set("Referrer-Policy", "same-origin")
		w.Header().Set("Content-Security-Policy", "default-src 'self'; script-src 'self'; style-src 'self' 'unsafe-inline'; img-src 'self' data:; object-src 'none'; base-uri 'self'; frame-ancestors 'none'; form-action 'self'")
		w.Header().Set("Cache-Control", "no-store")
		r.Body = http.MaxBytesReader(w, r.Body, 64<<10)
		protected.ServeHTTP(w, r)
	})
}

func (a *app) render(w http.ResponseWriter, p page, status int) {
	var err error
	p.Vehicles, err = a.vehicles()
	if err != nil {
		serverError(w, err)
		return
	}
	p.Tasks, p.Fuels, p.Labels, p.Today = tasks, fuels, labels, today()
	var buf bytes.Buffer
	if err := a.templates.ExecuteTemplate(&buf, "layout", p); err != nil {
		serverError(w, err)
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if status == http.StatusUnprocessableEntity {
		w.Header().Set("HX-Push-Url", "false")
	}
	w.WriteHeader(status)
	_, _ = buf.WriteTo(w)
}

func serverError(w http.ResponseWriter, err error) {
	log.Print(err)
	http.Error(w, "No se ha podido completar la operación. Inténtalo de nuevo.", http.StatusInternalServerError)
}
