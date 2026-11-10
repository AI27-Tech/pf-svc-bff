package agregacion

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"slices"
	"strconv"
	"strings"
	"testing"

	bffclient "github.com/AI27-Tech/pf-pkg-contracts/packages/api-client-go/bff"
	catclient "github.com/AI27-Tech/pf-pkg-contracts/packages/api-client-go/catalogos"
	idclient "github.com/AI27-Tech/pf-pkg-contracts/packages/api-client-go/identidad"
	opclient "github.com/AI27-Tech/pf-pkg-contracts/packages/api-client-go/operacion"
	"github.com/AI27-Tech/pf-pkg-contracts/packages/api-client-go/roles"
	"github.com/AI27-Tech/pf-pkg-platform-go/auth"
	platformerrors "github.com/AI27-Tech/pf-pkg-platform-go/errors"
	"github.com/google/uuid"
)

// --- dobles de la administración (tracer.40) ---

// gobiernoFake es el gobierno de identidad en el doble fakeIdentidad. Guarda
// tenants con estado (crear, desactivar, paginar) para probar el orden y la
// idempotencia de las organizaciones.
type gobiernoFake struct {
	registro     *[]string // orden compartido con orgFake
	llamadas     []string
	claimsVistos *auth.Claims
	errGob       error // error de las operaciones de sujetos y alcance

	detalle       *idclient.SujetoDetalle
	pagina        *idclient.PaginaSujetos
	listarRecibio idclient.ListarSujetosParams
	altaRecibio   *idclient.AltaSujetoEntrada
	recibio       any

	tenants             []idclient.Tenant
	porPagina           int
	errCrearTenant      error
	errDesactivarTenant error
}

func (g *gobiernoFake) anotar(ctx context.Context, metodo string) {
	g.llamadas = append(g.llamadas, metodo)
	if g.registro != nil {
		*g.registro = append(*g.registro, metodo)
	}
	g.claimsVistos = auth.ClaimsFromContext(ctx)
}

func (g *gobiernoFake) ListarSujetos(ctx context.Context, params idclient.ListarSujetosParams) (*idclient.PaginaSujetos, error) {
	g.anotar(ctx, "ListarSujetos")
	g.listarRecibio = params
	if g.pagina == nil {
		return &idclient.PaginaSujetos{Items: []idclient.SujetoDetalle{}}, g.errGob
	}
	return g.pagina, g.errGob
}

func (g *gobiernoFake) AltaSujeto(ctx context.Context, body idclient.AltaSujetoEntrada) (*idclient.Sujeto, error) {
	g.anotar(ctx, "AltaSujeto")
	g.altaRecibio = &body
	if g.errGob != nil {
		return nil, g.errGob
	}
	return &idclient.Sujeto{Id: uuid.New(), TenantId: body.TenantId, Activo: true, Nombre: &body.Nombre, IdentificadorUsuario: &body.IdentificadorUsuario, IdpSubject: ptr("kc-nuevo")}, nil
}

func (g *gobiernoFake) ObtenerSujeto(ctx context.Context, id idclient.Id) (*idclient.SujetoDetalle, error) {
	g.anotar(ctx, "ObtenerSujeto")
	if g.errGob != nil {
		return nil, g.errGob
	}
	if g.detalle == nil {
		return &idclient.SujetoDetalle{Sujeto: idclient.Sujeto{Id: id, Activo: true}}, nil
	}
	return g.detalle, nil
}

func (g *gobiernoFake) EditarSujeto(ctx context.Context, id idclient.Id, body idclient.EdicionSujetoEntrada) (*idclient.SujetoDetalle, error) {
	g.recibio = body
	return g.ObtenerSujeto(ctx, id)
}

func (g *gobiernoFake) DesactivarSujeto(ctx context.Context, _ idclient.Id) error {
	g.anotar(ctx, "DesactivarSujeto")
	return g.errGob
}

func (g *gobiernoFake) ReactivarSujeto(ctx context.Context, _ idclient.Id) error {
	g.anotar(ctx, "ReactivarSujeto")
	return g.errGob
}

func (g *gobiernoFake) FijarRolSujeto(ctx context.Context, _ idclient.Id, body idclient.AsignacionRolEntrada) error {
	g.anotar(ctx, "FijarRolSujeto")
	g.recibio = body
	return g.errGob
}

func (g *gobiernoFake) RevocarSesionesSujeto(ctx context.Context, _ idclient.Id, body idclient.RevocacionSesionesEntrada) (*idclient.RevocacionSesionesResultado, error) {
	g.anotar(ctx, "RevocarSesionesSujeto")
	g.recibio = body
	if g.errGob != nil {
		return nil, g.errGob
	}
	return &idclient.RevocacionSesionesResultado{SesionesRevocadas: 1}, nil
}

func (g *gobiernoFake) ObtenerAlcanceSujeto(ctx context.Context, _ idclient.Id) (*idclient.AlcanceSujeto, error) {
	g.anotar(ctx, "ObtenerAlcanceSujeto")
	if g.errGob != nil {
		return nil, g.errGob
	}
	u := uuid.New()
	return &idclient.AlcanceSujeto{UnidadesIds: []idclient.Id{u}, SubordinadosIds: []idclient.Id{}, UnidadesEfectivasIds: []idclient.Id{u}}, nil
}

func (g *gobiernoFake) FijarUnidadesSujeto(ctx context.Context, _ idclient.Id, body idclient.UnidadesSujetoEntrada) error {
	g.anotar(ctx, "FijarUnidadesSujeto")
	g.recibio = body
	return g.errGob
}

func (g *gobiernoFake) FijarSubordinadosSujeto(ctx context.Context, _ idclient.Id, body idclient.SubordinadosSujetoEntrada) error {
	g.anotar(ctx, "FijarSubordinadosSujeto")
	g.recibio = body
	return g.errGob
}

