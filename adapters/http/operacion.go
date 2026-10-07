package http

import (
	"context"
	"fmt"
	"time"

	opclient "github.com/AI27-Tech/pf-pkg-contracts/packages/api-client-go/operacion"
	bff "github.com/AI27-Tech/pf-svc-bff"
	"github.com/AI27-Tech/pf-svc-bff/internal/errores"
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

// CrearViaje reenvia la creacion a operacion.crearViaje. El estado inicial lo
// fija el rol del token en operacion (DEC-026); bff no decide.
func (a *OperacionHTTP) CrearViaje(ctx context.Context, body opclient.CrearViajeEntrada) (*opclient.Viaje, error) {
	ctx, cancel := context.WithTimeout(ctx, a.presupuesto)
	defer cancel()

	resp, err := a.cli.CrearViajeWithResponse(ctx, body)
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

	resp, err := a.cli.ListarViajesWithResponse(ctx, &params)
	if err != nil {
		return nil, errores.DeTransporte("ListarViajes", err)
	}
	if resp.JSON200 == nil {
		return nil, errores.DeStatus("ListarViajes", resp.StatusCode())
	}
	return resp.JSON200, nil
}

// ObtenerViaje lee el detalle para la composicion de hitos (ADR 27).
func (a *OperacionHTTP) ObtenerViaje(ctx context.Context, id opclient.Id) (*opclient.ViajeDetalle, error) {
	ctx, cancel := context.WithTimeout(ctx, a.presupuesto)
	defer cancel()

	resp, err := a.cli.ObtenerViajeWithResponse(ctx, id)
	if err != nil {
		return nil, errores.DeTransporte("ObtenerViaje", err)
	}
	if resp.JSON200 == nil {
		return nil, errores.DeStatus("ObtenerViaje", resp.StatusCode())
	}
	return resp.JSON200, nil
}
