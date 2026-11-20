package agregacion

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
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

// fakeSolicitudesOp registra qué método de solicitudes llamó el handler y con
// qué; devuelve sol/pagina/hist o err.
type fakeSolicitudesOp struct {
	llamadas []string
	id       opclient.Id
	cuerpo   any
	params   opclient.ListarSolicitudesParams
	bearer   string
	sol      *opclient.Solicitud
	pagina   *opclient.PaginaSolicitudes
	hist     *opclient.HistoricoSolicitud
	err      error
}

func (f *fakeSolicitudesOp) anotar(ctx context.Context, op string, id opclient.Id, cuerpo any) {
	f.llamadas = append(f.llamadas, op)
	f.id, f.cuerpo = id, cuerpo
}
func (f *fakeOperacion) CrearSolicitud(ctx context.Context, b opclient.CrearSolicitudEntrada) (*opclient.Solicitud, error) {
	f.sols.anotar(ctx, "CrearSolicitud", opclient.Id{}, b)
	return f.sols.sol, f.sols.err
}
func (f *fakeOperacion) ListarSolicitudes(ctx context.Context, p opclient.ListarSolicitudesParams) (*opclient.PaginaSolicitudes, error) {
	f.sols.anotar(ctx, "ListarSolicitudes", opclient.Id{}, nil)
	f.sols.params = p
	return f.sols.pagina, f.sols.err
}
func (f *fakeOperacion) ObtenerSolicitud(ctx context.Context, id opclient.Id) (*opclient.Solicitud, error) {
	f.sols.anotar(ctx, "ObtenerSolicitud", id, nil)
	return f.sols.sol, f.sols.err
}
func (f *fakeOperacion) ListarHistoricoSolicitud(ctx context.Context, id opclient.Id) (*opclient.HistoricoSolicitud, error) {
	f.sols.anotar(ctx, "ListarHistoricoSolicitud", id, nil)
	return f.sols.hist, f.sols.err
}
func (f *fakeOperacion) CotizarSolicitud(ctx context.Context, id opclient.Id, b opclient.CotizacionEntrada) (*opclient.Solicitud, error) {
	f.sols.anotar(ctx, "CotizarSolicitud", id, b)
	return f.sols.sol, f.sols.err
}
func (f *fakeOperacion) ReprogramarSolicitud(ctx context.Context, id opclient.Id, b opclient.ReprogramacionEntrada) (*opclient.Solicitud, error) {
	f.sols.anotar(ctx, "ReprogramarSolicitud", id, b)
	return f.sols.sol, f.sols.err
}
func (f *fakeOperacion) RechazarCotizacion(ctx context.Context, id opclient.Id, b opclient.AccionClienteEntrada) (*opclient.Solicitud, error) {
	f.sols.anotar(ctx, "RechazarCotizacion", id, b)
	return f.sols.sol, f.sols.err
}
func (f *fakeOperacion) CancelarSolicitud(ctx context.Context, id opclient.Id, b opclient.AccionClienteEntrada) (*opclient.Solicitud, error) {
	f.sols.anotar(ctx, "CancelarSolicitud", id, b)
	return f.sols.sol, f.sols.err
}
func (f *fakeOperacion) ConfirmarPago(ctx context.Context, id opclient.Id, b opclient.PagoEntrada) (*opclient.Solicitud, error) {
	f.sols.anotar(ctx, "ConfirmarPago", id, b)
	return f.sols.sol, f.sols.err
}
func (f *fakeOperacion) RevertirPago(ctx context.Context, id opclient.Id, b opclient.MotivoEntrada) (*opclient.Solicitud, error) {
	f.sols.anotar(ctx, "RevertirPago", id, b)
	return f.sols.sol, f.sols.err
}
func (f *fakeOperacion) AprobarSolicitud(ctx context.Context, id opclient.Id) (*opclient.Solicitud, error) {
	f.sols.anotar(ctx, "AprobarSolicitud", id, nil)
	return f.sols.sol, f.sols.err
}
func (f *fakeOperacion) RechazarSolicitud(ctx context.Context, id opclient.Id, b opclient.MotivoEntrada) (*opclient.Solicitud, error) {
	f.sols.anotar(ctx, "RechazarSolicitud", id, b)
	return f.sols.sol, f.sols.err
}

