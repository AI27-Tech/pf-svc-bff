package agregacion

import (
	"context"
	"net/http"
	"testing"

	bffclient "github.com/AI27-Tech/pf-pkg-contracts/packages/api-client-go/bff"
	catclient "github.com/AI27-Tech/pf-pkg-contracts/packages/api-client-go/catalogos"
	"github.com/AI27-Tech/pf-pkg-platform-go/auth"
	platformerrors "github.com/AI27-Tech/pf-pkg-platform-go/errors"
	"github.com/google/uuid"
)

// fakeCatalogos devuelve respuestas fijas y registra el bearer recibido.
type fakeCatalogos struct {
	tiposServicio  *catclient.PaginaValoresCatalogo
	bearerRecibido string
	err            error

	// Lecturas por id del detalle de Reacción (E8-T18).
	clientes       map[catclient.Id]*catclient.Cliente
	paginaClientes *catclient.PaginaClientes
	vehiculos      map[catclient.Id]*catclient.Vehiculo
	custodios      map[catclient.Id]*catclient.Custodio
	// errVehiculo, si no es nil, lo devuelve ObtenerVehiculo (catalogos caído).
	errVehiculo error
	// lotesVehiculos registra cada lectura por lote de vehículos (ids).
	lotesVehiculos [][]catclient.Id

	// Catálogo de GPS (E8-T12): respuesta y params recibidos.
	gps        *catclient.PaginaGps
	gpsRecibio *catclient.ListarGpsParams

	// Vehículos y custodios (tracer.39): el último método llamado, lo que
	// recibió y las respuestas.
	recursoLlamado  string
	recursoRecibio  any
	vehiculo        *catclient.Vehiculo
	custodio        *catclient.Custodio
	paginaVehiculos *catclient.PaginaVehiculos
	paginaCustodios *catclient.PaginaCustodios

	// Expediente de la organización (tracer.40).
	orgFake
}

func (f *fakeCatalogos) ObtenerCliente(ctx context.Context, bearer string, id catclient.Id) (*catclient.Cliente, error) {
	f.obtenerClienteLlamadas++
	if c, ok := f.clientes[id]; ok {
		return c, nil
	}
	return nil, platformerrors.NewNoEncontrado("cliente %s", id)
}
func (f *fakeCatalogos) ObtenerVehiculo(ctx context.Context, bearer string, id catclient.Id) (*catclient.Vehiculo, error) {
	if f.errVehiculo != nil {
		return nil, f.errVehiculo
	}
	if v, ok := f.vehiculos[id]; ok {
		return v, nil
	}
	return nil, platformerrors.NewNoEncontrado("vehiculo %s", id)
}
func (f *fakeCatalogos) ObtenerCustodio(ctx context.Context, bearer string, id catclient.Id) (*catclient.Custodio, error) {
	if c, ok := f.custodios[id]; ok {
		return c, nil
	}
	return nil, platformerrors.NewNoEncontrado("custodio %s", id)
}

func (f *fakeCatalogos) ListarClientes(ctx context.Context, bearer string, params catclient.ListarClientesParams) (*catclient.PaginaClientes, error) {
	if params.Ids != nil {
		return f.clientesPorLote(*params.Ids)
	}
	f.bearerRecibido = bearer
	return f.paginaClientes, f.err
}
func (f *fakeCatalogos) ListarTiposServicio(ctx context.Context, bearer string, params catclient.ListarValoresCatalogoParams) (*catclient.PaginaValoresCatalogo, error) {
	f.bearerRecibido = bearer
	return f.tiposServicio, f.err
}

func TestListarClientesSoloAudienciaInterna(t *testing.T) {
	for _, c := range []struct {
		rol  string
		want int
	}{
		{"administrador", http.StatusOK},
		{"jefe_operativo", http.StatusOK},
		{"ejecutivo", http.StatusOK}, // La bandeja resuelve nombres con este listado (DEC-118).
		{"cliente", http.StatusForbidden},
	} {
		t.Run(c.rol, func(t *testing.T) {
			cat := &fakeCatalogos{paginaClientes: &catclient.PaginaClientes{
				Items: []catclient.Cliente{{Id: uuid.New(), Nombre: "Cliente de prueba"}},
			}}
			rec := peticionRol(t, NewHandlers(Deps{Catalogos: cat}).ListarClientes, http.MethodGet, "/api/v1/clientes", "", "", c.rol)
			if rec.Code != c.want {
				t.Fatalf("status = %d, se esperaba %d; cuerpo: %s", rec.Code, c.want, rec.Body)
			}
			if c.want == http.StatusForbidden && cat.bearerRecibido != "" {
				t.Fatal("catalogos no debe recibir la solicitud de audiencia Cliente")
			}
			if c.want == http.StatusOK {
				var out bffclient.PaginaClientes
				decodificarCuerpo(t, rec, &out)
				if len(out.Items) != 1 || out.Items[0].Nombre != "Cliente de prueba" {
					t.Errorf("clientes = %+v, se esperaba Cliente de prueba", out.Items)
				}
			}
		})
	}
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
