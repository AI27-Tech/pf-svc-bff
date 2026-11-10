package agregacion

import (
	"bytes"
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	bffclient "github.com/AI27-Tech/pf-pkg-contracts/packages/api-client-go/bff"
	catclient "github.com/AI27-Tech/pf-pkg-contracts/packages/api-client-go/catalogos"
	opclient "github.com/AI27-Tech/pf-pkg-contracts/packages/api-client-go/operacion"
	"github.com/AI27-Tech/pf-pkg-platform-go/auth"
	platformerrors "github.com/AI27-Tech/pf-pkg-platform-go/errors"
	"github.com/AI27-Tech/pf-svc-bff/internal/tokenctx"
	"github.com/google/uuid"
)

// Métodos del roster en el doble de operacion: registran la llamada con flujo y
// capturan el Bearer del ctx.

func (f *fakeOperacion) CrearUnidad(ctx context.Context, body opclient.UnidadEntrada) (*opclient.Unidad, error) {
	f.bearerRecibido = tokenctx.De(ctx)
	if err := f.flujo("CrearUnidad", opclient.Id{}, opclient.Id{}, body); err != nil {
		return nil, err
	}
	return f.unidad, nil
}

func (f *fakeOperacion) ObtenerUnidad(ctx context.Context, id opclient.Id) (*opclient.Unidad, error) {
	f.bearerRecibido = tokenctx.De(ctx)
	if f.unidadesRoster != nil && !f.unidadesRoster[id] {
		return nil, platformerrors.NewNoEncontrado("unidad %s", id)
	}
	if err := f.flujo("ObtenerUnidad", id, opclient.Id{}, nil); err != nil {
		return nil, err
	}
	return f.unidad, nil
}

func (f *fakeOperacion) ModificarUnidad(ctx context.Context, id opclient.Id, body opclient.UnidadModificacion) (*opclient.Unidad, error) {
	f.bearerRecibido = tokenctx.De(ctx)
	if err := f.flujo("ModificarUnidad", id, opclient.Id{}, body); err != nil {
		return nil, err
	}
	return f.unidad, nil
}

// Métodos de vehículos y custodios en el doble de catalogos.

func (f *fakeCatalogos) recurso(metodo, bearer string, recibio any) {
	f.recursoLlamado, f.bearerRecibido, f.recursoRecibio = metodo, bearer, recibio
}

func (f *fakeCatalogos) ListarVehiculos(_ context.Context, bearer string, params catclient.ListarVehiculosParams) (*catclient.PaginaVehiculos, error) {
	f.recurso("ListarVehiculos", bearer, params)
	return f.paginaVehiculos, f.err
}

func (f *fakeCatalogos) CrearVehiculo(_ context.Context, bearer string, body catclient.VehiculoEntrada) (*catclient.Vehiculo, error) {
	f.recurso("CrearVehiculo", bearer, body)
	return f.vehiculo, f.err
}

func (f *fakeCatalogos) ActualizarVehiculo(_ context.Context, bearer string, id catclient.Id, body catclient.VehiculoCambios) (*catclient.Vehiculo, error) {
	f.recurso("ActualizarVehiculo", bearer, body)
	return f.vehiculo, f.err
}

func (f *fakeCatalogos) ListarCustodios(_ context.Context, bearer string, params catclient.ListarCustodiosParams) (*catclient.PaginaCustodios, error) {
	f.recurso("ListarCustodios", bearer, params)
	return f.paginaCustodios, f.err
}

func (f *fakeCatalogos) CrearCustodio(_ context.Context, bearer string, body catclient.CustodioEntrada) (*catclient.Custodio, error) {
	f.recurso("CrearCustodio", bearer, body)
	return f.custodio, f.err
}

func (f *fakeCatalogos) ActualizarCustodio(_ context.Context, bearer string, id catclient.Id, body catclient.CustodioCambios) (*catclient.Custodio, error) {
	f.recurso("ActualizarCustodio", bearer, body)
	return f.custodio, f.err
}