type fakeSolicitudesMsg struct {
	llamadas []string
	id       msgclient.Id
	bearer   string
	conv     *msgclient.Conversacion
	mensajes *msgclient.PaginaMensajes
	mensaje  *msgclient.Mensaje
	trabajo  *msgclient.TrabajoBandeja
	err      error
}

func (f *fakeSolicitudesMsg) anotar(op, bearer string, id msgclient.Id) {
	f.llamadas, f.bearer, f.id = append(f.llamadas, op), bearer, id
}
func (f *fakeMensajeria) ObtenerConversacionSolicitud(_ context.Context, b string, id msgclient.Id) (*msgclient.Conversacion, error) {
	f.sols.anotar("ObtenerConversacionSolicitud", b, id)
	return f.sols.conv, f.sols.err
}
func (f *fakeMensajeria) ListarMensajesSolicitud(_ context.Context, b string, id msgclient.Id, _ msgclient.ListarMensajesParams) (*msgclient.PaginaMensajes, error) {
	f.sols.anotar("ListarMensajesSolicitud", b, id)
	return f.sols.mensajes, f.sols.err
}
func (f *fakeMensajeria) EnviarMensajeSolicitud(_ context.Context, b string, id msgclient.Id, _ msgclient.EnviarMensajeEntrada) (*msgclient.Mensaje, error) {
	f.sols.anotar("EnviarMensajeSolicitud", b, id)
	return f.sols.mensaje, f.sols.err
}
func (f *fakeMensajeria) ListarTrabajoBandejaSolicitudes(_ context.Context, b string, _ msgclient.ListarTrabajoMiBandejaSolicitudesParams) (*msgclient.TrabajoBandeja, error) {
	f.sols.anotar("ListarTrabajoBandejaSolicitudes", b, msgclient.Id{})
	return f.sols.trabajo, f.sols.err
}

var (
	solID     = uuid.MustParse("aaaaaaaa-aaaa-aaaa-aaaa-aaaaaaaaaaaa")
	clienteID = uuid.MustParse("cccccccc-cccc-cccc-cccc-cccccccccccc")
	cliente   = &auth.Claims{SesionID: "s", Subject: "u", Rol: "cliente", Audiencia: auth.AudienciaCliente}
)

func interno(rol roles.Rol) *auth.Claims {
	return &auth.Claims{SesionID: "s", Subject: "u", Rol: string(rol), Audiencia: auth.AudienciaInterno}
}

func solicitudDe(estado opclient.EstadoSolicitud) *opclient.Solicitud {
	return &opclient.Solicitud{Id: solID, Estado: estado, ClienteId: &clienteID, Origen: "CDMX", Destinos: []opclient.Destino{{Nombre: "MTY"}}, Avisos: []string{}}
}

// peticionSol arma la petición con {id}, Bearer y claims.
func peticionSol(h http.HandlerFunc, metodo, url string, cuerpo string, c *auth.Claims) *httptest.ResponseRecorder {
	req := httptest.NewRequest(metodo, url, strings.NewReader(cuerpo))
	req.Header.Set("Authorization", "Bearer tok-sol")
	req.SetPathValue("id", solID.String())
	if c != nil {
		req = req.WithContext(auth.ContextWithClaims(req.Context(), c))
	}
	rec := httptest.NewRecorder()
	h(rec, req)
	return rec
}

func hSol(op *fakeOperacion, msg *fakeMensajeria) *Handlers {
	return NewHandlers(Deps{
		Operacion:  op,
		Mensajeria: msg,
		Catalogos:  &fakeCatalogos{clientes: map[catclient.Id]*catclient.Cliente{clienteID: {Id: clienteID, Nombre: "ACME"}}},
	})
}

