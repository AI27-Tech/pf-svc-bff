package agregacion

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	bffclient "github.com/AI27-Tech/pf-pkg-contracts/packages/api-client-go/bff"
	msgclient "github.com/AI27-Tech/pf-pkg-contracts/packages/api-client-go/mensajeria"
	"github.com/AI27-Tech/pf-pkg-platform-go/auth"
	"github.com/google/uuid"
)

const convID = "dddddddd-dddd-dddd-dddd-dddddddddddd"

// peticionSinClaims arma una petición SIN claims en el contexto (el middleware no
// corrió), para probar el 401 del handler interno. Los helpers peticionPanel y
// peticionConversacion siempre inyectan claims, por eso este caso va aparte.
func peticionSinClaims(t *testing.T, handler http.HandlerFunc, metodo, url string) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(metodo, url, nil)
	rec := httptest.NewRecorder()
	handler(rec, req)
	return rec
}

// TestListarTrabajoBandejaExitoDevuelveVistaCompleta: la bandeja (cola única, sin
// {id}) responde 200 con el trabajo completo. Cada conversación conserva lo que
// la vista de cliente oculta (reclamo_activo, version_reclamo), y el Bearer se
// propaga.
func TestListarTrabajoBandejaExitoDevuelveVistaCompleta(t *testing.T) {
	responsable := uuid.New()
	fake := &fakeMensajeria{trabajo: &msgclient.TrabajoBandeja{
		Revision: 42,
		Items: []msgclient.Conversacion{{
			Id:             uuid.MustParse(convID),
			ViajeId:        uuid.MustParse(viajeID),
			ClienteId:      uuid.New(),
			Estado:         msgclient.EstadoConversacionAbierta,
			VersionReclamo: 3,
			ReclamoActivo:  &msgclient.Reclamo{Id: uuid.MustParse(reclamoID), ResponsableId: responsable, Version: 3},
		}},
	}}
	h := NewHandlers(Deps{Mensajeria: fake})

	rec := peticionPanel(t, h.ListarTrabajoBandeja, http.MethodGet,
		"/api/v1/bandeja/trabajo", nil, "Bearer tok-int", "", "", "")

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rec.Code)
	}
	var out bffclient.TrabajoBandeja
	decodificarCuerpo(t, rec, &out)
	if out.Revision != 42 {
		t.Errorf("revision = %d, want 42", out.Revision)
	}
	if len(out.Items) != 1 {
		t.Fatalf("items = %d, want 1", len(out.Items))
	}
	c := out.Items[0]
	if c.VersionReclamo != 3 {
		t.Errorf("version_reclamo = %d, want 3 (la vista interna NO lo oculta)", c.VersionReclamo)
	}
	if c.ReclamoActivo == nil || c.ReclamoActivo.ResponsableId != responsable {
		t.Errorf("reclamo_activo debe conservarse en la vista interna: %+v", c.ReclamoActivo)
	}
	if fake.bearerRecibido != "Bearer tok-int" {
		t.Errorf("no se propagó el Bearer: %q", fake.bearerRecibido)
	}
}

// TestListarTrabajoBandejaRechazaAudienciaCliente: un actor Cliente recibe 403 y
// el puerto NO se invoca (esta superficie es solo interna).
func TestListarTrabajoBandejaRechazaAudienciaCliente(t *testing.T) {
	fake := &fakeMensajeria{trabajo: &msgclient.TrabajoBandeja{Revision: 1}}
	h := NewHandlers(Deps{Mensajeria: fake})

	rec := peticionConversacion(t, h.ListarTrabajoBandeja, http.MethodGet,
		"/api/v1/bandeja/trabajo", nil, auth.AudienciaCliente, "")

	if rec.Code != http.StatusForbidden {
		t.Fatalf("status = %d, want 403", rec.Code)
	}
	if fake.bearerRecibido != "" {
		t.Errorf("el puerto no debe invocarse para audiencia cliente; bearer = %q", fake.bearerRecibido)
	}
}

// TestListarTrabajoBandejaSinClaimsEs401: sin claims (middleware no corrió) el
// handler responde 401 y no llama al puerto.
func TestListarTrabajoBandejaSinClaimsEs401(t *testing.T) {
	fake := &fakeMensajeria{trabajo: &msgclient.TrabajoBandeja{Revision: 1}}
	h := NewHandlers(Deps{Mensajeria: fake})

	// Petición sin claims en el contexto: el helper peticionConversacion siempre
	// inyecta claims, así que aquí se arma a mano una petición pelada.
	rec := peticionSinClaims(t, h.ListarTrabajoBandeja, http.MethodGet, "/api/v1/bandeja/trabajo")

	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d, want 401", rec.Code)
	}
	if fake.bearerRecibido != "" {
		t.Errorf("el puerto no debe invocarse sin claims; bearer = %q", fake.bearerRecibido)
	}
}