// ListarTenants pagina g.tenants de porPagina en porPagina (todo si es 0).
func (g *gobiernoFake) ListarTenants(ctx context.Context, params idclient.ListarTenantsParams) (*idclient.PaginaTenants, error) {
	g.anotar(ctx, "ListarTenants")
	desde := 0
	if params.Cursor != nil {
		desde, _ = strconv.Atoi(*params.Cursor)
	}
	hasta := len(g.tenants)
	if g.porPagina > 0 && desde+g.porPagina < hasta {
		hasta = desde + g.porPagina
	}
	p := &idclient.PaginaTenants{Items: slices.Clone(g.tenants[desde:hasta])}
	if hasta < len(g.tenants) {
		p.CursorSiguiente = ptr(strconv.Itoa(hasta))
	}
	return p, nil
}

func (g *gobiernoFake) CrearTenant(ctx context.Context, id idclient.Id, body idclient.TenantEntrada) (*idclient.Tenant, bool, error) {
	g.anotar(ctx, "CrearTenant")
	if g.errCrearTenant != nil {
		return nil, false, g.errCrearTenant
	}
	for _, t := range g.tenants {
		if t.Id == id {
			return &t, false, nil
		}
	}
	t := idclient.Tenant{Id: id, Nombre: body.Nombre, Tipo: idclient.TenantTipoCliente, Activa: true}
	g.tenants = append(g.tenants, t)
	return &t, true, nil
}

func (g *gobiernoFake) DesactivarTenant(ctx context.Context, id idclient.Id) error {
	g.anotar(ctx, "DesactivarTenant")
	if g.errDesactivarTenant != nil {
		return g.errDesactivarTenant
	}
	for i := range g.tenants {
		if g.tenants[i].Id == id {
			g.tenants[i].Activa = false
			return nil
		}
	}
	return platformerrors.NewNoEncontrado("tenant %s", id)
}

// orgFake es el expediente de la organización en el doble fakeCatalogos.
type orgFake struct {
	registro               *[]string
	obtenerClienteLlamadas int
	loteRecibido           [][]catclient.Id
	errCrearCliente        error
	errActualizarCliente   error
	actualizarRecibio      *catclient.ClienteCambios
}

func (f *fakeCatalogos) anotarOrg(metodo string) {
	if f.registro != nil {
		*f.registro = append(*f.registro, metodo)
	}
}

func (f *fakeCatalogos) clientesPorLote(ids []catclient.Id) (*catclient.PaginaClientes, error) {
	f.anotarOrg("ListarClientesPorLote")
	f.loteRecibido = append(f.loteRecibido, ids)
	p := &catclient.PaginaClientes{Items: []catclient.Cliente{}}
	for _, id := range ids {
		if c, ok := f.clientes[id]; ok {
			p.Items = append(p.Items, *c)
		}
	}
	return p, f.err
}

func (f *fakeCatalogos) CrearCliente(_ context.Context, _ string, id catclient.Id, e catclient.ClienteEntrada) (*catclient.Cliente, bool, error) {
	f.anotarOrg("CrearCliente")
	if f.errCrearCliente != nil {
		return nil, false, f.errCrearCliente
	}
	if c, ok := f.clientes[id]; ok {
		return c, false, nil
	}
	if f.clientes == nil {
		f.clientes = map[catclient.Id]*catclient.Cliente{}
	}
	c := &catclient.Cliente{Id: id, Nombre: e.Nombre, Correo: &e.Correo, Telefono: &e.Telefono, Estado: catclient.ClienteEstadoActivo}
	f.clientes[id] = c
	return c, true, nil
}

func (f *fakeCatalogos) ActualizarCliente(_ context.Context, _ string, id catclient.Id, c catclient.ClienteCambios) (*catclient.Cliente, error) {
	f.anotarOrg("ActualizarCliente")
	f.actualizarRecibio = &c
	if f.errActualizarCliente != nil {
		return nil, f.errActualizarCliente
	}
	cl, ok := f.clientes[id]
	if !ok {
		return nil, platformerrors.NewNoEncontrado("cliente %s", id)
	}
	if c.Nombre != nil {
		cl.Nombre = *c.Nombre
	}
	if c.Correo != nil {
		cl.Correo = c.Correo
	}
	if c.Telefono != nil {
		cl.Telefono = c.Telefono
	}
	if c.Estado != nil {
		cl.Estado = catclient.ClienteEstado(*c.Estado)
	}
	return cl, nil
}

// --- ayudas ---

var (
	tenantInterno = idclient.Tenant{Id: uuid.MustParse("00000000-0000-0000-0000-0000000000a1"), Nombre: "AI27", Tipo: idclient.TenantTipoInterno, Activa: true}
	orgActiva     = idclient.Tenant{Id: uuid.MustParse("00000000-0000-0000-0000-0000000000c1"), Nombre: "ACME (tenant)", Tipo: idclient.TenantTipoCliente, Activa: true, UsuariosActivos: 3}
	orgInactiva   = idclient.Tenant{Id: uuid.MustParse("00000000-0000-0000-0000-0000000000c2"), Nombre: "Vieja", Tipo: idclient.TenantTipoCliente, Activa: false}
)

// mundo arma los dobles con un registro de orden compartido.
type mundo struct {
	id       *fakeIdentidad
	cat      *fakeCatalogos
	op       *fakeOperacion
	registro []string
	h        *Handlers
}

func nuevoMundo() *mundo {
	m := &mundo{}
	m.id = &fakeIdentidad{gobiernoFake: gobiernoFake{registro: &m.registro, tenants: []idclient.Tenant{tenantInterno, orgActiva, orgInactiva}}}
	m.cat = &fakeCatalogos{orgFake: orgFake{registro: &m.registro}, clientes: map[catclient.Id]*catclient.Cliente{
		orgActiva.Id: {Id: orgActiva.Id, Nombre: "ACME", Correo: ptr("contacto@acme.mx"), Telefono: ptr("5550001"), Estado: catclient.ClienteEstadoActivo},
	}}
	m.op = &fakeOperacion{}
	m.h = NewHandlers(Deps{Identidad: m.id, Catalogos: m.cat, Operacion: m.op})
	return m
}

