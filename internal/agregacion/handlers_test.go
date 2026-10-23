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
	estados "github.com/AI27-Tech/pf-pkg-contracts/packages/api-client-go/estados"
	idclient "github.com/AI27-Tech/pf-pkg-contracts/packages/api-client-go/identidad"
	opclient "github.com/AI27-Tech/pf-pkg-contracts/packages/api-client-go/operacion"
	"github.com/AI27-Tech/pf-pkg-platform-go/auth"
	platformerrors "github.com/AI27-Tech/pf-pkg-platform-go/errors"
	"github.com/google/uuid"
)

// Estas pruebas llaman a los handlers como los llama su usuario real: por una
// peticion HTTP, y afirman el resultado que ese usuario observa (status y cuerpo
// JSON decodificado) contra un valor literal esperado, mas la entrada que
// recibio el puerto aguas abajo. Ninguna pasaria si los handlers devolvieran una
// forma vacia: cada aserción compara contra un valor concreto
// (principle-test-behavior-not-implementation).

// --- IniciarSesion ---

func TestIniciarSesionDevuelveSesionYReenviaCredenciales(t *testing.T) {
	fakeID := &fakeIdentidad{
		sesion: &idclient.SesionEmitida{
			SesionId:     uuid.MustParse("11111111-1111-1111-1111-111111111111"),
			AccessToken:  "jwt-abc",
			ExpiraEn:     time.Date(2026, 10, 5, 12, 0, 0, 0, time.UTC),
			RefreshToken: ptr("refresh-xyz"),
		},
	}
	h := NewHandlers(Deps{Identidad: fakeID})

	rec := hacerPeticion(t, h.IniciarSesion, http.MethodPost, "/api/v1/sesion",
		bffclient.CredencialesEntrada{Usuario: "ana", Contrasena: "secreta"}, nil)

	if rec.Code != http.StatusCreated {
		t.Fatalf("status = %d, se esperaba 201; cuerpo: %s", rec.Code, rec.Body.String())
	}
	var got bffclient.Sesion
	decodificarCuerpo(t, rec, &got)
	if got.AccessToken != "jwt-abc" {
		t.Errorf("access_token = %q, se esperaba %q", got.AccessToken, "jwt-abc")
	}
	if got.SesionId != fakeID.sesion.SesionId {
		t.Errorf("sesion_id = %v, se esperaba %v", got.SesionId, fakeID.sesion.SesionId)
	}
	if got.RefreshToken == nil || *got.RefreshToken != "refresh-xyz" {
		t.Errorf("refresh_token = %v, se esperaba refresh-xyz", got.RefreshToken)
	}
	// Reenvio exacto de credenciales a identidad (bff no las valida).
	if fakeID.emitirRecibio.Usuario != "ana" || fakeID.emitirRecibio.Contrasena != "secreta" {
		t.Errorf("identidad recibio %+v, se esperaba usuario=ana contrasena=secreta", fakeID.emitirRecibio)
	}
}

func TestIniciarSesionTraduceErrorAguasAbajo(t *testing.T) {
	// Un aguas-abajo degradado se traduce a 503, nunca a 404 (bff.md (d)).
	fakeID := &fakeIdentidad{err: platformerrors.NewDegradado("identidad caida")}
	h := NewHandlers(Deps{Identidad: fakeID})

	rec := hacerPeticion(t, h.IniciarSesion, http.MethodPost, "/api/v1/sesion",
		bffclient.CredencialesEntrada{Usuario: "ana", Contrasena: "x"}, nil)

	if rec.Code != http.StatusServiceUnavailable {
		t.Fatalf("status = %d, se esperaba 503 (degradado)", rec.Code)
	}
	var e bffclient.Error
	decodificarCuerpo(t, rec, &e)
	if e.Codigo != platformerrors.CategoryDegradado.String() {
		t.Errorf("codigo = %q, se esperaba %q", e.Codigo, platformerrors.CategoryDegradado.String())
	}
}

func TestIniciarSesionCuerpoInvalidoEs400(t *testing.T) {
	h := NewHandlers(Deps{Identidad: &fakeIdentidad{}})
	req := httptest.NewRequest(http.MethodPost, "/api/v1/sesion", bytes.NewBufferString("{no-json"))
	rec := httptest.NewRecorder()
	h.IniciarSesion(rec, req)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, se esperaba 400 por cuerpo invalido", rec.Code)
	}
}

