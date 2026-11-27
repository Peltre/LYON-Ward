# LYON Ward

Página de fans (no oficial) que sigue a LYON, equipo de League of Legends en la LCS: próximos partidos, partidos en vivo, resultados, cuentas de soloQ de los jugadores y estadísticas. Backend en Go, HTML renderizado en el servidor con `html/template` y htmx.

## Cómo trabajar conmigo (léelo primero)

- Es mi primer proyecto en Go. El objetivo es doble: construir la página y aprender Go. Prioriza que yo entienda sobre terminar rápido.
- Responde siempre en español.
- Vamos archivo por archivo, en el orden de la hoja de ruta. Antes de escribir un archivo, explica qué hace y qué conceptos de Go nuevos aparecen. Después, explica las partes importantes del código.
- No crees ni modifiques varios archivos de golpe salvo que te lo pida. No refactorices archivos ya terminados sin preguntarme.
- Después de cada archivo corre `go build ./...`, `go vet ./...` y `go test ./...`.
- Usa solo la librería estándar. Pregúntame antes de agregar cualquier dependencia con `go get`.
- Mi entorno: Windows, PowerShell y VS Code. La carpeta local se llama `LYON Ward` (con espacio), así que pon comillas en las rutas. Trabajo desde dos computadoras: recuérdame `git pull` al empezar y `git push` al terminar.
- `go test -race` en Windows necesita un compilador de C (cgo). Si no está instalado, usa `go test ./...`.
- Cuando terminemos un archivo, actualiza la sección "Estado actual" de este documento.

## Estado actual

Verifica siempre el estado real del repo: esta lista puede estar atrasada.

- Hecho: `go.mod` (`module github.com/Peltre/LYONWard`), `internal/lolesports/types.go`, `client.go` y `client_test.go`; `internal/tracker/snapshot.go`, `snapshot_test.go`, `store.go` y `sync.go`. Todas las pruebas pasan.
- Los demás archivos existen pero solo tienen su comentario y la cláusula `package`: están vacíos de lógica.
- El template se llama `matches.html`, no `partidos.html`: los nombres de archivo van en inglés y solo el modelo propio va en español.
- Los mockups aprobados (sitio general y partida en vivo en espejo) ya no están en el repo: Claude los guarda en su memoria del proyecto.
- Sigue, en orden: `internal/web/server.go` con sus templates y CSS, `main.go` (reemplaza el hola mundo), `README.md` y `.gitignore`.

Windows bloquea el binario que `go test` crea en `%TEMP%` (Smart App Control: "An Application Control policy has blocked this file"). Se arregla una vez por computadora con `go env -w GOTMPDIR=C:\Users\dakke\go\tmp`.

## Arquitectura (fase 1)

Un sincronizador corre en una goroutine, consulta la API de lolesports cada cierto tiempo y guarda un `Snapshot` en memoria. El servidor web solo lee ese `Snapshot`. El navegador nunca llama a APIs externas, y una visita nunca gasta límites de ninguna API.

Dependencias entre paquetes, siempre en un solo sentido (Go prohíbe ciclos):
- `main` importa `web`, `tracker` y `lolesports`.
- `web` importa `tracker`.
- `tracker` importa `lolesports`.
- `lolesports` no sabe que existe LYON; `tracker` no sabe nada de HTML; `web` nunca llama a la API.

```
LYON Ward/
├── go.mod
├── main.go                 # arranque, configuración por env y apagado limpio
├── CLAUDE.md
├── README.md
├── .gitignore
└── internal/
    ├── lolesports/         # cliente HTTP y tipos que reflejan el JSON de la API
    │   ├── types.go
    │   ├── client.go
    │   └── client_test.go
    ├── tracker/            # lógica de LYON: modelo Partido, store y sincronizador
    │   ├── snapshot.go
    │   ├── snapshot_test.go
    │   ├── store.go
    │   └── sync.go
    └── web/                # rutas, templates y CSS embebidos en el binario
        ├── server.go
        ├── templates/
        │   ├── index.html
        │   └── partidos.html
        └── static/
            └── styles.css
```

## Contrato de lo que falta

Respeta estos nombres para que todo encaje con lo que ya existe.

**`internal/tracker/store.go`**
- `type Store struct { mu sync.RWMutex; snap Snapshot }` y `NewStore() *Store`.
- `Get() Snapshot` con `RLock`, `Set(Snapshot)` con `Lock`.
- `SetError(msg string)` conserva los últimos datos buenos y solo anota el error.
- Un `Snapshot` nunca se modifica después de guardarlo: siempre se crea uno nuevo.