func claimsRol(rol roles.Rol) *auth.Claims {
	aud := auth.AudienciaInterno
	if rol == roles.RolCliente {
		aud = auth.AudienciaCliente
	}
	return &auth.Claims{Subject: "kc-actor", Rol: string(rol), Audiencia: aud, SesionID: "s-1"}
}

// pedir sirve una petición con cuerpo JSON crudo, {id} y claims, como el mux
// tras el middleware.
func pedir(h http.HandlerFunc, metodo, url, cuerpo, id string, claims *auth.Claims) *httptest.ResponseRecorder {
	req := httptest.NewRequest(metodo, url, bytes.NewBufferString(cuerpo))
	req.Header.Set("Authorization", "Bearer tok-admin")
	if id != "" {
		req.SetPathValue("id", id)
	}
	if claims != nil {
		req = req.WithContext(auth.ContextWithClaims(req.Context(), claims))
	}
	rec := httptest.NewRecorder()
	h(rec, req)
	return rec
}

func codigoDe(t *testing.T, rec *httptest.ResponseRecorder) string {
	t.Helper()
	var e bffclient.Error
	decodificarCuerpo(t, rec, &e)
	return e.Codigo
}

// --- permisos ---

type opAdmin struct {
	nombre, metodo, url, cuerpo, id string
	handler                         func(*Handlers) http.HandlerFunc
}

func opsAdministracion() []opAdmin {
	sujeto := uuid.NewString()
	org := orgActiva.Id.String()
	return []opAdmin{
		{"listarSujetos", http.MethodGet, "/api/v1/sujetos", "", "", func(h *Handlers) http.HandlerFunc { return h.ListarSujetos }},
		{"listarSujetos?rol=supervisor", http.MethodGet, "/api/v1/sujetos?rol=supervisor", "", "", func(h *Handlers) http.HandlerFunc { return h.ListarSujetos }},
		{"listarSujetos?rol=monitorista", http.MethodGet, "/api/v1/sujetos?rol=monitorista", "", "", func(h *Handlers) http.HandlerFunc { return h.ListarSujetos }},
		{"altaSujeto", http.MethodPost, "/api/v1/sujetos", `{"nombre":"Ana","identificador_usuario":"ana@ai27.mx","rol":"monitorista"}`, "", func(h *Handlers) http.HandlerFunc { return h.AltaSujeto }},
		{"obtenerSujeto", http.MethodGet, "/api/v1/sujetos/" + sujeto, "", sujeto, func(h *Handlers) http.HandlerFunc { return h.ObtenerSujeto }},
		{"editarSujeto", http.MethodPatch, "/api/v1/sujetos/" + sujeto, `{"nombre":"Ana B"}`, sujeto, func(h *Handlers) http.HandlerFunc { return h.EditarSujeto }},
		{"desactivarSujeto", http.MethodDelete, "/api/v1/sujetos/" + sujeto, "", sujeto, func(h *Handlers) http.HandlerFunc { return h.DesactivarSujeto }},
		{"reactivarSujeto", http.MethodPost, "/api/v1/sujetos/" + sujeto + "/reactivacion", "", sujeto, func(h *Handlers) http.HandlerFunc { return h.ReactivarSujeto }},
		{"fijarRolSujeto", http.MethodPut, "/api/v1/sujetos/" + sujeto + "/rol", `{"rol":"supervisor"}`, sujeto, func(h *Handlers) http.HandlerFunc { return h.FijarRolSujeto }},
		{"revocarSesionesSujeto", http.MethodPost, "/api/v1/sujetos/" + sujeto + "/revocaciones", `{"motivo":"equipo perdido"}`, sujeto, func(h *Handlers) http.HandlerFunc { return h.RevocarSesionesSujeto }},
		{"obtenerAlcanceSujeto", http.MethodGet, "/api/v1/sujetos/" + sujeto + "/alcance", "", sujeto, func(h *Handlers) http.HandlerFunc { return h.ObtenerAlcanceSujeto }},
		{"fijarUnidadesSujeto", http.MethodPut, "/api/v1/sujetos/" + sujeto + "/alcance/unidades", `{"unidades_ids":[]}`, sujeto, func(h *Handlers) http.HandlerFunc { return h.FijarUnidadesSujeto }},
		{"fijarSubordinadosSujeto", http.MethodPut, "/api/v1/sujetos/" + sujeto + "/alcance/subordinados", `{"sujetos_ids":[]}`, sujeto, func(h *Handlers) http.HandlerFunc { return h.FijarSubordinadosSujeto }},
		{"listarOrganizaciones", http.MethodGet, "/api/v1/organizaciones", "", "", func(h *Handlers) http.HandlerFunc { return h.ListarOrganizaciones }},
		{"crearOrganizacion", http.MethodPost, "/api/v1/organizaciones", `{"nombre":"Nueva","correo":"a@nueva.mx","telefono":"55"}`, "", func(h *Handlers) http.HandlerFunc { return h.CrearOrganizacion }},
		{"actualizarOrganizacion", http.MethodPatch, "/api/v1/organizaciones/" + org, `{"telefono":"56"}`, org, func(h *Handlers) http.HandlerFunc { return h.ActualizarOrganizacion }},
		{"desactivarOrganizacion", http.MethodDelete, "/api/v1/organizaciones/" + org, "", org, func(h *Handlers) http.HandlerFunc { return h.DesactivarOrganizacion }},
	}
}