// --- RefrescarSesion ---

func TestRefrescarSesionDevuelveSesionYReenviaToken(t *testing.T) {
	fakeID := &fakeIdentidad{
		sesion: &idclient.SesionEmitida{
			SesionId:    uuid.MustParse("22222222-2222-2222-2222-222222222222"),
			AccessToken: "jwt-nuevo",
			ExpiraEn:    time.Date(2026, 10, 5, 13, 0, 0, 0, time.UTC),
		},
	}
	h := NewHandlers(Deps{Identidad: fakeID})

	rec := hacerPeticion(t, h.RefrescarSesion, http.MethodPost, "/api/v1/sesion/refresh",
		bffclient.RefrescoEntrada{RefreshToken: "refresh-viejo"}, nil)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, se esperaba 200; cuerpo: %s", rec.Code, rec.Body.String())
	}
	var got bffclient.Sesion
	decodificarCuerpo(t, rec, &got)
	if got.AccessToken != "jwt-nuevo" {
		t.Errorf("access_token = %q, se esperaba jwt-nuevo", got.AccessToken)
	}
	if fakeID.refrescarRecibio.RefreshToken != "refresh-viejo" {
		t.Errorf("identidad recibio refresh %q, se esperaba refresh-viejo", fakeID.refrescarRecibio.RefreshToken)
	}
}

// --- CerrarSesion ---

func TestCerrarSesionRevocaLaSesionDelToken(t *testing.T) {
	// El sesion_id a revocar sale de las claims, nunca de un parametro (P10).
	sesionID := uuid.MustParse("33333333-3333-3333-3333-333333333333")
	fakeID := &fakeIdentidad{}
	h := NewHandlers(Deps{Identidad: fakeID})

	claims := &auth.Claims{SesionID: sesionID.String(), Subject: "u-1"}
	rec := hacerPeticion(t, h.CerrarSesion, http.MethodDelete, "/api/v1/sesion", nil, claims)

	if rec.Code != http.StatusNoContent {
		t.Fatalf("status = %d, se esperaba 204; cuerpo: %s", rec.Code, rec.Body.String())
	}
	if !fakeID.revocarLlamado {
		t.Fatal("no se llamo a RevocarSesion")
	}
	if fakeID.revocarRecibio != sesionID {
		t.Errorf("se revoco %v, se esperaba %v (el del token)", fakeID.revocarRecibio, sesionID)
	}
}

func TestCerrarSesionSinClaimsEs401(t *testing.T) {
	h := NewHandlers(Deps{Identidad: &fakeIdentidad{}})
	// Sin claims en el contexto: ruta protegida sin autenticar.
	rec := hacerPeticion(t, h.CerrarSesion, http.MethodDelete, "/api/v1/sesion", nil, nil)
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d, se esperaba 401 sin claims", rec.Code)
	}
}

// --- CrearViaje ---

func TestCrearViajeReenviaEntradaTraducidaADevuelveViaje(t *testing.T) {
	creado := &opclient.Viaje{
		Id:     uuid.MustParse("44444444-4444-4444-4444-444444444444"),
		Estado: opclient.EstadoViaje(estados.EstadoSolicitado),
		Origen: "CDMX",
	}
	fakeOp := &fakeOperacion{viaje: creado}
	h := NewHandlers(Deps{Operacion: fakeOp})

	entrada := bffclient.CrearViajeEntrada{
		Origen: "CDMX",
		Destinos: []struct {
			Nombre      string  `json:"nombre"`
			NumeroEnvio *string `json:"numero_envio,omitempty"`
		}{
			{Nombre: "Monterrey", NumeroEnvio: ptr("ENV-9")},
		},
	}
	rec := hacerPeticion(t, h.CrearViaje, http.MethodPost, "/api/v1/viajes", entrada,
		&auth.Claims{SesionID: "s", Subject: "u"})

	if rec.Code != http.StatusCreated {
		t.Fatalf("status = %d, se esperaba 201; cuerpo: %s", rec.Code, rec.Body.String())
	}
	var got opclient.Viaje
	decodificarCuerpo(t, rec, &got)
	if got.Id != creado.Id {
		t.Errorf("id = %v, se esperaba %v", got.Id, creado.Id)
	}
	// Traduccion bff -> operacion: origen y destinos mapeados, sin asignacion.
	if fakeOp.crearRecibio.Origen != "CDMX" {
		t.Errorf("operacion recibio origen %q, se esperaba CDMX", fakeOp.crearRecibio.Origen)
	}
	if len(fakeOp.crearRecibio.Destinos) != 1 || fakeOp.crearRecibio.Destinos[0].Nombre != "Monterrey" {
		t.Errorf("destinos mal traducidos: %+v", fakeOp.crearRecibio.Destinos)
	}
	if fakeOp.crearRecibio.Asignacion != nil {
		t.Error("bff no debe fijar asignacion (bff.md (f))")
	}
}

