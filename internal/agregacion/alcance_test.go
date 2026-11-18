package agregacion

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"slices"
	"testing"
	"time"

	bffclient "github.com/AI27-Tech/pf-pkg-contracts/packages/api-client-go/bff"
	catclient "github.com/AI27-Tech/pf-pkg-contracts/packages/api-client-go/catalogos"
	msgclient "github.com/AI27-Tech/pf-pkg-contracts/packages/api-client-go/mensajeria"
	opclient "github.com/AI27-Tech/pf-pkg-contracts/packages/api-client-go/operacion"
	"github.com/AI27-Tech/pf-pkg-contracts/packages/api-client-go/roles"
	"github.com/AI27-Tech/pf-pkg-platform-go/auth"
	platformerrors "github.com/AI27-Tech/pf-pkg-platform-go/errors"
	"github.com/google/uuid"
)

// Alcance del monitoreo por cliente en el bff (DEC-149): composición de
// ViajeResumen por audiencia (#11, #14) y el 403 de Ejecutivo y Reacción en la
// lista y el detalle interno (#16), sin romper el detalle del caso de Reacción.

// paginaConEscolta arma dos viajes: A con cliente conocido y escolta de dos
// unidades (una con vehículo en el catálogo, otra con vehículo ausente) y B con
// cliente fuera del catálogo y sin escolta.
func paginaConEscolta() (*fakeOperacion, *fakeCatalogos, [2]opclient.Viaje, [2]opclient.Unidad) {
	clienteA, clienteB := uuid.New(), uuid.New()
	vehConocido, vehAusente := uuid.New(), uuid.New()
	viajes := [2]opclient.Viaje{
		{Id: uuid.New(), Estado: "EN_TRANSITO", Origen: "Manzanillo", ClienteId: &clienteA, Destinos: []opclient.Destino{{Nombre: "CDMX"}}},
		{Id: uuid.New(), Estado: "APROBADO", Origen: "Veracruz", ClienteId: &clienteB},
	}
	unidades := [2]opclient.Unidad{{Id: uuid.New(), VehiculoId: vehConocido}, {Id: uuid.New(), VehiculoId: vehAusente}}
	cursor := "c-sig"
	op := &fakeOperacion{
		pagina:   &opclient.PaginaViajes{Items: viajes[:], CursorSiguiente: &cursor},
		escoltas: map[opclient.Id]*opclient.EscoltaVigente{viajes[0].Id: {ViajeId: viajes[0].Id, Unidades: unidades[:]}},
	}
	eco := "P-17"
	cat := &fakeCatalogos{
		clientes:  map[catclient.Id]*catclient.Cliente{clienteA: {Id: clienteA, Nombre: "Acme Logística", Estado: catclient.ClienteEstadoActivo}},
		vehiculos: map[catclient.Id]*catclient.Vehiculo{vehConocido: {Id: vehConocido, Economico: &eco}},
	}
	return op, cat, viajes, unidades
}

// TestListarViajesInternoComponeClienteYUnidades: para la audiencia Interno
// cada fila lleva el cliente (con nombre si está en catalogos) y las unidades de
// su escolta (con económico si el vehículo está en catalogos); sin escolta,
// unidades vacío. Clientes y vehículos se leen en un solo lote cada uno.
func TestListarViajesInternoComponeClienteYUnidades(t *testing.T) {
	op, cat, viajes, unidades := paginaConEscolta()
	h := NewHandlers(Deps{Operacion: op, Catalogos: cat})

	rec := pedir(h.ListarViajes, http.MethodGet, "/api/v1/viajes", "", "", claimsRol(roles.RolMonitorista))
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d: %s", rec.Code, rec.Body)
	}
	var got bffclient.PaginaViajesResumen
	decodificarCuerpo(t, rec, &got)
	if len(got.Items) != 2 || got.CursorSiguiente == nil || *got.CursorSiguiente != "c-sig" {
		t.Fatalf("página = %+v", got)
	}
	a, b := got.Items[0], got.Items[1]
	if a.Id != viajes[0].Id || a.Estado != "EN_TRANSITO" || a.Origen != "Manzanillo" {
		t.Errorf("fila A = %+v", a)
	}
	if a.Cliente == nil || a.Cliente.Id != *viajes[0].ClienteId || a.Cliente.Nombre == nil || *a.Cliente.Nombre != "Acme Logística" {
		t.Errorf("cliente A = %+v, se esperaba Acme Logística", a.Cliente)
	}
	if a.Unidades == nil || len(*a.Unidades) != 2 {
		t.Fatalf("unidades A = %+v, se esperaban 2", a.Unidades)
	}
	u0, u1 := (*a.Unidades)[0], (*a.Unidades)[1]
	if u0.Id != unidades[0].Id || u0.Economico == nil || *u0.Economico != "P-17" || u1.Id != unidades[1].Id || u1.Economico != nil {
		t.Errorf("unidades A = %+v / %+v, se esperaba P-17 y una sin económico", u0, u1)
	}
	if b.Cliente == nil || b.Cliente.Id != *viajes[1].ClienteId || b.Cliente.Nombre != nil {
		t.Errorf("cliente B = %+v, se esperaba solo el id", b.Cliente)
	}
	if b.Unidades == nil || len(*b.Unidades) != 0 {
		t.Errorf("unidades B = %+v, se esperaba vacío", b.Unidades)
	}
	if op.asignacionLlamadas != 2 {
		t.Errorf("lecturas de escolta = %d, se esperaba una por viaje de la página", op.asignacionLlamadas)
	}
	if len(cat.loteRecibido) != 1 || len(cat.loteRecibido[0]) != 2 {
		t.Errorf("lotes de clientes = %v, se esperaba uno con los dos clientes", cat.loteRecibido)
	}
	if len(cat.lotesVehiculos) != 1 || !slices.Equal(cat.lotesVehiculos[0], []catclient.Id{unidades[0].VehiculoId, unidades[1].VehiculoId}) {
		t.Errorf("lotes de vehículos = %v, se esperaba uno con los dos vehículos", cat.lotesVehiculos)
	}
	// La fila es ViajeResumen: ni destinos ni cliente_id de operacion.
	var crudo struct{ Items []map[string]json.RawMessage }
	_ = json.Unmarshal(rec.Body.Bytes(), &crudo)
	for _, campo := range []string{"destinos", "cliente_id", "creado_en"} {
		if _, ok := crudo.Items[0][campo]; ok {
			t.Errorf("la fila trae %q, que no es de ViajeResumen", campo)
		}
	}
}