// TestAdministracionPermisos: el administrador pasa en todas; el Jefe
// Operativo solo en "mis unidades", el alcance y listarSujetos con
// rol=monitorista; cualquier otro rol interno y el Cliente reciben 403 sin que
// el bff toque ningún puerto.
func TestAdministracionPermisos(t *testing.T) {
	permitidasJefe := map[string]bool{"fijarUnidadesSujeto": true, "obtenerAlcanceSujeto": true, "listarSujetos?rol=monitorista": true}
	for _, rol := range roles.Todos {
		for _, op := range opsAdministracion() {
			t.Run(string(rol)+"/"+op.nombre, func(t *testing.T) {
				m := nuevoMundo()
				rec := pedir(op.handler(m.h), op.metodo, op.url, op.cuerpo, op.id, claimsRol(rol))
				permitido := rol == roles.RolAdministrador || (rol == roles.RolJefeOperativo && permitidasJefe[op.nombre])
				if permitido {
					if rec.Code < 200 || rec.Code > 299 {
						t.Fatalf("status = %d, se esperaba 2xx; cuerpo %s", rec.Code, rec.Body)
					}
					return
				}
				if rec.Code != http.StatusForbidden {
					t.Fatalf("status = %d, se esperaba 403; cuerpo %s", rec.Code, rec.Body)
				}
				if len(m.registro) != 0 || m.op.flujoLlamado != "" {
					t.Errorf("un 403 no debe tocar puertos; llamadas %v", m.registro)
				}
			})
		}
	}
}

// TestAdministracionSinClaimsEs401: sin token verificado no se llega a ningún
// puerto.
func TestAdministracionSinClaimsEs401(t *testing.T) {
	for _, op := range opsAdministracion() {
		m := nuevoMundo()
		if rec := pedir(op.handler(m.h), op.metodo, op.url, op.cuerpo, op.id, nil); rec.Code != http.StatusUnauthorized {
			t.Errorf("%s: status = %d, se esperaba 401", op.nombre, rec.Code)
		}
	}
}

// TestAdministracionClaimsLleganAIdentidad: la fachada en proceso de identidad
// resuelve el actor desde los claims del ctx; el bff debe pasárselos.
func TestAdministracionClaimsLleganAIdentidad(t *testing.T) {
	m := nuevoMundo()
	rec := pedir(m.h.FijarRolSujeto, http.MethodPut, "/", `{"rol":"supervisor"}`, uuid.NewString(), claimsRol(roles.RolAdministrador))
	if rec.Code != http.StatusNoContent {
		t.Fatalf("status = %d", rec.Code)
	}
	if m.id.claimsVistos == nil || m.id.claimsVistos.Subject != "kc-actor" {
		t.Fatalf("identidad no recibió los claims del token: %+v", m.id.claimsVistos)
	}
	if got := m.id.recibio.(idclient.AsignacionRolEntrada).Rol; got != idclient.RolSupervisor {
		t.Errorf("rol reenviado = %q", got)
	}
}

// --- UsuarioAdmin ---

// TestUsuarioAdminComposicion: es_propio compara idp_subject con el sub del
// token; la organización sale solo para un cliente, con el nombre de
// catalogos; los opcionales sin dato se omiten.
func TestUsuarioAdminComposicion(t *testing.T) {
	casos := []struct {
		nombre     string
		detalle    idclient.SujetoDetalle
		esPropio   bool
		org        *bffclient.OrganizacionResumen
		sinCampos  []string
		conNombres bool
	}{
		{"propio", idclient.SujetoDetalle{Sujeto: idclient.Sujeto{IdpSubject: ptr("kc-actor"), TenantId: tenantInterno.Id, Activo: true, Nombre: ptr("Yo"), IdentificadorUsuario: ptr("yo@ai27.mx")}, TieneRol: true, Rol: ptr(idclient.RolAdministrador)}, true, nil, []string{"organizacion"}, true},
		{"otro", idclient.SujetoDetalle{Sujeto: idclient.Sujeto{IdpSubject: ptr("kc-otro"), TenantId: tenantInterno.Id, Activo: true}, TieneRol: true, Rol: ptr(idclient.RolMonitorista)}, false, nil, []string{"organizacion", "nombre", "identificador_usuario"}, false},
		{"sin idp_subject", idclient.SujetoDetalle{Sujeto: idclient.Sujeto{TenantId: tenantInterno.Id}}, false, nil, []string{"organizacion", "rol"}, false},
		{"cliente", idclient.SujetoDetalle{Sujeto: idclient.Sujeto{IdpSubject: ptr("kc-c"), TenantId: orgActiva.Id, Activo: true}, TieneRol: true, Rol: ptr(idclient.RolCliente)}, false, &bffclient.OrganizacionResumen{Id: orgActiva.Id, Nombre: "ACME"}, nil, false},
		{"cliente sin expediente", idclient.SujetoDetalle{Sujeto: idclient.Sujeto{TenantId: orgInactiva.Id}, TieneRol: true, Rol: ptr(idclient.RolCliente)}, false, nil, []string{"organizacion"}, false},
	}
	for _, c := range casos {
		t.Run(c.nombre, func(t *testing.T) {
			m := nuevoMundo()
			d := c.detalle
			d.Sujeto.Id = uuid.New()
			m.id.detalle = &d
			rec := pedir(m.h.ObtenerSujeto, http.MethodGet, "/", "", d.Sujeto.Id.String(), claimsRol(roles.RolAdministrador))
			if rec.Code != http.StatusOK {
				t.Fatalf("status = %d: %s", rec.Code, rec.Body)
			}
			var u bffclient.UsuarioAdmin
			decodificarCuerpo(t, rec, &u)
			if u.Id != d.Sujeto.Id || u.Activo != d.Sujeto.Activo || u.EsPropio != c.esPropio {
				t.Errorf("usuario = %+v, se esperaba es_propio=%v", u, c.esPropio)
			}
			if (u.Organizacion == nil) != (c.org == nil) || (c.org != nil && *u.Organizacion != *c.org) {
				t.Errorf("organizacion = %+v, se esperaba %+v", u.Organizacion, c.org)
			}
			if c.conNombres && (u.Nombre == nil || *u.Nombre != "Yo" || *u.IdentificadorUsuario != "yo@ai27.mx" || u.Rol == nil || *u.Rol != bffclient.RolAdministrador) {
				t.Errorf("nombre/identificador/rol no compuestos: %+v", u)
			}
			var crudo map[string]any
			decodificarCuerpo(t, rec, &crudo)
			for _, campo := range c.sinCampos {
				if _, ok := crudo[campo]; ok {
					t.Errorf("el campo %q debía omitirse: %s", campo, rec.Body)
				}
			}
		})
	}
}

