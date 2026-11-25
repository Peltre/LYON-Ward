// Makes the HTTP requests to the lolesports API

package lolesports

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
)

// Valores por defecto del cliente. La llave es la misma que usa el sitio
// lolesports.com en el navegador: es pública, pero la rotan de vez en cuando.
// Si la API empieza a responder 403, hay que cambiarla con LOLESPORTS_API_KEY.
const (
	DefaultBaseURL = "https://esports-api.lolesports.com/persisted/gw"
	DefaultAPIKey  = "0TvQnueqKa5mxJntVWt0w4LpLfEkrV1Ta8rQBb9Z"
	DefaultLocale  = "es-MX"

	// Tiempo máximo de una sola petición, por si la API se queda colgada.
	requestTimeout = 15 * time.Second
)

// Client habla con la API de lolesports. Sus campos son privados para que nadie
// los cambie a medio vuelo: se configura una vez con NewClient.
type Client struct {
	httpClient *http.Client
	baseURL    string
	apiKey     string
	locale     string
}

// NewClient arma un cliente listo para usar. Los parámetros vacíos toman el
// valor por defecto, así que en las pruebas basta con pasar la URL del servidor
// falso y dejar la llave vacía.
func NewClient(baseURL, apiKey string) *Client {
	if baseURL == "" {
		baseURL = DefaultBaseURL
	}
	if apiKey == "" {
		apiKey = DefaultAPIKey
	}
	return &Client{
		// Nunca usamos http.DefaultClient: no tiene timeout y una API colgada
		// dejaría la petición esperando para siempre.
		httpClient: &http.Client{Timeout: requestTimeout},
		baseURL:    strings.TrimRight(baseURL, "/"),
		apiKey:     apiKey,
		locale:     DefaultLocale,
	}
}

// GetLeagues devuelve todas las ligas que conoce la API.
func (c *Client) GetLeagues(ctx context.Context) ([]League, error) {
	var resp LeaguesResponse
	if err := c.get(ctx, "getLeagues", nil, &resp); err != nil {
		return nil, fmt.Errorf("getLeagues: %w", err)
	}
	return resp.Data.Leagues, nil
}

// GetSchedule devuelve una página del calendario de una liga.
// pageToken vacío pide la página actual; para las anteriores se manda el token
// que vino en Pages.Older de la respuesta previa.
func (c *Client) GetSchedule(ctx context.Context, leagueID, pageToken string) (Schedule, error) {
	params := url.Values{}
	if leagueID != "" {
		params.Set("leagueId", leagueID)
	}
	if pageToken != "" {
		params.Set("pageToken", pageToken)
	}

	var resp ScheduleResponse
	if err := c.get(ctx, "getSchedule", params, &resp); err != nil {
		return Schedule{}, fmt.Errorf("getSchedule: %w", err)
	}
	return resp.Data.Schedule, nil
}

// LeagueIDsBySlug trae las ligas y las acomoda en un mapa slug -> ID, porque la
// configuración se escribe con slugs ("lcs") pero la API pide IDs numéricos.
// Los slugs quedan en minúsculas para que la comparación no dependa de cómo se
// escribió la variable de entorno.
func (c *Client) LeagueIDsBySlug(ctx context.Context) (map[string]string, error) {
	leagues, err := c.GetLeagues(ctx)
	if err != nil {
		return nil, err
	}

	ids := make(map[string]string, len(leagues))
	for _, liga := range leagues {
		ids[strings.ToLower(liga.Slug)] = liga.ID
	}
	return ids, nil
}

// get hace una petición GET a un endpoint y decodifica el JSON en dst, que debe
// ser un puntero al struct donde queremos guardar la respuesta.
// Es privada: todo el trabajo repetido (llave, idioma, revisar el código de
// estado) vive aquí una sola vez.
func (c *Client) get(ctx context.Context, endpoint string, params url.Values, dst any) error {
	if params == nil {
		params = url.Values{}
	}
	params.Set("hl", c.locale)

	destino := c.baseURL + "/" + endpoint + "?" + params.Encode()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, destino, nil)
	if err != nil {
		return fmt.Errorf("armar la petición: %w", err)
	}
	req.Header.Set("x-api-key", c.apiKey)

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("petición HTTP: %w", err)
	}
	// defer corre al salir de la función, pase lo que pase. Sin esto, cada
	// error dejaría una conexión abierta.
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		// Leemos solo un pedazo del cuerpo para el mensaje de error: si la API
		// devuelve una página de error enorme, no queremos cargarla entera.
		cuerpo, _ := io.ReadAll(io.LimitReader(resp.Body, 512))
		return fmt.Errorf("la API respondió %s: %s", resp.Status, strings.TrimSpace(string(cuerpo)))
	}

	if err := json.NewDecoder(resp.Body).Decode(dst); err != nil {
		return fmt.Errorf("decodificar el JSON: %w", err)
	}
	return nil
}
