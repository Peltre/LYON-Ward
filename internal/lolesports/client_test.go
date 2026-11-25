// client test file

package lolesports

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// Respuestas de mentiras, recortadas a lo que de verdad usamos.
const leaguesJSON = `{"data":{"leagues":[
	{"id":"98767991299243165","slug":"lcs","name":"LCS","region":"NORTH AMERICA"},
	{"id":"98767991325878492","slug":"worlds","name":"Worlds","region":"INTERNATIONAL"}
]}}`

const scheduleJSON = `{"data":{"schedule":{
	"pages":{"older":"token-viejo","newer":null},
	"events":[{
		"startTime":"2026-04-12T22:00:00Z",
		"state":"completed",
		"type":"match",
		"blockName":"Final",
		"league":{"name":"LCS","slug":"lcs"},
		"match":{
			"id":"110","flags":["hasVod"],
			"strategy":{"type":"bestOf","count":5},
			"teams":[
				{"name":"LYON","code":"LYON","result":{"outcome":"win","gameWins":3},"record":{"wins":1,"losses":0}},
				{"name":"Team Liquid","code":"TL","result":{"outcome":"loss","gameWins":0}}
			]
		}
	}]
}}}`

// servidorFalso levanta un servidor HTTP que solo vive durante la prueba y
// devuelve un Client apuntando a él.
func servidorFalso(t *testing.T, handler http.HandlerFunc) *Client {
	t.Helper()
	srv := httptest.NewServer(handler)
	t.Cleanup(srv.Close)
	return NewClient(srv.URL, "llave-de-prueba")
}

func TestGetLeagues(t *testing.T) {
	c := servidorFalso(t, func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/getLeagues" {
			t.Errorf("ruta = %q, se esperaba /getLeagues", r.URL.Path)
		}
		if got := r.Header.Get("x-api-key"); got != "llave-de-prueba" {
			t.Errorf("x-api-key = %q, se esperaba llave-de-prueba", got)
		}
		if got := r.URL.Query().Get("hl"); got != DefaultLocale {
			t.Errorf("hl = %q, se esperaba %q", got, DefaultLocale)
		}
		w.Write([]byte(leaguesJSON))
	})

	leagues, err := c.GetLeagues(context.Background())
	if err != nil {
		t.Fatalf("GetLeagues devolvió error: %v", err)
	}
	if len(leagues) != 2 {
		t.Fatalf("len(leagues) = %d, se esperaban 2", len(leagues))
	}
	if leagues[0].Slug != "lcs" || leagues[0].ID != "98767991299243165" {
		t.Errorf("primera liga = %+v", leagues[0])
	}
}

func TestLeagueIDsBySlug(t *testing.T) {
	c := servidorFalso(t, func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(leaguesJSON))
	})

	ids, err := c.LeagueIDsBySlug(context.Background())
	if err != nil {
		t.Fatalf("LeagueIDsBySlug devolvió error: %v", err)
	}
	if ids["lcs"] != "98767991299243165" {
		t.Errorf("ids[lcs] = %q", ids["lcs"])
	}
	if _, ok := ids["lck"]; ok {
		t.Error("ids trae una liga que el servidor nunca mandó")
	}
}

func TestGetSchedule(t *testing.T) {
	c := servidorFalso(t, func(w http.ResponseWriter, r *http.Request) {
		q := r.URL.Query()
		if q.Get("leagueId") != "98767991299243165" {
			t.Errorf("leagueId = %q", q.Get("leagueId"))
		}
		if q.Get("pageToken") != "token-viejo" {
			t.Errorf("pageToken = %q", q.Get("pageToken"))
		}
		w.Write([]byte(scheduleJSON))
	})

	sch, err := c.GetSchedule(context.Background(), "98767991299243165", "token-viejo")
	if err != nil {
		t.Fatalf("GetSchedule devolvió error: %v", err)
	}
	if len(sch.Events) != 1 {
		t.Fatalf("len(Events) = %d, se esperaba 1", len(sch.Events))
	}

	ev := sch.Events[0]
	if ev.State != StateCompleted {
		t.Errorf("State = %q", ev.State)
	}
	if ev.StartTime.Year() != 2026 {
		t.Errorf("StartTime no se parseó: %v", ev.StartTime)
	}
	if ev.Match == nil {
		t.Fatal("Match quedó en nil")
	}
	if ev.Match.Teams[0].Result.Outcome != OutcomeWin {
		t.Errorf("outcome de LYON = %q", ev.Match.Teams[0].Result.Outcome)
	}
	// El segundo equipo no trae "record" en el JSON: debe quedar en nil.
	if ev.Match.Teams[1].Record != nil {
		t.Error("Record debería ser nil cuando la API no lo manda")
	}
	if sch.Pages.Older == nil || *sch.Pages.Older != "token-viejo" {
		t.Errorf("Pages.Older = %v", sch.Pages.Older)
	}
	if sch.Pages.Newer != nil {
		t.Error("Pages.Newer debería ser nil")
	}
}

func TestGetScheduleErrorHTTP(t *testing.T) {
	c := servidorFalso(t, func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "forbidden", http.StatusForbidden)
	})

	_, err := c.GetSchedule(context.Background(), "123", "")
	if err == nil {
		t.Fatal("se esperaba un error y no hubo")
	}
	// El error debe decir qué endpoint falló y con qué código.
	if !strings.Contains(err.Error(), "getSchedule") || !strings.Contains(err.Error(), "403") {
		t.Errorf("mensaje poco útil: %v", err)
	}
}
