package agregacion

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	bffclient "github.com/AI27-Tech/pf-pkg-contracts/packages/api-client-go/bff"
	msgclient "github.com/AI27-Tech/pf-pkg-contracts/packages/api-client-go/mensajeria"
	"github.com/AI27-Tech/pf-pkg-platform-go/auth"
	"github.com/google/uuid"
)

// fakeMensajeria registra las entradas recibidas y devuelve respuestas/err fijos.
type fakeMensajeria struct {
	pagina                *msgclient.PaginaEmergencias
	caso                  *msgclient.CasoEmergencia
	reclamo               *msgclient.Reclamo
	err                   error
	bearerRecibido        string
	notaRecibida          *msgclient.NotaEmergenciaEntrada
	versionRecibida       *msgclient.VersionReclamo
	transferenciaRecibida *msgclient.TransferenciaEntrada
	resolucionRecibida    *msgclient.ResolucionEmergenciaEntrada
}

func (f *fakeMensajeria) ListarEmergencias(ctx context.Context, bearer string, viajeID msgclient.Id) (*msgclient.PaginaEmergencias, error) {
	f.bearerRecibido = bearer
	return f.pagina, f.err
}
func (f *fakeMensajeria) ActivarEmergencia(ctx context.Context, bearer string, viajeID msgclient.Id, body msgclient.ActivarEmergenciaEntrada) (*msgclient.CasoEmergencia, error) {
	f.bearerRecibido = bearer
	return f.caso, f.err
}
func (f *fakeMensajeria) AgregarNotaEmergencia(ctx context.Context, bearer string, viajeID, emergenciaID msgclient.Id, body msgclient.NotaEmergenciaEntrada) error {
	f.bearerRecibido = bearer
	f.notaRecibida = &body
	return f.err
}

func (f *fakeMensajeria) ReclamarEmergencia(ctx context.Context, bearer string, viajeID, emergenciaID msgclient.Id, body msgclient.ReclamarEmergenciaJSONRequestBody) (*msgclient.Reclamo, error) {
	f.bearerRecibido = bearer
	f.versionRecibida = &body.VersionEsperada
	return f.reclamo, f.err
}
func (f *fakeMensajeria) LiberarReclamoEmergencia(ctx context.Context, bearer string, viajeID, emergenciaID, reclamoID msgclient.Id, params msgclient.LiberarReclamoEmergenciaParams) error {
	f.bearerRecibido = bearer
	f.versionRecibida = &params.VersionEsperada
	return f.err
}
func (f *fakeMensajeria) TransferirEmergencia(ctx context.Context, bearer string, viajeID, emergenciaID msgclient.Id, body msgclient.TransferirEmergenciaJSONRequestBody) (*msgclient.Reclamo, error) {
	f.bearerRecibido = bearer
	f.transferenciaRecibida = &body
	return f.reclamo, f.err
}
func (f *fakeMensajeria) ResolverEmergencia(ctx context.Context, bearer string, viajeID, emergenciaID msgclient.Id, body msgclient.ResolverEmergenciaJSONRequestBody) (*msgclient.CasoEmergencia, error) {
	f.bearerRecibido = bearer
	f.resolucionRecibida = &body
	return f.caso, f.err
}
func (f *fakeMensajeria) CerrarEmergencia(ctx context.Context, bearer string, viajeID, emergenciaID msgclient.Id) (*msgclient.CasoEmergencia, error) {
	f.bearerRecibido = bearer
	return f.caso, f.err
}

const viajeID = "aaaaaaaa-aaaa-aaaa-aaaa-aaaaaaaaaaaa"
const emerID = "bbbbbbbb-bbbb-bbbb-bbbb-bbbbbbbbbbbb"

// peticionEmergencia arma una petición con claims, el Bearer en el header y hasta
// dos path values ({id}, {eid}).
func peticionEmergencia(t *testing.T, handler http.HandlerFunc, metodo, url string, cuerpo any, bearer, id, eid string) *httptest.ResponseRecorder {
	t.Helper()
	rec := httptest.NewRecorder()
	var req *http.Request
	if cuerpo != nil {
		b, err := json.Marshal(cuerpo)
		if err != nil {
			t.Fatalf("marshal cuerpo: %v", err)
		}
		req = httptest.NewRequest(metodo, url, bytes.NewReader(b))
		req.Header.Set("Content-Type", "application/json")
	} else {
		req = httptest.NewRequest(metodo, url, nil)
	}
	if bearer != "" {
		req.Header.Set("Authorization", bearer)
	}
	if id != "" {
		req.SetPathValue("id", id)
	}
	if eid != "" {
		req.SetPathValue("eid", eid)
	}
	req = req.WithContext(auth.ContextWithClaims(req.Context(), &auth.Claims{Subject: "u-cliente", Audiencia: auth.AudienciaCliente}))
	handler(rec, req)
	return rec
}