// TestListarSujetosOrganizacionesPorLote: los filtros se reenvían, las
// organizaciones de los clientes se leen en UNA lectura por lote sin repetir
// ids y sin lecturas por id, y es_propio sale por fila.
func TestListarSujetosOrganizacionesPorLote(t *testing.T) {
	m := nuevoMundo()
	m.id.pagina = &idclient.PaginaSujetos{CursorSiguiente: ptr("c2"), Items: []idclient.SujetoDetalle{
		{Sujeto: idclient.Sujeto{Id: uuid.New(), TenantId: orgActiva.Id, IdpSubject: ptr("kc-actor")}, TieneRol: true, Rol: ptr(idclient.RolCliente)},
		{Sujeto: idclient.Sujeto{Id: uuid.New(), TenantId: orgActiva.Id}, TieneRol: true, Rol: ptr(idclient.RolCliente)},
		{Sujeto: idclient.Sujeto{Id: uuid.New(), TenantId: tenantInterno.Id}, TieneRol: true, Rol: ptr(idclient.RolMonitorista)},
	}}
	url := "/api/v1/sujetos?q=an&rol=cliente&tenant_id=" + orgActiva.Id.String() + "&activo=false&cursor=c1"
	rec := pedir(m.h.ListarSujetos, http.MethodGet, url, "", "", claimsRol(roles.RolAdministrador))
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d: %s", rec.Code, rec.Body)
	}
	p := m.id.listarRecibio
	if *p.Q != "an" || *p.Rol != idclient.RolCliente || *p.TenantId != orgActiva.Id || *p.Activo || *p.Cursor != "c1" {
		t.Errorf("filtros reenviados = %+v", p)
	}
	if len(m.cat.loteRecibido) != 1 || !slices.Equal(m.cat.loteRecibido[0], []catclient.Id{orgActiva.Id}) || m.cat.obtenerClienteLlamadas != 0 {
		t.Fatalf("lecturas de catalogos: lote %v, por id %d; se esperaba un lote con un id", m.cat.loteRecibido, m.cat.obtenerClienteLlamadas)
	}
	var pag bffclient.PaginaUsuariosAdmin
	decodificarCuerpo(t, rec, &pag)
	if pag.CursorSiguiente == nil || *pag.CursorSiguiente != "c2" || len(pag.Items) != 3 {
		t.Fatalf("pagina = %+v", pag)
	}
	if !pag.Items[0].EsPropio || pag.Items[1].EsPropio || pag.Items[0].Organizacion == nil || pag.Items[0].Organizacion.Nombre != "ACME" || pag.Items[2].Organizacion != nil {
		t.Errorf("items = %+v", pag.Items)
	}
}

// --- altaSujeto (B-7) ---

func TestAltaSujeto(t *testing.T) {
	alta := func(rol, orgID string) string {
		s := `{"nombre":"Ana","identificador_usuario":"ana@x.mx","rol":"` + rol + `"`
		if orgID != "" {
			s += `,"organizacion_id":"` + orgID + `"`
		}
		return s + "}"
	}
	t.Run("cliente va al tenant de su organizacion", func(t *testing.T) {
		m := nuevoMundo()
		m.id.porPagina = 1 // la organización está en la segunda página
		rec := pedir(m.h.AltaSujeto, http.MethodPost, "/", alta("cliente", orgActiva.Id.String()), "", claimsRol(roles.RolAdministrador))
		if rec.Code != http.StatusCreated {
			t.Fatalf("status = %d: %s", rec.Code, rec.Body)
		}
		if m.id.altaRecibio.TenantId != orgActiva.Id || m.id.altaRecibio.Rol != idclient.RolCliente {
			t.Errorf("alta reenviada = %+v", m.id.altaRecibio)
		}
		var u bffclient.UsuarioAdmin
		decodificarCuerpo(t, rec, &u)
		if u.Organizacion == nil || u.Organizacion.Nombre != "ACME" || u.Rol == nil || *u.Rol != bffclient.RolCliente || u.EsPropio {
			t.Errorf("usuario = %+v", u)
		}
	})
	t.Run("interno va al tenant interno", func(t *testing.T) {
		m := nuevoMundo()
		m.id.porPagina = 1
		m.id.tenants = []idclient.Tenant{orgActiva, orgInactiva, tenantInterno} // interno en la última página
		rec := pedir(m.h.AltaSujeto, http.MethodPost, "/", alta("monitorista", ""), "", claimsRol(roles.RolAdministrador))
		if rec.Code != http.StatusCreated {
			t.Fatalf("status = %d: %s", rec.Code, rec.Body)
		}
		if m.id.altaRecibio.TenantId != tenantInterno.Id {
			t.Errorf("tenant = %s, se esperaba el interno", m.id.altaRecibio.TenantId)
		}
		var crudo map[string]any
		decodificarCuerpo(t, rec, &crudo)
		if _, ok := crudo["organizacion"]; ok {
			t.Errorf("un interno no lleva organizacion: %s", rec.Body)
		}
	})
	rechazos := []struct {
		nombre, cuerpo string
		tenants        []idclient.Tenant
		status         int
		codigo         string
	}{
		{"cliente sin organizacion", alta("cliente", ""), nil, http.StatusBadRequest, ""},
		{"interno con organizacion", alta("supervisor", orgActiva.Id.String()), nil, http.StatusBadRequest, ""},
		{"organizacion inexistente", alta("cliente", uuid.NewString()), nil, http.StatusBadRequest, ""},
		{"organizacion es el tenant interno", alta("cliente", tenantInterno.Id.String()), nil, http.StatusBadRequest, ""},
		{"organizacion desactivada", alta("cliente", orgInactiva.Id.String()), nil, http.StatusConflict, codigoOrganizacionDesactivada},
		{"rol fuera de catalogo", alta("jefe", ""), nil, http.StatusBadRequest, ""},
		{"sin tenant interno", alta("ejecutivo", ""), []idclient.Tenant{orgActiva}, http.StatusServiceUnavailable, ""},
		{"dos tenants internos", alta("ejecutivo", ""), []idclient.Tenant{tenantInterno, {Id: uuid.New(), Tipo: idclient.TenantTipoInterno}}, http.StatusServiceUnavailable, ""},
	}
	for _, c := range rechazos {
		t.Run(c.nombre, func(t *testing.T) {
			m := nuevoMundo()
			if c.tenants != nil {
				m.id.tenants = c.tenants
			}
			rec := pedir(m.h.AltaSujeto, http.MethodPost, "/", c.cuerpo, "", claimsRol(roles.RolAdministrador))
			if rec.Code != c.status {
				t.Fatalf("status = %d, se esperaba %d: %s", rec.Code, c.status, rec.Body)
			}
			if c.codigo != "" && codigoDe(t, rec) != c.codigo {
				t.Errorf("codigo = %q, se esperaba %q", codigoDe(t, rec), c.codigo)
			}
			if m.id.altaRecibio != nil {
				t.Errorf("no debía llegar a identidad.altaSujeto")
			}
		})
	}
}

