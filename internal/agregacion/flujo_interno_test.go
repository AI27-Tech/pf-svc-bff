package agregacion

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	bffclient "github.com/AI27-Tech/pf-pkg-contracts/packages/api-client-go/bff"
	opclient "github.com/AI27-Tech/pf-pkg-contracts/packages/api-client-go/operacion"
	"github.com/AI27-Tech/pf-pkg-platform-go/auth"
	platformerrors "github.com/AI27-Tech/pf-pkg-platform-go/errors"
	"github.com/google/uuid"
)

var (
	flujoViaje = uuid.MustParse("11111111-1111-1111-1111-111111111111")
	flujoSub   = uuid.MustParse("22222222-2222-2222-2222-222222222222")
)

// peticionFlujo arma la petición con {id} (y {iid}/{eid} si sub no está vacío),
// el Bearer y los claims de la audiencia dada, y la sirve con el handler.
func peticionFlujo(t *testing.T, handler http.HandlerFunc, metodo, url, subParam string, cuerpo any, aud auth.Audiencia) *httptest.ResponseRecorder {
	t.Helper()
	var raw []byte
	if cuerpo != nil {
		var err error
		if raw, err = json.Marshal(cuerpo); err != nil {
			t.Fatalf("marshal cuerpo: %v", err)
		}
	}
	req := httptest.NewRequest(metodo, url, bytes.NewReader(raw))
	req.Header.Set("Authorization", "Bearer tok-panel")
	req.SetPathValue("id", flujoViaje.String())
	if subParam != "" {
		req.SetPathValue(subParam, flujoSub.String())
	}
	req = req.WithContext(auth.ContextWithClaims(req.Context(), &auth.Claims{SesionID: "s", Subject: "u", Audiencia: aud}))
	rec := httptest.NewRecorder()
	handler(rec, req)
	return rec
}

// casoFlujo es una operación del flujo interno: su handler, su petición y la
// respuesta que debe reenviar.
type casoFlujo struct {
	op       string
	handler  func(*Handlers) http.HandlerFunc
	metodo   string
	url      string
	subParam string
	cuerpo   any
	// metodoPuerto es el método de OperacionPort que debe recibir la llamada
	// ("" para obtenerViajeInterno, que solo lee el detalle).
	metodoPuerto string
	status       int
	// verifica afirma el cuerpo de la respuesta y lo que recibió el puerto.
	verifica func(t *testing.T, rec *httptest.ResponseRecorder, f *fakeOperacion)
}