// --- ListarViajes ---

func TestListarViajesReenviaCursorYDevuelvePagina(t *testing.T) {
	pagina := &opclient.PaginaViajes{
		Items: []opclient.Viaje{{Id: uuid.New(), Origen: "A"}},
	}
	fakeOp := &fakeOperacion{pagina: pagina}
	h := NewHandlers(Deps{Operacion: fakeOp})

	rec := hacerPeticion(t, h.ListarViajes, http.MethodGet, "/api/v1/viajes?cursor=c-42", nil,
		&auth.Claims{SesionID: "s", Subject: "u"})

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, se esperaba 200; cuerpo: %s", rec.Code, rec.Body.String())
	}
	var got opclient.PaginaViajes
	decodificarCuerpo(t, rec, &got)
	if len(got.Items) != 1 {
		t.Errorf("items = %d, se esperaba 1", len(got.Items))
	}
	if fakeOp.listarRecibio.Cursor == nil || *fakeOp.listarRecibio.Cursor != "c-42" {
		t.Errorf("cursor reenviado = %v, se esperaba c-42", fakeOp.listarRecibio.Cursor)
	}
}

// --- SeguirViaje ---

func TestSeguirViajeComponeHitoYNoFiltraTokenTecnico(t *testing.T) {
	id := uuid.MustParse("55555555-5555-5555-5555-555555555555")
	fakeOp := &fakeOperacion{
		detalle: &opclient.ViajeDetalle{
			Viaje: opclient.Viaje{
				Id:     id,
				Estado: opclient.EstadoViaje(estados.EstadoEnTransito),
				Origen: "CDMX",
			},
			CondicionesActivas: []opclient.CondicionAbierta{},
		},
	}
	h := NewHandlers(Deps{Operacion: fakeOp})

	rec := hacerPeticionConPathID(t, h.SeguirViaje, id.String(),
		&auth.Claims{SesionID: "s", Subject: "u"})

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, se esperaba 200; cuerpo: %s", rec.Code, rec.Body.String())
	}
	var got bffclient.ViajeSeguimiento
	decodificarCuerpo(t, rec, &got)
	if got.Hito.Etiqueta == nil || *got.Hito.Etiqueta != "En camino" {
		t.Errorf("hito = %v, se esperaba 'En camino'", got.Hito.Etiqueta)
	}
	// El cliente NUNCA recibe el token tecnico como hito presentado (E2-T2).
	if got.Hito.Etiqueta != nil && *got.Hito.Etiqueta == string(estados.EstadoEnTransito) {
		t.Error("la etiqueta no debe ser el token tecnico")
	}
	if fakeOp.obtenerRecibio != id {
		t.Errorf("se pidio el viaje %v, se esperaba %v", fakeOp.obtenerRecibio, id)
	}
}

// tiposInternosDeIncidencia enumera TODOS los tipos internos del catalogo
// mapeado (§3 de la propuesta de clasificacion). La prueba de no filtrado los
// recorre uno por uno sobre el payload HTTP real del cliente.
var tiposInternosDeIncidencia = []string{
	"Asalto O Robo", "Intercepción de custodia", "Abandono de custodia",
	"Rompimiento de Convoy", "Sabotaje GPS", "Retiro de GPS",
	"Custodia da clave de amago incorrecta", "Custodia no da clave de amago",
	"Extraviado", "Accidente Del Transporte Por Volcadura", "Percance Automovilístico",
	"Falla Mecánica", "Falla mecánica de patrulla", "Falla en cámaras",
	"Bloqueo Carretero", "Desvió de Ruta", "Custodia no responde al teléfono",
	"Operador no responde al teléfono",
}