// --- revocación y alcance ---

func TestRevocarSesionesSujeto(t *testing.T) {
	m := nuevoMundo()
	for _, cuerpo := range []string{`{}`, `{"motivo":""}`} {
		if rec := pedir(m.h.RevocarSesionesSujeto, http.MethodPost, "/", cuerpo, uuid.NewString(), claimsRol(roles.RolAdministrador)); rec.Code != http.StatusBadRequest {
			t.Errorf("%s: status = %d, se esperaba 400", cuerpo, rec.Code)
		}
	}
	if len(m.id.llamadas) != 0 {
		t.Fatalf("sin motivo no se llama a identidad: %v", m.id.llamadas)
	}
	rec := pedir(m.h.RevocarSesionesSujeto, http.MethodPost, "/", `{"motivo":"equipo perdido"}`, uuid.NewString(), claimsRol(roles.RolAdministrador))
	if rec.Code != http.StatusOK || strings.TrimSpace(rec.Body.String()) != `{"sesiones_revocadas":1}` {
		t.Fatalf("status = %d, cuerpo %s", rec.Code, rec.Body)
	}
	if m.id.recibio.(idclient.RevocacionSesionesEntrada).Motivo != "equipo perdido" {
		t.Errorf("motivo reenviado = %+v", m.id.recibio)
	}
}

// TestFijarUnidadesSujetoValidaRoster: una unidad que no está en el roster es
// 400 y no llega a identidad; si todas existen, identidad recibe el conjunto.
func TestFijarUnidadesSujetoValidaRoster(t *testing.T) {
	u1, u2, fantasma := uuid.New(), uuid.New(), uuid.New()
	m := nuevoMundo()
	m.op.unidadesRoster = map[opclient.Id]bool{u1: true, u2: true}
	cuerpo := func(ids ...uuid.UUID) string {
		b, _ := json.Marshal(bffclient.UnidadesSujetoEntrada{UnidadesIds: ids})
		return string(b)
	}
	rec := pedir(m.h.FijarUnidadesSujeto, http.MethodPut, "/", cuerpo(u1, fantasma), uuid.NewString(), claimsRol(roles.RolJefeOperativo))
	if rec.Code != http.StatusBadRequest || !strings.Contains(rec.Body.String(), fantasma.String()) {
		t.Fatalf("status = %d, cuerpo %s; se esperaba 400 que nombre la unidad", rec.Code, rec.Body)
	}
	if len(m.id.llamadas) != 0 {
		t.Fatalf("con una unidad inexistente no se llama a identidad: %v", m.id.llamadas)
	}
	if rec := pedir(m.h.FijarUnidadesSujeto, http.MethodPut, "/", `{}`, uuid.NewString(), claimsRol(roles.RolJefeOperativo)); rec.Code != http.StatusBadRequest {
		t.Errorf("sin unidades_ids: status = %d, se esperaba 400", rec.Code)
	}
	rec = pedir(m.h.FijarUnidadesSujeto, http.MethodPut, "/", cuerpo(u1, u2, u1), uuid.NewString(), claimsRol(roles.RolJefeOperativo))
	if rec.Code != http.StatusNoContent {
		t.Fatalf("status = %d: %s", rec.Code, rec.Body)
	}
	if got := m.id.recibio.(idclient.UnidadesSujetoEntrada).UnidadesIds; !slices.Equal(got, []idclient.Id{u1, u2, u1}) {
		t.Errorf("unidades reenviadas = %v", got)
	}
}

// TestObtenerAlcanceSujetoEspejo: el alcance sale tal cual de identidad.
func TestObtenerAlcanceSujetoEspejo(t *testing.T) {
	m := nuevoMundo()
	rec := pedir(m.h.ObtenerAlcanceSujeto, http.MethodGet, "/", "", uuid.NewString(), claimsRol(roles.RolJefeOperativo))
	var a bffclient.AlcanceSujeto
	decodificarCuerpo(t, rec, &a)
	if rec.Code != http.StatusOK || len(a.UnidadesIds) != 1 || a.UnidadesEfectivasIds[0] != a.UnidadesIds[0] || a.SubordinadosIds == nil {
		t.Fatalf("status = %d, alcance %+v", rec.Code, a)
	}
}