**`internal/tracker/sync.go`**
- `type Syncer struct` con campos `Client *lolesports.Client`, `Store *Store`, `Log *slog.Logger`, `TeamCode string`, `LeagueSlugs []string`, `OlderPages int`, `Interval time.Duration`, `LiveEvery time.Duration` y el privado `leagueIDs map[string]string`.
- `Run(ctx)`: bucle que llama a `syncOnce` y espera con `select` entre `ctx.Done()` y `time.After(next)`.
- `syncOnce(ctx) time.Duration`:
  - Usa un timeout de 45 s.
  - La primera vez resuelve los IDs con `LeagueIDsBySlug` y avisa en el log qué slugs no existen.
  - Llama a `fetchAll` y luego a `BuildSnapshot`.
  - Si no encuentra al equipo, loguea `TeamCodes(events)` para depurar.
  - Devuelve `LiveEvery` si hay partido en vivo y `Interval` si no. Si hay error, usa `Store.SetError` y reintenta en 1 minuto.
- `fetchAll`: una goroutine por liga con `sync.WaitGroup`. Cada goroutine escribe en su propio índice de un slice de resultados, sin mutex. Los errores parciales se juntan con `errors.Join`.
- `fetchLeague`: la página actual más `OlderPages` páginas anteriores, siguiendo `Pages.Older`.

**`internal/web/server.go`**
- Usa `//go:embed templates/*.html static/*`.
- `New(store, log) (*Server, error)` con un FuncMap que tenga:
  - `iso`: fecha en UTC con formato RFC3339.
  - `fechaUTC`: texto de respaldo si el JavaScript no corre.
  - `plural(n, uno, varios)`.
- `Handler()` usa el `ServeMux` estándar con estas rutas:
  - `GET /{$}` pinta el template `index`.
  - `GET /partials/partidos` pinta el template `partidos`.
  - `GET /static/` sirve archivos con `http.FileServerFS`.
  - `GET /healthz` responde que el servidor está vivo.
- Renderiza a un `bytes.Buffer` antes de escribir la respuesta.

**Templates**
- `index.html` define `index`: el layout completo, con `<main hx-get="/partials/partidos" hx-trigger="every 60s" hx-swap="innerHTML">`.
  - Carga htmx 2.0.4 desde unpkg.
  - Un script pequeño convierte los `<time datetime data-local="largo|corto|hora">` a la hora local del visitante con `toLocaleString("es-MX")`, y se vuelve a correr en `htmx:afterSwap`.
- `partidos.html` define `partidos`, con estas secciones:
  - En vivo.
  - Siguiente partido, usando `.Proximo`.
  - Después.
  - Resultados, con el récord.
  - Estado: error o última actualización.
- El footer lleva el aviso legal de Riot (obligatorio).

**`main.go`**
- Usa `slog` para los logs.
- `signal.NotifyContext` captura Ctrl+C y SIGTERM.
- Lanza `go syncer.Run(ctx)`.
- Crea un `http.Server` con `ReadHeaderTimeout`.
- Apaga limpio con `srv.Shutdown` y un tiempo máximo de 5 s.

**Variables de entorno**

| Variable | Default | Para qué |
|---|---|---|
| `PORT` | `8080` | Puerto del servidor |
| `TEAM_CODE` | `LYON` | Código o nombre del equipo |
| `LEAGUE_SLUGS` | `lcs,msi,worlds` | Ligas a revisar |
| `SYNC_INTERVAL` | `10m` | Intervalo normal (cada 1 min si hay partido en vivo) |
| `LOLESPORTS_API_KEY` | llave pública | Cambiarla si la API responde 403 |
| `LOLESPORTS_BASE_URL` | API real | Apuntar a un servidor falso al desarrollar |

## Convenciones

- Formato con `gofmt`. Comentarios en español.
- Los tipos de la API van en inglés y reflejan el JSON. El modelo propio va en español (`Partido`, `Snapshot`, `Victorias`…).
- `context.Context` siempre es el primer parámetro.
- Errores envueltos con contexto: `fmt.Errorf("getSchedule: %w", err)`.
- Nunca uses `http.DefaultClient`, porque no tiene timeout.
- La lógica va en funciones puras para poder probarla. Por ejemplo, la hora actual entra como parámetro `now`.
- Las pruebas viven junto al código y usan servidores falsos con `httptest`. Nunca le pegues a una API real en las pruebas.
- Los textos de la interfaz van en español, con mayúscula solo al inicio.

## Fuentes de datos y reglas

**API de lolesports (fase 1)**
- No es oficial: es la que usa lolesports.com, puede cambiar sin aviso y su llave pública se rota.
- Base: `https://esports-api.lolesports.com/persisted/gw`, con el header `x-api-key`.
- Endpoints que usamos: `getLeagues` y `getSchedule` (parámetros `leagueId`, `pageToken`, `hl=es-MX`).

**API oficial de Riot (fase 2)**
- `account-v1` convierte el Riot ID en PUUID, `league-v4` da rango y LP, y `match-v5` el historial.
- Guarda el PUUID, no el nombre, porque los jugadores se lo cambian.
- La llave de desarrollo expira cada 24 h, y un sitio público necesita llave de producción (Riot pide un prototipo funcionando).
- La llave nunca va en el navegador ni en el repo: va en `.env`, que está en `.gitignore`.