const (
	cotBody   = `{"monto":"100.00","moneda":"MXN","fecha_hora_servicio":"2026-12-01T10:00:00Z","notas":null}`
	pagoBody  = `{"fecha_pago":"2026-11-25","referencia":null}`
	motivo    = `{"motivo":"x"}`
	reprogram = `{"fecha_hora_servicio":"2026-12-02T10:00:00Z","motivo":"x"}`
)

// accionSol describe una operación de {id} con su cuerpo, quién puede y cuál es
// la llamada esperada a operacion.
type accionSol struct {
	nombre    string
	handler   func(*Handlers) http.HandlerFunc
	metodo    string
	cuerpo    string
	llamada   string
	permitido []*auth.Claims
	negado    []*auth.Claims
}

func accionesSol() []accionSol {
	todosInt := []*auth.Claims{interno(roles.RolEjecutivo), interno(roles.RolSupervisor), interno(roles.RolCoordinador), interno(roles.RolMonitorista), interno(roles.RolReaccion)}
	concat := func(a []*auth.Claims, b ...*auth.Claims) []*auth.Claims {
		return append(append([]*auth.Claims{}, a...), b...)
	}
	cot, adm, jefe := interno(roles.RolCotizacion), interno(roles.RolAdministrador), interno(roles.RolJefeOperativo)
	return []accionSol{
		{"cotizarSolicitud", func(h *Handlers) http.HandlerFunc { return h.CotizarSolicitud }, "PUT", cotBody, "CotizarSolicitud",
			[]*auth.Claims{cot, adm}, concat(todosInt, cliente, jefe)},
		{"reprogramarSolicitud", func(h *Handlers) http.HandlerFunc { return h.ReprogramarSolicitud }, "PUT", reprogram, "ReprogramarSolicitud",
			[]*auth.Claims{cot, adm}, concat(todosInt, cliente, jefe)},
		{"confirmarPago", func(h *Handlers) http.HandlerFunc { return h.ConfirmarPago }, "PUT", pagoBody, "ConfirmarPago",
			[]*auth.Claims{cot, adm}, concat(todosInt, cliente, jefe)},
		{"revertirPago", func(h *Handlers) http.HandlerFunc { return h.RevertirPago }, "POST", motivo, "RevertirPago",
			[]*auth.Claims{adm}, concat(todosInt, cliente, jefe, cot)},
		{"aprobarSolicitud", func(h *Handlers) http.HandlerFunc { return h.AprobarSolicitud }, "POST", ``, "AprobarSolicitud",
			[]*auth.Claims{jefe, adm}, concat(todosInt, cliente, cot)},
		{"rechazarSolicitud", func(h *Handlers) http.HandlerFunc { return h.RechazarSolicitud }, "POST", motivo, "RechazarSolicitud",
			[]*auth.Claims{jefe, adm}, concat(todosInt, cliente, cot)},
		{"rechazarCotizacion", func(h *Handlers) http.HandlerFunc { return h.RechazarCotizacion }, "POST", ``, "RechazarCotizacion",
			[]*auth.Claims{cliente}, concat(todosInt, jefe, cot, adm)},
		{"cancelarSolicitud", func(h *Handlers) http.HandlerFunc { return h.CancelarSolicitud }, "POST", motivo, "CancelarSolicitud",
			[]*auth.Claims{cliente}, concat(todosInt, jefe, cot, adm)},
		{"obtenerSolicitud", func(h *Handlers) http.HandlerFunc { return h.ObtenerSolicitud }, "GET", ``, "ObtenerSolicitud",
			[]*auth.Claims{cliente, cot, adm, jefe}, todosInt},
		{"listarHistoricoSolicitud", func(h *Handlers) http.HandlerFunc { return h.ListarHistoricoSolicitud }, "GET", ``, "ListarHistoricoSolicitud",
			[]*auth.Claims{cot, adm, jefe}, concat(todosInt, cliente)},
	}
}