// TestListarEmergenciasClienteCalculaIndicadorYPropagaToken: la lista con un caso
// abierto marca emergencia_activa=true, traduce a la vista cliente y propaga el
// Bearer aguas abajo.
func TestListarEmergenciasClienteCalculaIndicadorYPropagaToken(t *testing.T) {
	fake := &fakeMensajeria{pagina: &msgclient.PaginaEmergencias{
		Items: []msgclient.CasoEmergencia{
			{Id: uuid.MustParse(emerID), Estado: msgclient.EstadoEmergenciaAbierta, ActivadaEn: time.Date(2026, 10, 20, 8, 0, 0, 0, time.UTC)},
		},
	}}
	h := NewHandlers(Deps{Mensajeria: fake})

	rec := peticionEmergencia(t, h.ListarEmergenciasCliente, http.MethodGet,
		"/api/v1/viajes/"+viajeID+"/emergencias", nil, "Bearer tok-123", viajeID, "")

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rec.Code)
	}
	var out bffclient.EmergenciasCliente
	decodificarCuerpo(t, rec, &out)
	if !out.EmergenciaActiva {
		t.Error("emergencia_activa debe ser true con un caso abierto")
	}
	if len(out.Casos) != 1 || out.Casos[0].Estado != bffclient.EmergenciaClienteEstadoAbierta {
		t.Fatalf("vista cliente incorrecta: %+v", out.Casos)
	}
	if fake.bearerRecibido != "Bearer tok-123" {
		t.Errorf("no se propagó el Bearer: %q", fake.bearerRecibido)
	}
}

// TestListarEmergenciasResueltaNoSostieneIndicador: un caso resuelto no marca
// emergencia_activa.
func TestListarEmergenciasResueltaNoSostieneIndicador(t *testing.T) {
	fake := &fakeMensajeria{pagina: &msgclient.PaginaEmergencias{
		Items: []msgclient.CasoEmergencia{
			{Id: uuid.MustParse(emerID), Estado: msgclient.EstadoEmergenciaResuelta, ActivadaEn: time.Now().UTC()},
		},
	}}
	h := NewHandlers(Deps{Mensajeria: fake})

	rec := peticionEmergencia(t, h.ListarEmergenciasCliente, http.MethodGet,
		"/api/v1/viajes/"+viajeID+"/emergencias", nil, "Bearer t", viajeID, "")

	var out bffclient.EmergenciasCliente
	decodificarCuerpo(t, rec, &out)
	if out.EmergenciaActiva {
		t.Error("un caso resuelto NO debe sostener emergencia_activa (ADR 21 §6)")
	}
}

// TestActivarEmergenciaClienteDevuelveVistaCliente: activar devuelve 201 con la
// vista cliente del caso.
func TestActivarEmergenciaClienteDevuelveVistaCliente(t *testing.T) {
	fake := &fakeMensajeria{caso: &msgclient.CasoEmergencia{
		Id: uuid.MustParse(emerID), Estado: msgclient.EstadoEmergenciaAbierta, ActivadaEn: time.Now().UTC(),
	}}
	h := NewHandlers(Deps{Mensajeria: fake})

	rec := peticionEmergencia(t, h.ActivarEmergenciaCliente, http.MethodPost,
		"/api/v1/viajes/"+viajeID+"/emergencias",
		bffclient.ActivarEmergenciaClienteEntrada{ActivacionId: uuid.New()}, "Bearer t", viajeID, "")

	if rec.Code != http.StatusCreated {
		t.Fatalf("status = %d, want 201", rec.Code)
	}
	var out bffclient.EmergenciaCliente
	decodificarCuerpo(t, rec, &out)
	if out.Id != uuid.MustParse(emerID) {
		t.Errorf("id devuelto = %v, want %s", out.Id, emerID)
	}
}

// TestAgregarNotaClienteReenviaCuerpoYResponde201: la nota se reenvía con su
// cuerpo y la respuesta del portal cliente es 201 sin body.
func TestAgregarNotaClienteReenviaCuerpoYResponde201(t *testing.T) {
	fake := &fakeMensajeria{}
	h := NewHandlers(Deps{Mensajeria: fake})

	rec := peticionEmergencia(t, h.AgregarNotaEmergenciaCliente, http.MethodPost,
		"/api/v1/viajes/"+viajeID+"/emergencias/"+emerID+"/notas",
		bffclient.NotaEmergenciaClienteEntrada{Cuerpo: "el conductor reporta un bloqueo"}, "Bearer t", viajeID, emerID)

	if rec.Code != http.StatusCreated {
		t.Fatalf("status = %d, want 201", rec.Code)
	}
	if fake.notaRecibida == nil || fake.notaRecibida.Cuerpo != "el conductor reporta un bloqueo" {
		t.Fatalf("no se reenvió el cuerpo de la nota: %+v", fake.notaRecibida)
	}
}