// TestReclamarConversacionReenviaVersionYDevuelveReclamo: reclamar propaga la
// version_esperada del cuerpo y devuelve 201 con el reclamo completo.
func TestReclamarConversacionReenviaVersionYDevuelveReclamo(t *testing.T) {
	fake := &fakeMensajeria{reclamo: &msgclient.Reclamo{
		Id: uuid.MustParse(reclamoID), ConversacionId: uuid.MustParse(convID), Version: 1,
	}}
	h := NewHandlers(Deps{Mensajeria: fake})

	rec := peticionConversacion(t, h.ReclamarConversacion, http.MethodPost,
		"/api/v1/conversaciones/"+convID+"/reclamos",
		bffclient.VersionEsperadaEntrada{VersionEsperada: 0}, auth.AudienciaInterno, convID)

	if rec.Code != http.StatusCreated {
		t.Fatalf("status = %d, want 201", rec.Code)
	}
	if fake.reclamarRecibido == nil || fake.reclamarRecibido.VersionEsperada != 0 {
		t.Fatalf("no se reenvió version_esperada: %+v", fake.reclamarRecibido)
	}
	var out bffclient.Reclamo
	decodificarCuerpo(t, rec, &out)
	if out.Id != uuid.MustParse(reclamoID) || out.Version != 1 {
		t.Errorf("reclamo devuelto incorrecto: %+v", out)
	}
}

// TestReclamarConversacionRechazaAudienciaCliente: un actor Cliente recibe 403 y
// el puerto NO se invoca.
func TestReclamarConversacionRechazaAudienciaCliente(t *testing.T) {
	fake := &fakeMensajeria{reclamo: &msgclient.Reclamo{Id: uuid.MustParse(reclamoID)}}
	h := NewHandlers(Deps{Mensajeria: fake})

	rec := peticionConversacion(t, h.ReclamarConversacion, http.MethodPost,
		"/api/v1/conversaciones/"+convID+"/reclamos",
		bffclient.VersionEsperadaEntrada{VersionEsperada: 0}, auth.AudienciaCliente, convID)

	if rec.Code != http.StatusForbidden {
		t.Fatalf("status = %d, want 403", rec.Code)
	}
	if fake.reclamarRecibido != nil {
		t.Errorf("el puerto no debe invocarse para audiencia cliente: %+v", fake.reclamarRecibido)
	}
}

// TestEnviarMensajeInternoPropagaVersionReclamoYDevuelveVistaCompleta: a
// diferencia del envío de cliente, el interno SÍ propaga version_reclamo_esperada
// y la respuesta conserva actor_real y rol_al_enviar (la vista de cliente los
// ocultaría). Responde 201.
func TestEnviarMensajeInternoPropagaVersionReclamoYDevuelveVistaCompleta(t *testing.T) {
	cmid := uuid.New()
	actor := uuid.New()
	rol := msgclient.Rol("ejecutivo")
	fake := &fakeMensajeria{mensaje: &msgclient.Mensaje{
		Id:             uuid.New(),
		ConversacionId: uuid.MustParse(convID),
		Secuencia:      8,
		Cuerpo:         "en seguimiento",
		CreadoEn:       time.Now().UTC(),
		AudienciaAutor: msgclient.AudienciaInterno,
		ActorReal:      &actor,
		RolAlEnviar:    &rol,
		AliasMostrado:  "AI27 Operations",
	}}
	h := NewHandlers(Deps{Mensajeria: fake})

	version := msgclient.VersionReclamo(5)
	entrada := bffclient.EnviarMensajeInternaEntrada{
		ClientMessageId:        cmid,
		Cuerpo:                 "en seguimiento",
		VersionReclamoEsperada: &version,
	}
	rec := peticionConversacion(t, h.EnviarMensaje, http.MethodPost,
		"/api/v1/conversaciones/"+convID+"/mensajes", entrada, auth.AudienciaInterno, convID)

	if rec.Code != http.StatusCreated {
		t.Fatalf("status = %d, want 201", rec.Code)
	}
	if fake.envioRecibido == nil {
		t.Fatal("no se invocó EnviarMensaje")
	}
	if fake.envioRecibido.ClientMessageId != cmid || fake.envioRecibido.Cuerpo != "en seguimiento" {
		t.Errorf("entrada no se tradujo bien: %+v", fake.envioRecibido)
	}
	if fake.envioRecibido.VersionReclamoEsperada == nil || *fake.envioRecibido.VersionReclamoEsperada != 5 {
		t.Errorf("la superficie interna DEBE propagar version_reclamo_esperada: %+v", fake.envioRecibido.VersionReclamoEsperada)
	}
	if fake.envioRecibido.MotivoOverride != nil {
		t.Error("la entrada interna del bff no define motivo_override; no debe fijarse")
	}
	var out bffclient.Mensaje
	decodificarCuerpo(t, rec, &out)
	if out.ActorReal == nil || *out.ActorReal != actor {
		t.Errorf("la vista interna debe conservar actor_real: %+v", out.ActorReal)
	}
	if out.RolAlEnviar == nil || *out.RolAlEnviar != bffclient.Rol("ejecutivo") {
		t.Errorf("la vista interna debe conservar rol_al_enviar: %+v", out.RolAlEnviar)
	}
}

