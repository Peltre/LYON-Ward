// Routing and each page gets created

// Package web sirve el sitio: rutas, templates y archivos estáticos. Solo lee
// el Snapshot del Store; nunca llama a la API.
package web

import (
	"bytes"
	"embed"
	"fmt"
	"html/template"
	"log/slog"
	"net/http"
	"time"

	"github.com/Peltre/LYONWard/internal/tracker"
)

// archivos trae los templates y el CSS metidos dentro del binario al compilar.
// La línea //go:embed de arriba le dice al compilador qué archivos copiar; así
// el programa no depende de encontrar la carpeta en disco al correr.
//
//go:embed templates/*.html static/*
var archivos embed.FS

// templatesRequeridos son los que el servidor pinta. Si falta alguno, New
// falla al arrancar en vez de que la página truene con la primera visita.
var templatesRequeridos = []string{"index", "partidos"}

// Server guarda lo que necesitan los handlers.
type Server struct {
	store *tracker.Store
	log   *slog.Logger
	tmpl  *template.Template
}

// New lee los templates una sola vez y devuelve el servidor listo.
func New(store *tracker.Store, log *slog.Logger) (*Server, error) {
	// FuncMap son funciones extra que los templates pueden llamar, por ejemplo
	// {{iso .Inicio}}. Tienen que registrarse ANTES de leer los archivos.
	funcs := template.FuncMap{
		"iso":      iso,
		"fechaUTC": fechaUTC,
		"plural":   plural,
	}

	tmpl, err := template.New("").Funcs(funcs).ParseFS(archivos, "templates/*.html")
	if err != nil {
		return nil, fmt.Errorf("web: leer templates: %w", err)
	}
	for _, nombre := range templatesRequeridos {
		if tmpl.Lookup(nombre) == nil {
			return nil, fmt.Errorf("web: ningún archivo define el template %q", nombre)
		}
	}

	return &Server{store: store, log: log, tmpl: tmpl}, nil
}

// Handler arma las rutas. Desde Go 1.22 el ServeMux estándar entiende el
// método ("GET ...") y comodines, así que no hace falta un router externo.
func (s *Server) Handler() http.Handler {
	mux := http.NewServeMux()

	// {$} significa "exactamente /": sin él, "/" atraparía cualquier ruta.
	mux.HandleFunc("GET /{$}", s.pagina("index"))
	mux.HandleFunc("GET /partials/partidos", s.pagina("partidos"))

	// El FS embebido ya tiene la carpeta "static/", así que la URL
	// /static/styles.css cae justo en el archivo static/styles.css.
	mux.Handle("GET /static/", http.FileServerFS(archivos))

	mux.HandleFunc("GET /healthz", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/plain; charset=utf-8")
		fmt.Fprintln(w, "ok")
	})

	return mux
}

// pagina devuelve un handler que pinta el template indicado con el Snapshot
// actual. Es una función que fabrica funciones: así las dos rutas comparten el
// mismo código y solo cambia el nombre del template.
func (s *Server) pagina(nombre string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		snap := s.store.Get()

		// Pintamos primero en un buffer. Si el template falla a la mitad, el
		// visitante recibe un error limpio y no media página rota con
		// código 200.
		var buf bytes.Buffer
		if err := s.tmpl.ExecuteTemplate(&buf, nombre, snap); err != nil {
			s.log.Error("no se pudo pintar el template", "template", nombre, "err", err)
			http.Error(w, "Algo salió mal al armar la página.", http.StatusInternalServerError)
			return
		}

		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		// Los datos cambian solos cada minuto: que el navegador no guarde copia.
		w.Header().Set("Cache-Control", "no-store")
		if _, err := buf.WriteTo(w); err != nil {
			// El visitante cerró la pestaña a medio camino; no hay nada que hacer.
			s.log.Debug("no se pudo enviar la respuesta", "err", err)
		}
	}
}

// iso da la fecha en UTC con formato RFC3339, que es lo que espera el atributo
// datetime de <time> y lo que el JavaScript convierte a la hora local.
func iso(t time.Time) string {
	return t.UTC().Format(time.RFC3339)
}

// meses va en español a mano: time.Format solo sabe los nombres en inglés.
var meses = [...]string{"ene", "feb", "mar", "abr", "may", "jun", "jul", "ago", "sep", "oct", "nov", "dic"}

// fechaUTC es el texto de respaldo dentro de <time> por si el JavaScript no
// corre. Por ejemplo: "14 mar 2027, 18:00 UTC".
func fechaUTC(t time.Time) string {
	t = t.UTC()
	return fmt.Sprintf("%d %s %d, %02d:%02d UTC", t.Day(), meses[t.Month()-1], t.Year(), t.Hour(), t.Minute())
}

// plural elige la palabra según la cantidad. Solo devuelve la palabra; el
// número lo pone el template: {{.Victorias}} {{plural .Victorias "victoria" "victorias"}}.
func plural(n int, uno, varios string) string {
	if n == 1 {
		return uno
	}
	return varios
}