// TestListarViajesEscoltaFallida: si la escolta de un viaje
// responde 404 (salió del alcance entre la lista y la lectura), la fila sale
// sin unidades; cualquier otro error tumba la página.
func TestListarViajesEscoltaFallida(t *testing.T) {
	op, cat, _, _ := paginaConEscolta()
	op.escoltaErr = platformerrors.NewNoEncontrado("viaje")
	rec := pedir(NewHandlers(Deps{Operacion: op, Catalogos: cat}).ListarViajes, http.MethodGet, "/", "", "", claimsRol(roles.RolJefeOperativo))
	var got bffclient.PaginaViajesResumen
	decodificarCuerpo(t, rec, &got)
	if rec.Code != http.StatusOK || len(got.Items) != 2 || got.Items[0].Unidades == nil || len(*got.Items[0].Unidades) != 0 || got.Items[0].Cliente == nil {
		t.Fatalf("404 de escolta: status %d, página %+v", rec.Code, got)
	}
	op.escoltaErr = platformerrors.NewDegradado("operacion caída")
	if rec := pedir(NewHandlers(Deps{Operacion: op, Catalogos: cat}).ListarViajes, http.MethodGet, "/", "", "", claimsRol(roles.RolJefeOperativo)); rec.Code != http.StatusServiceUnavailable {
		t.Errorf("operacion caída: status %d, se esperaba 503", rec.Code)
	}
}

// TestListarViajesClienteOmiteClienteYUnidades (DEC-149 #14): para el Cliente
// la fila no lleva cliente ni unidades, y el bff no lee escoltas ni catalogos.
func TestListarViajesClienteOmiteClienteYUnidades(t *testing.T) {
	op, cat, viajes, _ := paginaConEscolta()
	rec := pedir(NewHandlers(Deps{Operacion: op, Catalogos: cat}).ListarViajes, http.MethodGet, "/", "", "", claimsRol(roles.RolCliente))
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d: %s", rec.Code, rec.Body)
	}
	var crudo struct{ Items []map[string]json.RawMessage }
	if err := json.Unmarshal(rec.Body.Bytes(), &crudo); err != nil || len(crudo.Items) != 2 {
		t.Fatalf("cuerpo %s", rec.Body)
	}
	for i, fila := range crudo.Items {
		if len(fila) != 3 || string(fila["id"]) != `"`+viajes[i].Id.String()+`"` {
			t.Errorf("fila %d = %v, se esperaba solo id, estado y origen", i, fila)
		}
	}
	if op.asignacionLlamadas != 0 || len(cat.loteRecibido) != 0 || len(cat.lotesVehiculos) != 0 {
		t.Errorf("para el Cliente no se compone nada: escoltas %d, lotes %v / %v", op.asignacionLlamadas, cat.loteRecibido, cat.lotesVehiculos)
	}
}

