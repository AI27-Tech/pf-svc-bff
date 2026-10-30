package agregacion

import (
	"bytes"
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

// peticionConversacion arma una petición al handler con claims de la audiencia
// dada. A diferencia de peticionEmergencia (que fija Cliente), aquí la audiencia
// es parámetro para poder probar el rechazo del actor interno.
func peticionConversacion(t *testing.T, handler http.HandlerFunc, metodo, url string, cuerpo any, aud auth.Audiencia, id string) *httptest.ResponseRecorder {
	t.Helper()
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
	req.Header.Set("Authorization", "Bearer tok-conv")
	if id != "" {
		req.SetPathValue("id", id)
	}
	req = req.WithContext(auth.ContextWithClaims(req.Context(), &auth.Claims{Subject: "u-cliente", Audiencia: aud}))
	rec := httptest.NewRecorder()
	handler(rec, req)
	return rec
}

// TestObtenerConversacionRecortaYPropagaToken: la vista de cliente lleva
// viaje_id, estado y atendida (derivada de atendida_en != null), descarta todo lo
// interno (cliente_id, bandeja, reclamo, secuencias) y propaga el Bearer.
func TestObtenerConversacionRecortaYPropagaToken(t *testing.T) {
	atendida := time.Date(2026, 10, 20, 9, 0, 0, 0, time.UTC)
	fake := &fakeMensajeria{conversacion: &msgclient.Conversacion{
		Id:              uuid.New(),
		ViajeId:         uuid.MustParse(viajeID),
		ClienteId:       uuid.New(),
		Estado:          msgclient.EstadoConversacionAbierta,
		AtendidaEn:      &atendida,
		UltimaSecuencia: 7,
		VersionReclamo:  2,
	}}
	h := NewHandlers(Deps{Mensajeria: fake})

	rec := peticionConversacion(t, h.ObtenerConversacionViaje, http.MethodGet,
		"/api/v1/viajes/"+viajeID+"/conversacion", nil, auth.AudienciaCliente, viajeID)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rec.Code)
	}
	var out bffclient.ConversacionCliente
	decodificarCuerpo(t, rec, &out)
	if out.ViajeId != uuid.MustParse(viajeID) {
		t.Errorf("viaje_id = %v", out.ViajeId)
	}
	if out.Estado != bffclient.ConversacionClienteEstadoAbierta {
		t.Errorf("estado = %q, want abierta", out.Estado)
	}
	if out.Atendida == nil || !*out.Atendida {
		t.Errorf("atendida debe ser true (atendida_en presente): %+v", out.Atendida)
	}
	if fake.bearerRecibido != "Bearer tok-conv" {
		t.Errorf("no se propagó el Bearer: %q", fake.bearerRecibido)
	}
}

// TestObtenerConversacionSinAtenderDaFalse: sin atendida_en, atendida=false.
func TestObtenerConversacionSinAtenderDaFalse(t *testing.T) {
	fake := &fakeMensajeria{conversacion: &msgclient.Conversacion{
		ViajeId: uuid.MustParse(viajeID),
		Estado:  msgclient.EstadoConversacionAbierta,
	}}
	h := NewHandlers(Deps{Mensajeria: fake})

	rec := peticionConversacion(t, h.ObtenerConversacionViaje, http.MethodGet,
		"/api/v1/viajes/"+viajeID+"/conversacion", nil, auth.AudienciaCliente, viajeID)

	var out bffclient.ConversacionCliente
	decodificarCuerpo(t, rec, &out)
	if out.Atendida == nil || *out.Atendida {
		t.Errorf("atendida debe ser false sin atendida_en: %+v", out.Atendida)
	}
}

// TestConversacionRechazaAudienciaInterna: un actor interno recibe 403 y el
// puerto NO se invoca (esta superficie es solo del portal cliente).
func TestConversacionRechazaAudienciaInterna(t *testing.T) {
	fake := &fakeMensajeria{conversacion: &msgclient.Conversacion{ViajeId: uuid.MustParse(viajeID)}}
	h := NewHandlers(Deps{Mensajeria: fake})

	rec := peticionConversacion(t, h.ObtenerConversacionViaje, http.MethodGet,
		"/api/v1/viajes/"+viajeID+"/conversacion", nil, auth.AudienciaInterno, viajeID)

	if rec.Code != http.StatusForbidden {
		t.Fatalf("status = %d, want 403", rec.Code)
	}
	if fake.bearerRecibido != "" {
		t.Errorf("el puerto no debe invocarse para audiencia interna; bearer = %q", fake.bearerRecibido)
	}
}

