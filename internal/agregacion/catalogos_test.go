package agregacion

import (
	"context"
	"net/http"
	"testing"

	bffclient "github.com/AI27-Tech/pf-pkg-contracts/packages/api-client-go/bff"
	catclient "github.com/AI27-Tech/pf-pkg-contracts/packages/api-client-go/catalogos"
	"github.com/AI27-Tech/pf-pkg-platform-go/auth"
	"github.com/google/uuid"
)

// fakeCatalogos devuelve respuestas fijas y registra el bearer recibido.
type fakeCatalogos struct {
	tiposServicio  *catclient.PaginaValoresCatalogo
	bearerRecibido string
	err            error
}

func (f *fakeCatalogos) ListarClientes(ctx context.Context, bearer string, params catclient.ListarClientesParams) (*catclient.PaginaClientes, error) {
	return nil, f.err
}
func (f *fakeCatalogos) ListarTiposServicio(ctx context.Context, bearer string, params catclient.ListarValoresCatalogoParams) (*catclient.PaginaValoresCatalogo, error) {
	f.bearerRecibido = bearer
	return f.tiposServicio, f.err
}

// TestListarTiposServicioFiltraActivosYRecorta: el handler descarta los valores
// inactivos y recorta a codigo/etiqueta, y propaga el Bearer. Es el valor nuevo
// de la ola: un inactivo de catalogos no debe llegar al selector del cliente.
func TestListarTiposServicioFiltraActivosYRecorta(t *testing.T) {
	fake := &fakeCatalogos{tiposServicio: &catclient.PaginaValoresCatalogo{
		Items: []catclient.ValorCatalogo{
			{Id: uuid.New(), Codigo: "custodia_armada", Etiqueta: "Custodia armada", Estado: catclient.ValorCatalogoEstadoActivo},
			{Id: uuid.New(), Codigo: "viejo", Etiqueta: "Retirado", Estado: catclient.ValorCatalogoEstadoInactivo},
			{Id: uuid.New(), Codigo: "monitoreo_gps", Etiqueta: "Monitoreo GPS", Estado: catclient.ValorCatalogoEstadoActivo},
		},
	}}
	h := NewHandlers(Deps{Catalogos: fake})

	rec := peticionConversacion(t, h.ListarTiposServicio, http.MethodGet,
		"/api/v1/catalogos/tipos-servicio", nil, auth.AudienciaCliente, "")

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rec.Code)
	}
	var out bffclient.PaginaTiposServicio
	decodificarCuerpo(t, rec, &out)
	if len(out.Items) != 2 {
		t.Fatalf("items = %d, want 2 (inactivo descartado): %+v", len(out.Items), out.Items)
	}
	if out.Items[0].Codigo != "custodia_armada" || out.Items[1].Codigo != "monitoreo_gps" {
		t.Errorf("recorte/orden incorrecto: %+v", out.Items)
	}
	if fake.bearerRecibido != "Bearer tok-conv" {
		t.Errorf("no se propagó el Bearer: %q", fake.bearerRecibido)
	}
}

// TestListarTiposServicioRechazaAudienciaInterna: un actor interno recibe 403 y
// el puerto NO se invoca (esta superficie es del wizard del cliente).
func TestListarTiposServicioRechazaAudienciaInterna(t *testing.T) {
	fake := &fakeCatalogos{tiposServicio: &catclient.PaginaValoresCatalogo{}}
	h := NewHandlers(Deps{Catalogos: fake})

	rec := peticionConversacion(t, h.ListarTiposServicio, http.MethodGet,
		"/api/v1/catalogos/tipos-servicio", nil, auth.AudienciaInterno, "")

	if rec.Code != http.StatusForbidden {
		t.Fatalf("status = %d, want 403", rec.Code)
	}
	if fake.bearerRecibido != "" {
		t.Errorf("el puerto no debe invocarse para audiencia interna; bearer = %q", fake.bearerRecibido)
	}
}
