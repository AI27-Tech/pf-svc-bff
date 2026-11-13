package agregacion

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	bffclient "github.com/AI27-Tech/pf-pkg-contracts/packages/api-client-go/bff"
	catclient "github.com/AI27-Tech/pf-pkg-contracts/packages/api-client-go/catalogos"
	msgclient "github.com/AI27-Tech/pf-pkg-contracts/packages/api-client-go/mensajeria"
	opclient "github.com/AI27-Tech/pf-pkg-contracts/packages/api-client-go/operacion"
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
	conversacion          *msgclient.Conversacion
	paginaMensajes        *msgclient.PaginaMensajes
	mensaje               *msgclient.Mensaje
	paramsRecibidos       *msgclient.ListarMensajesParams
	envioRecibido         *msgclient.EnviarMensajeEntrada

	// Bandeja y conversación interna del pool (E8-T15).
	trabajo              *msgclient.TrabajoBandeja
	paramsBandeja        *msgclient.ListarTrabajoMiBandejaParams
	reclamarRecibido     *msgclient.VersionEsperadaEntrada
	liberarRecibido      *msgclient.LiberarReclamoParams
	transferenciaConvRec *msgclient.TransferenciaEntrada
	atendidaRecibido     *msgclient.VersionEsperadaEntrada
	reciboEntregadoRec   *msgclient.ReciboEntrada
	reciboLeidoRec       *msgclient.ReciboEntrada

	// Activación (E8-T18): reentrega idempotente. Bandeja de Reacción y caso por
	// id (tracer.34): respuestas por tipo.
	idempotente  bool
	bandejaEmerg *msgclient.PaginaBandejaEmergencias
	notasEmerg   *msgclient.PaginaNotasEmergencia
	reaccion     *msgclient.EmergenciaReaccion
}

func (f *fakeMensajeria) ListarBandejaEmergencias(ctx context.Context, bearer string, params msgclient.ListarBandejaEmergenciasParams) (*msgclient.PaginaBandejaEmergencias, error) {
	f.bearerRecibido = bearer
	return f.bandejaEmerg, f.err
}
func (f *fakeMensajeria) ObtenerEmergencia(ctx context.Context, bearer string, emergenciaID msgclient.Id) (*msgclient.CasoEmergencia, error) {
	f.bearerRecibido = bearer
	return f.caso, f.err
}
func (f *fakeMensajeria) ListarNotasEmergenciaPorId(ctx context.Context, bearer string, emergenciaID msgclient.Id, params msgclient.ListarNotasEmergenciaPorIdParams) (*msgclient.PaginaNotasEmergencia, error) {
	f.bearerRecibido = bearer
	return f.notasEmerg, f.err
}
func (f *fakeMensajeria) ReclamarEmergenciaPorId(ctx context.Context, bearer string, emergenciaID msgclient.Id, body msgclient.VersionEsperadaEntrada) (*msgclient.Reclamo, error) {
	f.bearerRecibido = bearer
	return f.reclamo, f.err
}
func (f *fakeMensajeria) LiberarReclamoEmergenciaPorId(ctx context.Context, bearer string, emergenciaID, reclamoID msgclient.Id, params msgclient.LiberarReclamoEmergenciaPorIdParams) error {
	f.bearerRecibido = bearer
	f.versionRecibida = &params.VersionEsperada
	return f.err
}
func (f *fakeMensajeria) TransferirEmergenciaPorId(ctx context.Context, bearer string, emergenciaID msgclient.Id, body msgclient.TransferenciaEntrada) (*msgclient.Reclamo, error) {
	f.bearerRecibido = bearer
	f.transferenciaRecibida = &body
	return f.reclamo, f.err
}
func (f *fakeMensajeria) ResolverEmergenciaPorId(ctx context.Context, bearer string, emergenciaID msgclient.Id, body msgclient.ResolucionEmergenciaEntrada) (*msgclient.EmergenciaReaccion, error) {
	f.bearerRecibido = bearer
	f.resolucionRecibida = &body
	return f.reaccion, f.err
}
func (f *fakeMensajeria) CerrarEmergenciaPorId(ctx context.Context, bearer string, emergenciaID msgclient.Id) (*msgclient.CasoEmergencia, error) {
	f.bearerRecibido = bearer
	return f.caso, f.err
}

