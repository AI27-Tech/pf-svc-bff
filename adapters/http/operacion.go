package http

import (
	"context"
	"fmt"
	"net/http"
	"time"

	opclient "github.com/AI27-Tech/pf-pkg-contracts/packages/api-client-go/operacion"
	bff "github.com/AI27-Tech/pf-svc-bff"
	"github.com/AI27-Tech/pf-svc-bff/internal/errores"
	"github.com/AI27-Tech/pf-svc-bff/internal/tokenctx"
)

// OperacionHTTP implementa bff.OperacionPort con el cliente generado de
// operacion. Respeta el presupuesto de operacion (bff.PresupuestoOperacion, el
// mismo que debe usar el adaptador en proceso de core, I3): presupuesto corto,
// sin reintento ciego, fallo visible y marcado (bff.md (i)). Traduce los errores
// a la taxonomia de la plataforma.
type OperacionHTTP struct {
	cli         *opclient.ClientWithResponses
	presupuesto time.Duration
}

var _ bff.OperacionPort = (*OperacionHTTP)(nil)

// NewOperacionHTTP crea el adaptador apuntando a la URL base de operacion.
func NewOperacionHTTP(baseURL string, doer opclient.HttpRequestDoer) (*OperacionHTTP, error) {
	opts := []opclient.ClientOption{}
	if doer != nil {
		opts = append(opts, opclient.WithHTTPClient(doer))
	}
	cli, err := opclient.NewClientWithResponses(baseURL, opts...)
	if err != nil {
		return nil, fmt.Errorf("bff: crear cliente de operacion: %w", err)
	}
	return &OperacionHTTP{cli: cli, presupuesto: bff.PresupuestoOperacion}, nil
}

// conBearerOperacion propaga el Bearer del usuario (tomado del ctx) hacia
// operacion. Operacion deriva rol/alcance/actor del token (DEC-053, E1-T7), así
// que toda operacion de usuario DEBE llevarlo. Bearer ausente no añade cabecera
// (operacion responderá 401). El adaptador en proceso de core no usa esto: ahí
// los claims ya viajan en el ctx.
func conBearerOperacion(ctx context.Context) opclient.RequestEditorFn {
	bearer := tokenctx.De(ctx)
	return func(_ context.Context, req *http.Request) error {
		if bearer != "" {
			req.Header.Set("Authorization", bearer)
		}
		return nil
	}
}

// CrearViaje reenvia la creacion a operacion.crearViaje. El estado inicial lo
// fija el rol del token en operacion (DEC-026); bff no decide.
func (a *OperacionHTTP) CrearViaje(ctx context.Context, body opclient.CrearViajeEntrada) (*opclient.Viaje, error) {
	ctx, cancel := context.WithTimeout(ctx, a.presupuesto)
	defer cancel()

	resp, err := a.cli.CrearViajeWithResponse(ctx, body, conBearerOperacion(ctx))
	if err != nil {
		return nil, errores.DeTransporte("CrearViaje", err)
	}
	if resp.JSON201 == nil {
		return nil, errores.DeStatus("CrearViaje", resp.StatusCode())
	}
	return resp.JSON201, nil
}

// ListarViajes lista por alcance. El alcance (unidad/cliente) lo resuelve
// operacion en servidor; bff solo reenvia el cursor (DEC-053).
func (a *OperacionHTTP) ListarViajes(ctx context.Context, params opclient.ListarViajesParams) (*opclient.PaginaViajes, error) {
	ctx, cancel := context.WithTimeout(ctx, a.presupuesto)
	defer cancel()

	resp, err := a.cli.ListarViajesWithResponse(ctx, &params, conBearerOperacion(ctx))
	if err != nil {
		return nil, errores.DeTransporte("ListarViajes", err)
	}
	if resp.JSON200 == nil {
		return nil, errores.DeStatus("ListarViajes", resp.StatusCode())
	}
	return resp.JSON200, nil
}

// ObtenerViaje lee el detalle para la composicion de hitos (ADR 27) y para la
// ficha tecnica del expediente de descarga (E6-T3).
func (a *OperacionHTTP) ObtenerViaje(ctx context.Context, id opclient.Id) (*opclient.ViajeDetalle, error) {
	ctx, cancel := context.WithTimeout(ctx, a.presupuesto)
	defer cancel()

	resp, err := a.cli.ObtenerViajeWithResponse(ctx, id, conBearerOperacion(ctx))
	if err != nil {
		return nil, errores.DeTransporte("ObtenerViaje", err)
	}
	if resp.JSON200 == nil {
		return nil, errores.DeStatus("ObtenerViaje", resp.StatusCode())
	}
	return resp.JSON200, nil
}

// ListarBitacora lee UNA pagina del historico de bitacora del viaje (E6-T3). La
// composicion de la descarga la invoca en bucle hasta agotar el cursor; cada
// pagina respeta el presupuesto de operacion.
func (a *OperacionHTTP) ListarBitacora(ctx context.Context, id opclient.Id, params opclient.ListarBitacoraParams) (*opclient.PaginaBitacora, error) {
	ctx, cancel := context.WithTimeout(ctx, a.presupuesto)
	defer cancel()

	resp, err := a.cli.ListarBitacoraWithResponse(ctx, id, &params, conBearerOperacion(ctx))
	if err != nil {
		return nil, errores.DeTransporte("ListarBitacora", err)
	}
	if resp.JSON200 == nil {
		return nil, errores.DeStatus("ListarBitacora", resp.StatusCode())
	}
	return resp.JSON200, nil
}

// ConsultarAuditoria lee UNA pagina de la auditoria relevante del viaje (E6-T3).
// La composicion la invoca en bucle hasta agotar el cursor.
func (a *OperacionHTTP) ConsultarAuditoria(ctx context.Context, id opclient.Id, params opclient.ConsultarAuditoriaParams) (*opclient.PaginaAuditoria, error) {
	ctx, cancel := context.WithTimeout(ctx, a.presupuesto)
	defer cancel()

	resp, err := a.cli.ConsultarAuditoriaWithResponse(ctx, id, &params, conBearerOperacion(ctx))
	if err != nil {
		return nil, errores.DeTransporte("ConsultarAuditoria", err)
	}
	if resp.JSON200 == nil {
		return nil, errores.DeStatus("ConsultarAuditoria", resp.StatusCode())
	}
	return resp.JSON200, nil
}

// RegistrarDescargaViaje pide a operacion que deje el rastro de auditoria de la
// descarga (E1-T7). Sin cuerpo: el actor sale de los claims del token que el
// adaptador propaga aguas abajo. operacion escribe el registro en su propia
// transaccion (bff no tiene base de datos). Devuelve el Viaje descargado.
func (a *OperacionHTTP) RegistrarDescargaViaje(ctx context.Context, id opclient.Id) (*opclient.Viaje, error) {
	ctx, cancel := context.WithTimeout(ctx, a.presupuesto)
	defer cancel()

	resp, err := a.cli.RegistrarDescargaViajeWithResponse(ctx, id, conBearerOperacion(ctx))
	if err != nil {
		return nil, errores.DeTransporte("RegistrarDescargaViaje", err)
	}
	if resp.JSON200 == nil {
		return nil, errores.DeStatus("RegistrarDescargaViaje", resp.StatusCode())
	}
	return resp.JSON200, nil
}
