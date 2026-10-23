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
	pagina         *msgclient.PaginaEmergencias
	caso           *msgclient.CasoEmergencia
	err            error
	bearerRecibido string
	notaRecibida   *msgclient.NotaEmergenciaEntrada
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
