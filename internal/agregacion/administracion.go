package agregacion

import (
	"context"
	"net/http"
	"slices"
	"strconv"

	bffclient "github.com/AI27-Tech/pf-pkg-contracts/packages/api-client-go/bff"
	catclient "github.com/AI27-Tech/pf-pkg-contracts/packages/api-client-go/catalogos"
	idclient "github.com/AI27-Tech/pf-pkg-contracts/packages/api-client-go/identidad"
	"github.com/AI27-Tech/pf-pkg-contracts/packages/api-client-go/roles"
	platformerrors "github.com/AI27-Tech/pf-pkg-platform-go/errors"
	"github.com/AI27-Tech/pf-svc-bff/internal/autorizacion"
	"github.com/AI27-Tech/pf-svc-bff/internal/tokenctx"
	"github.com/google/uuid"
)

// --- Administración de usuarios, alcance y organizaciones (tracer.40, E8-T19) ---
//
// Usuarios y alcance: proyección espejo de identidad (gobierno), que es la
// autoridad de rol y de reglas; sus 403 y 409 llegan con su `codigo`. El bff
// pone su propia guarda de rol antes de llamar y compone `UsuarioAdmin`.
// Organizaciones: operaciones propias del bff que escriben en catalogos
// (expediente) y en identidad (tenant) con el mismo id (DEC-144 #2).

// rolesAdministracion leen y escriben usuarios y organizaciones.
var rolesAdministracion = map[roles.Rol]bool{
	roles.RolAdministrador: true,
}

// rolesMisUnidades también entran a "mis unidades" de un Monitorista: el Jefe
// Operativo las fija y las lee en /roster (DEC-140 #1, DEC-142 #2).
var rolesMisUnidades = map[roles.Rol]bool{
	roles.RolAdministrador: true,
	roles.RolJefeOperativo: true,
}

// codigoOrganizacionDesactivada es el 409 de una organización desactivada:
// desactivar es terminal, no se edita ni se revierte con un alta (DEC-139 #4).
const codigoOrganizacionDesactivada = "organizacion_desactivada"

// conToken devuelve el ctx con el Bearer (para el adaptador HTTP de identidad)
// y el Bearer mismo (para catalogos, que lo recibe explícito).
func conToken(r *http.Request) (context.Context, string) {
	bearer := r.Header.Get("Authorization")
	return tokenctx.Con(r.Context(), bearer), bearer
}

// subDe es el `sub` del token, para resolver `es_propio` (B-14).
func subDe(r *http.Request) string {
	if c := autorizacion.ClaimsDe(r.Context()); c != nil {
		return c.Subject
	}
	return ""
}

// rolValido dice si r está en el catálogo de roles del contrato.
func rolValido(r string) bool {
	return slices.Contains(roles.Todos, roles.Rol(r))
}

// esCliente dice si el usuario es de una organización cliente. Ninguna cuenta
// cruza entre cliente e interno (DEC-141 #1), así que el rol lo decide; un
// usuario sin rol activo sale sin organización.
func esCliente(d idclient.SujetoDetalle) bool {
	return d.TieneRol && d.Rol != nil && *d.Rol == idclient.RolCliente
}

// usuarioAdmin compone el UsuarioAdmin desde el detalle de identidad. sub es el
// del token: `sub` no es el id del sujeto (DEC-072), se compara contra
// idp_subject. org es la organización legible, o nil.
func usuarioAdmin(d idclient.SujetoDetalle, sub string, org *bffclient.OrganizacionResumen) bffclient.UsuarioAdmin {
	s := d.Sujeto
	u := bffclient.UsuarioAdmin{
		Id:                   s.Id,
		Nombre:               s.Nombre,
		IdentificadorUsuario: s.IdentificadorUsuario,
		Activo:               s.Activo,
		EsPropio:             sub != "" && s.IdpSubject != nil && *s.IdpSubject == sub,
		Organizacion:         org,
	}
	if d.TieneRol && d.Rol != nil {
		rol := bffclient.Rol(*d.Rol)
		u.Rol = &rol
	}
	return u
}

