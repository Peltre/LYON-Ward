// Main file

// Arranque del sitio: lee la configuración del entorno, lanza el
// sincronizador en segundo plano, levanta el servidor web y se apaga limpio
// con Ctrl+C.
package main

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"github.com/Peltre/LYONWard/internal/lolesports"
	"github.com/Peltre/LYONWard/internal/tracker"
	"github.com/Peltre/LYONWard/internal/web"
)

const (
	olderPages     = 2               // páginas viejas por liga, para tener resultados
	liveEvery      = time.Minute     // espera cuando hay partido en vivo
	tiempoApagado  = 5 * time.Second // lo más que esperamos a las visitas en curso
	tiempoCabecera = 5 * time.Second // ReadHeaderTimeout
)

func main() {
	// main solo llama a run y decide el código de salida. Así run puede
	// devolver errores normales y sus defer sí se ejecutan (os.Exit se los
	// saltaría).
	if err := run(); err != nil {
		slog.Error("el programa terminó con error", "err", err)
		os.Exit(1)
	}
}

func run() error {
	log := slog.New(slog.NewTextHandler(os.Stdout, nil))
	slog.SetDefault(log)

	// ---------- Configuración ----------
	port := envOr("PORT", "8080")
	teamCode := envOr("TEAM_CODE", "LYON")
	slugs := strings.Split(envOr("LEAGUE_SLUGS", "lcs,msi,worlds"), ",")

	interval, err := time.ParseDuration(envOr("SYNC_INTERVAL", "10m"))
	if err != nil {
		return fmt.Errorf("SYNC_INTERVAL inválido (ejemplos: 10m, 90s): %w", err)
	}

	// ---------- Apagado ----------
	// ctx se cancela solo cuando llega Ctrl+C (os.Interrupt) o SIGTERM, que
	// es como los servidores en la nube piden que el programa termine.
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	// ---------- Piezas ----------
	store := tracker.NewStore()
	client := lolesports.NewClient(os.Getenv("LOLESPORTS_BASE_URL"), os.Getenv("LOLESPORTS_API_KEY"))

	syncer := &tracker.Syncer{
		Client:      client,
		Store:       store,
		Log:         log.With("parte", "sync"),
		TeamCode:    teamCode,
		LeagueSlugs: slugs,
		OlderPages:  olderPages,
		Interval:    interval,
		LiveEvery:   liveEvery,
	}
	go syncer.Run(ctx)

	site, err := web.New(store, log.With("parte", "web"))
	if err != nil {
		return err
	}

	srv := &http.Server{
		Addr:    ":" + port,
		Handler: site.Handler(),
		// Si un cliente manda las cabeceras muy lento, lo cortamos: sin esto
		// alguien podría dejar miles de conexiones abiertas a propósito.
		ReadHeaderTimeout: tiempoCabecera,
	}

	// ListenAndServe bloquea hasta que el servidor se cierra, así que va en
	// su propia goroutine. Su error llega por un canal.
	errServidor := make(chan error, 1)
	go func() {
		log.Info("sitio listo", "url", "http://localhost:"+port, "equipo", teamCode, "ligas", slugs)
		errServidor <- srv.ListenAndServe()
	}()

	// Esperamos lo que pase primero: que el servidor falle al arrancar (por
	// ejemplo, el puerto ya está ocupado) o que nos pidan apagar.
	select {
	case err := <-errServidor:
		return fmt.Errorf("servidor: %w", err)
	case <-ctx.Done():
	}

	log.Info("apagando…")
	// Contexto NUEVO: el de arriba ya está cancelado. Shutdown deja de aceptar
	// visitas y espera hasta 5 s a que terminen las que estaban en curso.
	apagado, cancel := context.WithTimeout(context.Background(), tiempoApagado)
	defer cancel()
	if err := srv.Shutdown(apagado); err != nil && !errors.Is(err, http.ErrServerClosed) {
		return fmt.Errorf("apagado: %w", err)
	}
	log.Info("listo, hasta luego")
	return nil
}

// envOr devuelve la variable de entorno o, si está vacía, el valor por defecto.
func envOr(clave, porDefecto string) string {
	if v := strings.TrimSpace(os.Getenv(clave)); v != "" {
		return v
	}
	return porDefecto
}