// TestSeguirViajeNoFiltraNingunTipoInternoEnElPayload es la prueba de no
// divulgacion a nivel HTTP (E2-T3, DEC-006): para cada tipo interno, compone la
// respuesta real del cliente y verifica que el tipo interno NO aparece en el
// cuerpo JSON crudo, que llega una incidencia curada y que su categoria no es el
// tipo interno. Enumera todos los tipos: si uno se filtrara, la prueba cae.
func TestSeguirViajeNoFiltraNingunTipoInternoEnElPayload(t *testing.T) {
	id := uuid.MustParse("66666666-6666-6666-6666-666666666666")
	for _, tipo := range tiposInternosDeIncidencia {
		t.Run(tipo, func(t *testing.T) {
			fakeOp := &fakeOperacion{
				detalle: &opclient.ViajeDetalle{
					Viaje: opclient.Viaje{
						Id:     id,
						Estado: opclient.EstadoViaje(estados.EstadoEnTransito),
						Origen: "CDMX",
					},
					CondicionesActivas: []opclient.CondicionAbierta{},
					IncidenciasActivas: &[]opclient.IncidenciaActiva{{
						Id:          uuid.New(),
						TipoInterno: tipo,
						Estado:      opclient.IncidenciaActivaEstadoEnAtencion,
						AbiertaEn:   time.Now().UTC(),
					}},
				},
			}
			h := NewHandlers(Deps{Operacion: fakeOp})
			rec := hacerPeticionConPathID(t, h.SeguirViaje, id.String(),
				&auth.Claims{SesionID: "s", Subject: "u"})
			if rec.Code != http.StatusOK {
				t.Fatalf("status = %d, se esperaba 200; cuerpo: %s", rec.Code, rec.Body.String())
			}
			// El cuerpo JSON crudo NUNCA debe contener el tipo interno (DEC-006).
			cuerpo := rec.Body.String()
			if strings.Contains(cuerpo, tipo) {
				t.Errorf("el tipo interno %q se filtro en el payload del cliente: %s", tipo, cuerpo)
			}
			var got bffclient.ViajeSeguimiento
			decodificarCuerpo(t, rec, &got)
			if len(got.Incidencias) != 1 {
				t.Fatalf("se esperaba 1 incidencia curada, dio %d", len(got.Incidencias))
			}
			if got.Incidencias[0].Categoria == tipo {
				t.Errorf("la categoria expuesta es el tipo interno %q", tipo)
			}
		})
	}
}

// TestSeguirViajeSabotajeGpsUnaSolaEntrada verifica el caso de doble dimension a
// nivel HTTP: "Sabotaje GPS" (incidencia + condicion de señal a la vez) se ve
// como UNA sola "Incidencia de seguridad", nunca dos entradas ni el nombre
// tecnico.
func TestSeguirViajeSabotajeGpsUnaSolaEntrada(t *testing.T) {
	id := uuid.MustParse("77777777-7777-7777-7777-777777777777")
	fakeOp := &fakeOperacion{
		detalle: &opclient.ViajeDetalle{
			Viaje: opclient.Viaje{
				Id:     id,
				Estado: opclient.EstadoViaje(estados.EstadoEnTransito),
				Origen: "CDMX",
			},
			// La misma realidad abre una condicion de señal ademas de la incidencia.
			CondicionesActivas: []opclient.CondicionAbierta{
				{Tipo: opclient.SINSENAL, AbiertaEn: time.Now().UTC()},
			},
			IncidenciasActivas: &[]opclient.IncidenciaActiva{{
				Id:          uuid.New(),
				TipoInterno: "Sabotaje GPS",
				Estado:      opclient.IncidenciaActivaEstadoEnAtencion,
				AbiertaEn:   time.Now().UTC(),
			}},
		},
	}
	h := NewHandlers(Deps{Operacion: fakeOp})
	rec := hacerPeticionConPathID(t, h.SeguirViaje, id.String(),
		&auth.Claims{SesionID: "s", Subject: "u"})
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, se esperaba 200; cuerpo: %s", rec.Code, rec.Body.String())
	}
	if strings.Contains(rec.Body.String(), "Sabotaje GPS") {
		t.Errorf("el nombre tecnico 'Sabotaje GPS' se filtro al cliente: %s", rec.Body.String())
	}
	var got bffclient.ViajeSeguimiento
	decodificarCuerpo(t, rec, &got)
	if len(got.Incidencias) != 1 {
		t.Fatalf("Sabotaje GPS debe dar UNA sola incidencia, dio %d", len(got.Incidencias))
	}
	if got.Incidencias[0].Categoria != "Incidencia de seguridad" {
		t.Errorf("categoria = %q, se esperaba 'Incidencia de seguridad'", got.Incidencias[0].Categoria)
	}
}

