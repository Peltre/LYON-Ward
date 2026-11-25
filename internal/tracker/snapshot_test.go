// Snapshot testing

package tracker

import (
	"testing"
	"time"

	"github.com/Peltre/LYONWard/internal/lolesports"
)

// Hora fija para todas las pruebas: así los resultados nunca dependen de
// cuándo se corran.
var ahora = time.Date(2026, 11, 24, 18, 0, 0, 0, time.UTC)

// eventoFalso describe un evento de mentiras de forma corta. Su método
// construir() lo infla al Event completo que espera la API.
type eventoFalso struct {
	id      string
	estado  string
	inicio  time.Time
	codigo  string // código del equipo que nos interesa
	outcome string // "win", "loss" o vacío si aún no hay resultado
	juegos  int
}

func (e eventoFalso) construir() lolesports.Event {
	nuestro := lolesports.Team{Name: "LYON Gaming", Code: e.codigo}
	if e.outcome != "" {
		nuestro.Result = &lolesports.Result{Outcome: e.outcome, GameWins: e.juegos}
	}

	return lolesports.Event{
		StartTime: e.inicio,
		State:     e.estado,
		Type:      "match",
		BlockName: "Semana 1",
		League:    lolesports.EventLeague{Name: "LCS", Slug: "lcs"},
		Match: &lolesports.Match{
			ID:       e.id,
			Strategy: lolesports.Strategy{Type: "bestOf", Count: 3},
			Teams: []lolesports.Team{
				nuestro,
				{Name: "Cloud9", Code: "C9"},
			},
		},
	}
}

func TestBuildSnapshotClasificaYCuenta(t *testing.T) {
	events := []lolesports.Event{
		eventoFalso{id: "1", estado: lolesports.StateCompleted, inicio: ahora.AddDate(0, 0, -2), codigo: "LYON", outcome: "loss", juegos: 1}.construir(),
		eventoFalso{id: "2", estado: lolesports.StateCompleted, inicio: ahora.AddDate(0, 0, -1), codigo: "LYON", outcome: "win", juegos: 2}.construir(),
		eventoFalso{id: "3", estado: lolesports.StateInProgress, inicio: ahora, codigo: "LYON"}.construir(),
		eventoFalso{id: "4", estado: lolesports.StateUnstarted, inicio: ahora.AddDate(0, 0, 1), codigo: "LYON"}.construir(),
		// Agendado hace diez días y nunca se marcó como jugado: se descarta.
		eventoFalso{id: "5", estado: lolesports.StateUnstarted, inicio: ahora.AddDate(0, 0, -10), codigo: "LYON"}.construir(),
	}

	snap := BuildSnapshot(events, "LYON", ahora)

	if len(snap.EnVivo) != 1 || snap.EnVivo[0].ID != "3" {
		t.Errorf("EnVivo = %+v", snap.EnVivo)
	}
	if len(snap.Proximos) != 1 || snap.Proximos[0].ID != "4" {
		t.Errorf("Proximos = %+v", snap.Proximos)
	}
	if len(snap.Resultados) != 2 {
		t.Fatalf("len(Resultados) = %d, se esperaban 2", len(snap.Resultados))
	}
	// Los resultados van del más reciente al más viejo.
	if snap.Resultados[0].ID != "2" {
		t.Errorf("el primer resultado debería ser el más reciente, fue %q", snap.Resultados[0].ID)
	}
	if snap.Victorias != 1 || snap.Derrotas != 1 {
		t.Errorf("récord = %d-%d, se esperaba 1-1", snap.Victorias, snap.Derrotas)
	}
	if !snap.Actualizado.Equal(ahora) {
		t.Errorf("Actualizado = %v", snap.Actualizado)
	}
}

func TestBuildSnapshotArmaElPartido(t *testing.T) {
	ev := eventoFalso{id: "7", estado: lolesports.StateCompleted, inicio: ahora, codigo: "LYON", outcome: "win", juegos: 3}.construir()

	snap := BuildSnapshot([]lolesports.Event{ev}, "LYON", ahora)
	if len(snap.Resultados) != 1 {
		t.Fatalf("no se armó el partido: %+v", snap)
	}

	p := snap.Resultados[0]
	if p.Nuestro.Codigo != "LYON" || p.Rival.Codigo != "C9" {
		t.Errorf("equipos mal asignados: %+v / %+v", p.Nuestro, p.Rival)
	}
	if p.Nuestro.Juegos != 3 {
		t.Errorf("Juegos = %d, se esperaban 3", p.Nuestro.Juegos)
	}
	if !p.Ganamos {
		t.Error("Ganamos = false con outcome win")
	}
	if p.Formato != "Bo3" {
		t.Errorf("Formato = %q, se esperaba Bo3", p.Formato)
	}
	if p.Liga != "LCS" || p.Bloque != "Semana 1" {
		t.Errorf("liga o bloque mal: %q / %q", p.Liga, p.Bloque)
	}
}