// TestListarMensajesPropagaSecuenciaYRecorta: los params de secuencia viajan tal
// cual a mensajeria; cada mensaje se recorta a la vista de cliente y de_cliente
// deriva de la audiencia del autor (nunca se expone actor_real ni rol).
func TestListarMensajesPropagaSecuenciaYRecorta(t *testing.T) {
	actor := uuid.New()
	rol := msgclient.Rol("monitorista")
	fake := &fakeMensajeria{paginaMensajes: &msgclient.PaginaMensajes{
		HayMas: true,
		Items: []msgclient.Mensaje{
			{Id: uuid.New(), Secuencia: 3, Cuerpo: "hola", CreadoEn: time.Date(2026, 10, 20, 8, 0, 0, 0, time.UTC), AudienciaAutor: msgclient.AudienciaCliente},
			{Id: uuid.New(), Secuencia: 4, Cuerpo: "en seguimiento", CreadoEn: time.Date(2026, 10, 20, 8, 5, 0, 0, time.UTC), AudienciaAutor: msgclient.AudienciaInterno, ActorReal: &actor, RolAlEnviar: &rol, AliasMostrado: "AI27 Operations"},
		},
	}}
	h := NewHandlers(Deps{Mensajeria: fake})

	rec := peticionConversacion(t, h.ListarMensajesViaje, http.MethodGet,
		"/api/v1/viajes/"+viajeID+"/conversacion/mensajes?antes_de=10&limite=25", nil, auth.AudienciaCliente, viajeID)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rec.Code)
	}
	// Params de secuencia propagados tal cual.
	if fake.paramsRecibidos == nil || fake.paramsRecibidos.AntesDe == nil || *fake.paramsRecibidos.AntesDe != 10 {
		t.Errorf("antes_de no se propagó: %+v", fake.paramsRecibidos)
	}
	if fake.paramsRecibidos.Limite == nil || *fake.paramsRecibidos.Limite != 25 {
		t.Errorf("limite no se propagó: %+v", fake.paramsRecibidos)
	}
	var out bffclient.PaginaMensajesCliente
	decodificarCuerpo(t, rec, &out)
	if !out.HayMas {
		t.Error("hay_mas debe conservarse")
	}
	if len(out.Items) != 2 {
		t.Fatalf("items = %d, want 2", len(out.Items))
	}
	if !out.Items[0].DeCliente {
		t.Error("el primer mensaje (autor Cliente) debe tener de_cliente=true")
	}
	if out.Items[1].DeCliente {
		t.Error("el segundo mensaje (autor Interno) debe tener de_cliente=false")
	}
	// La vista de cliente no tiene campos para actor_real ni rol; basta con que
	// el recorte compile y no los exponga (el schema MensajeCliente no los lleva).
}

// TestEnviarMensajeTraduceEntradaYDevuelveVistaCliente: el envío reenvía
// client_message_id y cuerpo (sin version_reclamo_esperada ni motivo_override) y
// responde 201 con la vista de cliente del mensaje creado.
func TestEnviarMensajeTraduceEntradaYDevuelveVistaCliente(t *testing.T) {
	cmid := uuid.New()
	fake := &fakeMensajeria{mensaje: &msgclient.Mensaje{
		Id: uuid.New(), Secuencia: 5, Cuerpo: "gracias", CreadoEn: time.Now().UTC(), AudienciaAutor: msgclient.AudienciaCliente,
	}}
	h := NewHandlers(Deps{Mensajeria: fake})

	entrada := bffclient.EnviarMensajeClienteEntrada{ClientMessageId: cmid, Cuerpo: "gracias"}
	rec := peticionConversacion(t, h.EnviarMensajeViaje, http.MethodPost,
		"/api/v1/viajes/"+viajeID+"/conversacion/mensajes", entrada, auth.AudienciaCliente, viajeID)

	if rec.Code != http.StatusCreated {
		t.Fatalf("status = %d, want 201", rec.Code)
	}
	if fake.envioRecibido == nil || fake.envioRecibido.ClientMessageId != cmid || fake.envioRecibido.Cuerpo != "gracias" {
		t.Errorf("entrada no se tradujo bien: %+v", fake.envioRecibido)
	}
	if fake.envioRecibido.VersionReclamoEsperada != nil || fake.envioRecibido.MotivoOverride != nil {
		t.Error("la superficie de cliente no debe mandar version_reclamo_esperada ni motivo_override")
	}
	var out bffclient.MensajeCliente
	decodificarCuerpo(t, rec, &out)
	if out.Secuencia != 5 || !out.DeCliente {
		t.Errorf("vista cliente incorrecta: %+v", out)
	}
}