// TestAdministracionConservaCodigo: los 403 y 409 de identidad llegan al panel
// con su status y su `codigo`.
func TestAdministracionConservaCodigo(t *testing.T) {
	ops := map[string]opAdmin{}
	for _, op := range opsAdministracion() {
		ops[op.nombre] = op
	}
	casos := []struct {
		op, codigo string
		categoria  platformerrors.Category
	}{
		{"fijarRolSujeto", "autoescalamiento", platformerrors.CategoryProhibido},
		{"fijarRolSujeto", "rol_igual", platformerrors.CategoryConflicto},
		{"fijarRolSujeto", "sujeto_desactivado", platformerrors.CategoryConflicto},
		{"fijarRolSujeto", "ultimo_administrador", platformerrors.CategoryConflicto},
		{"fijarRolSujeto", "rol_cruza_audiencia", platformerrors.CategoryConflicto},
		{"desactivarSujeto", "ultimo_administrador", platformerrors.CategoryConflicto},
		{"fijarUnidadesSujeto", "rol_sin_alcance", platformerrors.CategoryConflicto},
		{"fijarSubordinadosSujeto", "rol_sin_alcance", platformerrors.CategoryConflicto},
		{"fijarSubordinadosSujeto", "subordinado_rol_invalido", platformerrors.CategoryConflicto},
		{"fijarSubordinadosSujeto", "subordinado_con_superior", platformerrors.CategoryConflicto},
		{"obtenerAlcanceSujeto", "prohibido", platformerrors.CategoryProhibido},
	}
	for _, c := range casos {
		t.Run(c.op+"/"+c.codigo, func(t *testing.T) {
			m := nuevoMundo()
			m.id.errGob = &platformerrors.PlatformError{Category: c.categoria, Message: "identidad", Codigo: c.codigo}
			op := ops[c.op]
			rec := pedir(op.handler(m.h), op.metodo, op.url, op.cuerpo, op.id, claimsRol(roles.RolAdministrador))
			if rec.Code != c.categoria.HTTPStatus() || codigoDe(t, rec) != c.codigo {
				t.Errorf("status %d codigo %q; se esperaba %d %q", rec.Code, codigoDe(t, rec), c.categoria.HTTPStatus(), c.codigo)
			}
		})
	}
}

// --- organizaciones (DEC-144 #2) ---

func crearOrg(m *mundo, cuerpo string) *httptest.ResponseRecorder {
	return pedir(m.h.CrearOrganizacion, http.MethodPost, "/", cuerpo, "", claimsRol(roles.RolAdministrador))
}

func listarOrgs(t *testing.T, m *mundo) bffclient.PaginaOrganizaciones {
	t.Helper()
	rec := pedir(m.h.ListarOrganizaciones, http.MethodGet, "/", "", "", claimsRol(roles.RolAdministrador))
	if rec.Code != http.StatusOK {
		t.Fatalf("listarOrganizaciones: status %d: %s", rec.Code, rec.Body)
	}
	var p bffclient.PaginaOrganizaciones
	decodificarCuerpo(t, rec, &p)
	return p
}

func TestCrearOrganizacion(t *testing.T) {
	t.Run("catalogos primero y luego identidad, mismo id", func(t *testing.T) {
		m := nuevoMundo()
		id := uuid.New()
		rec := crearOrg(m, `{"id":"`+id.String()+`","nombre":"Nueva","correo":"a@nueva.mx","telefono":"55"}`)
		if rec.Code != http.StatusCreated {
			t.Fatalf("status = %d: %s", rec.Code, rec.Body)
		}
		if !slices.Equal(m.registro, []string{"CrearCliente", "CrearTenant"}) {
			t.Errorf("orden = %v", m.registro)
		}
		var o bffclient.Organizacion
		decodificarCuerpo(t, rec, &o)
		want := bffclient.Organizacion{Id: id, Nombre: "Nueva", Correo: ptr("a@nueva.mx"), Telefono: ptr("55"), Activa: true}
		if o.Id != want.Id || o.Nombre != want.Nombre || *o.Correo != *want.Correo || *o.Telefono != *want.Telefono || !o.Activa || o.UsuariosActivos != 0 {
			t.Errorf("organizacion = %+v", o)
		}
		if m.cat.clientes[id] == nil || m.id.tenants[len(m.id.tenants)-1].Id != id {
			t.Errorf("los dos lados no quedaron con el mismo id")
		}
	})
	t.Run("sin id lo genera el bff", func(t *testing.T) {
		m := nuevoMundo()
		rec := crearOrg(m, `{"nombre":"Nueva","correo":"a@nueva.mx","telefono":"55"}`)
		var o bffclient.Organizacion
		decodificarCuerpo(t, rec, &o)
		if rec.Code != http.StatusCreated || o.Id == uuid.Nil || m.cat.clientes[o.Id] == nil || m.id.tenants[len(m.id.tenants)-1].Id != o.Id {
			t.Fatalf("status %d, organizacion %+v", rec.Code, o)
		}
	})
	t.Run("alta a medias es invisible y el reintento la completa", func(t *testing.T) {
		m := nuevoMundo()
		id := uuid.New()
		cuerpo := `{"id":"` + id.String() + `","nombre":"Nueva","correo":"a@nueva.mx","telefono":"55"}`
		m.id.errCrearTenant = platformerrors.NewDegradado("identidad caida")
		if rec := crearOrg(m, cuerpo); rec.Code != http.StatusServiceUnavailable {
			t.Fatalf("status = %d, se esperaba 503", rec.Code)
		}
		for _, o := range listarOrgs(t, m).Items {
			if o.Id == id {
				t.Fatalf("un alta a medias no debe listarse")
			}
		}
		m.id.errCrearTenant = nil
		otro := `{"id":"` + id.String() + `","nombre":"Otro nombre","correo":"b@otro.mx","telefono":"99"}`
		rec := crearOrg(m, otro)
		if rec.Code != http.StatusCreated {
			t.Fatalf("reintento: status = %d: %s", rec.Code, rec.Body)
		}
		var o bffclient.Organizacion
		decodificarCuerpo(t, rec, &o)
		if o.Nombre != "Nueva" || *o.Correo != "a@nueva.mx" || m.id.tenants[len(m.id.tenants)-1].Nombre != "Nueva" {
			t.Errorf("el reintento debe conservar lo guardado en los dos lados: %+v", o)
		}
		var listada bool
		for _, o := range listarOrgs(t, m).Items {
			listada = listada || o.Id == id
		}
		if !listada {
			t.Errorf("la organización completada debe listarse")
		}
	})
	t.Run("repetir un alta completa no cambia nada", func(t *testing.T) {
		m := nuevoMundo()
		rec := crearOrg(m, `{"id":"`+orgActiva.Id.String()+`","nombre":"Cambiado","correo":"x@y.mx","telefono":"1"}`)
		var o bffclient.Organizacion
		decodificarCuerpo(t, rec, &o)
		if rec.Code != http.StatusCreated || o.Nombre != "ACME" || o.UsuariosActivos != 3 || len(m.id.tenants) != 3 {
			t.Fatalf("status %d, organizacion %+v", rec.Code, o)
		}
	})
	t.Run("organizacion desactivada", func(t *testing.T) {
		m := nuevoMundo()
		m.cat.clientes[orgInactiva.Id] = &catclient.Cliente{Id: orgInactiva.Id, Nombre: "Vieja", Estado: catclient.ClienteEstadoInactivo}
		rec := crearOrg(m, `{"id":"`+orgInactiva.Id.String()+`","nombre":"Vieja","correo":"v@v.mx","telefono":"1"}`)
		if rec.Code != http.StatusConflict || codigoDe(t, rec) != codigoOrganizacionDesactivada {
			t.Fatalf("status %d codigo %q", rec.Code, codigoDe(t, rec))
		}
	})
}