// TestActivarEmergenciaIdInvalidoEs400: un id de viaje mal formado es 400 y no
// llega al puerto.
func TestActivarEmergenciaIdInvalidoEs400(t *testing.T) {
	fake := &fakeMensajeria{}
	h := NewHandlers(Deps{Mensajeria: fake})

	rec := peticionEmergencia(t, h.ActivarEmergenciaCliente, http.MethodPost,
		"/api/v1/viajes/no-uuid/emergencias",
		bffclient.ActivarEmergenciaClienteEntrada{ActivacionId: uuid.New()}, "Bearer t", "no-uuid", "")

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400", rec.Code)
	}
}

const reclamoID = "cccccccc-cccc-cccc-cccc-cccccccccccc"

// peticionPanel arma una petición del panel interno: claims de audiencia interna,
// Bearer en el header, path values {id}/{eid} y, opcionalmente, {rid} y un query
// crudo (para version_esperada).
func peticionPanel(t *testing.T, handler http.HandlerFunc, metodo, url string, cuerpo any, bearer, id, eid, rid string) *httptest.ResponseRecorder {
	t.Helper()
	rec := httptest.NewRecorder()
	var req *http.Request
	if cuerpo != nil {
		b, err := json.Marshal(cuerpo)
		if err != nil {
			t.Fatalf("marshal cuerpo: %v", err)
		}
		req = httptest.NewRequest(metodo, url, bytes.NewReader(b))
		req.Header.Set("Content-Type", "application/json")
	} else {
		req = httptest.NewRequest(metodo, url, nil)
	}
	if bearer != "" {
		req.Header.Set("Authorization", bearer)
	}
	if id != "" {
		req.SetPathValue("id", id)
	}
	if eid != "" {
		req.SetPathValue("eid", eid)
	}
	if rid != "" {
		req.SetPathValue("rid", rid)
	}
	req = req.WithContext(auth.ContextWithClaims(req.Context(), &auth.Claims{Subject: "u-reaccion", Audiencia: auth.AudienciaInterno}))
	handler(rec, req)
	return rec
}

// TestListarEmergenciasPanelReenviaVistaCompleta: el panel reenvía la
// PaginaEmergencias de mensajeria sin recortar y propaga el Bearer.
func TestListarEmergenciasPanelReenviaVistaCompleta(t *testing.T) {
	origen := msgclient.OrigenActivacionOperacion
	fake := &fakeMensajeria{pagina: &msgclient.PaginaEmergencias{
		Items: []msgclient.CasoEmergencia{
			{Id: uuid.MustParse(emerID), Estado: msgclient.EstadoEmergenciaEnAtencion, Origen: origen, ActivadaEn: time.Now().UTC()},
		},
	}}
	h := NewHandlers(Deps{Mensajeria: fake})

	rec := peticionPanel(t, h.ListarEmergenciasPanel, http.MethodGet,
		"/api/v1/panel/viajes/"+viajeID+"/emergencias", nil, "Bearer tok-int", viajeID, "", "")

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rec.Code)
	}
	var out msgclient.PaginaEmergencias
	decodificarCuerpo(t, rec, &out)
	if len(out.Items) != 1 || out.Items[0].Origen != msgclient.OrigenActivacionOperacion {
		t.Fatalf("la vista interna debe conservar el origen: %+v", out.Items)
	}
	if fake.bearerRecibido != "Bearer tok-int" {
		t.Errorf("no se propagó el Bearer: %q", fake.bearerRecibido)
	}
}

// TestReclamarEmergenciaPanelReenviaVersionYDevuelveReclamo: reclamar propaga la
// version_esperada del cuerpo y devuelve 201 con el reclamo.
func TestReclamarEmergenciaPanelReenviaVersionYDevuelveReclamo(t *testing.T) {
	fake := &fakeMensajeria{reclamo: &msgclient.Reclamo{Id: uuid.MustParse(reclamoID), Version: 1}}
	h := NewHandlers(Deps{Mensajeria: fake})

	rec := peticionPanel(t, h.ReclamarEmergenciaPanel, http.MethodPost,
		"/api/v1/panel/viajes/"+viajeID+"/emergencias/"+emerID+"/reclamos",
		msgclient.VersionEsperadaEntrada{VersionEsperada: 0}, "Bearer t", viajeID, emerID, "")

	if rec.Code != http.StatusCreated {
		t.Fatalf("status = %d, want 201", rec.Code)
	}
	if fake.versionRecibida == nil || *fake.versionRecibida != 0 {
		t.Fatalf("no se reenvió version_esperada: %+v", fake.versionRecibida)
	}
	var out msgclient.Reclamo
	decodificarCuerpo(t, rec, &out)
	if out.Id != uuid.MustParse(reclamoID) {
		t.Errorf("reclamo devuelto = %v, want %s", out.Id, reclamoID)
	}
}