func casosFlujo() []casoFlujo {
	base := "/api/v1/viajes/" + flujoViaje.String()
	eta := time.Date(2026, 11, 3, 10, 0, 0, 0, time.UTC)
	return []casoFlujo{
		{
			op: "obtenerViajeInterno", handler: func(h *Handlers) http.HandlerFunc { return h.ObtenerViajeInterno },
			metodo: http.MethodGet, url: base + "/detalle", status: http.StatusOK,
			verifica: func(t *testing.T, rec *httptest.ResponseRecorder, f *fakeOperacion) {
				var got bffclient.ViajeDetalle
				decodificarCuerpo(t, rec, &got)
				if got.Viaje.Estado != "EN_TRANSITO" || len(got.CondicionesActivas) != 1 {
					t.Errorf("detalle = %+v, se esperaba estado EN_TRANSITO y una condición (vista sin recortar)", got)
				}
				if f.obtenerRecibio != flujoViaje || f.bearerRecibido != "Bearer tok-panel" {
					t.Errorf("ObtenerViaje recibió id %v bearer %q", f.obtenerRecibio, f.bearerRecibido)
				}
			},
		},
		{
			op: "fijarEta", handler: func(h *Handlers) http.HandlerFunc { return h.FijarEta },
			metodo: http.MethodPatch, url: base + "/eta", cuerpo: bffclient.EtaEntrada{EtaEstimado: eta},
			metodoPuerto: "FijarEta", status: http.StatusOK,
			verifica: func(t *testing.T, rec *httptest.ResponseRecorder, f *fakeOperacion) {
				if got := f.flujoRecibio.(opclient.EtaEntrada); !got.EtaEstimado.Equal(eta) {
					t.Errorf("eta reenviado = %v, se esperaba %v", got.EtaEstimado, eta)
				}
				// Responde el detalle interno recompuesto, no el Viaje plano.
				var got bffclient.ViajeDetalle
				decodificarCuerpo(t, rec, &got)
				if got.Viaje.Id != flujoViaje || f.obtenerRecibio != flujoViaje {
					t.Errorf("no recompuso el detalle: id %v, ObtenerViaje(%v)", got.Viaje.Id, f.obtenerRecibio)
				}
			},
		},
		{
			op: "asignarRecursos", handler: func(h *Handlers) http.HandlerFunc { return h.AsignarRecursos },
			metodo: http.MethodPost, url: base + "/asignacion", cuerpo: bffclient.AsignacionEntrada{UnidadesIds: []bffclient.Id{flujoSub}},
			metodoPuerto: "AsignarRecursos", status: http.StatusOK,
			verifica: func(t *testing.T, rec *httptest.ResponseRecorder, f *fakeOperacion) {
				if got := f.flujoRecibio.(opclient.AsignacionEntrada); len(got.UnidadesIds) != 1 || got.UnidadesIds[0] != flujoSub {
					t.Errorf("unidades reenviadas = %v", got.UnidadesIds)
				}
				var got bffclient.Asignacion
				decodificarCuerpo(t, rec, &got)
				if got.Estado != "ASIGNADO" || len(got.Unidades) != 1 || len(got.Unidades[0].Aptitud) != 1 {
					t.Errorf("asignación = %+v, se esperaba ASIGNADO con la aptitud sin recortar", got)
				}
			},
		},
		{
			op: "listarBitacora", handler: func(h *Handlers) http.HandlerFunc { return h.ListarBitacora },
			metodo: http.MethodGet, url: base + "/bitacora?cursor=c-2",
			metodoPuerto: "ListarBitacora", status: http.StatusOK,
			verifica: func(t *testing.T, rec *httptest.ResponseRecorder, f *fakeOperacion) {
				if got := f.flujoRecibio.(opclient.ListarBitacoraParams); got.Cursor == nil || *got.Cursor != "c-2" {
					t.Errorf("cursor reenviado = %v, se esperaba c-2", got.Cursor)
				}
				var got bffclient.PaginaBitacora
				decodificarCuerpo(t, rec, &got)
				if len(got.Items) != 1 || got.Items[0].Autor != "jefe-1" {
					t.Errorf("página = %+v, se esperaba una entrada de jefe-1", got)
				}
			},
		},
		{
			op: "registrarEntradaBitacora", handler: func(h *Handlers) http.HandlerFunc { return h.RegistrarEntradaBitacora },
			metodo: http.MethodPost, url: base + "/bitacora", cuerpo: bffclient.EntradaBitacoraEntrada{Texto: "salida"},
			metodoPuerto: "RegistrarEntradaBitacora", status: http.StatusCreated,
			verifica: func(t *testing.T, rec *httptest.ResponseRecorder, f *fakeOperacion) {
				if got := f.flujoRecibio.(opclient.EntradaBitacoraEntrada); got.Texto != "salida" {
					t.Errorf("texto reenviado = %q", got.Texto)
				}
				var got bffclient.EntradaBitacora
				decodificarCuerpo(t, rec, &got)
				if got.Texto != "salida" || got.Autor != "jefe-1" {
					t.Errorf("entrada = %+v", got)
				}
			},
		},
		{
			op: "abrirIncidencia", handler: func(h *Handlers) http.HandlerFunc { return h.AbrirIncidencia },
			metodo: http.MethodPost, url: base + "/incidencias", cuerpo: bffclient.IncidenciaEntrada{TipoInterno: "falla_mecanica"},
			metodoPuerto: "AbrirIncidencia", status: http.StatusCreated,
			verifica: func(t *testing.T, rec *httptest.ResponseRecorder, f *fakeOperacion) {
				if got := f.flujoRecibio.(opclient.IncidenciaEntrada); got.TipoInterno != "falla_mecanica" {
					t.Errorf("tipo_interno reenviado = %q", got.TipoInterno)
				}
				var got bffclient.Incidencia
				decodificarCuerpo(t, rec, &got)
				if got.TipoInterno != "falla_mecanica" {
					t.Errorf("incidencia = %+v, se esperaba tipo_interno crudo (vista interna)", got)
				}
			},
		},
		{
			op: "resolverIncidencia", handler: func(h *Handlers) http.HandlerFunc { return h.ResolverIncidencia },
			metodo: http.MethodPatch, url: base + "/incidencias/" + flujoSub.String(), subParam: "iid",
			cuerpo:       bffclient.ResolucionIncidenciaEntrada{Resolucion: "reparada"},
			metodoPuerto: "ResolverIncidencia", status: http.StatusOK,
			verifica: func(t *testing.T, _ *httptest.ResponseRecorder, f *fakeOperacion) {
				if got := f.flujoRecibio.(opclient.ResolucionIncidenciaEntrada); got.Resolucion != "reparada" || f.flujoSubID != flujoSub {
					t.Errorf("reenvió resolución %q iid %v", got.Resolucion, f.flujoSubID)
				}
			},
		},
		{
			op: "listarEstadias", handler: func(h *Handlers) http.HandlerFunc { return h.ListarEstadias },
			metodo: http.MethodGet, url: base + "/estadias?cursor=c-3",
			metodoPuerto: "ListarEstadias", status: http.StatusOK,
			verifica: func(t *testing.T, rec *httptest.ResponseRecorder, f *fakeOperacion) {
				if got := f.flujoRecibio.(opclient.ListarEstadiasParams); got.Cursor == nil || *got.Cursor != "c-3" {
					t.Errorf("cursor reenviado = %v, se esperaba c-3", got.Cursor)
				}
				var got bffclient.PaginaEstadias
				decodificarCuerpo(t, rec, &got)
				if len(got.Items) != 1 || got.Items[0].Motivo != "combustible" {
					t.Errorf("página = %+v", got)
				}
			},
		},
		{
			op: "abrirEstadia", handler: func(h *Handlers) http.HandlerFunc { return h.AbrirEstadia },
			metodo: http.MethodPost, url: base + "/estadias",
			cuerpo:       bffclient.EstadiaEntrada{Motivo: "combustible", EjeAutorizacion: "autorizada"},
			metodoPuerto: "AbrirEstadia", status: http.StatusCreated,
			verifica: func(t *testing.T, _ *httptest.ResponseRecorder, f *fakeOperacion) {
				got := f.flujoRecibio.(opclient.EstadiaEntrada)
				if got.Motivo != opclient.Combustible || got.EjeAutorizacion != opclient.Autorizada {
					t.Errorf("estadía reenviada = %+v", got)
				}
			},
		},
		{
			op: "cerrarEstadia", handler: func(h *Handlers) http.HandlerFunc { return h.CerrarEstadia },
			metodo: http.MethodPatch, url: base + "/estadias/" + flujoSub.String(), subParam: "eid", cuerpo: map[string]any{},
			metodoPuerto: "CerrarEstadia", status: http.StatusOK,
			verifica: func(t *testing.T, rec *httptest.ResponseRecorder, f *fakeOperacion) {
				if f.flujoSubID != flujoSub {
					t.Errorf("eid reenviado = %v", f.flujoSubID)
				}
				var got bffclient.Estadia
				decodificarCuerpo(t, rec, &got)
				if got.Cierre == nil {
					t.Errorf("estadía = %+v, se esperaba con cierre", got)
				}
			},
		},
		// Flujo interno, grupo 2.
		{
			op: "listarUnidades", handler: func(h *Handlers) http.HandlerFunc { return h.ListarUnidades },
			metodo: http.MethodGet, url: "/api/v1/unidades?cursor=c-4",
			metodoPuerto: "ListarUnidades", status: http.StatusOK,
			verifica: func(t *testing.T, rec *httptest.ResponseRecorder, f *fakeOperacion) {
				if got := f.flujoRecibio.(opclient.ListarUnidadesParams); got.Cursor == nil || *got.Cursor != "c-4" {
					t.Errorf("cursor reenviado = %v, se esperaba c-4", got.Cursor)
				}
				if f.bearerRecibido != "Bearer tok-panel" {
					t.Errorf("bearer = %q", f.bearerRecibido)
				}
				var got bffclient.PaginaUnidades
				decodificarCuerpo(t, rec, &got)
				if len(got.Items) != 1 || got.Items[0].Id != flujoSub || len(got.Items[0].CustodiosIds) != 1 {
					t.Errorf("página = %+v, se esperaba la unidad del fake", got)
				}
			},
		},
		{
			op: "completarFechaHora", handler: func(h *Handlers) http.HandlerFunc { return h.CompletarFechaHora },
			metodo: http.MethodPatch, url: base + "/fecha-hora", cuerpo: bffclient.FechaHoraEntrada{FechaHoraServicio: eta},
			metodoPuerto: "CompletarFechaHora", status: http.StatusOK,
			verifica: func(t *testing.T, rec *httptest.ResponseRecorder, f *fakeOperacion) {
				if got := f.flujoRecibio.(opclient.FechaHoraEntrada); !got.FechaHoraServicio.Equal(eta) {
					t.Errorf("fecha reenviada = %v, se esperaba %v", got.FechaHoraServicio, eta)
				}
				var got bffclient.ViajeDetalle
				decodificarCuerpo(t, rec, &got)
				if got.Viaje.Id != flujoViaje || f.obtenerRecibio != flujoViaje {
					t.Errorf("no recompuso el detalle: id %v, ObtenerViaje(%v)", got.Viaje.Id, f.obtenerRecibio)
				}
			},
		},
		{
			op: "listarIncidencias", handler: func(h *Handlers) http.HandlerFunc { return h.ListarIncidencias },
			metodo: http.MethodGet, url: base + "/incidencias?cursor=c-5",
			metodoPuerto: "ListarIncidencias", status: http.StatusOK,
			verifica: func(t *testing.T, rec *httptest.ResponseRecorder, f *fakeOperacion) {
				if got := f.flujoRecibio.(opclient.ListarIncidenciasParams); got.Cursor == nil || *got.Cursor != "c-5" {
					t.Errorf("cursor reenviado = %v, se esperaba c-5", got.Cursor)
				}
				var got bffclient.PaginaIncidencias
				decodificarCuerpo(t, rec, &got)
				if len(got.Items) != 1 || got.Items[0].TipoInterno != "falla_mecanica" || got.Items[0].Estado != "resuelta" {
					t.Errorf("página = %+v, se esperaba la incidencia resuelta con tipo_interno crudo", got)
				}
			},
		},
		{
			op: "obtenerFicha", handler: func(h *Handlers) http.HandlerFunc { return h.ObtenerFicha },
			metodo: http.MethodGet, url: base + "/ficha",
			metodoPuerto: "ObtenerFicha", status: http.StatusOK,
			verifica: func(t *testing.T, rec *httptest.ResponseRecorder, _ *fakeOperacion) {
				var got bffclient.FichaTecnica
				decodificarCuerpo(t, rec, &got)
				if got.ViajeId != flujoViaje || got.Tracto == nil || got.Tracto.Placas == nil || *got.Tracto.Placas != "ABC-123" {
					t.Errorf("ficha = %+v, se esperaba la del fake con tracto ABC-123", got)
				}
			},
		},
		{
			op: "guardarFicha", handler: func(h *Handlers) http.HandlerFunc { return h.GuardarFicha },
			metodo: http.MethodPatch, url: base + "/ficha",
			cuerpo:       json.RawMessage(`{"tracto":{"placas":"ABC-123"},"gps_ids":["` + flujoSub.String() + `"]}`),
			metodoPuerto: "GuardarFicha", status: http.StatusOK,
			verifica: func(t *testing.T, rec *httptest.ResponseRecorder, f *fakeOperacion) {
				got := f.flujoRecibio.(opclient.FichaTecnicaEntrada)
				if got.Tracto == nil || got.Tracto.Placas == nil || *got.Tracto.Placas != "ABC-123" || got.GpsIds == nil || (*got.GpsIds)[0] != flujoSub {
					t.Errorf("entrada reenviada = %+v", got)
				}
				var ficha bffclient.FichaTecnica
				decodificarCuerpo(t, rec, &ficha)
				if ficha.ViajeId != flujoViaje {
					t.Errorf("ficha = %+v", ficha)
				}
			},
		},
	}
}

