// Repeats the last query every n time

package tracker

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"sync"
	"time"

	"github.com/Peltre/LYONWard/internal/lolesports"
)

const (
	// timeoutVuelta es lo más que puede tardar una vuelta completa: todas las
	// ligas y todas sus páginas.
	timeoutVuelta = 45 * time.Second
	// reintento es cuánto esperamos después de un error.
	reintento = time.Minute
)

// Syncer consulta la API cada cierto tiempo y deja el resultado en el Store.
// Es lo único del programa que habla con la API.
type Syncer struct {
	Client      *lolesports.Client
	Store       *Store
	Log         *slog.Logger
	TeamCode    string
	LeagueSlugs []string
	OlderPages  int           // páginas viejas a revisar por liga, para los resultados
	Interval    time.Duration // espera normal
	LiveEvery   time.Duration // espera cuando hay partido en vivo

	// leagueIDs traduce slug -> ID. Se llena en la primera vuelta que
	// funcione. Solo lo toca la goroutine de Run, así que no necesita candado.
	leagueIDs map[string]string
}

// Run repite syncOnce hasta que cancelen ctx (por ejemplo, con Ctrl+C).
// Se lanza con "go syncer.Run(ctx)".
func (s *Syncer) Run(ctx context.Context) {
	for {
		next := s.syncOnce(ctx)

		// select espera lo que pase primero: que nos pidan apagar o que se
		// cumpla el tiempo de la siguiente vuelta.
		select {
		case <-ctx.Done():
			s.Log.Info("sincronizador detenido")
			return
		case <-time.After(next):
		}
	}
}

// syncOnce hace una vuelta completa y dice cuánto esperar para la siguiente.
func (s *Syncer) syncOnce(ctx context.Context) time.Duration {
	// Un contexto hijo con límite de tiempo: si la API se cuelga, cortamos.
	// cancel libera el temporizador aunque terminemos antes.
	ctx, cancel := context.WithTimeout(ctx, timeoutVuelta)
	defer cancel()

	if s.leagueIDs == nil {
		if err := s.resolverLigas(ctx); err != nil {
			return s.fallo(err)
		}
	}

	events, err := s.fetchAll(ctx)
	if err != nil && len(events) == 0 {
		// No llegó nada: conservamos los datos que ya teníamos.
		return s.fallo(err)
	}

	snap := BuildSnapshot(events, s.TeamCode, time.Now())
	if len(snap.EnVivo)+len(snap.Proximos)+len(snap.Resultados) == 0 {
		s.Log.Warn("no encontramos al equipo en el calendario",
			"team_code", s.TeamCode, "codigos_disponibles", TeamCodes(events))
	}

	next := s.Interval
	if len(snap.EnVivo) > 0 {
		next = s.LiveEvery
	}

	if err != nil {
		// Falló alguna liga, pero otras sí respondieron: mostramos lo que hay,
		// avisamos del error y reintentamos pronto.
		snap.Error = err.Error()
		s.Log.Warn("sincronización parcial", "err", err)
		next = min(next, reintento)
	}

	s.Store.Set(snap)
	s.Log.Info("sincronización lista",
		"eventos", len(events), "en_vivo", len(snap.EnVivo),
		"proximos", len(snap.Proximos), "resultados", len(snap.Resultados),
		"siguiente", next)
	return next
}

// fallo anota el error sin borrar los datos buenos y pide reintentar pronto.
func (s *Syncer) fallo(err error) time.Duration {
	s.Log.Error("sincronización fallida", "err", err)
	s.Store.SetError("No pudimos actualizar los datos. Reintentamos en un minuto.")
	return reintento
}

// resolverLigas convierte los slugs de la configuración en IDs de la API y
// avisa cuáles no existen.
func (s *Syncer) resolverLigas(ctx context.Context) error {
	todas, err := s.Client.LeagueIDsBySlug(ctx)
	if err != nil {
		return fmt.Errorf("resolver ligas: %w", err)
	}

	ids := make(map[string]string, len(s.LeagueSlugs))
	for _, slug := range s.LeagueSlugs {
		slug = strings.ToLower(strings.TrimSpace(slug))
		if id, ok := todas[slug]; ok {
			ids[slug] = id
		} else {
			s.Log.Warn("la liga no existe en la API", "slug", slug)
		}
	}

	if len(ids) == 0 {
		disponibles := make([]string, 0, len(todas))
		for slug := range todas {
			disponibles = append(disponibles, slug)
		}
		s.Log.Warn("slugs disponibles", "slugs", disponibles)
		return fmt.Errorf("resolver ligas: ninguno de %v existe", s.LeagueSlugs)
	}

	s.leagueIDs = ids
	return nil
}

// fetchAll pide todas las ligas al mismo tiempo, una goroutine por liga.
// Devuelve los eventos que sí llegaron aunque alguna liga falle.
func (s *Syncer) fetchAll(ctx context.Context) ([]lolesports.Event, error) {
	// Pasamos el mapa a un slice para que cada liga tenga un índice fijo.
	type liga struct{ slug, id string }
	ligas := make([]liga, 0, len(s.leagueIDs))
	for slug, id := range s.leagueIDs {
		ligas = append(ligas, liga{slug, id})
	}

	// Cada goroutine escribe solo en su casilla, resultados[i]. Como ninguna
	// toca la de otra, no hace falta mutex.
	type resultado struct {
		events []lolesports.Event
		err    error
	}
	resultados := make([]resultado, len(ligas))

	var wg sync.WaitGroup
	for i, l := range ligas {
		wg.Add(1)
		go func() {
			defer wg.Done()
			events, err := s.fetchLeague(ctx, l.id)
			if err != nil {
				err = fmt.Errorf("liga %s: %w", l.slug, err)
			}
			resultados[i] = resultado{events, err}
		}()
	}
	wg.Wait() // aquí ya terminaron todas

	var events []lolesports.Event
	var errs []error
	for _, r := range resultados {
		events = append(events, r.events...)
		if r.err != nil {
			errs = append(errs, r.err)
		}
	}
	// errors.Join devuelve nil si la lista está vacía.
	return events, errors.Join(errs...)
}

// fetchLeague trae la página actual de una liga y OlderPages páginas hacia
// atrás, siguiendo el token Pages.Older.
func (s *Syncer) fetchLeague(ctx context.Context, leagueID string) ([]lolesports.Event, error) {
	var events []lolesports.Event
	token := "" // vacío = página actual

	for pagina := 0; pagina <= s.OlderPages; pagina++ {
		sched, err := s.Client.GetSchedule(ctx, leagueID, token)
		if err != nil {
			return events, err
		}
		events = append(events, sched.Events...)

		if sched.Pages.Older == nil || *sched.Pages.Older == "" {
			break // ya no hay páginas más viejas
		}
		token = *sched.Pages.Older
	}
	return events, nil
}
