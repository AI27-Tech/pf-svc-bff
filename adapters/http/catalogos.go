package http

import (
	"context"
	"fmt"
	"net/http"
	"time"

	catclient "github.com/AI27-Tech/pf-pkg-contracts/packages/api-client-go/catalogos"
	bff "github.com/AI27-Tech/pf-svc-bff"
	"github.com/AI27-Tech/pf-svc-bff/internal/agregacion"
	"github.com/AI27-Tech/pf-svc-bff/internal/errores"
)

// CatalogosHTTP implementa bff.CatalogosPort con el cliente generado de
// catalogos. Respeta el presupuesto de catalogos (bff.PresupuestoCatalogos, el
// mismo que debe usar el adaptador en proceso de core, I3): presupuesto corto,
// sin reintento ciego, fallo visible y marcado (bff.md (i)). Traduce los errores
// a la taxonomia de la plataforma.
type CatalogosHTTP struct {
	cli         *catclient.ClientWithResponses
	presupuesto time.Duration
}

var _ agregacion.CatalogosPort = (*CatalogosHTTP)(nil)

// NewCatalogosHTTP crea el adaptador apuntando a la URL base de catalogos.
func NewCatalogosHTTP(baseURL string, doer catclient.HttpRequestDoer) (*CatalogosHTTP, error) {
	opts := []catclient.ClientOption{}
	if doer != nil {
		opts = append(opts, catclient.WithHTTPClient(doer))
	}
	cli, err := catclient.NewClientWithResponses(baseURL, opts...)
	if err != nil {
		return nil, fmt.Errorf("bff: crear cliente de catalogos: %w", err)
	}
	return &CatalogosHTTP{cli: cli, presupuesto: bff.PresupuestoCatalogos}, nil
}

// conBearerCatalogos propaga el Bearer del usuario hacia catalogos. Catalogos
// autoriza por los claims del token; el Bearer debe viajar aguas abajo. Bearer
// vacío no añade cabecera (catalogos responderá 401). El adaptador en proceso de
// core no usa esto: ahí los claims ya viajan en el ctx.
func conBearerCatalogos(bearer string) catclient.RequestEditorFn {
	return func(_ context.Context, req *http.Request) error {
		if bearer != "" {
			req.Header.Set("Authorization", bearer)
		}
		return nil
	}
}

// ListarClientes reenvia la lectura a catalogos.listarClientes. catalogos
// devuelve SOLO clientes activos (DEC-101 #1); bff solo reenvia el cursor. El
// filtro de alcance del jefe_operativo (DEC-026 #3) todavia NO se aplica
// (pendiente, DEC-101 #2).
func (a *CatalogosHTTP) ListarClientes(ctx context.Context, bearer string, params catclient.ListarClientesParams) (*catclient.PaginaClientes, error) {
	ctx, cancel := context.WithTimeout(ctx, a.presupuesto)
	defer cancel()

	resp, err := a.cli.ListarClientesWithResponse(ctx, &params, conBearerCatalogos(bearer))
	if err != nil {
		return nil, errores.DeTransporte("ListarClientes", err)
	}
	if resp.JSON200 == nil {
		return nil, errores.DeStatus("ListarClientes", resp.StatusCode())
	}
	return resp.JSON200, nil
}

// ListarTiposServicio reenvía a catalogos.listarValoresCatalogo fijando el tipo
// tipos_servicio (E9-T4): el cliente no elige familia. Reenvía el cursor; el
// filtro a solo-activos y el recorte a la vista de cliente los hace el handler.
func (a *CatalogosHTTP) ListarTiposServicio(ctx context.Context, bearer string, params catclient.ListarValoresCatalogoParams) (*catclient.PaginaValoresCatalogo, error) {
	ctx, cancel := context.WithTimeout(ctx, a.presupuesto)
	defer cancel()

	resp, err := a.cli.ListarValoresCatalogoWithResponse(ctx, catclient.TiposServicio, &params, conBearerCatalogos(bearer))
	if err != nil {
		return nil, errores.DeTransporte("ListarTiposServicio", err)
	}
	if resp.JSON200 == nil {
		return nil, errores.DeStatus("ListarTiposServicio", resp.StatusCode())
	}
	return resp.JSON200, nil
}