// Camino feliz: cada operación reenvía a su método de operacion con el id y el
// cuerpo, y responde 200. El historial devuelve su propio tipo.
func TestSolicitudesCaminoFelizReenviaACadaOperacion(t *testing.T) {
	for _, a := range accionesSol() {
		for _, c := range a.permitido {
			op := &fakeOperacion{}
			op.sols.sol = solicitudDe(opclient.EstadoSolicitud("PAGADO"))
			op.sols.hist = &opclient.HistoricoSolicitud{Items: []opclient.HistoricoSolicitudItem{}}
			rec := peticionSol(a.handler(hSol(op, &fakeMensajeria{})), a.metodo, "/x", a.cuerpo, c)
			if rec.Code != http.StatusOK {
				t.Errorf("%s como %s: status %d, quiere 200 (%s)", a.nombre, c.Rol, rec.Code, rec.Body)
				continue
			}
			if len(op.sols.llamadas) != 1 || op.sols.llamadas[0] != a.llamada || op.sols.id != solID {
				t.Errorf("%s como %s: llamadas %v id %v", a.nombre, c.Rol, op.sols.llamadas, op.sols.id)
			}
		}
	}
}

// 403 por rol y audiencia, sin llegar a operacion.
func TestSolicitudesRolSinAccesoEs403SinTocarOperacion(t *testing.T) {
	for _, a := range accionesSol() {
		for _, c := range a.negado {
			op := &fakeOperacion{}
			rec := peticionSol(a.handler(hSol(op, &fakeMensajeria{})), a.metodo, "/x", a.cuerpo, c)
			if rec.Code != http.StatusForbidden {
				t.Errorf("%s como %s: status %d, quiere 403", a.nombre, c.Rol, rec.Code)
			}
			if len(op.sols.llamadas) != 0 {
				t.Errorf("%s como %s: no debe llamar a operacion", a.nombre, c.Rol)
			}
		}
		rec := peticionSol(a.handler(hSol(&fakeOperacion{}, &fakeMensajeria{})), a.metodo, "/x", a.cuerpo, nil)
		if rec.Code != http.StatusUnauthorized {
			t.Errorf("%s sin claims: status %d, quiere 401", a.nombre, rec.Code)
		}
	}
}

// El error de operacion llega con su status y su código de máquina.
func TestSolicitudesPropagaErrorYCodigoDeOperacion(t *testing.T) {
	casos := map[string]error{
		"transicion_no_permitida":       platformerrors.NewConflictoCodigo("transicion_no_permitida", "x"),
		"fecha_pago_fuera_de_vigencia":  platformerrors.NewConflictoCodigo("fecha_pago_fuera_de_vigencia", "x"),
		"fecha_hora_servicio_requerida": platformerrors.NewConflictoCodigo("fecha_hora_servicio_requerida", "x"),
	}
	for codigo, e := range casos {
		for _, a := range accionesSol() {
			op := &fakeOperacion{}
			op.sols.err = e
			rec := peticionSol(a.handler(hSol(op, &fakeMensajeria{})), a.metodo, "/x", a.cuerpo, a.permitido[0])
			if rec.Code != http.StatusConflict || !strings.Contains(rec.Body.String(), codigo) {
				t.Errorf("%s con %s: status %d cuerpo %s", a.nombre, codigo, rec.Code, rec.Body)
			}
		}
	}
	op := &fakeOperacion{}
	op.sols.err = platformerrors.NewNoEncontrado("x")
	rec := peticionSol(hSol(op, &fakeMensajeria{}).ObtenerSolicitud, "GET", "/x", "", interno(roles.RolJefeOperativo))
	if rec.Code != http.StatusNotFound {
		t.Errorf("404 de operacion (fuera de alcance): status %d", rec.Code)
	}
}