func (f *fakeMensajeria) ListarEmergencias(ctx context.Context, bearer string, viajeID msgclient.Id) (*msgclient.PaginaEmergencias, error) {
	f.bearerRecibido = bearer
	return f.pagina, f.err
}
func (f *fakeMensajeria) ActivarEmergencia(ctx context.Context, bearer string, viajeID msgclient.Id, body msgclient.ActivarEmergenciaEntrada) (*msgclient.CasoEmergencia, bool, error) {
	f.bearerRecibido = bearer
	return f.caso, f.idempotente, f.err
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

func (f *fakeMensajeria) ObtenerConversacion(ctx context.Context, bearer string, viajeID msgclient.Id) (*msgclient.Conversacion, error) {
	f.bearerRecibido = bearer
	return f.conversacion, f.err
}
func (f *fakeMensajeria) ListarMensajes(ctx context.Context, bearer string, viajeID msgclient.Id, params msgclient.ListarMensajesParams) (*msgclient.PaginaMensajes, error) {
	f.bearerRecibido = bearer
	f.paramsRecibidos = &params
	return f.paginaMensajes, f.err
}
func (f *fakeMensajeria) EnviarMensaje(ctx context.Context, bearer string, viajeID msgclient.Id, body msgclient.EnviarMensajeEntrada) (*msgclient.Mensaje, error) {
	f.bearerRecibido = bearer
	f.envioRecibido = &body
	return f.mensaje, f.err
}

// --- Bandeja y conversación interna del pool (E8-T15) ---
// ObtenerConversacion/ListarMensajes/EnviarMensaje se reutilizan de arriba.

func (f *fakeMensajeria) ListarTrabajoBandeja(ctx context.Context, bearer string, params msgclient.ListarTrabajoMiBandejaParams) (*msgclient.TrabajoBandeja, error) {
	f.bearerRecibido = bearer
	f.paramsBandeja = &params
	return f.trabajo, f.err
}
func (f *fakeMensajeria) ReclamarConversacion(ctx context.Context, bearer string, conversacionID msgclient.Id, body msgclient.VersionEsperadaEntrada) (*msgclient.Reclamo, error) {
	f.bearerRecibido = bearer
	f.reclamarRecibido = &body
	return f.reclamo, f.err
}
func (f *fakeMensajeria) LiberarReclamo(ctx context.Context, bearer string, conversacionID, reclamoID msgclient.Id, params msgclient.LiberarReclamoParams) error {
	f.bearerRecibido = bearer
	f.liberarRecibido = &params
	return f.err
}
func (f *fakeMensajeria) TransferirConversacion(ctx context.Context, bearer string, conversacionID msgclient.Id, body msgclient.TransferenciaEntrada) (*msgclient.Reclamo, error) {
	f.bearerRecibido = bearer
	f.transferenciaConvRec = &body
	return f.reclamo, f.err
}
func (f *fakeMensajeria) MarcarAtendida(ctx context.Context, bearer string, conversacionID msgclient.Id, body msgclient.VersionEsperadaEntrada) error {
	f.bearerRecibido = bearer
	f.atendidaRecibido = &body
	return f.err
}
func (f *fakeMensajeria) RegistrarReciboEntregado(ctx context.Context, bearer string, conversacionID msgclient.Id, body msgclient.ReciboEntrada) error {
	f.bearerRecibido = bearer
	f.reciboEntregadoRec = &body
	return f.err
}
func (f *fakeMensajeria) RegistrarReciboLeido(ctx context.Context, bearer string, conversacionID msgclient.Id, body msgclient.ReciboEntrada) error {
	f.bearerRecibido = bearer
	f.reciboLeidoRec = &body
	return f.err
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

// viajeEn arma un fakeOperacion cuyo ObtenerViaje devuelve el viaje en el estado dado.
func viajeEn(estado opclient.EstadoViaje) *fakeOperacion {
	return &fakeOperacion{detalle: &opclient.ViajeDetalle{Viaje: opclient.Viaje{Id: uuid.MustParse(viajeID), Estado: estado}}}
}

// TestActivarEmergenciaClienteDevuelveVistaCliente: activar devuelve 201 con la
// vista cliente del caso, y 200 en la reentrega idempotente.
func TestActivarEmergenciaClienteDevuelveVistaCliente(t *testing.T) {
	fake := &fakeMensajeria{caso: &msgclient.CasoEmergencia{
		Id: uuid.MustParse(emerID), Estado: msgclient.EstadoEmergenciaAbierta, ActivadaEn: time.Now().UTC(),
	}}
	h := NewHandlers(Deps{Mensajeria: fake, Operacion: viajeEn(opclient.ENTRANSITO)})

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

	fake.idempotente = true
	rec = peticionEmergencia(t, h.ActivarEmergenciaCliente, http.MethodPost,
		"/api/v1/viajes/"+viajeID+"/emergencias",
		bffclient.ActivarEmergenciaClienteEntrada{ActivacionId: uuid.New()}, "Bearer t", viajeID, "")
	if rec.Code != http.StatusOK {
		t.Fatalf("reentrega: status = %d, want 200", rec.Code)
	}
}

// TestActivarEmergenciaViajeTerminalEs409: en los cuatro estados terminales las
// dos activaciones (cliente y monitoreo) responden 409 viaje_en_estado_terminal
// sin llamar a mensajeria (DEC-135 #5). Un estado no terminal sí reenvía.
func TestActivarEmergenciaViajeTerminalEs409(t *testing.T) {
	for _, estado := range []opclient.EstadoViaje{opclient.CERRADO, opclient.CANCELADO, opclient.RECHAZADOPORAI27, opclient.RECHAZADOPORCLIENTE} {
		for nombre, activar := range map[string]func(*Handlers) http.HandlerFunc{
			"cliente": func(h *Handlers) http.HandlerFunc { return h.ActivarEmergenciaCliente },
			"interno": func(h *Handlers) http.HandlerFunc { return h.ActivarEmergenciaInterno },
		} {
			fake := &fakeMensajeria{caso: &msgclient.CasoEmergencia{Id: uuid.MustParse(emerID)}}
			h := NewHandlers(Deps{Mensajeria: fake, Operacion: viajeEn(estado)})
			rec := peticionEmergencia(t, activar(h), http.MethodPost, "/x",
				map[string]any{"activacion_id": uuid.New()}, "Bearer t", viajeID, "")
			if rec.Code != http.StatusConflict || !strings.Contains(rec.Body.String(), "viaje_en_estado_terminal") {
				t.Errorf("%s en %s: status %d cuerpo %s; quiere 409 viaje_en_estado_terminal", nombre, estado, rec.Code, rec.Body)
			}
			if fake.bearerRecibido != "" {
				t.Errorf("%s en %s: no debe llamar a mensajeria", nombre, estado)
			}
		}
	}
	fake := &fakeMensajeria{caso: &msgclient.CasoEmergencia{Id: uuid.MustParse(emerID), Estado: msgclient.EstadoEmergenciaAbierta}}
	// Un viaje no terminal pero fuera de EN_TRANSITO tampoco admite la activación.
	h := NewHandlers(Deps{Mensajeria: fake, Operacion: viajeEn(opclient.SOLICITADO)})
	rec := peticionEmergencia(t, h.ActivarEmergenciaInterno, http.MethodPost, "/x",
		bffclient.ActivarEmergenciaInternoEntrada{ActivacionId: uuid.New()}, "Bearer t", viajeID, "")
	if rec.Code != http.StatusConflict || !strings.Contains(rec.Body.String(), "viaje_no_admite_emergencia") || fake.bearerRecibido != "" {
		t.Fatalf("SOLICITADO: status %d cuerpo %s; quiere 409 viaje_no_admite_emergencia sin llamar a mensajeria", rec.Code, rec.Body)
	}

	h = NewHandlers(Deps{Mensajeria: fake, Operacion: viajeEn(opclient.ENTRANSITO)})
	rec = peticionEmergencia(t, h.ActivarEmergenciaInterno, http.MethodPost, "/x",
		bffclient.ActivarEmergenciaInternoEntrada{ActivacionId: uuid.New()}, "Bearer t", viajeID, "")
	var out bffclient.EmergenciaViaje
	decodificarCuerpo(t, rec, &out)
	if rec.Code != http.StatusCreated || out.Id != uuid.MustParse(emerID) || out.Estado != "abierta" {
		t.Fatalf("EN_TRANSITO: status %d fila %+v; quiere 201 con la fila mínima", rec.Code, out)
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

// TestListarEmergenciasViajeRecortaAFilaMinima: el panel reenvía a mensajeria
// con el Bearer y recorta cada caso a id, estado y activada_en (DEC-135 #1), sin
// origen, actor ni reclamo.
func TestListarEmergenciasViajeRecortaAFilaMinima(t *testing.T) {
	actor := uuid.New()
	fake := &fakeMensajeria{pagina: &msgclient.PaginaEmergencias{
		Items: []msgclient.CasoEmergencia{
			{Id: uuid.MustParse(emerID), Estado: msgclient.EstadoEmergenciaCerrada, Origen: msgclient.OrigenActivacionOperacion,
				ActorReal: &actor, ActivadaEn: time.Now().UTC()},
		},
	}}
	h := NewHandlers(Deps{Mensajeria: fake})

	rec := peticionPanel(t, h.ListarEmergenciasViaje, http.MethodGet,
		"/api/v1/panel/viajes/"+viajeID+"/emergencias", nil, "Bearer tok-int", viajeID, "", "")

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rec.Code)
	}
	if body := rec.Body.String(); strings.Contains(body, "origen") || strings.Contains(body, actor.String()) {
		t.Fatalf("la fila mínima no lleva origen ni actor: %s", body)
	}
	var out bffclient.PaginaEmergenciasViaje
	decodificarCuerpo(t, rec, &out)
	if len(out.Items) != 1 || out.Items[0].Id != uuid.MustParse(emerID) || out.Items[0].Estado != bffclient.EstadoEmergencia("cerrada") {
		t.Fatalf("fila mínima: %+v; quiere el caso %s cerrado (incluye cerrados)", out.Items, emerID)
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

// TestObtenerEmergenciaComponeViajeYEscolta: el detalle de Reacción junta el caso
// de mensajeria, el viaje de operacion, el nombre del cliente y la escolta con
// placas y teléfonos de catalogos (DEC-121 #2), sin viaje_id. Si falta un
// custodio en catalogos el detalle falla en vez de salir con la escolta
// incompleta.
func TestObtenerEmergenciaComponeViajeYEscolta(t *testing.T) {
	clienteID, vehID, cusID, uniID := uuid.New(), uuid.New(), uuid.New(), uuid.New()
	patrulla := opclient.Patrulla
	economico, placas, tel := "P-17", "ABC-123", "5550001111"
	reclamo := msgclient.Reclamo{Id: uuid.New(), ResponsableId: uuid.New(), Version: 2}
	men := &fakeMensajeria{caso: &msgclient.CasoEmergencia{
		Id: uuid.MustParse(emerID), ViajeId: uuid.MustParse(viajeID), Estado: msgclient.EstadoEmergenciaEnAtencion,
		Origen: msgclient.OrigenActivacionCliente, ReclamoActivo: &reclamo, ActivadaEn: time.Now().UTC(),
	}}
	op := viajeEn(opclient.ENTRANSITO)
	op.detalle.Viaje.ClienteId = &clienteID
	op.detalle.Viaje.Origen = "Manzanillo"
	op.detalle.Viaje.Destinos = []opclient.Destino{{Nombre: "CDMX"}}
	op.escolta = &opclient.EscoltaVigente{Unidades: []opclient.Unidad{{Id: uniID, Tipo: &patrulla, VehiculoId: vehID, CustodiosIds: []opclient.Id{cusID}}}}
	cat := &fakeCatalogos{
		clientes:  map[catclient.Id]*catclient.Cliente{clienteID: {Nombre: "Acme Logística"}},
		vehiculos: map[catclient.Id]*catclient.Vehiculo{vehID: {Economico: &economico, Placas: &placas}},
		custodios: map[catclient.Id]*catclient.Custodio{cusID: {Nombre: "Juan Pérez", Telefono: &tel}},
	}
	h := NewHandlers(Deps{Mensajeria: men, Operacion: op, Catalogos: cat})

	rec := peticionPanel(t, h.ObtenerEmergencia, http.MethodGet, "/api/v1/panel/emergencias/"+emerID, nil, "Bearer r", "", emerID, "")
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d cuerpo %s, want 200", rec.Code, rec.Body)
	}
	if strings.Contains(rec.Body.String(), "viaje_id") {
		t.Fatalf("el detalle no lleva viaje_id: %s", rec.Body)
	}
	var out bffclient.DetalleEmergenciaReaccion
	decodificarCuerpo(t, rec, &out)
	v := out.Viaje
	if out.Caso.Id != uuid.MustParse(emerID) || out.Caso.ReclamoActivo == nil || out.Caso.ReclamoActivo.Version != 2 {
		t.Fatalf("caso: %+v; quiere el caso con su reclamo v2", out.Caso)
	}
	if v.ClienteNombre != "Acme Logística" || v.Estado != "EN_TRANSITO" || v.Origen != "Manzanillo" || len(v.Destinos) != 1 {
		t.Fatalf("viaje: %+v", v)
	}
	if len(v.Escolta) != 1 || v.Escolta[0].Tipo != "patrulla" || *v.Escolta[0].Placas != placas ||
		len(v.Escolta[0].Custodios) != 1 || v.Escolta[0].Custodios[0].Nombre != "Juan Pérez" || *v.Escolta[0].Custodios[0].Telefono != tel {
		t.Fatalf("escolta: %+v", v.Escolta)
	}

	delete(cat.custodios, cusID)
	if rec := peticionPanel(t, h.ObtenerEmergencia, http.MethodGet, "/x", nil, "Bearer r", "", emerID, ""); rec.Code != http.StatusNotFound {
		t.Fatalf("custodio faltante: status %d, quiere el error de catalogos (404)", rec.Code)
	}
}
