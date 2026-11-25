// Turns API events into LYON matches

// Package tracker traduce los datos crudos de la API al modelo propio del
// sitio: partidos de un equipo, clasificados y listos para mostrar.
package tracker

import (
	"fmt"
	"slices"
	"strings"
	"time"

	"github.com/Peltre/LYONWard/internal/lolesports"
)

// Estados de un partido, ya en nuestro idioma.
const (
	EstadoEnVivo    = "en vivo"
	EstadoPorJugar  = "por jugar"
	EstadoTerminado = "terminado"
)

// margenEnVivo es cuánto tiempo seguimos considerando "en vivo" un partido cuya
// hora ya pasó pero que la API todavía marca como no empezado. Más viejo que
// eso, lo tomamos como un evento fantasma y lo descartamos.
const margenEnVivo = 4 * time.Hour

// Equipo es uno de los dos lados de un partido, aplanado para la plantilla.
type Equipo struct {
	Nombre string
	Codigo string
	Imagen string
	Juegos int // mapas ganados en este partido
}

// Partido es un enfrentamiento visto desde nuestro equipo: siempre hay un
// Nuestro y un Rival, nunca "equipo 1" y "equipo 2".
type Partido struct {
	ID      string
	Inicio  time.Time
	Liga    string
	Bloque  string // "Week 3", "Final"...
	Formato string // "Bo1", "Bo3", "Bo5"
	Estado  string
	Nuestro Equipo
	Rival   Equipo
	Ganamos bool // solo significa algo si Estado es EstadoTerminado
}

// Snapshot es la foto completa que el sitio muestra. Se construye entera cada
// vez: nadie modifica una ya guardada.
type Snapshot struct {
	EnVivo      []Partido
	Proximos    []Partido
	Resultados  []Partido
	Victorias   int
	Derrotas    int
	Actualizado time.Time
	Error       string
}

// Proximo es el siguiente partido, o nil si no hay ninguno agendado.
// Devuelve puntero para que la plantilla pueda preguntar si existe.
func (s Snapshot) Proximo() *Partido {
	if len(s.Proximos) == 0 {
		return nil
	}
	return &s.Proximos[0]
}

// Despues son los partidos agendados que vienen tras el próximo.
func (s Snapshot) Despues() []Partido {
	if len(s.Proximos) < 2 {
		return nil
	}
	return s.Proximos[1:]
}

// HayDatos dice si ya alcanzamos a traer algo de la API.
func (s Snapshot) HayDatos() bool {
	return !s.Actualizado.IsZero()
}

// BuildSnapshot filtra los eventos donde juega teamCode y los acomoda.
// teamCode se compara contra el código y el nombre del equipo, sin distinguir
// mayúsculas, porque la variable de entorno la escribe una persona.
func BuildSnapshot(events []lolesports.Event, teamCode string, now time.Time) Snapshot {
	snap := Snapshot{Actualizado: now}

	for _, ev := range events {
		p, ok := convertir(ev, teamCode, now)
		if !ok {
			continue
		}

		switch p.Estado {
		case EstadoEnVivo:
			snap.EnVivo = append(snap.EnVivo, p)
		case EstadoPorJugar:
			snap.Proximos = append(snap.Proximos, p)
		case EstadoTerminado:
			snap.Resultados = append(snap.Resultados, p)
			if p.Ganamos {
				snap.Victorias++
			} else {
				snap.Derrotas++
			}
		}
	}

	// Lo que viene primero arriba: los próximos del más cercano al más lejano,
	// los resultados del más reciente al más viejo.
	porFecha := func(a, b Partido) int { return a.Inicio.Compare(b.Inicio) }
	slices.SortFunc(snap.EnVivo, porFecha)
	slices.SortFunc(snap.Proximos, porFecha)
	slices.SortFunc(snap.Resultados, func(a, b Partido) int { return porFecha(b, a) })

	return snap
}

// convertir pasa un evento de la API a Partido. El segundo valor es false
// cuando el evento no nos sirve: no es un partido, no juega nuestro equipo, o
// está agendado para una hora que ya pasó hace rato.
func convertir(ev lolesports.Event, teamCode string, now time.Time) (Partido, bool) {
	if ev.Match == nil || len(ev.Match.Teams) != 2 {
		return Partido{}, false
	}

	// Averiguamos cuál de los dos equipos es el nuestro.
	indice := -1
	for i, t := range ev.Match.Teams {
		if esNuestro(t, teamCode) {
			indice = i
			break
		}
	}
	if indice == -1 {
		return Partido{}, false
	}

	nuestro := ev.Match.Teams[indice]
	rival := ev.Match.Teams[1-indice] // si el nuestro es 0, el rival es 1, y al revés

	p := Partido{
		ID:      ev.Match.ID,
		Inicio:  ev.StartTime,
		Liga:    ev.League.Name,
		Bloque:  ev.BlockName,
		Formato: formato(ev.Match.Strategy),
		Nuestro: aEquipo(nuestro),
		Rival:   aEquipo(rival),
		Ganamos: nuestro.Result != nil && nuestro.Result.Outcome == lolesports.OutcomeWin,
	}

	switch ev.State {
	case lolesports.StateInProgress:
		p.Estado = EstadoEnVivo
	case lolesports.StateCompleted:
		p.Estado = EstadoTerminado
	default:
		switch {
		case p.Inicio.After(now):
			p.Estado = EstadoPorJugar
		case now.Sub(p.Inicio) < margenEnVivo:
			// Ya debería estar jugándose; la API tarda en moverlo a inProgress.
			p.Estado = EstadoEnVivo
		default:
			return Partido{}, false
		}
	}

	return p, true
}

// esNuestro compara contra el código y el nombre, sin distinguir mayúsculas.
func esNuestro(t lolesports.Team, teamCode string) bool {
	return strings.EqualFold(t.Code, teamCode) || strings.EqualFold(t.Name, teamCode)
}

func aEquipo(t lolesports.Team) Equipo {
	e := Equipo{Nombre: t.Name, Codigo: t.Code, Imagen: t.Image}
	if t.Result != nil {
		e.Juegos = t.Result.GameWins
	}
	return e
}

// formato convierte la estrategia de la API en algo corto: "Bo5".
func formato(s lolesports.Strategy) string {
	if s.Type != "bestOf" || s.Count <= 0 {
		return ""
	}
	return fmt.Sprintf("Bo%d", s.Count)
}

// TeamCodes lista los códigos de equipo que aparecen en los eventos, sin
// repetir y en orden. Sirve para depurar cuando TEAM_CODE no encuentra nada.
func TeamCodes(events []lolesports.Event) []string {
	vistos := make(map[string]bool)
	for _, ev := range events {
		if ev.Match == nil {
			continue
		}
		for _, t := range ev.Match.Teams {
			if t.Code != "" {
				vistos[t.Code] = true
			}
		}
	}

	codigos := make([]string, 0, len(vistos))
	for c := range vistos {
		codigos = append(codigos, c)
	}
	slices.Sort(codigos)
	return codigos
}