// organizacionDe lee en catalogos la organización de un usuario cliente (su
// tenant ES su cliente_id, DEC-112). Sin expediente en catalogos, nil.
func (h *Handlers) organizacionDe(ctx context.Context, bearer string, d idclient.SujetoDetalle) (*bffclient.OrganizacionResumen, error) {
	if !esCliente(d) {
		return nil, nil
	}
	c, err := h.deps.Catalogos.ObtenerCliente(ctx, bearer, d.Sujeto.TenantId)
	if platformerrors.AsCategory(err) == platformerrors.CategoryNoEncontrado {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &bffclient.OrganizacionResumen{Id: c.Id, Nombre: c.Nombre}, nil
}

// clientesPorLote lee de una vez los expedientes de ids (listarClientes con
// `ids`), sin N+1. Lo que catalogos no devuelva falta en el mapa.
func (h *Handlers) clientesPorLote(ctx context.Context, bearer string, ids []catclient.Id) (map[catclient.Id]catclient.Cliente, error) {
	if len(ids) == 0 {
		return nil, nil
	}
	p, err := h.deps.Catalogos.ListarClientes(ctx, bearer, catclient.ListarClientesParams{Ids: &ids})
	if err != nil {
		return nil, err
	}
	m := make(map[catclient.Id]catclient.Cliente, len(p.Items))
	for _, c := range p.Items {
		m[c.Id] = c
	}
	return m, nil
}

// usuarioUnico responde un usuario compuesto (detalle, edición, alta).
func (h *Handlers) usuarioUnico(w http.ResponseWriter, r *http.Request, status int, d idclient.SujetoDetalle) {
	ctx, bearer := conToken(r)
	org, err := h.organizacionDe(ctx, bearer, d)
	if err != nil {
		escribirError(w, err)
		return
	}
	escribirJSON(w, status, usuarioAdmin(d, subDe(r), org))
}

// tenants lee todas las páginas de identidad.listarTenants.
// shortcut: recorre el listado completo porque identidad no expone obtenerTenant; con miles de organizaciones, pedir la lectura por id en el contrato.
func (h *Handlers) tenants(ctx context.Context) ([]idclient.Tenant, error) {
	var todos []idclient.Tenant
	var cursor *idclient.Cursor
	for {
		p, err := h.deps.Identidad.ListarTenants(ctx, idclient.ListarTenantsParams{Cursor: cursor})
		if err != nil {
			return nil, err
		}
		todos = append(todos, p.Items...)
		if p.CursorSiguiente == nil || *p.CursorSiguiente == "" {
			return todos, nil
		}
		if cursor != nil && *cursor == *p.CursorSiguiente {
			return nil, platformerrors.NewRoto("bff: listarTenants: el cursor no avanza")
		}
		cursor = p.CursorSiguiente
	}
}

// tenantCliente busca el tenant de la organización id. nil si no existe o si
// es el tenant interno, que no es una organización.
func (h *Handlers) tenantCliente(ctx context.Context, id idclient.Id) (*idclient.Tenant, error) {
	ts, err := h.tenants(ctx)
	if err != nil {
		return nil, err
	}
	for i := range ts {
		if ts[i].Id == id && ts[i].Tipo == idclient.TenantTipoCliente {
			return &ts[i], nil
		}
	}
	return nil, nil
}

// --- Usuarios ---

// ListarSujetos delega en identidad.listarSujetos y compone cada UsuarioAdmin;
// las organizaciones se leen por lote. Lee el administrador; el Jefe Operativo
// solo con rol=monitorista.
func (h *Handlers) ListarSujetos(w http.ResponseWriter, r *http.Request) {
	if !h.soloRoles(w, r, "listarSujetos", rolesMisUnidades) {
		return
	}
	q := r.URL.Query()
	params := idclient.ListarSujetosParams{Cursor: cursorDe(r)}
	if v := q.Get("q"); v != "" {
		params.Q = &v
	}
	if v := q.Get("rol"); v != "" {
		if !rolValido(v) {
			escribirError(w, platformerrors.NewValidacion("bff: listarSujetos: rol invalido"))
			return
		}
		rol := idclient.Rol(v)
		params.Rol = &rol
	}
	if v := q.Get("tenant_id"); v != "" {
		id, err := uuidDe(v)
		if err != nil {
			escribirError(w, platformerrors.NewValidacion("bff: listarSujetos: tenant_id invalido"))
			return
		}
		params.TenantId = &id
	}
	if v := q.Get("activo"); v != "" {
		b, err := strconv.ParseBool(v)
		if err != nil {
			escribirError(w, platformerrors.NewValidacion("bff: listarSujetos: activo invalido"))
			return
		}
		params.Activo = &b
	}
	if roles.Rol(autorizacion.ClaimsDe(r.Context()).Rol) == roles.RolJefeOperativo &&
		(params.Rol == nil || *params.Rol != idclient.RolMonitorista) {
		escribirError(w, platformerrors.NewProhibido("bff: listarSujetos: el jefe_operativo solo lista con rol=monitorista"))
		return
	}
	ctx, bearer := conToken(r)
	p, err := h.deps.Identidad.ListarSujetos(ctx, params)
	if err != nil {
		escribirError(w, err)
		return
	}
	var ids []catclient.Id
	for _, d := range p.Items {
		if esCliente(d) && !slices.Contains(ids, d.Sujeto.TenantId) {
			ids = append(ids, d.Sujeto.TenantId)
		}
	}
	clientes, err := h.clientesPorLote(ctx, bearer, ids)
	if err != nil {
		escribirError(w, err)
		return
	}
	sub := subDe(r)
	items := make([]bffclient.UsuarioAdmin, 0, len(p.Items))
	for _, d := range p.Items {
		var org *bffclient.OrganizacionResumen
		if c, ok := clientes[d.Sujeto.TenantId]; ok && esCliente(d) {
			org = &bffclient.OrganizacionResumen{Id: c.Id, Nombre: c.Nombre}
		}
		items = append(items, usuarioAdmin(d, sub, org))
	}
	escribirJSON(w, http.StatusOK, bffclient.PaginaUsuariosAdmin{Items: items, CursorSiguiente: p.CursorSiguiente})
}

// AltaSujeto traduce organizacion_id al tenant_id de identidad (B-7) y delega
// en identidad.altaSujeto. Un cliente va al tenant de su organización, que debe
// existir y estar activa; un rol interno, al único tenant interno de AI27.
func (h *Handlers) AltaSujeto(w http.ResponseWriter, r *http.Request) {
	if !h.soloRoles(w, r, "altaSujeto", rolesAdministracion) {
		return
	}
	var e bffclient.AltaSujetoEntrada
	if err := decodificar(r, &e); err != nil {
		escribirError(w, err)
		return
	}
	if !rolValido(string(e.Rol)) {
		escribirError(w, platformerrors.NewValidacion("bff: altaSujeto: rol invalido"))
		return
	}
	ctx, _ := conToken(r)
	var tenantID idclient.Id
	if e.Rol == bffclient.RolCliente {
		if e.OrganizacionId == nil {
			escribirError(w, platformerrors.NewValidacion("bff: altaSujeto: organizacion_id es obligatorio para el rol cliente"))
			return
		}
		t, err := h.tenantCliente(ctx, *e.OrganizacionId)
		if err != nil {
			escribirError(w, err)
			return
		}
		if t == nil {
			escribirError(w, platformerrors.NewValidacion("bff: altaSujeto: la organizacion no existe"))
			return
		}
		if !t.Activa {
			escribirError(w, platformerrors.NewConflictoCodigo(codigoOrganizacionDesactivada, "bff: altaSujeto: la organizacion esta desactivada"))
			return
		}
		tenantID = t.Id
	} else {
		if e.OrganizacionId != nil {
			escribirError(w, platformerrors.NewValidacion("bff: altaSujeto: organizacion_id no aplica a un rol interno"))
			return
		}
		ts, err := h.tenants(ctx)
		if err != nil {
			escribirError(w, err)
			return
		}
		var internos []idclient.Id
		for _, t := range ts {
			if t.Tipo == idclient.TenantTipoInterno {
				internos = append(internos, t.Id)
			}
		}
		if len(internos) != 1 {
			escribirError(w, platformerrors.NewDegradado("bff: altaSujeto: se esperaba un tenant interno y hay %d", len(internos)))
			return
		}
		tenantID = internos[0]
	}
	rol := idclient.Rol(e.Rol)
	s, err := h.deps.Identidad.AltaSujeto(ctx, idclient.AltaSujetoEntrada{
		Nombre:               e.Nombre,
		IdentificadorUsuario: e.IdentificadorUsuario,
		Rol:                  rol,
		TenantId:             tenantID,
	})
	if err != nil {
		escribirError(w, err)
		return
	}
	h.usuarioUnico(w, r, http.StatusCreated, idclient.SujetoDetalle{Sujeto: *s, TieneRol: true, Rol: &rol})
}

// ObtenerSujeto delega en identidad.obtenerSujeto y compone el UsuarioAdmin.
func (h *Handlers) ObtenerSujeto(w http.ResponseWriter, r *http.Request) {
	id, ok := h.recursoConRol(w, r, "obtenerSujeto", rolesAdministracion)
	if !ok {
		return
	}
	ctx, _ := conToken(r)
	d, err := h.deps.Identidad.ObtenerSujeto(ctx, id)
	if err != nil {
		escribirError(w, err)
		return
	}
	h.usuarioUnico(w, r, http.StatusOK, *d)
}

// EditarSujeto delega en identidad.editarSujeto (solo el nombre, DEC-139 #5).
func (h *Handlers) EditarSujeto(w http.ResponseWriter, r *http.Request) {
	id, ok := h.recursoConRol(w, r, "editarSujeto", rolesAdministracion)
	if !ok {
		return
	}
	var e bffclient.EdicionSujetoEntrada
	if err := decodificar(r, &e); err != nil {
		escribirError(w, err)
		return
	}
	ctx, _ := conToken(r)
	d, err := h.deps.Identidad.EditarSujeto(ctx, id, idclient.EdicionSujetoEntrada{Nombre: e.Nombre})
	if err != nil {
		escribirError(w, err)
		return
	}
	h.usuarioUnico(w, r, http.StatusOK, *d)
}

// DesactivarSujeto delega en identidad.desactivarSujeto (204). El 409
// ultimo_administrador llega con su codigo.
func (h *Handlers) DesactivarSujeto(w http.ResponseWriter, r *http.Request) {
	id, ok := h.recursoConRol(w, r, "desactivarSujeto", rolesAdministracion)
	if !ok {
		return
	}
	ctx, _ := conToken(r)
	responderSinCuerpo(w, h.deps.Identidad.DesactivarSujeto(ctx, id))
}

// ReactivarSujeto delega en identidad.reactivarSujeto (204).
func (h *Handlers) ReactivarSujeto(w http.ResponseWriter, r *http.Request) {
	id, ok := h.recursoConRol(w, r, "reactivarSujeto", rolesAdministracion)
	if !ok {
		return
	}
	ctx, _ := conToken(r)
	responderSinCuerpo(w, h.deps.Identidad.ReactivarSujeto(ctx, id))
}

// FijarRolSujeto delega en identidad.fijarRolSujeto (204). Los 403
// autoescalamiento y los 409 (rol_igual, sujeto_desactivado,
// ultimo_administrador, rol_cruza_audiencia) llegan con su codigo.
func (h *Handlers) FijarRolSujeto(w http.ResponseWriter, r *http.Request) {
	id, ok := h.recursoConRol(w, r, "fijarRolSujeto", rolesAdministracion)
	if !ok {
		return
	}
	var e bffclient.AsignacionRolEntrada
	if err := decodificar(r, &e); err != nil {
		escribirError(w, err)
		return
	}
	if !rolValido(string(e.Rol)) {
		escribirError(w, platformerrors.NewValidacion("bff: fijarRolSujeto: rol invalido"))
		return
	}
	ctx, _ := conToken(r)
	responderSinCuerpo(w, h.deps.Identidad.FijarRolSujeto(ctx, id, idclient.AsignacionRolEntrada{Rol: idclient.Rol(e.Rol)}))
}

// RevocarSesionesSujeto delega en identidad.revocarSesionesSujeto. El motivo
// es obligatorio (DEC-139 #1).
func (h *Handlers) RevocarSesionesSujeto(w http.ResponseWriter, r *http.Request) {
	id, ok := h.recursoConRol(w, r, "revocarSesionesSujeto", rolesAdministracion)
	if !ok {
		return
	}
	var e bffclient.RevocacionSesionesEntrada
	if err := decodificar(r, &e); err != nil {
		escribirError(w, err)
		return
	}
	if e.Motivo == "" {
		escribirError(w, platformerrors.NewValidacion("bff: revocarSesionesSujeto: el motivo es obligatorio"))
		return
	}
	ctx, _ := conToken(r)
	res, err := h.deps.Identidad.RevocarSesionesSujeto(ctx, id, idclient.RevocacionSesionesEntrada{Motivo: e.Motivo})
	if err != nil {
		escribirError(w, err)
		return
	}
	escribirJSON(w, http.StatusOK, bffclient.RevocacionSesionesResultado{SesionesRevocadas: res.SesionesRevocadas})
}

// --- Alcance de monitoreo ---

// ObtenerAlcanceSujeto delega en identidad.obtenerAlcanceSujeto. El bff deja
// pasar al administrador y al Jefe Operativo; que el Jefe Operativo solo lea a
// un Monitorista lo decide identidad (403), porque el bff no puede leer el rol
// del objetivo con su token: obtenerSujeto es solo del administrador.
func (h *Handlers) ObtenerAlcanceSujeto(w http.ResponseWriter, r *http.Request) {
	id, ok := h.recursoConRol(w, r, "obtenerAlcanceSujeto", rolesMisUnidades)
	if !ok {
		return
	}
	ctx, _ := conToken(r)
	a, err := h.deps.Identidad.ObtenerAlcanceSujeto(ctx, id)
	if err != nil {
		escribirError(w, err)
		return
	}
	escribirJSON(w, http.StatusOK, bffclient.AlcanceSujeto(*a))
}

// FijarUnidadesSujeto valida contra operacion que cada unidad exista en el
// roster (identidad no puede: sin FK entre módulos) y delega en
// identidad.fijarUnidadesSujeto (204).
func (h *Handlers) FijarUnidadesSujeto(w http.ResponseWriter, r *http.Request) {
	id, ok := h.recursoConRol(w, r, "fijarUnidadesSujeto", rolesMisUnidades)
	if !ok {
		return
	}
	var e bffclient.UnidadesSujetoEntrada
	if err := decodificar(r, &e); err != nil {
		escribirError(w, err)
		return
	}
	// Sin el campo, un nil llegaría como "quitarle todas".
	if e.UnidadesIds == nil {
		escribirError(w, platformerrors.NewValidacion("bff: fijarUnidadesSujeto: unidades_ids es obligatorio"))
		return
	}
	ctx, _ := conToken(r)
	// shortcut: una lectura por unidad porque operacion no expone lectura por lote del roster; la lista de un Monitorista es corta.
	var vistas []uuid.UUID
	for _, u := range e.UnidadesIds {
		if slices.Contains(vistas, u) {
			continue
		}
		vistas = append(vistas, u)
		_, err := h.deps.Operacion.ObtenerUnidad(ctx, u)
		if platformerrors.AsCategory(err) == platformerrors.CategoryNoEncontrado {
			escribirError(w, platformerrors.NewValidacion("bff: fijarUnidadesSujeto: la unidad %s no existe en el roster", u))
			return
		}
		if err != nil {
			escribirError(w, err)
			return
		}
	}
	responderSinCuerpo(w, h.deps.Identidad.FijarUnidadesSujeto(ctx, id, idclient.UnidadesSujetoEntrada{UnidadesIds: e.UnidadesIds}))
}

// FijarSubordinadosSujeto delega en identidad.fijarSubordinadosSujeto (204).
// Los 409 rol_sin_alcance, subordinado_rol_invalido y subordinado_con_superior
// llegan con su codigo.
func (h *Handlers) FijarSubordinadosSujeto(w http.ResponseWriter, r *http.Request) {
	id, ok := h.recursoConRol(w, r, "fijarSubordinadosSujeto", rolesAdministracion)
	if !ok {
		return
	}
	var e bffclient.SubordinadosSujetoEntrada
	if err := decodificar(r, &e); err != nil {
		escribirError(w, err)
		return
	}
	if e.SujetosIds == nil {
		escribirError(w, platformerrors.NewValidacion("bff: fijarSubordinadosSujeto: sujetos_ids es obligatorio"))
		return
	}
	ctx, _ := conToken(r)
	responderSinCuerpo(w, h.deps.Identidad.FijarSubordinadosSujeto(ctx, id, idclient.SubordinadosSujetoEntrada{SujetosIds: e.SujetosIds}))
}

// responderSinCuerpo escribe 204 o el error.
func responderSinCuerpo(w http.ResponseWriter, err error) {
	if err != nil {
		escribirError(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// --- Organizaciones ---

// organizacion compone la Organizacion: el expediente de catalogos manda en
// nombre y contacto; el tenant aporta activa y usuarios_activos. Sin
// expediente, sale con el nombre del tenant y sin contacto.
func organizacion(cl *catclient.Cliente, t idclient.Tenant) bffclient.Organizacion {
	o := bffclient.Organizacion{Id: t.Id, Nombre: t.Nombre, Activa: t.Activa, UsuariosActivos: t.UsuariosActivos}
	if cl != nil {
		o.Nombre, o.Correo, o.Telefono = cl.Nombre, cl.Correo, cl.Telefono
	}
	return o
}

// ListarOrganizaciones pagina por los tenants de identidad (solo los de
// cliente, activos y desactivados) y lee los expedientes de catalogos por lote.
// Un alta a medias (expediente sin tenant) no aparece hasta completarse.
func (h *Handlers) ListarOrganizaciones(w http.ResponseWriter, r *http.Request) {
	if !h.soloRoles(w, r, "listarOrganizaciones", rolesAdministracion) {
		return
	}
	ctx, bearer := conToken(r)
	p, err := h.deps.Identidad.ListarTenants(ctx, idclient.ListarTenantsParams{Cursor: cursorDe(r)})
	if err != nil {
		escribirError(w, err)
		return
	}
	var tenants []idclient.Tenant
	var ids []catclient.Id
	for _, t := range p.Items {
		if t.Tipo == idclient.TenantTipoCliente {
			tenants = append(tenants, t)
			ids = append(ids, t.Id)
		}
	}
	clientes, err := h.clientesPorLote(ctx, bearer, ids)
	if err != nil {
		escribirError(w, err)
		return
	}
	items := make([]bffclient.Organizacion, 0, len(tenants))
	for _, t := range tenants {
		var cl *catclient.Cliente
		if c, ok := clientes[t.Id]; ok {
			cl = &c
		}
		items = append(items, organizacion(cl, t))
	}
	escribirJSON(w, http.StatusOK, bffclient.PaginaOrganizaciones{Items: items, CursorSiguiente: p.CursorSiguiente})
}

// CrearOrganizacion da de alta el expediente en catalogos y DESPUÉS el tenant
// en identidad, con el mismo id (DEC-144 #2). Ese orden permite reintentar: el
// listado sale de los tenants, así que un alta a medias queda invisible, y
// repetir con el mismo id la completa (las dos altas son idempotentes por id).
// Responde 201 también al completar o repetir, como fija el contrato.
func (h *Handlers) CrearOrganizacion(w http.ResponseWriter, r *http.Request) {
	if !h.soloRoles(w, r, "crearOrganizacion", rolesAdministracion) {
		return
	}
	var e bffclient.OrganizacionEntrada
	if err := decodificar(r, &e); err != nil {
		escribirError(w, err)
		return
	}
	id := uuid.New()
	if e.Id != nil {
		id = *e.Id
	}
	ctx, bearer := conToken(r)
	cl, _, err := h.deps.Catalogos.CrearCliente(ctx, bearer, id, catclient.ClienteEntrada{Nombre: e.Nombre, Correo: e.Correo, Telefono: e.Telefono})
	if err != nil {
		escribirError(w, err)
		return
	}
	// El nombre del tenant es el del expediente guardado: si el reintento trae
	// otro nombre, catalogos conservó el primero y los dos lados coinciden.
	t, _, err := h.deps.Identidad.CrearTenant(ctx, id, idclient.TenantEntrada{Nombre: cl.Nombre})
	if err != nil {
		escribirError(w, err)
		return
	}
	if !t.Activa {
		escribirError(w, platformerrors.NewConflictoCodigo(codigoOrganizacionDesactivada, "bff: crearOrganizacion: la organizacion esta desactivada"))
		return
	}
	escribirJSON(w, http.StatusCreated, organizacion(cl, *t))
}

// organizacionActiva lee el {id} y exige que su tenant exista (404) y esté
// activo (409 organizacion_desactivada).
func (h *Handlers) organizacionActiva(w http.ResponseWriter, r *http.Request, op string) (*idclient.Tenant, bool) {
	id, ok := h.recursoConRol(w, r, op, rolesAdministracion)
	if !ok {
		return nil, false
	}
	ctx, _ := conToken(r)
	t, err := h.tenantCliente(ctx, id)
	if err != nil {
		escribirError(w, err)
		return nil, false
	}
	if t == nil {
		escribirError(w, platformerrors.NewNoEncontrado("bff: %s: organizacion no encontrada", op))
		return nil, false
	}
	if !t.Activa {
		escribirError(w, platformerrors.NewConflictoCodigo(codigoOrganizacionDesactivada, "bff: %s: la organizacion esta desactivada", op))
		return nil, false
	}
	return t, true
}

// ActualizarOrganizacion cambia nombre, correo y teléfono en catalogos. Una
// organización desactivada no se edita (DEC-139 #4); el estado no cambia aquí.
func (h *Handlers) ActualizarOrganizacion(w http.ResponseWriter, r *http.Request) {
	t, ok := h.organizacionActiva(w, r, "actualizarOrganizacion")
	if !ok {
		return
	}
	var e bffclient.OrganizacionCambios
	if err := decodificar(r, &e); err != nil {
		escribirError(w, err)
		return
	}
	ctx, bearer := conToken(r)
	cl, err := h.deps.Catalogos.ActualizarCliente(ctx, bearer, t.Id, catclient.ClienteCambios{Nombre: e.Nombre, Correo: e.Correo, Telefono: e.Telefono})
	if err != nil {
		escribirError(w, err)
		return
	}
	escribirJSON(w, http.StatusOK, organizacion(cl, *t))
}

// DesactivarOrganizacion desactiva el tenant en identidad y después el
// expediente en catalogos. Es terminal (DEC-139 #4) e idempotente: identidad
// responde 204 sobre un tenant ya inactivo, así que repetir completa una
// desactivación a medias.
func (h *Handlers) DesactivarOrganizacion(w http.ResponseWriter, r *http.Request) {
	id, ok := h.recursoConRol(w, r, "desactivarOrganizacion", rolesAdministracion)
	if !ok {
		return
	}
	ctx, bearer := conToken(r)
	if err := h.deps.Identidad.DesactivarTenant(ctx, id); err != nil {
		escribirError(w, err)
		return
	}
	inactivo := catclient.ClienteCambiosEstadoInactivo
	_, err := h.deps.Catalogos.ActualizarCliente(ctx, bearer, id, catclient.ClienteCambios{Estado: &inactivo})
	responderSinCuerpo(w, err)
}