// fakeFlujo devuelve un puerto con una respuesta para cada operación del flujo.
func fakeFlujo() *fakeOperacion {
	ahora := time.Date(2026, 11, 2, 12, 0, 0, 0, time.UTC)
	return &fakeOperacion{
		viaje: &opclient.Viaje{Id: flujoViaje, Estado: "EN_TRANSITO", Origen: "CDMX"},
		detalle: &opclient.ViajeDetalle{
			Viaje:              opclient.Viaje{Id: flujoViaje, Estado: "EN_TRANSITO", Origen: "CDMX"},
			CondicionesActivas: []opclient.CondicionAbierta{{Tipo: "ESTADIA", AbiertaEn: ahora}},
		},
		asignacion: &opclient.Asignacion{ViajeId: flujoViaje, Estado: opclient.ASIGNADO, Unidades: []opclient.UnidadAsignada{{
			UnidadId: flujoSub, Aptitud: []opclient.AptitudRecurso{{RecursoId: uuid.New(), Estado: "apto", Recurso: "vehiculo"}},
		}}},
		bitacoraPaginas: []*opclient.PaginaBitacora{{Items: []opclient.EntradaBitacora{{Id: uuid.New(), ViajeId: flujoViaje, Autor: "jefe-1", Texto: "salida", Ts: ahora}}}},
		entrada:         &opclient.EntradaBitacora{Id: uuid.New(), ViajeId: flujoViaje, Autor: "jefe-1", Texto: "salida", Ts: ahora},
		incidencia:      &opclient.Incidencia{Id: flujoSub, ViajeId: flujoViaje, TipoInterno: "falla_mecanica", Estado: "en_atencion", AbiertaEn: ahora},
		estadia:         &opclient.Estadia{Id: flujoSub, ViajeId: flujoViaje, Motivo: opclient.Combustible, EjeAutorizacion: opclient.Autorizada, Inicio: ahora, Cierre: &ahora},
		paginaEstadia:   &opclient.PaginaEstadias{Items: []opclient.Estadia{{Id: flujoSub, ViajeId: flujoViaje, Motivo: opclient.Combustible, EjeAutorizacion: opclient.Autorizada, Inicio: ahora}}},
		unidades:        &opclient.PaginaUnidades{Items: []opclient.Unidad{{Id: flujoSub, VehiculoId: uuid.New(), CustodiosIds: []opclient.Id{uuid.New()}, Activa: true}}},
		paginaIncidencias: &opclient.PaginaIncidencias{Items: []opclient.Incidencia{{
			Id: flujoSub, ViajeId: flujoViaje, TipoInterno: "falla_mecanica", Estado: opclient.IncidenciaEstadoResuelta, AbiertaEn: ahora, ResueltaEn: &ahora,
		}}},
		ficha: &opclient.FichaTecnica{ViajeId: flujoViaje, Tracto: &opclient.Tracto{Placas: ptr("ABC-123")}},
	}
}

