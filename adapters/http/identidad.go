// Package http contiene los adaptadores HTTP de los puertos de salida de bff,
// sobre los clientes generados de pf-pkg-contracts. El adaptador EN PROCESO de
// los mismos puertos NO vive aqui: vive en pf-svc-core, el unico repo que ve a
// bff y a sus destinos a la vez (regla 2 de 00). Estos adaptadores los usa
// cmd/api (standalone, I5) y cualquier raiz que no co-ubique el destino.
package http

import (
	"context"
	"fmt"
	"net/http"
	"time"

	idclient "github.com/AI27-Tech/pf-pkg-contracts/packages/api-client-go/identidad"
	bff "github.com/AI27-Tech/pf-svc-bff"
	"github.com/AI27-Tech/pf-svc-bff/internal/errores"
)

// IdentidadHTTP implementa bff.IdentidadPort con el cliente generado de
// identidad. Respeta el presupuesto de identidad (bff.PresupuestoIdentidad, el
// mismo que debe usar el adaptador en proceso de core, I3) y traduce los errores
// a la taxonomia de la plataforma.
type IdentidadHTTP struct {
	cli         *idclient.ClientWithResponses
	presupuesto time.Duration
}

var _ bff.IdentidadPort = (*IdentidadHTTP)(nil)

// NewIdentidadHTTP crea el adaptador apuntando a la URL base de identidad.
func NewIdentidadHTTP(baseURL string, doer idclient.HttpRequestDoer) (*IdentidadHTTP, error) {
	opts := []idclient.ClientOption{}
	if doer != nil {
		opts = append(opts, idclient.WithHTTPClient(doer))
	}
	cli, err := idclient.NewClientWithResponses(baseURL, opts...)
	if err != nil {
		return nil, fmt.Errorf("bff: crear cliente de identidad: %w", err)
	}
	return &IdentidadHTTP{cli: cli, presupuesto: bff.PresupuestoIdentidad}, nil
}

// EmitirSesion reenvia credenciales a identidad.emitirSesion.
func (a *IdentidadHTTP) EmitirSesion(ctx context.Context, body idclient.CredencialesEntrada) (*idclient.SesionEmitida, error) {
	ctx, cancel := context.WithTimeout(ctx, a.presupuesto)
	defer cancel()

	resp, err := a.cli.EmitirSesionWithResponse(ctx, body)
	if err != nil {
		return nil, errores.DeTransporte("EmitirSesion", err)
	}
	if resp.JSON201 == nil {
		return nil, errores.DeStatus("EmitirSesion", resp.StatusCode())
	}
	return resp.JSON201, nil
}

// RefrescarSesion reenvia el refresh token a identidad.refrescarSesion.
func (a *IdentidadHTTP) RefrescarSesion(ctx context.Context, body idclient.RefrescoEntrada) (*idclient.SesionEmitida, error) {
	ctx, cancel := context.WithTimeout(ctx, a.presupuesto)
	defer cancel()

	resp, err := a.cli.RefrescarSesionWithResponse(ctx, body)
	if err != nil {
		return nil, errores.DeTransporte("RefrescarSesion", err)
	}
	if resp.JSON200 == nil {
		return nil, errores.DeStatus("RefrescarSesion", resp.StatusCode())
	}
	return resp.JSON200, nil
}

// RevocarSesion revoca la sesion dada; identidad publica sesion.revocada.
func (a *IdentidadHTTP) RevocarSesion(ctx context.Context, id idclient.Id) error {
	ctx, cancel := context.WithTimeout(ctx, a.presupuesto)
	defer cancel()

	resp, err := a.cli.RevocarSesionWithResponse(ctx, id)
	if err != nil {
		return errores.DeTransporte("RevocarSesion", err)
	}
	if resp.StatusCode() != http.StatusNoContent && resp.StatusCode() != http.StatusOK {
		return errores.DeStatus("RevocarSesion", resp.StatusCode())
	}
	return nil
}

// ListarSesionesRevocadas lee el snapshot de reconstruccion de tombstones.
func (a *IdentidadHTTP) ListarSesionesRevocadas(ctx context.Context) (*idclient.PaginaRevocaciones, error) {
	ctx, cancel := context.WithTimeout(ctx, a.presupuesto)
	defer cancel()

	resp, err := a.cli.ListarSesionesRevocadasWithResponse(ctx, &idclient.ListarSesionesRevocadasParams{})
	if err != nil {
		return nil, errores.DeTransporte("ListarSesionesRevocadas", err)
	}
	if resp.JSON200 == nil {
		return nil, errores.DeStatus("ListarSesionesRevocadas", resp.StatusCode())
	}
	return resp.JSON200, nil
}

// ObtenerSesion consulta el estado de una sesion (fallback de ADR 26). NUNCA
// verifica el token: solo lee estado.
func (a *IdentidadHTTP) ObtenerSesion(ctx context.Context, id idclient.Id) (*idclient.EstadoSesion, error) {
	ctx, cancel := context.WithTimeout(ctx, a.presupuesto)
	defer cancel()

	resp, err := a.cli.ObtenerSesionWithResponse(ctx, id)
	if err != nil {
		return nil, errores.DeTransporte("ObtenerSesion", err)
	}
	if resp.JSON200 == nil {
		return nil, errores.DeStatus("ObtenerSesion", resp.StatusCode())
	}
	return resp.JSON200, nil
}