// El bff reenvía el cuerpo tal cual: no inventa transiciones ni valida el estado.
func TestSolicitudesReenviaCuerpoSinInventarTransiciones(t *testing.T) {
	op := &fakeOperacion{}
	op.sols.sol = solicitudDe("COTIZADO")
	peticionSol(hSol(op, &fakeMensajeria{}).CotizarSolicitud, "PUT", "/x", cotBody, interno(roles.RolCotizacion))
	got, ok := op.sols.cuerpo.(opclient.CotizacionEntrada)
	if !ok || got.Monto != "100.00" || got.Moneda != "MXN" {
		t.Fatalf("cuerpo reenviado = %+v", op.sols.cuerpo)
	}
	// Una solicitud en cualquier estado se devuelve sin filtrar ni cambiar.
	for _, e := range []opclient.EstadoSolicitud{"SOLICITADO", "VENCIDA", "RECHAZADO_POR_AI27", "APROBADO"} {
		op := &fakeOperacion{}
		op.sols.sol = solicitudDe(e)
		rec := peticionSol(hSol(op, &fakeMensajeria{}).ObtenerSolicitud, "GET", "/x", "", cliente)
		var out bffclient.Solicitud
		decodificarCuerpo(t, rec, &out)
		if string(out.Estado) != string(e) {
			t.Errorf("estado %s devuelto como %s", e, out.Estado)
		}
	}
}

// El cuerpo es opcional en rechazarCotizacion y cancelarSolicitud.
func TestAccionClienteAceptaCuerpoVacio(t *testing.T) {
	op := &fakeOperacion{}
	op.sols.sol = solicitudDe("CANCELADO_POR_CLIENTE")
	rec := peticionSol(hSol(op, &fakeMensajeria{}).CancelarSolicitud, "POST", "/x", "", cliente)
	if rec.Code != http.StatusOK {
		t.Fatalf("status %d: %s", rec.Code, rec.Body)
	}
}

// Solicitud.cliente solo para la audiencia Interno.
func TestSolicitudClienteSoloParaInterno(t *testing.T) {
	for _, c := range []*auth.Claims{cliente, interno(roles.RolCotizacion)} {
		op := &fakeOperacion{}
		op.sols.sol = solicitudDe("PAGADO")
		rec := peticionSol(hSol(op, &fakeMensajeria{}).ObtenerSolicitud, "GET", "/x", "", c)
		var out bffclient.Solicitud
		decodificarCuerpo(t, rec, &out)
		if c.Audiencia == auth.AudienciaCliente {
			if out.Cliente != nil {
				t.Errorf("el Cliente no debe recibir `cliente`: %+v", out.Cliente)
			}
			if strings.Contains(rec.Body.String(), `"cliente":`) {
				t.Errorf("el cuerpo del Cliente trae `cliente`: %s", rec.Body)
			}
		} else if out.Cliente == nil || out.Cliente.Nombre == nil || *out.Cliente.Nombre != "ACME" {
			t.Errorf("Interno debe recibir el nombre desde catalogos: %+v", out.Cliente)
		}
	}
}

