// Command api es el arranque standalone de bff (invariante I5): compila y sirve
// las rutas de la fase (a) de bff.yaml SIN core, cableando los adaptadores HTTP
// hacia identidad y operacion. Produccion NO lo usa (bff vive dentro de core);
// existe para levantar y probar el modulo de forma aislada.
//
// bff no es dueno de ningun dato: no abre pool ni aplica migraciones (bff.md
// (e)). La verificacion de JWT es local, con auth.NewVerifier contra el JWKS del
// IdP interno (ADR 26); en desarrollo se apunta a un JWKS de prueba.
package main

import (
	"context"
	"log/slog"
	"os"
	"time"

	"github.com/AI27-Tech/pf-pkg-platform-go/auth"
	"github.com/AI27-Tech/pf-pkg-platform-go/config"
	"github.com/AI27-Tech/pf-pkg-platform-go/observability"
	"github.com/AI27-Tech/pf-pkg-platform-go/server"
	bff "github.com/AI27-Tech/pf-svc-bff"
	bffhttp "github.com/AI27-Tech/pf-svc-bff/adapters/http"
	"github.com/AI27-Tech/pf-svc-bff/internal/autorizacion"
)

// Claves de entorno propias de bff: las URL base de sus aristas de fase (a) y la
// audiencia esperada del JWT. La URL del JWKS y la direccion HTTP las aporta
// config.Load (JWKS_URL, HTTP_ADDR). bff no tiene DB_*_URL: no es dueno de datos.
const (
	envIdentidadBase = "IDENTIDAD_BASE_URL"
	envOperacionBase = "OPERACION_BASE_URL"
	envAudiencia     = "JWT_AUDIENCIA_ESPERADA"
)

// ttlJWKS es el TTL de la cache de claves del verificador. Valor de arranque
// razonable; la raiz (core) lo ajusta por configuracion en produccion.
const ttlJWKS = 5 * time.Minute

func main() {
	if err := run(); err != nil {
		slog.Error("bff termino con error", slog.Any("error", err))
		os.Exit(1)
	}
}

func run() error {
	ctx := context.Background()

	cfg, err := config.Load(envIdentidadBase, envOperacionBase, config.EnvJWKSURL)
	if err != nil {
		return err
	}

	logger := slog.New(slog.NewJSONHandler(os.Stdout, nil))
	logger.Info("arrancando bff (standalone)", slog.String("service", cfg.ServiceName))

	shutdownOtel, err := observability.Init(ctx, observability.Config{
		ServiceName: cfg.ServiceName,
		ExporterURL: cfg.OtelExporterURL,
	})
	if err != nil {
		logger.Warn("observabilidad no inicializada", slog.Any("error", err))
	} else {
		defer func() { _ = shutdownOtel(context.Background()) }()
	}

	// Adaptadores HTTP de fase (a). En core se usan los en proceso.
	identidad, err := bffhttp.NewIdentidadHTTP(os.Getenv(envIdentidadBase), nil)
	if err != nil {
		return err
	}
	operacion, err := bffhttp.NewOperacionHTTP(os.Getenv(envOperacionBase), nil)
	if err != nil {
		return err
	}

	// Verificacion local de JWT contra el JWKS del IdP interno (ADR 26). La
	// audiencia de dominio (Cliente|Interno) se lee de auth.Claims.Audiencia; el
	// claim tecnico `aud` NO autoriza (DEC-054).
	verificador := auth.NewVerifier(cfg.JWKSURL, os.Getenv(envAudiencia), ttlJWKS)

	// Proyeccion de tombstones (ADR 26). En standalone arranca vacia: sin el
	// worker de sesion.revocada cableado por core, no hay difusion. La raiz
	// reconstruye por snapshot antes de servir trafico protegido en produccion.
	revocacion := autorizacion.NuevaProyeccion()

	mod, err := bff.New(bff.Pools{}, bff.Deps{
		Identidad:   identidad,
		Operacion:   operacion,
		Verificador: verificador,
		Revocacion:  revocacion,
	})
	if err != nil {
		return err
	}

	srv := server.New(cfg.HTTPAddr, server.WithLogger(logger))
	mod.Routes(srv.Mux())

	srv.MarkReady()
	return srv.Start(ctx)
}