// ListarGps reenvía a catalogos.listarGps: solo dispositivos activos (filtro en
// servidor, DEC-104 #5). Un cursor no UUID es el 400 de catalogos.
func (a *CatalogosHTTP) ListarGps(ctx context.Context, bearer string, params catclient.ListarGpsParams) (*catclient.PaginaGps, error) {
	ctx, cancel := context.WithTimeout(ctx, a.presupuesto)
	defer cancel()

	resp, err := a.cli.ListarGpsWithResponse(ctx, &params, conBearerCatalogos(bearer))
	if err != nil {
		return nil, errores.DeTransporte("ListarGps", err)
	}
	if resp.JSON200 == nil {
		return nil, errores.DeStatus("ListarGps", resp.StatusCode())
	}
	return resp.JSON200, nil
}

// codigoCatalogos409 extrae el codigo de maquina de un 409 de catalogos, igual
// que codigo409 para operacion.
func codigoCatalogos409(e *catclient.Error) string {
	if e == nil {
		return ""
	}
	return e.Codigo
}

// ListarVehiculos reenvía a catalogos.listarVehiculos: activos e inactivos,
// paginado por cursor (E8-T16, DEC-142 #6).
func (a *CatalogosHTTP) ListarVehiculos(ctx context.Context, bearer string, params catclient.ListarVehiculosParams) (*catclient.PaginaVehiculos, error) {
	ctx, cancel := context.WithTimeout(ctx, a.presupuesto)
	defer cancel()

	resp, err := a.cli.ListarVehiculosWithResponse(ctx, &params, conBearerCatalogos(bearer))
	if err != nil {
		return nil, errores.DeTransporte("ListarVehiculos", err)
	}
	if resp.JSON200 == nil {
		return nil, errores.DeStatus("ListarVehiculos", resp.StatusCode())
	}
	return resp.JSON200, nil
}

// CrearVehiculo reenvía a catalogos.crearVehiculo (versión 1).
func (a *CatalogosHTTP) CrearVehiculo(ctx context.Context, bearer string, body catclient.VehiculoEntrada) (*catclient.Vehiculo, error) {
	ctx, cancel := context.WithTimeout(ctx, a.presupuesto)
	defer cancel()

	resp, err := a.cli.CrearVehiculoWithResponse(ctx, body, conBearerCatalogos(bearer))
	if err != nil {
		return nil, errores.DeTransporte("CrearVehiculo", err)
	}
	if resp.JSON201 == nil {
		return nil, errores.DeRespuesta("CrearVehiculo", resp.StatusCode(), codigoCatalogos409(resp.JSON409))
	}
	return resp.JSON201, nil
}

// ActualizarVehiculo reenvía a catalogos.actualizarVehiculo (versión siguiente).
func (a *CatalogosHTTP) ActualizarVehiculo(ctx context.Context, bearer string, id catclient.Id, body catclient.VehiculoCambios) (*catclient.Vehiculo, error) {
	ctx, cancel := context.WithTimeout(ctx, a.presupuesto)
	defer cancel()

	resp, err := a.cli.ActualizarVehiculoWithResponse(ctx, id, body, conBearerCatalogos(bearer))
	if err != nil {
		return nil, errores.DeTransporte("ActualizarVehiculo", err)
	}
	if resp.JSON200 == nil {
		return nil, errores.DeRespuesta("ActualizarVehiculo", resp.StatusCode(), codigoCatalogos409(resp.JSON409))
	}
	return resp.JSON200, nil
}

