package http

import (
	"context"
	"net/http"
	"strings"
	"testing"
	"time"

	opclient "github.com/AI27-Tech/pf-pkg-contracts/packages/api-client-go/operacion"
	repclient "github.com/AI27-Tech/pf-pkg-contracts/packages/api-client-go/reportes"
	platformerrors "github.com/AI27-Tech/pf-pkg-platform-go/errors"
	"github.com/AI27-Tech/pf-svc-bff/internal/tokenctx"
	"github.com/google/uuid"
)

// opclientEntrada es una intención de subida válida para los tests.
func opclientEntrada() opclient.IntencionSubidaAdjuntoEntrada {
	return opclient.IntencionSubidaAdjuntoEntrada{
		Nombre: "acta.pdf", TipoMedio: "application/pdf", TamanoBytes: 1, ChecksumSha256: strings.Repeat("a", 64),
	}
}

// TestReportesViajesPropagaBearerPeriodoYFrescura: el adaptador reenvía el
// Bearer, el periodo viaja como query desde/hasta y la respuesta conserva
// proyeccion_al y stale (DEC-033).
func TestReportesViajesPropagaBearerPeriodoYFrescura(t *testing.T) {
	doer := &doerCaptura{cuerpo: `{"filas":[],"total":0,"proyeccion_al":"2026-11-03T12:00:00Z","stale":true}`}
	ad, err := NewReportesHTTP("http://reportes.local", doer)
	if err != nil {
		t.Fatalf("NewReportesHTTP: %v", err)
	}

	desde := time.Date(2026, 11, 1, 0, 0, 0, 0, time.UTC)
	hasta := time.Date(2026, 11, 8, 0, 0, 0, 0, time.UTC)
	ctx := tokenctx.Con(context.Background(), "Bearer tok-rep")
	rep, err := ad.ReporteViajes(ctx, repclient.ReporteViajesParams{Desde: desde, Hasta: hasta})
	if err != nil {
		t.Fatalf("ReporteViajes: %v", err)
	}
	if doer.authRecibido != "Bearer tok-rep" {
		t.Errorf("Authorization aguas abajo = %q, want Bearer tok-rep", doer.authRecibido)
	}
	if doer.rutaRecibida != "/api/v1/reportes/viajes" {
		t.Errorf("ruta = %q, want /api/v1/reportes/viajes", doer.rutaRecibida)
	}
	if !strings.Contains(doer.queryRecibida, "desde=") || !strings.Contains(doer.queryRecibida, "hasta=") {
		t.Errorf("query = %q, se esperaban desde y hasta", doer.queryRecibida)
	}
	if rep.Stale == nil || !*rep.Stale || rep.ProyeccionAl == nil {
		t.Errorf("reporte = %+v, se esperaban stale=true y proyeccion_al", rep)
	}
}

// TestReportesEstadosSinBearerNoAgregaCabecera: sin Bearer en el ctx no se
// inventa una cabecera (reportes responderá 401).
func TestReportesEstadosSinBearerNoAgregaCabecera(t *testing.T) {
	doer := &doerCaptura{cuerpo: `{"conteos":[],"proyeccion_al":null}`}
	ad, err := NewReportesHTTP("http://reportes.local", doer)
	if err != nil {
		t.Fatalf("NewReportesHTTP: %v", err)
	}
	if _, err := ad.ReporteEstados(context.Background(), repclient.ReporteEstadosParams{}); err != nil {
		t.Fatalf("ReporteEstados: %v", err)
	}
	if doer.authRecibido != "" {
		t.Errorf("no debía ir Authorization sin bearer, fue %q", doer.authRecibido)
	}
}

// TestReportesTraduceStatusDeError: el 403 de reportes (audiencia no Interno)
// sale como Prohibido, no como error de servidor.
func TestReportesTraduceStatusDeError(t *testing.T) {
	doer := &doerCaptura{status: http.StatusForbidden, cuerpo: `{"codigo":"prohibido","mensaje":"no"}`}
	ad, err := NewReportesHTTP("http://reportes.local", doer)
	if err != nil {
		t.Fatalf("NewReportesHTTP: %v", err)
	}
	_, err = ad.ReporteViajes(context.Background(), repclient.ReporteViajesParams{})
	if got := platformerrors.AsCategory(err); got != platformerrors.CategoryProhibido {
		t.Errorf("categoría = %v, se esperaba Prohibido (err=%v)", got, err)
	}
}

// TestOperacionIntencionSubidaLee201: la intención de subida responde 201 y el
// adaptador la lee de JSON201 con el Bearer propagado.
func TestOperacionIntencionSubidaLee201(t *testing.T) {
	adjunto := uuid.New()
	doer := &doerCaptura{
		status: http.StatusCreated,
		cuerpo: `{"adjunto_id":"` + adjunto.String() + `","url_subida":"https://minio.local/s?f=x","expira_en":"2026-11-03T12:05:00Z"}`,
	}
	ad, err := NewOperacionHTTP("http://operacion.local", doer)
	if err != nil {
		t.Fatalf("NewOperacionHTTP: %v", err)
	}

	ctx := tokenctx.Con(context.Background(), "Bearer tok-ev")
	got, err := ad.CrearIntencionSubidaAdjunto(ctx, uuid.New(), opclientEntrada())
	if err != nil {
		t.Fatalf("CrearIntencionSubidaAdjunto: %v", err)
	}
	if got.AdjuntoId != adjunto || got.UrlSubida != "https://minio.local/s?f=x" {
		t.Errorf("intención = %+v, se esperaba la de operacion sin cambios", got)
	}
	if doer.authRecibido != "Bearer tok-ev" {
		t.Errorf("Authorization aguas abajo = %q, want Bearer tok-ev", doer.authRecibido)
	}
}

// TestOperacionAdjuntosTraducenStatus: el 403 (fuera de alcance) y el 404 de
// operacion salen con su categoría en los tres métodos de adjuntos.
func TestOperacionAdjuntosTraducenStatus(t *testing.T) {
	casos := map[int]platformerrors.Category{
		http.StatusForbidden: platformerrors.CategoryProhibido,
		http.StatusNotFound:  platformerrors.CategoryNoEncontrado,
	}
	for status, want := range casos {
		doer := &doerCaptura{status: status, cuerpo: `{"codigo":"x","mensaje":"y"}`}
		ad, err := NewOperacionHTTP("http://operacion.local", doer)
		if err != nil {
			t.Fatalf("NewOperacionHTTP: %v", err)
		}
		ctx := context.Background()
		_, e1 := ad.ListarAdjuntos(ctx, uuid.New())
		_, e2 := ad.CrearIntencionSubidaAdjunto(ctx, uuid.New(), opclientEntrada())
		_, e3 := ad.DescargarAdjunto(ctx, uuid.New(), uuid.New())
		for nombre, err := range map[string]error{"ListarAdjuntos": e1, "CrearIntencionSubidaAdjunto": e2, "DescargarAdjunto": e3} {
			if got := platformerrors.AsCategory(err); got != want {
				t.Errorf("%s con %d: categoría = %v, se esperaba %v", nombre, status, got, want)
			}
		}
	}
}