// TestObtenerConversacionInternaNoRecorta: la superficie interna reutiliza
// ObtenerConversacion del puerto pero devuelve el Conversacion COMPLETO (DEC-006
// invertido): conserva bandeja_id, reclamo_activo y version_reclamo que la vista
// de cliente oculta.
func TestObtenerConversacionInternaNoRecorta(t *testing.T) {
	bandeja := uuid.New()
	fake := &fakeMensajeria{conversacion: &msgclient.Conversacion{
		Id:             uuid.MustParse(convID),
		ViajeId:        uuid.MustParse(viajeID),
		ClienteId:      uuid.New(),
		Estado:         msgclient.EstadoConversacionSoloLectura,
		BandejaId:      &bandeja,
		VersionReclamo: 4,
		ReclamoActivo:  &msgclient.Reclamo{Id: uuid.MustParse(reclamoID), Version: 4},
	}}
	h := NewHandlers(Deps{Mensajeria: fake})

	rec := peticionConversacion(t, h.ObtenerConversacion, http.MethodGet,
		"/api/v1/conversaciones/"+convID, nil, auth.AudienciaInterno, convID)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rec.Code)
	}
	var out bffclient.Conversacion
	decodificarCuerpo(t, rec, &out)
	if out.Estado != bffclient.EstadoConversacion("solo_lectura") {
		t.Errorf("estado canónico = %q, want solo_lectura (sin recortar)", out.Estado)
	}
	if out.BandejaId == nil || *out.BandejaId != bandeja {
		t.Errorf("bandeja_id debe conservarse en la vista interna: %+v", out.BandejaId)
	}
	if out.VersionReclamo != 4 {
		t.Errorf("version_reclamo = %d, want 4 (la vista interna NO lo oculta)", out.VersionReclamo)
	}
	if out.ReclamoActivo == nil || out.ReclamoActivo.Version != 4 {
		t.Errorf("reclamo_activo debe conservarse: %+v", out.ReclamoActivo)
	}
	if fake.bearerRecibido != "Bearer tok-conv" {
		t.Errorf("no se propagó el Bearer: %q", fake.bearerRecibido)
	}
}

// TestLiberarReclamoExigeVersionEnQuery: liberar toma version_esperada del query
// (obligatoria) y responde 204 sin cuerpo; ausente es 400 y no llega al puerto.
func TestLiberarReclamoExigeVersionEnQuery(t *testing.T) {
	fake := &fakeMensajeria{}
	h := NewHandlers(Deps{Mensajeria: fake})

	rec := peticionPanel(t, h.LiberarReclamo, http.MethodDelete,
		"/api/v1/conversaciones/"+convID+"/reclamos/"+reclamoID+"?version_esperada=7",
		nil, "Bearer t", convID, "", reclamoID)

	if rec.Code != http.StatusNoContent {
		t.Fatalf("status = %d, want 204", rec.Code)
	}
	if fake.liberarRecibido == nil || fake.liberarRecibido.VersionEsperada != 7 {
		t.Fatalf("version_esperada del query no llegó: %+v", fake.liberarRecibido)
	}

	// Ausente: 400 y sin invocar el puerto.
	fake2 := &fakeMensajeria{}
	h2 := NewHandlers(Deps{Mensajeria: fake2})
	rec2 := peticionPanel(t, h2.LiberarReclamo, http.MethodDelete,
		"/api/v1/conversaciones/"+convID+"/reclamos/"+reclamoID, nil, "Bearer t", convID, "", reclamoID)
	if rec2.Code != http.StatusBadRequest {
		t.Fatalf("version ausente: status = %d, want 400", rec2.Code)
	}
	if fake2.liberarRecibido != nil {
		t.Error("no debió llamarse al puerto sin version_esperada")
	}
}