// TestSeguirViajeIncidenciaResueltaDejaDeVerse verifica el flujo de resolucion a
// nivel HTTP: tras resolver en operacion, la incidencia sale de
// incidencias_activas y la siguiente lectura del cliente ya no la trae.
func TestSeguirViajeIncidenciaResueltaDejaDeVerse(t *testing.T) {
	id := uuid.MustParse("88888888-8888-8888-8888-888888888888")
	base := func(incs *[]opclient.IncidenciaActiva) *fakeOperacion {
		return &fakeOperacion{
			detalle: &opclient.ViajeDetalle{
				Viaje: opclient.Viaje{
					Id:     id,
					Estado: opclient.EstadoViaje(estados.EstadoEnTransito),
					Origen: "CDMX",
				},
				CondicionesActivas: []opclient.CondicionAbierta{},
				IncidenciasActivas: incs,
			},
		}
	}
	// Antes: incidencia abierta -> el cliente la ve.
	antes := base(&[]opclient.IncidenciaActiva{{
		Id: uuid.New(), TipoInterno: "Asalto O Robo",
		Estado: opclient.IncidenciaActivaEstadoEnAtencion, AbiertaEn: time.Now().UTC(),
	}})
	hAntes := NewHandlers(Deps{Operacion: antes})
	recAntes := hacerPeticionConPathID(t, hAntes.SeguirViaje, id.String(),
		&auth.Claims{SesionID: "s", Subject: "u"})
	var gotAntes bffclient.ViajeSeguimiento
	decodificarCuerpo(t, recAntes, &gotAntes)
	if len(gotAntes.Incidencias) != 1 {
		t.Fatalf("antes de resolver, el cliente debe ver 1 incidencia, dio %d", len(gotAntes.Incidencias))
	}

	// Despues: operacion ya no la lista -> el cliente deja de verla, sin accion extra.
	despues := base(&[]opclient.IncidenciaActiva{})
	hDespues := NewHandlers(Deps{Operacion: despues})
	recDespues := hacerPeticionConPathID(t, hDespues.SeguirViaje, id.String(),
		&auth.Claims{SesionID: "s", Subject: "u"})
	var gotDespues bffclient.ViajeSeguimiento
	decodificarCuerpo(t, recDespues, &gotDespues)
	if len(gotDespues.Incidencias) != 0 {
		t.Errorf("tras resolver, el cliente no debe ver la incidencia; dio %d", len(gotDespues.Incidencias))
	}
}

func TestSeguirViajeIdInvalidoEs400(t *testing.T) {
	h := NewHandlers(Deps{Operacion: &fakeOperacion{}})
	rec := hacerPeticionConPathID(t, h.SeguirViaje, "no-es-uuid",
		&auth.Claims{SesionID: "s", Subject: "u"})
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, se esperaba 400 por id invalido", rec.Code)
	}
}

func TestSeguirViajeNoEncontradoSePropagaComo404(t *testing.T) {
	// operacion afirma ausencia real (404) -> se propaga como 404. Esto NO
	// contradice bff.md (d): la regla es que un FALLO de transporte no se vuelva
	// 404, no que un 404 genuino se oculte.
	fakeOp := &fakeOperacion{err: platformerrors.NewNoEncontrado("viaje no existe")}
	h := NewHandlers(Deps{Operacion: fakeOp})
	id := uuid.New()
	rec := hacerPeticionConPathID(t, h.SeguirViaje, id.String(),
		&auth.Claims{SesionID: "s", Subject: "u"})
	if rec.Code != http.StatusNotFound {
		t.Fatalf("status = %d, se esperaba 404", rec.Code)
	}
}

// --- helpers de prueba ---

func ptr[T any](v T) *T { return &v }

