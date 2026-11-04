package http

import (
	"context"
	"fmt"
	"net/http"
	"time"

	repclient "github.com/AI27-Tech/pf-pkg-contracts/packages/api-client-go/reportes"

	bff "github.com/AI27-Tech/pf-svc-bff"
	"github.com/AI27-Tech/pf-svc-bff/internal/errores"
	"github.com/AI27-Tech/pf-svc-bff/internal/tokenctx"
)

// ReportesHTTP implementa bff.ReportesPort con el cliente generado de reportes.
// Mismo presupuesto corto y sin reintento ciego que las demás aristas (bff.md
// (i)). Propaga el Bearer del usuario: reportes autoriza la audiencia por los
// claims del token.
type ReportesHTTP struct {
	cli         *repclient.ClientWithResponses
	presupuesto time.Duration
}

var _ bff.ReportesPort = (*ReportesHTTP)(nil)

// NewReportesHTTP crea el adaptador apuntando a la URL base de reportes.
func NewReportesHTTP(baseURL string, doer repclient.HttpRequestDoer) (*ReportesHTTP, error) {
	opts := []repclient.ClientOption{}
	if doer != nil {
		opts = append(opts, repclient.WithHTTPClient(doer))
	}
	cli, err := repclient.NewClientWithResponses(baseURL, opts...)
	if err != nil {
		return nil, fmt.Errorf("bff: crear cliente de reportes: %w", err)
	}
	return &ReportesHTTP{cli: cli, presupuesto: bff.PresupuestoReportes}, nil
}

// conBearerReportes propaga el Bearer del usuario (tomado del ctx) hacia
// reportes. Bearer ausente no añade cabecera (reportes responderá 401).
func conBearerReportes(ctx context.Context) repclient.RequestEditorFn {
	bearer := tokenctx.De(ctx)
	return func(_ context.Context, req *http.Request) error {
		if bearer != "" {
			req.Header.Set("Authorization", bearer)
		}
		return nil
	}
}

// ReporteViajes reenvia a reportes.reporteViajes. Devuelve el reporte tal cual,
// con proyeccion_al y stale (DEC-033).
func (a *ReportesHTTP) ReporteViajes(ctx context.Context, params repclient.ReporteViajesParams) (*repclient.ReporteViajes, error) {
	ctx, cancel := context.WithTimeout(ctx, a.presupuesto)
	defer cancel()

	resp, err := a.cli.ReporteViajesWithResponse(ctx, &params, conBearerReportes(ctx))
	if err != nil {
		return nil, errores.DeTransporte("ReporteViajes", err)
	}
	if resp.JSON200 == nil {
		return nil, errores.DeStatus("ReporteViajes", resp.StatusCode())
	}
	return resp.JSON200, nil
}

// ReporteEstados reenvia a reportes.reporteEstados.
func (a *ReportesHTTP) ReporteEstados(ctx context.Context, params repclient.ReporteEstadosParams) (*repclient.ReporteEstados, error) {
	ctx, cancel := context.WithTimeout(ctx, a.presupuesto)
	defer cancel()

	resp, err := a.cli.ReporteEstadosWithResponse(ctx, &params, conBearerReportes(ctx))
	if err != nil {
		return nil, errores.DeTransporte("ReporteEstados", err)
	}
	if resp.JSON200 == nil {
		return nil, errores.DeStatus("ReporteEstados", resp.StatusCode())
	}
	return resp.JSON200, nil
}