// ListarCustodios reenvía a catalogos.listarCustodios: activos e inactivos,
// paginado por cursor.
func (a *CatalogosHTTP) ListarCustodios(ctx context.Context, bearer string, params catclient.ListarCustodiosParams) (*catclient.PaginaCustodios, error) {
	ctx, cancel := context.WithTimeout(ctx, a.presupuesto)
	defer cancel()

	resp, err := a.cli.ListarCustodiosWithResponse(ctx, &params, conBearerCatalogos(bearer))
	if err != nil {
		return nil, errores.DeTransporte("ListarCustodios", err)
	}
	if resp.JSON200 == nil {
		return nil, errores.DeStatus("ListarCustodios", resp.StatusCode())
	}
	return resp.JSON200, nil
}

// CrearCustodio reenvía a catalogos.crearCustodio (versión 1).
func (a *CatalogosHTTP) CrearCustodio(ctx context.Context, bearer string, body catclient.CustodioEntrada) (*catclient.Custodio, error) {
	ctx, cancel := context.WithTimeout(ctx, a.presupuesto)
	defer cancel()

	resp, err := a.cli.CrearCustodioWithResponse(ctx, body, conBearerCatalogos(bearer))
	if err != nil {
		return nil, errores.DeTransporte("CrearCustodio", err)
	}
	if resp.JSON201 == nil {
		return nil, errores.DeRespuesta("CrearCustodio", resp.StatusCode(), codigoCatalogos409(resp.JSON409))
	}
	return resp.JSON201, nil
}

// ActualizarCustodio reenvía a catalogos.actualizarCustodio (versión siguiente).
func (a *CatalogosHTTP) ActualizarCustodio(ctx context.Context, bearer string, id catclient.Id, body catclient.CustodioCambios) (*catclient.Custodio, error) {
	ctx, cancel := context.WithTimeout(ctx, a.presupuesto)
	defer cancel()

	resp, err := a.cli.ActualizarCustodioWithResponse(ctx, id, body, conBearerCatalogos(bearer))
	if err != nil {
		return nil, errores.DeTransporte("ActualizarCustodio", err)
	}
	if resp.JSON200 == nil {
		return nil, errores.DeRespuesta("ActualizarCustodio", resp.StatusCode(), codigoCatalogos409(resp.JSON409))
	}
	return resp.JSON200, nil
}

// CrearCliente reenvía a catalogos.crearCliente (PUT idempotente por id,
// tracer.40): 201 si lo creó, 200 si ya existía y se devuelve sin cambios.
func (a *CatalogosHTTP) CrearCliente(ctx context.Context, bearer string, id catclient.Id, body catclient.ClienteEntrada) (*catclient.Cliente, bool, error) {
	ctx, cancel := context.WithTimeout(ctx, a.presupuesto)
	defer cancel()

	resp, err := a.cli.CrearClienteWithResponse(ctx, id, body, conBearerCatalogos(bearer))
	if err != nil {
		return nil, false, errores.DeTransporte("CrearCliente", err)
	}
	if resp.JSON201 != nil {
		return resp.JSON201, true, nil
	}
	if resp.JSON200 != nil {
		return resp.JSON200, false, nil
	}
	return nil, false, errores.DeStatus("CrearCliente", resp.StatusCode())
}

// ActualizarCliente reenvía a catalogos.actualizarCliente (versión siguiente;
// desactivar es `estado` inactivo).
func (a *CatalogosHTTP) ActualizarCliente(ctx context.Context, bearer string, id catclient.Id, body catclient.ClienteCambios) (*catclient.Cliente, error) {
	ctx, cancel := context.WithTimeout(ctx, a.presupuesto)
	defer cancel()

	resp, err := a.cli.ActualizarClienteWithResponse(ctx, id, body, conBearerCatalogos(bearer))
	if err != nil {
		return nil, errores.DeTransporte("ActualizarCliente", err)
	}
	if resp.JSON200 == nil {
		return nil, errores.DeStatus("ActualizarCliente", resp.StatusCode())
	}
	return resp.JSON200, nil
}