// hacerPeticion arma una peticion con cuerpo JSON (si cuerpo != nil) y claims en
// el contexto (si claims != nil), la ejecuta contra el handler y devuelve el
// recorder.
func hacerPeticion(t *testing.T, handler http.HandlerFunc, metodo, url string, cuerpo any, claims *auth.Claims) *httptest.ResponseRecorder {
	t.Helper()
	var body *bytes.Buffer
	if cuerpo != nil {
		raw, err := json.Marshal(cuerpo)
		if err != nil {
			t.Fatalf("marshal cuerpo: %v", err)
		}
		body = bytes.NewBuffer(raw)
	} else {
		body = bytes.NewBuffer(nil)
	}
	req := httptest.NewRequest(metodo, url, body)
	if claims != nil {
		req = req.WithContext(auth.ContextWithClaims(req.Context(), claims))
	}
	rec := httptest.NewRecorder()
	handler(rec, req)
	return rec
}

// hacerPeticionConPathID arma una peticion GET con el path value {id} seteado,
// como lo haria el mux de net/http al resolver GET /api/v1/viajes/{id}.
func hacerPeticionConPathID(t *testing.T, handler http.HandlerFunc, id string, claims *auth.Claims) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/viajes/"+id, nil)
	req.SetPathValue("id", id)
	if claims != nil {
		req = req.WithContext(auth.ContextWithClaims(req.Context(), claims))
	}
	rec := httptest.NewRecorder()
	handler(rec, req)
	return rec
}

func decodificarCuerpo(t *testing.T, rec *httptest.ResponseRecorder, v any) {
	t.Helper()
	if err := json.Unmarshal(rec.Body.Bytes(), v); err != nil {
		t.Fatalf("decodificar cuerpo %q: %v", rec.Body.String(), err)
	}
}

// --- puertos falsos ---

// fakeIdentidad registra las entradas que recibe y devuelve una respuesta o un
// error fijos. Implementa IdentidadPort.
type fakeIdentidad struct {
	sesion *idclient.SesionEmitida
	err    error

	emitirRecibio    idclient.CredencialesEntrada
	refrescarRecibio idclient.RefrescoEntrada
	revocarLlamado   bool
	revocarRecibio   idclient.Id
}

func (f *fakeIdentidad) EmitirSesion(_ context.Context, body idclient.CredencialesEntrada) (*idclient.SesionEmitida, error) {
	f.emitirRecibio = body
	if f.err != nil {
		return nil, f.err
	}
	return f.sesion, nil
}

func (f *fakeIdentidad) RefrescarSesion(_ context.Context, body idclient.RefrescoEntrada) (*idclient.SesionEmitida, error) {
	f.refrescarRecibio = body
	if f.err != nil {
		return nil, f.err
	}
	return f.sesion, nil
}

func (f *fakeIdentidad) RevocarSesion(_ context.Context, id idclient.Id) error {
	f.revocarLlamado = true
	f.revocarRecibio = id
	return f.err
}

func (f *fakeIdentidad) ListarSesionesRevocadas(_ context.Context) (*idclient.PaginaRevocaciones, error) {
	return &idclient.PaginaRevocaciones{}, f.err
}

func (f *fakeIdentidad) ObtenerSesion(_ context.Context, _ idclient.Id) (*idclient.EstadoSesion, error) {
	return &idclient.EstadoSesion{}, f.err
}

// fakeOperacion registra las entradas que recibe y devuelve respuestas o un
// error fijos. Implementa OperacionPort.
type fakeOperacion struct {
	viaje   *opclient.Viaje
	pagina  *opclient.PaginaViajes
	detalle *opclient.ViajeDetalle
	err     error

	crearRecibio   opclient.CrearViajeEntrada
	listarRecibio  opclient.ListarViajesParams
	obtenerRecibio opclient.Id
}

func (f *fakeOperacion) CrearViaje(_ context.Context, body opclient.CrearViajeEntrada) (*opclient.Viaje, error) {
	f.crearRecibio = body
	if f.err != nil {
		return nil, f.err
	}
	return f.viaje, nil
}

func (f *fakeOperacion) ListarViajes(_ context.Context, params opclient.ListarViajesParams) (*opclient.PaginaViajes, error) {
	f.listarRecibio = params
	if f.err != nil {
		return nil, f.err
	}
	return f.pagina, nil
}

func (f *fakeOperacion) ObtenerViaje(_ context.Context, id opclient.Id) (*opclient.ViajeDetalle, error) {
	f.obtenerRecibio = id
	if f.err != nil {
		return nil, f.err
	}
	return f.detalle, nil
}