**Leaguepedia (fase 3)**
- Usa la API Cargo, con las tablas `ScoreboardGames`, `ScoreboardPlayers` y `PicksAndBansS7`.
- Hay que cachear y respetar sus límites de uso.

**Legal**
- Es una página de fans sin relación oficial con LYON, y el aviso de Riot va en el footer.
- No recrees el logo de LYON: solo se usa el archivo oficial, con su permiso o según sus lineamientos.

## Hoja de ruta

1. **Fase 1 (actual):** calendario, partidos en vivo y resultados. Todo en memoria.
2. **Fase 2:** soloQ.
   - Cuentas de los jugadores editables, nunca fijas en el código, con la región de cada una (los pros juegan en KR durante los bootcamps).
   - Postgres con `pgx` para el historial de LP.
   - Limitador de peticiones con `golang.org/x/time/rate`.
3. **Fase 3:** estadísticas por partida desde Leaguepedia.
4. **Fase 4:**
   - En vivo detallado con el feed de livestats de lolesports.
   - Avisos.
   - Despliegue como binario único en Fly.io o Railway.

## Marca y diseño

**Colores oficiales** (Brand Manual v1 de LYON, publicados en el Media Kit de lyon.gg):

| Variable | Hex | Para qué |
|---|---|---|
| `--lyon-oro` | `#B59A75` | Acento: botones, victorias, bordes destacados y enlace activo |
| `--lyon-carbon` | `#1B1E25` | Fondo principal; también va en `<meta name="theme-color">` |
| `--lyon-blanco` | `#F1F1F1` | Texto principal |
| `--lyon-negro` | `#000000` | Franjas de máximo contraste y footer |

**Derivados** (nuestros, no oficiales):

| Variable | Hex | Para qué |
|---|---|---|
| `--superficie` | `#252932` | Paneles |
| `--linea` | `#2E333D` | Divisores |
| `--tenue` | `#A7A9AF` | Texto secundario |

**Contraste**
- Oro sobre carbón: 6.3:1.
- Blanco sobre carbón: ~15:1.
- Carbón sobre oro: 6.3:1.
- Nunca uses el oro como color de texto sobre fondos claros (2.4:1, no pasa).

**Tipografía**
- Títulos: Barlow Condensed 700 en MAYÚSCULAS, `letter-spacing: 0.02em`. Si te confirmo la fuente real de lyon.gg y está en Google Fonts, se cambia por esa.
- Énfasis: en un título, UNA sola palabra en cursiva y color oro (ejemplo: "SIGUE A LA *MANADA*"). Máximo una por título.
- Texto corrido: Barlow 400 y 500.
- Cifras (marcadores, LP, récords): Barlow Condensed 700, grandes, con `font-variant-numeric: tabular-nums`.

**Reglas**
- Tema oscuro por defecto, con fondo carbón.
- Victoria = relleno oro con texto carbón. Derrota = contorno blanco. Nunca solo color.
- Logo: solo el isotipo oficial (versión para fondo oscuro, del Media Kit de lyon.gg), guardado en `internal/web/static/lyon-isotipo.png` y usado con `<img alt="LYON">`. No lo dibujes, no lo recrees en SVG y no lo modifiques.
- No uses los lemas oficiales de LYON ("Join the family", "Nacidos para ganar") como si fueran nuestros: la página es de fans y no oficial. Usa frases propias.
- El footer conserva el aviso legal de Riot y el de "página de fans sin relación oficial con LYON".
- Las fechas se muestran en la hora local del visitante.
- Pensado primero para celular.
- El nombre visible provisional es "La Manada" (a LYON le dicen The Pride); falta decidirlo.

Cuando lleguemos a `internal/web/static/styles.css`, estos colores se definen como variables en `:root` y todo lo demás los usa desde ahí. Explícame cómo funcionan las variables de CSS cuando toque escribirlo.

## Contexto del equipo (noviembre de 2026)

- LYON fue bicampeón de la LCS 2026: ganó el Lock-In y el Spring (3–0 a Team Liquid en la final).
- Llegó a la semifinal del MSI 2026: cayó 2–3 ante Hanwha Life en la final de la llave de perdedores.
- Jugó Worlds 2026.
- Plantilla 2026: Dhokla (top), Inspired (jungla), Saint (medio), Berserker (tirador) e Isles (soporte). Puede cambiar en este offseason.
- El calendario de la LCS 2027 todavía no se publica, así que es normal que "Siguiente partido" salga vacío.
- `TEAM_CODE` y `LEAGUE_SLUGS` no se han verificado contra la API real. En la primera corrida, confírmalos con los logs: el programa lista los códigos y slugs que sí existen.

## Comandos

```powershell
go run .                # levanta el sitio en http://localhost:8080
go build ./...          # compila todos los paquetes
go vet ./...            # busca errores comunes
go test ./...           # corre todas las pruebas
go test -count=1 ./...  # corre las pruebas sin usar la caché
gofmt -l .              # lista archivos sin formatear
```