// peticionRol sirve la petición con {id} (si no está vacío), el Bearer y los
// claims del rol dado; la audiencia sale del rol como en el IdP (DEC-054).
func peticionRol(t *testing.T, handler http.HandlerFunc, metodo, url, id, cuerpo, rol string) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(metodo, url, bytes.NewBufferString(cuerpo))
	req.Header.Set("Authorization", "Bearer tok-roster")
	if id != "" {
		req.SetPathValue("id", id)
	}
	aud := auth.AudienciaInterno
	if rol == "cliente" {
		aud = auth.AudienciaCliente
	}
	req = req.WithContext(auth.ContextWithClaims(req.Context(), &auth.Claims{SesionID: "s", Subject: "u", Rol: rol, Audiencia: aud}))
	rec := httptest.NewRecorder()
	handler(rec, req)
	return rec
}

// rosterLegible arma una unidad de operacion y el catálogo con sus nombres.
func rosterLegible() (*opclient.Unidad, *fakeCatalogos) {
	veh, cus := uuid.New(), uuid.New()
	eco, placas := "P-101", "ABC-1234"
	tipo := opclient.UnidadTipo("patrulla")
	u := &opclient.Unidad{Id: uuid.New(), VehiculoId: veh, CustodiosIds: []opclient.Id{cus}, Tipo: &tipo, Activa: true}
	cat := &fakeCatalogos{
		vehiculos: map[catclient.Id]*catclient.Vehiculo{veh: {Id: veh, Economico: &eco, Placas: &placas}},
		custodios: map[catclient.Id]*catclient.Custodio{cus: {Id: cus, Nombre: "Juan Pérez"}},
	}
	return u, cat
}

// TestUnidadLegibleEnCrearObtenerModificar: las tres operaciones del roster
// reenvían a operacion con el Bearer y responden la unidad con el vehículo y los
// custodios legibles de catalogos, como listarUnidades.
func TestUnidadLegibleEnCrearObtenerModificar(t *testing.T) {
	id := uuid.New()
	casos := []struct {
		op, metodo, cuerpo string
		handler            func(*Handlers) http.HandlerFunc
		status             int
	}{
		{"CrearUnidad", http.MethodPost, `{"vehiculo_id":"` + uuid.NewString() + `","custodios_ids":["` + uuid.NewString() + `"]}`, func(h *Handlers) http.HandlerFunc { return h.CrearUnidad }, http.StatusCreated},
		{"ObtenerUnidad", http.MethodGet, "", func(h *Handlers) http.HandlerFunc { return h.ObtenerUnidad }, http.StatusOK},
		{"ModificarUnidad", http.MethodPatch, `{"activa":false}`, func(h *Handlers) http.HandlerFunc { return h.ModificarUnidad }, http.StatusOK},
	}
	for _, c := range casos {
		t.Run(c.op, func(t *testing.T) {
			u, cat := rosterLegible()
			f := &fakeOperacion{unidad: u}
			rec := peticionRol(t, c.handler(NewHandlers(Deps{Operacion: f, Catalogos: cat})), c.metodo, "/api/v1/unidades", id.String(), c.cuerpo, "jefe_operativo")
			if rec.Code != c.status {
				t.Fatalf("status = %d, se esperaba %d; cuerpo: %s", rec.Code, c.status, rec.Body)
			}
			if f.flujoLlamado != c.op || f.bearerRecibido != "Bearer tok-roster" {
				t.Errorf("reenvío: llamó %q con Bearer %q", f.flujoLlamado, f.bearerRecibido)
			}
			if c.op != "CrearUnidad" && f.flujoID != id {
				t.Errorf("id reenviado = %s, se esperaba %s", f.flujoID, id)
			}
			if c.op == "ModificarUnidad" {
				if m := f.flujoRecibio.(opclient.UnidadModificacion); m.Activa == nil || *m.Activa {
					t.Errorf("modificación reenviada = %+v, se esperaba activa=false", m)
				}
			}
			var got bffclient.Unidad
			decodificarCuerpo(t, rec, &got)
			if got.Id != u.Id || got.Vehiculo == nil || *got.Vehiculo.Economico != "P-101" || *got.Vehiculo.Placas != "ABC-1234" {
				t.Errorf("unidad = %+v, se esperaba vehículo P-101 / ABC-1234", got)
			}
			if got.Custodios == nil || len(*got.Custodios) != 1 || (*got.Custodios)[0].Nombre != "Juan Pérez" {
				t.Errorf("custodios = %+v, se esperaba [Juan Pérez]", got.Custodios)
			}
		})
	}
}