func TestListarSolicitudesReenviaFiltrosYComponeClienteEnLote(t *testing.T) {
	op := &fakeOperacion{}
	op.sols.pagina = &opclient.PaginaSolicitudes{Items: []opclient.Solicitud{*solicitudDe("PAGADO"), *solicitudDe("COTIZADO")}}
	cat := &fakeCatalogos{clientes: map[catclient.Id]*catclient.Cliente{clienteID: {Id: clienteID, Nombre: "ACME"}}}
	h := NewHandlers(Deps{Operacion: op, Catalogos: cat})
	rec := peticionSol(h.ListarSolicitudes, "GET", "/api/v1/solicitudes?estado=PAGADO&cursor=c1", "", interno(roles.RolJefeOperativo))
	if rec.Code != http.StatusOK {
		t.Fatalf("status %d: %s", rec.Code, rec.Body)
	}
	if op.sols.params.Estado == nil || string(*op.sols.params.Estado) != "PAGADO" || op.sols.params.Cursor == nil || string(*op.sols.params.Cursor) != "c1" {
		t.Errorf("params reenviados: %+v", op.sols.params)
	}
	var out bffclient.PaginaSolicitudes
	decodificarCuerpo(t, rec, &out)
	if len(out.Items) != 2 || out.Items[1].Cliente == nil || len(cat.loteRecibido) != 1 || len(cat.loteRecibido[0]) != 1 {
		t.Errorf("sin el cliente compuesto en un solo lote: %+v lotes %v", out.Items, cat.loteRecibido)
	}
	// El Cliente no dispara lectura de catalogos ni recibe `cliente`.
	cat2 := &fakeCatalogos{}
	rec = peticionSol(NewHandlers(Deps{Operacion: op, Catalogos: cat2}).ListarSolicitudes, "GET", "/x", "", cliente)
	if rec.Code != http.StatusOK || len(cat2.loteRecibido) != 0 || strings.Contains(rec.Body.String(), `"cliente":`) {
		t.Errorf("Cliente: status %d lotes %v cuerpo %s", rec.Code, cat2.loteRecibido, rec.Body)
	}
	for _, c := range []*auth.Claims{interno(roles.RolEjecutivo), interno(roles.RolMonitorista)} {
		op2 := &fakeOperacion{}
		rec = peticionSol(hSol(op2, nil).ListarSolicitudes, "GET", "/x", "", c)
		if rec.Code != http.StatusForbidden || len(op2.sols.llamadas) != 0 {
			t.Errorf("%s: status %d", c.Rol, rec.Code)
		}
	}
}

func TestCrearSolicitudSoloClienteYReenvia(t *testing.T) {
	cuerpo := `{"origen":"CDMX","destinos":[{"nombre":"MTY"}],"fecha_hora_servicio":null,"tracto":null}`
	op := &fakeOperacion{}
	op.sols.sol = solicitudDe("SOLICITADO")
	rec := peticionSol(hSol(op, nil).CrearSolicitud, "POST", "/x", cuerpo, cliente)
	got, _ := op.sols.cuerpo.(opclient.CrearSolicitudEntrada)
	if rec.Code != http.StatusCreated || got.Origen != "CDMX" || len(got.Destinos) != 1 {
		t.Fatalf("status %d reenviado %+v", rec.Code, op.sols.cuerpo)
	}
	for _, c := range []*auth.Claims{interno(roles.RolCotizacion), interno(roles.RolAdministrador), interno(roles.RolJefeOperativo)} {
		op := &fakeOperacion{}
		rec := peticionSol(hSol(op, nil).CrearSolicitud, "POST", "/x", cuerpo, c)
		if rec.Code != http.StatusForbidden || len(op.sols.llamadas) != 0 {
			t.Errorf("%s: status %d, quiere 403", c.Rol, rec.Code)
		}
	}
	op = &fakeOperacion{}
	op.sols.err = platformerrors.NewValidacion("x")
	if rec := peticionSol(hSol(op, nil).CrearSolicitud, "POST", "/x", cuerpo, cliente); rec.Code != http.StatusBadRequest {
		t.Errorf("error de operacion: status %d", rec.Code)
	}
}

func TestHistoricoSolicitudEspejaItems(t *testing.T) {
	en := time.Now().UTC()
	a := opclient.EstadoSolicitud("COTIZADO")
	op := &fakeOperacion{}
	op.sols.hist = &opclient.HistoricoSolicitud{Items: []opclient.HistoricoSolicitudItem{{Accion: "cotizar", AEstado: &a, En: en}}}
	rec := peticionSol(hSol(op, nil).ListarHistoricoSolicitud, "GET", "/x", "", interno(roles.RolCotizacion))
	var out bffclient.HistoricoSolicitud
	decodificarCuerpo(t, rec, &out)
	if len(out.Items) != 1 || out.Items[0].Accion != "cotizar" || out.Items[0].AEstado == nil || string(*out.Items[0].AEstado) != "COTIZADO" {
		t.Errorf("historico = %+v", out)
	}
}

