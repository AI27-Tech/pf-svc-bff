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