// TestEjecutivoYReaccionSinViajes (DEC-149 #16): listarViajes y
// obtenerViajeInterno responden 403 a `ejecutivo` y `reaccion` sin llegar a
// operacion; el monitoreo sí pasa, y un viaje fuera de su alcance es el 404 de
// operacion.
func TestEjecutivoYReaccionSinViajes(t *testing.T) {
	id := uuid.NewString()
	ops := []struct {
		nombre, url, id string
		handler         func(*Handlers) http.HandlerFunc
	}{
		{"listarViajes", "/api/v1/viajes", "", func(h *Handlers) http.HandlerFunc { return h.ListarViajes }},
		{"obtenerViajeInterno", "/api/v1/viajes/" + id + "/detalle", id, func(h *Handlers) http.HandlerFunc { return h.ObtenerViajeInterno }},
	}
	for _, op := range ops {
		for _, rol := range []roles.Rol{roles.RolEjecutivo, roles.RolReaccion} {
			t.Run(op.nombre+"/"+string(rol), func(t *testing.T) {
				f := viajeEn(opclient.ENTRANSITO)
				f.pagina = &opclient.PaginaViajes{Items: []opclient.Viaje{}}
				rec := pedir(op.handler(NewHandlers(Deps{Operacion: f, Catalogos: &fakeCatalogos{}})), http.MethodGet, op.url, "", op.id, claimsRol(rol))
				if rec.Code != http.StatusForbidden {
					t.Fatalf("status = %d, se esperaba 403", rec.Code)
				}
				if f.listarRecibio != (opclient.ListarViajesParams{}) || f.obtenerRecibio != (opclient.Id{}) {
					t.Errorf("un 403 no llega a operacion")
				}
			})
		}
		t.Run(op.nombre+"/monitorista", func(t *testing.T) {
			f := viajeEn(opclient.ENTRANSITO)
			f.pagina = &opclient.PaginaViajes{Items: []opclient.Viaje{}}
			h := NewHandlers(Deps{Operacion: f, Catalogos: &fakeCatalogos{}})
			if rec := pedir(op.handler(h), http.MethodGet, op.url, "", op.id, claimsRol(roles.RolMonitorista)); rec.Code != http.StatusOK {
				t.Fatalf("status = %d, se esperaba 200", rec.Code)
			}
		})
	}
	f := &fakeOperacion{err: platformerrors.NewNoEncontrado("operacion: viaje fuera de alcance")}
	rec := pedir(NewHandlers(Deps{Operacion: f}).ObtenerViajeInterno, http.MethodGet, "/", "", id, claimsRol(roles.RolMonitorista))
	if rec.Code != http.StatusNotFound {
		t.Errorf("fuera de alcance: status = %d, se esperaba 404", rec.Code)
	}
}

// TestReaccionSigueViendoElViajeDeSuCaso (DEC-149 #16, DEC-121 #2): el 403 de
// Reacción en el detalle interno no toca obtenerEmergencia, que lee el viaje
// de operacion con su token y compone el bloque del viaje.
func TestReaccionSigueViendoElViajeDeSuCaso(t *testing.T) {
	clienteID := uuid.New()
	men := &fakeMensajeria{caso: &msgclient.CasoEmergencia{
		Id: uuid.MustParse(emerID), ViajeId: uuid.MustParse(viajeID), Estado: msgclient.EstadoEmergenciaEnAtencion,
		Origen: msgclient.OrigenActivacionCliente, ActivadaEn: time.Now().UTC(),
	}}
	op := viajeEn(opclient.ENTRANSITO)
	op.detalle.Viaje.ClienteId = &clienteID
	cat := &fakeCatalogos{clientes: map[catclient.Id]*catclient.Cliente{clienteID: {Nombre: "Acme Logística"}}}
	h := NewHandlers(Deps{Mensajeria: men, Operacion: op, Catalogos: cat})

	req := httptest.NewRequest(http.MethodGet, "/api/v1/panel/emergencias/"+emerID, nil)
	req.SetPathValue("eid", emerID)
	req.Header.Set("Authorization", "Bearer tok-admin")
	req = req.WithContext(auth.ContextWithClaims(req.Context(), claimsRol(roles.RolReaccion)))
	rec := httptest.NewRecorder()
	h.ObtenerEmergencia(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d: %s", rec.Code, rec.Body)
	}
	var out bffclient.DetalleEmergenciaReaccion
	decodificarCuerpo(t, rec, &out)
	if out.Viaje.ClienteNombre != "Acme Logística" || out.Viaje.Estado != "EN_TRANSITO" {
		t.Errorf("viaje del caso = %+v", out.Viaje)
	}
	if op.obtenerRecibio != uuid.MustParse(viajeID) || op.bearerRecibido != "Bearer tok-admin" {
		t.Errorf("operacion.obtenerViaje recibió %v con %q; se esperaba el viaje del caso con el token de Reacción", op.obtenerRecibio, op.bearerRecibido)
	}
}