// --- crearViaje: solo Operación ---

func TestCrearViajeSoloOperacion(t *testing.T) {
	cuerpo := `{"origen":"CDMX","destinos":[{"nombre":"MTY"}]}`
	denegados := []*auth.Claims{cliente, interno(roles.RolCotizacion), interno(roles.RolEjecutivo)}
	for _, c := range denegados {
		op := &fakeOperacion{}
		rec := peticionSol(hSol(op, nil).CrearViaje, "POST", "/x", cuerpo, c)
		if rec.Code != http.StatusForbidden || op.crearRecibio.Origen != "" {
			t.Errorf("%s: status %d, quiere 403 sin llegar a operacion", c.Rol, rec.Code)
		}
	}
	for _, c := range []*auth.Claims{interno(roles.RolJefeOperativo), interno(roles.RolAdministrador)} {
		op := &fakeOperacion{viaje: &opclient.Viaje{Id: solID}}
		if rec := peticionSol(hSol(op, nil).CrearViaje, "POST", "/x", cuerpo, c); rec.Code != http.StatusCreated {
			t.Errorf("%s: status %d, quiere 201", c.Rol, rec.Code)
		}
	}
}

// --- Conversación de la solicitud ---

func TestConversacionSolicitudClienteRecortaYReenvia(t *testing.T) {
	ahora := time.Now().UTC()
	msg := &fakeMensajeria{}
	msg.sols.conv = &msgclient.Conversacion{Id: uuid.New(), ViajeId: solID, Estado: msgclient.EstadoConversacionAbierta, Tipo: ptr("solicitud"), SinRespuesta: ptr(true), AtendidaEn: &ahora}
	msg.sols.mensajes = &msgclient.PaginaMensajes{Items: []msgclient.Mensaje{{Id: uuid.New(), Cuerpo: "hola", ActorReal: ptr(uuid.New())}}}
	msg.sols.mensaje = &msgclient.Mensaje{Id: uuid.New(), Cuerpo: "hola", ActorReal: ptr(uuid.New())}
	h := hSol(&fakeOperacion{}, msg)

	rec := peticionSol(h.ObtenerConversacionSolicitud, "GET", "/x", "", cliente)
	if rec.Code != http.StatusOK || msg.sols.id != solID || msg.sols.bearer != "Bearer tok-sol" {
		t.Fatalf("obtener: status %d id %v bearer %q", rec.Code, msg.sols.id, msg.sols.bearer)
	}
	if strings.Contains(rec.Body.String(), "sin_respuesta") || strings.Contains(rec.Body.String(), "bandeja") {
		t.Errorf("el Cliente no ve datos internos: %s", rec.Body)
	}
	rec = peticionSol(h.ListarMensajesSolicitud, "GET", "/x?limite=5", "", cliente)
	if rec.Code != http.StatusOK || strings.Contains(rec.Body.String(), "actor_real") {
		t.Errorf("listar: status %d cuerpo %s", rec.Code, rec.Body)
	}
	rec = peticionSol(h.EnviarMensajeSolicitud, "POST", "/x", `{"client_message_id":"`+uuid.NewString()+`","cuerpo":"hola"}`, cliente)
	if rec.Code != http.StatusCreated || strings.Contains(rec.Body.String(), "actor_real") {
		t.Errorf("enviar: status %d cuerpo %s", rec.Code, rec.Body)
	}
	if got := strings.Join(msg.sols.llamadas, ","); got != "ObtenerConversacionSolicitud,ListarMensajesSolicitud,EnviarMensajeSolicitud" {
		t.Errorf("llamadas = %s", got)
	}
}