// TestUnidadesPropaganConflictoConCodigo: los tres 409 del roster salen con su
// codigo y no se compone nada después.
func TestUnidadesPropaganConflictoConCodigo(t *testing.T) {
	casos := []struct {
		codigo  string
		metodo  string
		cuerpo  string
		handler func(*Handlers) http.HandlerFunc
	}{
		{"vehiculo_en_otra_unidad", http.MethodPost, `{"vehiculo_id":"` + uuid.NewString() + `","custodios_ids":["` + uuid.NewString() + `"]}`, func(h *Handlers) http.HandlerFunc { return h.CrearUnidad }},
		{"custodio_en_otra_unidad", http.MethodPost, `{"vehiculo_id":"` + uuid.NewString() + `","custodios_ids":["` + uuid.NewString() + `"]}`, func(h *Handlers) http.HandlerFunc { return h.CrearUnidad }},
		{"unidad_con_reserva_vigente", http.MethodPatch, `{"activa":false}`, func(h *Handlers) http.HandlerFunc { return h.ModificarUnidad }},
	}
	for _, c := range casos {
		t.Run(c.codigo, func(t *testing.T) {
			f := &fakeOperacion{flujoErr: platformerrors.NewConflictoCodigo(c.codigo, "bff: conflicto aguas abajo")}
			rec := peticionRol(t, c.handler(NewHandlers(Deps{Operacion: f, Catalogos: &fakeCatalogos{}})), c.metodo, "/api/v1/unidades", uuid.NewString(), c.cuerpo, "administrador")
			if rec.Code != http.StatusConflict {
				t.Fatalf("status = %d, se esperaba 409", rec.Code)
			}
			var got bffclient.Error
			decodificarCuerpo(t, rec, &got)
			if got.Codigo != c.codigo {
				t.Errorf("codigo = %q, se esperaba %q", got.Codigo, c.codigo)
			}
		})
	}
}

// TestCrearUnidadCampoFueraDeContratoEs400: `tipo` se deriva del vehículo; como
// en operacion, mandarlo es 400 y no se reenvía.
func TestCrearUnidadCampoFueraDeContratoEs400(t *testing.T) {
	f := &fakeOperacion{}
	rec := peticionRol(t, NewHandlers(Deps{Operacion: f}).CrearUnidad, http.MethodPost, "/api/v1/unidades", "",
		`{"vehiculo_id":"`+uuid.NewString()+`","custodios_ids":["`+uuid.NewString()+`"],"tipo":"moto"}`, "jefe_operativo")
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, se esperaba 400", rec.Code)
	}
	if f.flujoLlamado != "" {
		t.Errorf("no debió reenviar (llamó %q)", f.flujoLlamado)
	}
}

// TestUnidadesClienteEs403: audiencia Cliente no llega a operacion.
func TestUnidadesClienteEs403(t *testing.T) {
	f := &fakeOperacion{}
	rec := peticionRol(t, NewHandlers(Deps{Operacion: f}).ObtenerUnidad, http.MethodGet, "/api/v1/unidades", uuid.NewString(), "", "cliente")
	if rec.Code != http.StatusForbidden || f.flujoLlamado != "" {
		t.Errorf("status = %d, llamó %q; se esperaba 403 sin reenvío", rec.Code, f.flujoLlamado)
	}
}

