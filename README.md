# LYON Ward
Página de fans **no oficial** de LYON, equipo de League of Legends en la LCS. Muestra los partidos en vivo, el siguiente partido, el calendario y los resultados con el récord del equipo.

Está hecha en Go con la librería estándar: el HTML se arma en el servidor con `html/template` y [htmx](https://htmx.org) lo refresca cada minuto. El navegador nunca llama a APIs externas.

> LW es una página de fans sin relación oficial con LYON. No está respaldada por Riot Games y no refleja los puntos de vista ni las opiniones de Riot Games ni de nadie que participe oficialmente en la producción o gestión de las propiedades de Riot Games. Riot Games y todas las propiedades asociadas son marcas comerciales o marcas registradas de Riot Games, Inc.

## Cómo correrla

Necesitas [Go](https://go.dev/dl/) 1.27 o más nuevo.

```sh
go run .
```

Abre http://localhost:8080. Para apagarla, Ctrl+C.

Los templates y el CSS van dentro del binario (`go:embed`), así que después de cambiarlos hay que reiniciar.

Para tener un solo ejecutable:

```sh
go build -o lyonward .
```

## Configuración

Todo es opcional y se pasa por variables de entorno.

| Variable | Default | Para qué |
|---|---|---|
| `PORT` | `8080` | Puerto del servidor |
| `TEAM_CODE` | `LYON` | Código o nombre del equipo a seguir |
| `LEAGUE_SLUGS` | `lcs,msi,worlds` | Ligas a revisar, separadas por comas |
| `SYNC_INTERVAL` | `10m` | Cada cuánto consultar la API (cada minuto si hay partido en vivo) |
| `LOLESPORTS_API_KEY` | llave pública | Cambiarla si la API responde 403 |
| `LOLESPORTS_BASE_URL` | API real | Apuntar a un servidor falso al desarrollar |

Ejemplo en PowerShell:

```powershell
$env:PORT = "3000"; go run .
```

Si `TEAM_CODE` o alguna liga no existen, los logs listan los códigos y slugs que sí existen.

## Cómo funciona

```
API de lolesports ──► Syncer (goroutine) ──► Store (Snapshot en memoria) ──► servidor web ──► navegador
```

- **`internal/lolesports`**: cliente HTTP y tipos que reflejan el JSON de la API. No sabe que existe LYON.
- **`internal/tracker`**: convierte los eventos en partidos de LYON (`Snapshot`), los guarda en memoria (`Store`) y los actualiza en segundo plano (`Syncer`). Pide todas las ligas en paralelo.
- **`internal/web`**: rutas, templates y CSS. Solo lee el `Snapshot`, así que una visita nunca gasta límites de ninguna API.
- **`main.go`**: lee la configuración, arranca todo y se apaga limpio.

Si la API falla, el sitio sigue mostrando los últimos datos buenos con un aviso y reintenta en un minuto.

### Rutas

| Ruta | Qué devuelve |
|---|---|
| `GET /` | La página completa |
| `GET /partials/partidos` | Solo el bloque de partidos (lo pide htmx cada 60 s) |
| `GET /static/…` | CSS y archivos estáticos |
| `GET /healthz` | `ok` si el servidor está vivo |

## Pruebas

```sh
go test ./...
```

Las pruebas usan servidores falsos con `httptest`; nunca llaman a la API real.

## Fuente de datos

Usamos la API que alimenta lolesports.com. No es oficial: puede cambiar sin aviso y su llave pública se rota de vez en cuando.

## Hoja de ruta

1. **Calendario, partidos en vivo y resultados.** ✅
2. **SoloQ:** rango, LP e historial de las cuentas de los jugadores, con la API oficial de Riot.
3. **Estadísticas por partida** desde Leaguepedia.
4. **Partida en vivo detallada**, avisos y despliegue como binario único.