func TestConversacionSolicitudRolesInternosSon403(t *testing.T) {
	body := `{"client_message_id":"` + uuid.NewString() + `","cuerpo":"hola"}`
	for _, rol := range []roles.Rol{roles.RolEjecutivo, roles.RolSupervisor, roles.RolCoordinador, roles.RolJefeOperativo, roles.RolMonitorista, roles.RolReaccion, roles.RolCotizacion, roles.RolAdministrador} {
		msg := &fakeMensajeria{}
		h := hSol(&fakeOperacion{}, msg)
		for nombre, hd := range map[string]http.HandlerFunc{"obtener": h.ObtenerConversacionSolicitud, "listar": h.ListarMensajesSolicitud, "enviar": h.EnviarMensajeSolicitud} {
			if rec := peticionSol(hd, "POST", "/x", body, interno(rol)); rec.Code != http.StatusForbidden {
				t.Errorf("%s como %s: status %d, quiere 403", nombre, rol, rec.Code)
			}
		}
		if len(msg.sols.llamadas) != 0 {
			t.Errorf("%s: no debe llamar a mensajeria", rol)
		}
	}
}

func TestConversacionSolicitudPropagaErrores(t *testing.T) {
	msg := &fakeMensajeria{}
	msg.sols.err = platformerrors.NewConflictoCodigo("conversacion_solo_lectura", "x")
	h := hSol(&fakeOperacion{}, msg)
	rec := peticionSol(h.EnviarMensajeSolicitud, "POST", "/x", `{"client_message_id":"`+uuid.NewString()+`","cuerpo":"hola"}`, cliente)
	if rec.Code != http.StatusConflict || !strings.Contains(rec.Body.String(), "conversacion_solo_lectura") {
		t.Errorf("status %d cuerpo %s", rec.Code, rec.Body)
	}
	msg.sols.err = platformerrors.NewNoEncontrado("x")
	if rec := peticionSol(h.ObtenerConversacionSolicitud, "GET", "/x", "", cliente); rec.Code != http.StatusNotFound {
		t.Errorf("404: status %d", rec.Code)
	}
}

func TestBandejaSolicitudesPorRol(t *testing.T) {
	casos := []struct {
		c      *auth.Claims
		status int
	}{
		{interno(roles.RolCotizacion), 200}, {interno(roles.RolAdministrador), 200},
		{cliente, 403},
		{interno(roles.RolEjecutivo), 404}, {interno(roles.RolSupervisor), 404}, {interno(roles.RolCoordinador), 404},
		{interno(roles.RolJefeOperativo), 404}, {interno(roles.RolMonitorista), 404}, {interno(roles.RolReaccion), 404},
	}
	for _, c := range casos {
		msg := &fakeMensajeria{}
		msg.sols.trabajo = &msgclient.TrabajoBandeja{Items: []msgclient.Conversacion{{Id: uuid.New(), Tipo: ptr("solicitud"), SinRespuesta: ptr(true)}}}
		rec := peticionSol(hSol(&fakeOperacion{}, msg).ListarTrabajoBandejaSolicitudes, "GET", "/x?desde_revision=7", "", c.c)
		if rec.Code != c.status {
			t.Errorf("%s: status %d, quiere %d", c.c.Rol, rec.Code, c.status)
		}
		if c.status != 200 && len(msg.sols.llamadas) != 0 {
			t.Errorf("%s: no debe llamar a mensajeria", c.c.Rol)
		}
		if c.status == 200 && !strings.Contains(rec.Body.String(), `"sin_respuesta":true`) {
			t.Errorf("%s: falta sin_respuesta: %s", c.c.Rol, rec.Body)
		}
	}
	msg := &fakeMensajeria{}
	msg.sols.err = platformerrors.NewConflictoCodigo("stream_resync_required", "x")
	rec := peticionSol(hSol(&fakeOperacion{}, msg).ListarTrabajoBandejaSolicitudes, "GET", "/x", "", interno(roles.RolCotizacion))
	if rec.Code != http.StatusConflict || !strings.Contains(rec.Body.String(), "stream_resync_required") {
		t.Errorf("409: status %d %s", rec.Code, rec.Body)
	}
}