// TestRecursosGuardaDeRol: catalogos no revisa rol (DEC-142 #6). Leen
// jefe_operativo y administrador; escribe solo administrador; monitorista y
// cliente reciben 403 sin que catalogos se toque.
func TestRecursosGuardaDeRol(t *testing.T) {
	veh, cus := uuid.New(), uuid.New()
	handlers := map[string]struct {
		metodo, cuerpo, id string
		handler            func(*Handlers) http.HandlerFunc
		ok                 int
	}{
		"listarVehiculos":    {http.MethodGet, "", "", func(h *Handlers) http.HandlerFunc { return h.ListarVehiculos }, http.StatusOK},
		"obtenerVehiculo":    {http.MethodGet, "", veh.String(), func(h *Handlers) http.HandlerFunc { return h.ObtenerVehiculo }, http.StatusOK},
		"crearVehiculo":      {http.MethodPost, `{"tipo":"moto","estado":"activa"}`, "", func(h *Handlers) http.HandlerFunc { return h.CrearVehiculo }, http.StatusCreated},
		"actualizarVehiculo": {http.MethodPatch, `{"estado":"inactiva"}`, veh.String(), func(h *Handlers) http.HandlerFunc { return h.ActualizarVehiculo }, http.StatusOK},
		"listarCustodios":    {http.MethodGet, "", "", func(h *Handlers) http.HandlerFunc { return h.ListarCustodios }, http.StatusOK},
		"obtenerCustodio":    {http.MethodGet, "", cus.String(), func(h *Handlers) http.HandlerFunc { return h.ObtenerCustodio }, http.StatusOK},
		"crearCustodio":      {http.MethodPost, `{"nombre":"Ana López","estado":"activo"}`, "", func(h *Handlers) http.HandlerFunc { return h.CrearCustodio }, http.StatusCreated},
		"actualizarCustodio": {http.MethodPatch, `{"estado":"inactivo"}`, cus.String(), func(h *Handlers) http.HandlerFunc { return h.ActualizarCustodio }, http.StatusOK},
	}
	lectura := map[string]bool{"listarVehiculos": true, "obtenerVehiculo": true, "listarCustodios": true, "obtenerCustodio": true}
	for _, rol := range []string{"administrador", "jefe_operativo", "monitorista", "supervisor", "cliente"} {
		for op, c := range handlers {
			t.Run(rol+"/"+op, func(t *testing.T) {
				cat := &fakeCatalogos{
					vehiculos:       map[catclient.Id]*catclient.Vehiculo{veh: {Id: veh}},
					custodios:       map[catclient.Id]*catclient.Custodio{cus: {Id: cus, Nombre: "Ana López"}},
					vehiculo:        &catclient.Vehiculo{Id: veh},
					custodio:        &catclient.Custodio{Id: cus},
					paginaVehiculos: &catclient.PaginaVehiculos{Items: []catclient.Vehiculo{}},
					paginaCustodios: &catclient.PaginaCustodios{Items: []catclient.Custodio{}},
				}
				puede := rol == "administrador" || (rol == "jefe_operativo" && lectura[op])
				want := http.StatusForbidden
				if puede {
					want = c.ok
				}
				rec := peticionRol(t, c.handler(NewHandlers(Deps{Catalogos: cat})), c.metodo, "/api/v1/recurso", c.id, c.cuerpo, rol)
				if rec.Code != want {
					t.Fatalf("status = %d, se esperaba %d; cuerpo: %s", rec.Code, want, rec.Body)
				}
				if !puede && cat.recursoLlamado != "" {
					t.Errorf("catalogos no debió tocarse (llamó %q)", cat.recursoLlamado)
				}
				if puede && !lectura[op] && cat.bearerRecibido != "Bearer tok-roster" {
					t.Errorf("Bearer reenviado = %q", cat.bearerRecibido)
				}
			})
		}
	}
}