// TestLiberarReclamoPanelLeeVersionDelQueryYResponde204: liberar toma
// version_esperada del query, usa el rid de la ruta y responde 204 sin cuerpo.
func TestLiberarReclamoPanelLeeVersionDelQueryYResponde204(t *testing.T) {
	fake := &fakeMensajeria{}
	h := NewHandlers(Deps{Mensajeria: fake})

	rec := peticionPanel(t, h.LiberarReclamoEmergenciaPanel, http.MethodDelete,
		"/api/v1/panel/viajes/"+viajeID+"/emergencias/"+emerID+"/reclamos/"+reclamoID+"?version_esperada=3",
		nil, "Bearer t", viajeID, emerID, reclamoID)

	if rec.Code != http.StatusNoContent {
		t.Fatalf("status = %d, want 204", rec.Code)
	}
	if fake.versionRecibida == nil || *fake.versionRecibida != 3 {
		t.Fatalf("version_esperada del query no llegó: %+v", fake.versionRecibida)
	}
}

// TestLiberarReclamoPanelVersionInvalidaEs400: una version_esperada no numérica
// es 400 y no llega al puerto.
func TestLiberarReclamoPanelVersionInvalidaEs400(t *testing.T) {
	fake := &fakeMensajeria{}
	h := NewHandlers(Deps{Mensajeria: fake})

	rec := peticionPanel(t, h.LiberarReclamoEmergenciaPanel, http.MethodDelete,
		"/api/v1/panel/viajes/"+viajeID+"/emergencias/"+emerID+"/reclamos/"+reclamoID+"?version_esperada=abc",
		nil, "Bearer t", viajeID, emerID, reclamoID)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400", rec.Code)
	}
	if fake.versionRecibida != nil {
		t.Error("no debió llamarse al puerto con version inválida")
	}
}

// TestResolverEmergenciaPanelReenviaNota: resolver propaga la nota y devuelve 200
// con el caso.
func TestResolverEmergenciaPanelReenviaNota(t *testing.T) {
	fake := &fakeMensajeria{caso: &msgclient.CasoEmergencia{Id: uuid.MustParse(emerID), Estado: msgclient.EstadoEmergenciaResuelta, ActivadaEn: time.Now().UTC()}}
	h := NewHandlers(Deps{Mensajeria: fake})

	rec := peticionPanel(t, h.ResolverEmergenciaPanel, http.MethodPost,
		"/api/v1/panel/viajes/"+viajeID+"/emergencias/"+emerID+"/resolucion",
		msgclient.ResolucionEmergenciaEntrada{Nota: "unidad liberada, sin novedad", VersionEsperada: 2}, "Bearer t", viajeID, emerID, "")

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rec.Code)
	}
	if fake.resolucionRecibida == nil || fake.resolucionRecibida.Nota != "unidad liberada, sin novedad" {
		t.Fatalf("no se reenvió la nota de resolución: %+v", fake.resolucionRecibida)
	}
}

// TestCerrarEmergenciaPanelDevuelveCaso: cerrar no lleva cuerpo y devuelve 200
// con el caso cerrado.
func TestCerrarEmergenciaPanelDevuelveCaso(t *testing.T) {
	fake := &fakeMensajeria{caso: &msgclient.CasoEmergencia{Id: uuid.MustParse(emerID), Estado: msgclient.EstadoEmergenciaCerrada, ActivadaEn: time.Now().UTC()}}
	h := NewHandlers(Deps{Mensajeria: fake})

	rec := peticionPanel(t, h.CerrarEmergenciaPanel, http.MethodPost,
		"/api/v1/panel/viajes/"+viajeID+"/emergencias/"+emerID+"/cierre",
		nil, "Bearer t", viajeID, emerID, "")

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rec.Code)
	}
	var out msgclient.CasoEmergencia
	decodificarCuerpo(t, rec, &out)
	if out.Estado != msgclient.EstadoEmergenciaCerrada {
		t.Errorf("estado devuelto = %v, want cerrada", out.Estado)
	}
}