// TestFlujoInternoAudienciaInternaReenvia: con audiencia Interna cada handler
// reenvía al método homónimo del puerto con el id, el cuerpo y el Bearer, y
// responde lo que operacion devolvió, sin recortar.
func TestFlujoInternoAudienciaInternaReenvia(t *testing.T) {
	for _, c := range casosFlujo() {
		t.Run(c.op, func(t *testing.T) {
			f := fakeFlujo()
			rec := peticionFlujo(t, c.handler(NewHandlers(Deps{Operacion: f})), c.metodo, c.url, c.subParam, c.cuerpo, auth.AudienciaInterno)
			if rec.Code != c.status {
				t.Fatalf("status = %d, se esperaba %d; cuerpo: %s", rec.Code, c.status, rec.Body.String())
			}
			if f.flujoLlamado != c.metodoPuerto {
				t.Errorf("método del puerto = %q, se esperaba %q", f.flujoLlamado, c.metodoPuerto)
			}
			if c.metodoPuerto != "" && f.flujoID != flujoViaje {
				t.Errorf("id reenviado = %v, se esperaba %v", f.flujoID, flujoViaje)
			}
			c.verifica(t, rec, f)
		})
	}
}

// TestFlujoInternoAudienciaClienteEs403: un actor Cliente recibe 403 antes de
// que el handler toque el puerto.
func TestFlujoInternoAudienciaClienteEs403(t *testing.T) {
	for _, c := range casosFlujo() {
		t.Run(c.op, func(t *testing.T) {
			f := fakeFlujo()
			rec := peticionFlujo(t, c.handler(NewHandlers(Deps{Operacion: f})), c.metodo, c.url, c.subParam, c.cuerpo, auth.AudienciaCliente)
			if rec.Code != http.StatusForbidden {
				t.Fatalf("status = %d, se esperaba 403; cuerpo: %s", rec.Code, rec.Body.String())
			}
			if f.flujoLlamado != "" || f.obtenerRecibio != uuid.Nil {
				t.Errorf("no debió reenviar con audiencia cliente (llamó %q, ObtenerViaje %v)", f.flujoLlamado, f.obtenerRecibio)
			}
		})
	}
}

// TestFlujoInternoPropagaConflictoConCodigo: el 409 de operacion con su codigo
// de máquina (recurso_no_disponible) llega tal cual al panel.
func TestFlujoInternoPropagaConflictoConCodigo(t *testing.T) {
	f := fakeFlujo()
	f.flujoErr = platformerrors.NewConflictoCodigo("recurso_no_disponible", "bff: AsignarRecursos: conflicto aguas abajo")
	rec := peticionFlujo(t, NewHandlers(Deps{Operacion: f}).AsignarRecursos, http.MethodPost,
		"/api/v1/viajes/"+flujoViaje.String()+"/asignacion", "", bffclient.AsignacionEntrada{UnidadesIds: []bffclient.Id{flujoSub}}, auth.AudienciaInterno)
	if rec.Code != http.StatusConflict {
		t.Fatalf("status = %d, se esperaba 409; cuerpo: %s", rec.Code, rec.Body.String())
	}
	var got bffclient.Error
	decodificarCuerpo(t, rec, &got)
	if got.Codigo != "recurso_no_disponible" {
		t.Errorf("codigo = %q, se esperaba recurso_no_disponible", got.Codigo)
	}
}