// TestCrearVehiculoDescartaClienteId: DEC-143. El cliente_id que mande el
// llamante no llega al puerto de catalogos; el resto del alta sí.
func TestCrearVehiculoDescartaClienteId(t *testing.T) {
	cat := &fakeCatalogos{vehiculo: &catclient.Vehiculo{Id: uuid.New()}}
	rec := peticionRol(t, NewHandlers(Deps{Catalogos: cat}).CrearVehiculo, http.MethodPost, "/api/v1/vehiculos", "",
		`{"tipo":"coche","economico":"P-102","estado":"activa","cliente_id":"`+uuid.NewString()+`"}`, "administrador")
	if rec.Code != http.StatusCreated {
		t.Fatalf("status = %d; cuerpo: %s", rec.Code, rec.Body)
	}
	got := cat.recursoRecibio.(catclient.VehiculoEntrada)
	if got.ClienteId != nil {
		t.Errorf("cliente_id reenviado = %s, se esperaba ausente", got.ClienteId)
	}
	if got.Tipo != "coche" || got.Estado != "activa" || got.Economico == nil || *got.Economico != "P-102" {
		t.Errorf("alta reenviada = %+v", got)
	}
}

// TestObtenerAsignacion: la escolta del viaje sale con unidades legibles para
// cualquier rol interno; sin asignación, `unidades` es [] (no null); el Cliente
// recibe 404 sin tocar operacion.
func TestObtenerAsignacion(t *testing.T) {
	viaje := uuid.New()
	url := "/api/v1/viajes/" + viaje.String() + "/asignacion"

	t.Run("monitorista ve unidades legibles", func(t *testing.T) {
		u, cat := rosterLegible()
		f := &fakeOperacion{escolta: &opclient.EscoltaVigente{ViajeId: viaje, Unidades: []opclient.Unidad{*u}}}
		rec := peticionRol(t, NewHandlers(Deps{Operacion: f, Catalogos: cat}).ObtenerAsignacion, http.MethodGet, url, viaje.String(), "", "monitorista")
		if rec.Code != http.StatusOK {
			t.Fatalf("status = %d; cuerpo: %s", rec.Code, rec.Body)
		}
		var got bffclient.EscoltaVigente
		decodificarCuerpo(t, rec, &got)
		if got.ViajeId != viaje || len(got.Unidades) != 1 {
			t.Fatalf("escolta = %+v", got)
		}
		g := got.Unidades[0]
		if g.Vehiculo == nil || *g.Vehiculo.Economico != "P-101" || g.Custodios == nil || (*g.Custodios)[0].Nombre != "Juan Pérez" {
			t.Errorf("unidad = %+v, se esperaba P-101 con Juan Pérez", g)
		}
	})
	t.Run("sin asignación es vacío", func(t *testing.T) {
		rec := peticionRol(t, NewHandlers(Deps{Operacion: &fakeOperacion{}, Catalogos: &fakeCatalogos{}}).ObtenerAsignacion, http.MethodGet, url, viaje.String(), "", "reaccion")
		if rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), `"unidades":[]`) {
			t.Errorf("status = %d cuerpo = %s, se esperaba 200 con unidades []", rec.Code, rec.Body)
		}
	})
	t.Run("cliente 404", func(t *testing.T) {
		f := &fakeOperacion{escoltaErr: platformerrors.NewRoto("no debió llamarse")}
		rec := peticionRol(t, NewHandlers(Deps{Operacion: f}).ObtenerAsignacion, http.MethodGet, url, viaje.String(), "", "cliente")
		if rec.Code != http.StatusNotFound {
			t.Errorf("status = %d, se esperaba 404", rec.Code)
		}
	})
}