// Nuestro equipo puede venir en cualquiera de los dos lugares del slice.
func TestBuildSnapshotEncuentraAlEquipoSegundo(t *testing.T) {
	ev := eventoFalso{id: "8", estado: lolesports.StateInProgress, inicio: ahora, codigo: "LYON"}.construir()
	ev.Match.Teams[0], ev.Match.Teams[1] = ev.Match.Teams[1], ev.Match.Teams[0]

	snap := BuildSnapshot([]lolesports.Event{ev}, "LYON", ahora)
	if len(snap.EnVivo) != 1 {
		t.Fatal("no encontró al equipo cuando estaba en el segundo lugar")
	}
	if snap.EnVivo[0].Rival.Codigo != "C9" {
		t.Errorf("Rival = %q", snap.EnVivo[0].Rival.Codigo)
	}
}

func TestBuildSnapshotIdentificaAlEquipo(t *testing.T) {
	ev := eventoFalso{id: "9", estado: lolesports.StateInProgress, inicio: ahora, codigo: "LYON"}.construir()

	casos := []struct {
		nombre    string
		teamCode  string
		encuentra bool
	}{
		{"código exacto", "LYON", true},
		{"código en minúsculas", "lyon", true},
		{"nombre completo", "LYON Gaming", true},
		{"otro equipo", "TL", false},
		{"vacío", "", false},
	}

	for _, c := range casos {
		t.Run(c.nombre, func(t *testing.T) {
			snap := BuildSnapshot([]lolesports.Event{ev}, c.teamCode, ahora)
			if encontro := len(snap.EnVivo) == 1; encontro != c.encuentra {
				t.Errorf("con teamCode %q encontró=%v, se esperaba %v", c.teamCode, encontro, c.encuentra)
			}
		})
	}
}

// Un partido que ya pasó su hora pero que la API no ha movido a inProgress se
// muestra como en vivo, no como próximo.
func TestBuildSnapshotUnstartedRecienteEsEnVivo(t *testing.T) {
	ev := eventoFalso{id: "10", estado: lolesports.StateUnstarted, inicio: ahora.Add(-time.Hour), codigo: "LYON"}.construir()

	snap := BuildSnapshot([]lolesports.Event{ev}, "LYON", ahora)
	if len(snap.EnVivo) != 1 {
		t.Errorf("EnVivo = %+v, Proximos = %+v", snap.EnVivo, snap.Proximos)
	}
}

func TestProximoYDespues(t *testing.T) {
	var vacio Snapshot
	if vacio.Proximo() != nil {
		t.Error("un snapshot vacío no debería tener próximo")
	}
	if vacio.Despues() != nil {
		t.Error("un snapshot vacío no debería tener partidos después")
	}
	if vacio.HayDatos() {
		t.Error("un snapshot vacío no tiene datos")
	}

	events := []lolesports.Event{
		eventoFalso{id: "lejano", estado: lolesports.StateUnstarted, inicio: ahora.AddDate(0, 0, 5), codigo: "LYON"}.construir(),
		eventoFalso{id: "cercano", estado: lolesports.StateUnstarted, inicio: ahora.AddDate(0, 0, 1), codigo: "LYON"}.construir(),
		eventoFalso{id: "medio", estado: lolesports.StateUnstarted, inicio: ahora.AddDate(0, 0, 3), codigo: "LYON"}.construir(),
	}

	snap := BuildSnapshot(events, "LYON", ahora)
	if snap.Proximo() == nil || snap.Proximo().ID != "cercano" {
		t.Errorf("Proximo = %+v", snap.Proximo())
	}
	if len(snap.Despues()) != 2 {
		t.Errorf("Despues = %+v", snap.Despues())
	}
	if !snap.HayDatos() {
		t.Error("HayDatos = false después de construir")
	}
}

func TestBuildSnapshotIgnoraEventosRaros(t *testing.T) {
	sinMatch := lolesports.Event{StartTime: ahora, State: lolesports.StateInProgress, Type: "show"}

	unSoloEquipo := eventoFalso{id: "11", estado: lolesports.StateInProgress, inicio: ahora, codigo: "LYON"}.construir()
	unSoloEquipo.Match.Teams = unSoloEquipo.Match.Teams[:1]

	snap := BuildSnapshot([]lolesports.Event{sinMatch, unSoloEquipo}, "LYON", ahora)
	if len(snap.EnVivo) != 0 || len(snap.Proximos) != 0 || len(snap.Resultados) != 0 {
		t.Errorf("se colaron eventos raros: %+v", snap)
	}
}

func TestTeamCodes(t *testing.T) {
	events := []lolesports.Event{
		eventoFalso{id: "1", estado: lolesports.StateCompleted, inicio: ahora, codigo: "TL"}.construir(),
		eventoFalso{id: "2", estado: lolesports.StateCompleted, inicio: ahora, codigo: "LYON"}.construir(),
		eventoFalso{id: "3", estado: lolesports.StateCompleted, inicio: ahora, codigo: "LYON"}.construir(),
		{StartTime: ahora, Type: "show"},
	}

	codigos := TeamCodes(events)
	esperados := []string{"C9", "LYON", "TL"} // sin repetir y en orden alfabético

	if len(codigos) != len(esperados) {
		t.Fatalf("TeamCodes = %v, se esperaba %v", codigos, esperados)
	}
	for i, c := range esperados {
		if codigos[i] != c {
			t.Errorf("TeamCodes = %v, se esperaba %v", codigos, esperados)
			break
		}
	}
}
