// Package lolesports contiene el cliente HTTP y los tipos que reflejan el JSON
// de la API de lolesports.com. Este paquete no sabe nada de LYON: solo traduce
// respuestas de la API a tipos de Go.
package lolesports

import "time"

// Estados posibles de un evento, tal como los manda la API en el campo "state".
const (
	StateUnstarted  = "unstarted"
	StateInProgress = "inProgress"
	StateCompleted  = "completed"
)

// Resultados posibles de un equipo en un partido terminado ("outcome").
const (
	OutcomeWin  = "win"
	OutcomeLoss = "loss"
)

// LeaguesResponse es la respuesta completa de getLeagues.
// La API envuelve todo en un objeto "data", así que lo reflejamos con un
// struct anónimo: no necesitamos ese nivel en ninguna otra parte.
type LeaguesResponse struct {
	Data struct {
		Leagues []League `json:"leagues"`
	} `json:"data"`
}

// League es una liga: LCS, MSI, Worlds...
// El Slug es el nombre corto que usamos para configurar qué ligas seguimos.
type League struct {
	ID       string `json:"id"`
	Slug     string `json:"slug"`
	Name     string `json:"name"`
	Region   string `json:"region"`
	Image    string `json:"image"`
	Priority int    `json:"priority"`
}

// ScheduleResponse es la respuesta completa de getSchedule.
type ScheduleResponse struct {
	Data struct {
		Schedule Schedule `json:"schedule"`
	} `json:"data"`
}

// Schedule es el calendario de una liga: una página de eventos más los tokens
// para pedir la página anterior o la siguiente.
type Schedule struct {
	Pages  Pages   `json:"pages"`
	Events []Event `json:"events"`
}

// Pages trae los tokens de paginación. Son punteros porque la API manda null
// cuando ya no hay más páginas, y necesitamos distinguir "no hay" de "token
// vacío".
type Pages struct {
	Older *string `json:"older"`
	Newer *string `json:"newer"`
}

// Event es una entrada del calendario. Casi siempre es un partido, pero la API
// también manda eventos de tipo "show" (transmisiones sin partido).
type Event struct {
	StartTime time.Time   `json:"startTime"`
	State     string      `json:"state"`
	Type      string      `json:"type"`
	BlockName string      `json:"blockName"`
	League    EventLeague `json:"league"`
	Match     *Match      `json:"match"`
}

// EventLeague es la liga tal como viene dentro de un evento: una versión
// recortada de League, sin ID ni imagen.
type EventLeague struct {
	Name string `json:"name"`
	Slug string `json:"slug"`
}

// Match son los datos del enfrentamiento. Es un puntero dentro de Event porque
// los eventos que no son partidos no lo traen.
type Match struct {
	ID       string   `json:"id"`
	Flags    []string `json:"flags"`
	Teams    []Team   `json:"teams"`
	Strategy Strategy `json:"strategy"`
}

// Team es uno de los dos equipos de un partido.
// Code es la abreviatura ("LYON", "TL", "C9") y es con lo que identificamos al
// equipo que seguimos.
type Team struct {
	Name   string  `json:"name"`
	Code   string  `json:"code"`
	Image  string  `json:"image"`
	Result *Result `json:"result"`
	Record *Record `json:"record"`
}

// Result es el marcador del equipo en este partido. Es un puntero porque solo
// existe cuando el partido ya empezó.
type Result struct {
	Outcome  string `json:"outcome"`
	GameWins int    `json:"gameWins"`
}

// Record es el récord del equipo en la temporada, no en este partido.
// También es puntero: la API lo omite en algunos torneos.
type Record struct {
	Wins   int `json:"wins"`
	Losses int `json:"losses"`
}

// Strategy es el formato del partido: Type suele ser "bestOf" y Count el
// número de mapas (1, 3 o 5).
type Strategy struct {
	Type  string `json:"type"`
	Count int    `json:"count"`
}