func TestListarOrganizaciones(t *testing.T) {
	m := nuevoMundo()
	p := listarOrgs(t, m)
	if len(p.Items) != 2 {
		t.Fatalf("se esperaban las 2 organizaciones cliente (sin el tenant interno): %+v", p.Items)
	}
	acme, vieja := p.Items[0], p.Items[1]
	if acme.Id != orgActiva.Id || acme.Nombre != "ACME" || *acme.Correo != "contacto@acme.mx" || *acme.Telefono != "5550001" || !acme.Activa || acme.UsuariosActivos != 3 {
		t.Errorf("acme = %+v", acme)
	}
	if vieja.Nombre != "Vieja" || vieja.Correo != nil || vieja.Telefono != nil || vieja.Activa {
		t.Errorf("sin expediente sale con el nombre del tenant y sin contacto: %+v", vieja)
	}
	if len(m.cat.loteRecibido) != 1 || len(m.cat.loteRecibido[0]) != 2 {
		t.Errorf("se esperaba una lectura por lote con 2 ids: %v", m.cat.loteRecibido)
	}
	m.id.porPagina = 2
	rec := pedir(m.h.ListarOrganizaciones, http.MethodGet, "/?cursor=2", "", "", claimsRol(roles.RolAdministrador))
	decodificarCuerpo(t, rec, &p)
	if len(p.Items) != 1 || p.Items[0].Id != orgInactiva.Id || p.CursorSiguiente != nil {
		t.Errorf("el cursor se reenvía a identidad: %+v", p)
	}
}

func TestActualizarOrganizacion(t *testing.T) {
	m := nuevoMundo()
	rec := pedir(m.h.ActualizarOrganizacion, http.MethodPatch, "/", `{"telefono":"5559999"}`, orgActiva.Id.String(), claimsRol(roles.RolAdministrador))
	var o bffclient.Organizacion
	decodificarCuerpo(t, rec, &o)
	if rec.Code != http.StatusOK || *o.Telefono != "5559999" || o.Nombre != "ACME" || !o.Activa {
		t.Fatalf("status %d, organizacion %+v", rec.Code, o)
	}
	if c := m.cat.actualizarRecibio; c.Estado != nil || c.Nombre != nil || *c.Telefono != "5559999" {
		t.Errorf("cambios reenviados = %+v", c)
	}

	m = nuevoMundo()
	rec = pedir(m.h.ActualizarOrganizacion, http.MethodPatch, "/", `{"nombre":"X"}`, orgInactiva.Id.String(), claimsRol(roles.RolAdministrador))
	if rec.Code != http.StatusConflict || codigoDe(t, rec) != codigoOrganizacionDesactivada || m.cat.actualizarRecibio != nil {
		t.Errorf("desactivada: status %d codigo %q, catalogos llamado %v", rec.Code, codigoDe(t, rec), m.cat.actualizarRecibio != nil)
	}
	for _, id := range []uuid.UUID{uuid.New(), tenantInterno.Id} {
		if rec := pedir(m.h.ActualizarOrganizacion, http.MethodPatch, "/", `{}`, id.String(), claimsRol(roles.RolAdministrador)); rec.Code != http.StatusNotFound {
			t.Errorf("%s: status = %d, se esperaba 404", id, rec.Code)
		}
	}
}

func TestDesactivarOrganizacion(t *testing.T) {
	m := nuevoMundo()
	m.cat.errActualizarCliente = platformerrors.NewDegradado("catalogos caido")
	desactivar := func() int {
		return pedir(m.h.DesactivarOrganizacion, http.MethodDelete, "/", "", orgActiva.Id.String(), claimsRol(roles.RolAdministrador)).Code
	}
	if code := desactivar(); code != http.StatusServiceUnavailable {
		t.Fatalf("a medias: status = %d, se esperaba 503", code)
	}
	if !slices.Equal(m.registro, []string{"DesactivarTenant", "ActualizarCliente"}) {
		t.Errorf("orden = %v", m.registro)
	}
	m.cat.errActualizarCliente = nil
	for i := range 2 {
		if code := desactivar(); code != http.StatusNoContent {
			t.Fatalf("intento %d: status = %d, se esperaba 204", i+2, code)
		}
	}
	if m.id.tenants[1].Activa || m.cat.clientes[orgActiva.Id].Estado != catclient.ClienteEstadoInactivo {
		t.Errorf("los dos lados deben quedar inactivos: tenant %+v, cliente %+v", m.id.tenants[1], m.cat.clientes[orgActiva.Id])
	}
	m.id.errDesactivarTenant = platformerrors.NewNoEncontrado("tenant")
	m.registro = nil
	if code := desactivar(); code != http.StatusNotFound || len(m.registro) != 1 {
		t.Errorf("sin tenant: status %d, llamadas %v; catalogos no debe tocarse", code, m.registro)
	}
}
