package agregacion

import (
	"encoding/json"
	"net/http"

	bffclient "github.com/AI27-Tech/pf-pkg-contracts/packages/api-client-go/bff"
	catclient "github.com/AI27-Tech/pf-pkg-contracts/packages/api-client-go/catalogos"
	opclient "github.com/AI27-Tech/pf-pkg-contracts/packages/api-client-go/operacion"
	"github.com/AI27-Tech/pf-pkg-contracts/packages/api-client-go/roles"
	"github.com/AI27-Tech/pf-pkg-platform-go/auth"
	platformerrors "github.com/AI27-Tech/pf-pkg-platform-go/errors"
	"github.com/AI27-Tech/pf-svc-bff/internal/autorizacion"
	"github.com/AI27-Tech/pf-svc-bff/internal/tokenctx"
)

// --- /roster y vehículos y custodios de /catalogos (tracer.39, E8-T21) -------
//
// Unidades: reenvío a operacion, que decide el rol (jefe_operativo y
// administrador, DEC-142 #3) y las reglas del roster; el bff solo exige
// audiencia Interna, como listarUnidades, y responde la Unidad legible.
// Vehículos y custodios: reenvío a catalogos, que NO revisa rol, así que la
// guarda la aplica el bff (DEC-142 #6). La escolta del viaje (obtenerAsignacion)
// la lee cualquier rol interno; el Cliente recibe 404.

// rolesLecturaRecursos leen vehículos y custodios: el Jefe Operativo los
// necesita para armar unidades.
var rolesLecturaRecursos = map[roles.Rol]bool{
	roles.RolJefeOperativo: true,
	roles.RolAdministrador: true,
}

// rolesEscrituraRecursos dan de alta y editan vehículos y custodios.
var rolesEscrituraRecursos = map[roles.Rol]bool{
	roles.RolAdministrador: true,
}

// decodificarEstricto lee el cuerpo rechazando propiedades fuera del contrato
// (400), como operacion en el alta y la edición de unidades (p. ej. `tipo`, que
// se deriva del vehículo).
func decodificarEstricto(r *http.Request, v any) error {
	dec := json.NewDecoder(r.Body)
	dec.DisallowUnknownFields()
	if err := dec.Decode(v); err != nil {
		return platformerrors.NewValidacion("bff: cuerpo de la peticion invalido: %v", err)
	}
	return nil
}

// CrearUnidad delega en operacion.crearUnidad y responde la unidad legible
// (201). Los 409 de exclusividad llegan con su codigo.
func (h *Handlers) CrearUnidad(w http.ResponseWriter, r *http.Request) {
	if !h.soloInterno(w, r, "crearUnidad") {
		return
	}
	var cuerpo bffclient.UnidadEntrada
	if err := decodificarEstricto(r, &cuerpo); err != nil {
		escribirError(w, err)
		return
	}
	bearer := r.Header.Get("Authorization")
	ctx := tokenctx.Con(r.Context(), bearer)
	u, err := h.deps.Operacion.CrearUnidad(ctx, opclient.UnidadEntrada(cuerpo))
	if err != nil {
		escribirError(w, err)
		return
	}
	legible, err := h.unidadLegible(ctx, bearer, *u)
	if err != nil {
		escribirError(w, err)
		return
	}
	escribirJSON(w, http.StatusCreated, legible)
}

// ObtenerUnidad delega en operacion.obtenerUnidad y responde la unidad legible.
// viajeInterno sirve aquí: exige audiencia Interna y lee el {id} de la ruta.
func (h *Handlers) ObtenerUnidad(w http.ResponseWriter, r *http.Request) {
	id, ok := h.viajeInterno(w, r, "obtenerUnidad")
	if !ok {
		return
	}
	bearer := r.Header.Get("Authorization")
	ctx := tokenctx.Con(r.Context(), bearer)
	u, err := h.deps.Operacion.ObtenerUnidad(ctx, id)
	if err != nil {
		escribirError(w, err)
		return
	}
	legible, err := h.unidadLegible(ctx, bearer, *u)
	if err != nil {
		escribirError(w, err)
		return
	}
	escribirJSON(w, http.StatusOK, legible)
}

// ModificarUnidad delega en operacion.modificarUnidad (incluye activar y
// desactivar) y responde la unidad legible. Los 409 (exclusividad,
// unidad_con_reserva_vigente) llegan con su codigo.
func (h *Handlers) ModificarUnidad(w http.ResponseWriter, r *http.Request) {
	id, ok := h.viajeInterno(w, r, "modificarUnidad")
	if !ok {
		return
	}
	var cuerpo bffclient.UnidadModificacion
	if err := decodificarEstricto(r, &cuerpo); err != nil {
		escribirError(w, err)
		return
	}
	bearer := r.Header.Get("Authorization")
	ctx := tokenctx.Con(r.Context(), bearer)
	u, err := h.deps.Operacion.ModificarUnidad(ctx, id, opclient.UnidadModificacion(cuerpo))
	if err != nil {
		escribirError(w, err)
		return
	}
	legible, err := h.unidadLegible(ctx, bearer, *u)
	if err != nil {
		escribirError(w, err)
		return
	}
	escribirJSON(w, http.StatusOK, legible)
}

// ObtenerAsignacion delega en operacion.obtenerAsignacion y responde la escolta
// del viaje con cada unidad legible, para la ficha técnica (E8-T12). Cualquier
// rol interno; el Cliente recibe 404 sin tocar ningún puerto. Sin asignación
// vigente, `unidades` sale vacío.
func (h *Handlers) ObtenerAsignacion(w http.ResponseWriter, r *http.Request) {
	claims := autorizacion.ClaimsDe(r.Context())
	if claims == nil {
		escribirError(w, platformerrors.NewNoAutorizado("bff: obtenerAsignacion: sin claims"))
		return
	}
	if claims.Audiencia != auth.AudienciaInterno {
		escribirError(w, platformerrors.NewNoEncontrado("bff: obtenerAsignacion: viaje no encontrado"))
		return
	}
	id, err := uuidDe(r.PathValue("id"))
	if err != nil {
		escribirError(w, platformerrors.NewValidacion("bff: obtenerAsignacion: id invalido"))
		return
	}
	bearer := r.Header.Get("Authorization")
	ctx := tokenctx.Con(r.Context(), bearer)
	escolta, err := h.deps.Operacion.ObtenerAsignacion(ctx, id)
	if err != nil {
		escribirError(w, err)
		return
	}
	unidades, err := h.unidadesLegibles(ctx, bearer, escolta.Unidades)
	if err != nil {
		escribirError(w, err)
		return
	}
	escribirJSON(w, http.StatusOK, bffclient.EscoltaVigente{ViajeId: escolta.ViajeId, Unidades: unidades})
}

// recursoConRol exige uno de los roles permitidos y lee el {id} de la ruta.
func (h *Handlers) recursoConRol(w http.ResponseWriter, r *http.Request, op string, permitidos map[roles.Rol]bool) (catclient.Id, bool) {
	if !h.soloRoles(w, r, op, permitidos) {
		return catclient.Id{}, false
	}
	id, err := uuidDe(r.PathValue("id"))
	if err != nil {
		escribirError(w, platformerrors.NewValidacion("bff: %s: id invalido", op))
		return catclient.Id{}, false
	}
	return id, true
}

// cursorDe lee el cursor opcional del query.
func cursorDe(r *http.Request) *string {
	if c := r.URL.Query().Get("cursor"); c != "" {
		return &c
	}
	return nil
}

// ListarVehiculos delega en catalogos.listarVehiculos (activos e inactivos). La
// respuesta de catalogos tiene la forma del bff y sale tal cual.
func (h *Handlers) ListarVehiculos(w http.ResponseWriter, r *http.Request) {
	if !h.soloRoles(w, r, "listarVehiculos", rolesLecturaRecursos) {
		return
	}
	pagina, err := h.deps.Catalogos.ListarVehiculos(r.Context(), r.Header.Get("Authorization"), catclient.ListarVehiculosParams{Cursor: cursorDe(r)})
	if err != nil {
		escribirError(w, err)
		return
	}
	escribirJSON(w, http.StatusOK, pagina)
}

// CrearVehiculo delega en catalogos.crearVehiculo. La flota es de AI27
// (DEC-143): el cuerpo del bff no trae cliente_id y al reenviar no se manda,
// aunque el llamante lo incluya.
func (h *Handlers) CrearVehiculo(w http.ResponseWriter, r *http.Request) {
	if !h.soloRoles(w, r, "crearVehiculo", rolesEscrituraRecursos) {
		return
	}
	var e bffclient.VehiculoEntrada
	if err := decodificar(r, &e); err != nil {
		escribirError(w, err)
		return
	}
	v, err := h.deps.Catalogos.CrearVehiculo(r.Context(), r.Header.Get("Authorization"), catclient.VehiculoEntrada{
		Tipo:      catclient.VehiculoEntradaTipo(e.Tipo),
		Estado:    catclient.VehiculoEntradaEstado(e.Estado),
		Economico: e.Economico,
		Placas:    e.Placas,
	})
	if err != nil {
		escribirError(w, err)
		return
	}
	escribirJSON(w, http.StatusCreated, v)
}

// ObtenerVehiculo delega en catalogos.obtenerVehiculo (versión vigente).
func (h *Handlers) ObtenerVehiculo(w http.ResponseWriter, r *http.Request) {
	id, ok := h.recursoConRol(w, r, "obtenerVehiculo", rolesLecturaRecursos)
	if !ok {
		return
	}
	v, err := h.deps.Catalogos.ObtenerVehiculo(r.Context(), r.Header.Get("Authorization"), id)
	if err != nil {
		escribirError(w, err)
		return
	}
	escribirJSON(w, http.StatusOK, v)
}

// ActualizarVehiculo delega en catalogos.actualizarVehiculo (versión siguiente;
// activar o desactivar es cambiar `estado`).
func (h *Handlers) ActualizarVehiculo(w http.ResponseWriter, r *http.Request) {
	id, ok := h.recursoConRol(w, r, "actualizarVehiculo", rolesEscrituraRecursos)
	if !ok {
		return
	}
	var c bffclient.VehiculoCambios
	if err := decodificar(r, &c); err != nil {
		escribirError(w, err)
		return
	}
	v, err := h.deps.Catalogos.ActualizarVehiculo(r.Context(), r.Header.Get("Authorization"), id, catclient.VehiculoCambios{
		Tipo:      (*catclient.VehiculoCambiosTipo)(c.Tipo),
		Estado:    (*catclient.VehiculoCambiosEstado)(c.Estado),
		Economico: c.Economico,
		Placas:    c.Placas,
	})
	if err != nil {
		escribirError(w, err)
		return
	}
	escribirJSON(w, http.StatusOK, v)
}

// ListarCustodios delega en catalogos.listarCustodios (activos e inactivos).
func (h *Handlers) ListarCustodios(w http.ResponseWriter, r *http.Request) {
	if !h.soloRoles(w, r, "listarCustodios", rolesLecturaRecursos) {
		return
	}
	pagina, err := h.deps.Catalogos.ListarCustodios(r.Context(), r.Header.Get("Authorization"), catclient.ListarCustodiosParams{Cursor: cursorDe(r)})
	if err != nil {
		escribirError(w, err)
		return
	}
	escribirJSON(w, http.StatusOK, pagina)
}

// CrearCustodio delega en catalogos.crearCustodio.
func (h *Handlers) CrearCustodio(w http.ResponseWriter, r *http.Request) {
	if !h.soloRoles(w, r, "crearCustodio", rolesEscrituraRecursos) {
		return
	}
	var e bffclient.CustodioEntrada
	if err := decodificar(r, &e); err != nil {
		escribirError(w, err)
		return
	}
	c, err := h.deps.Catalogos.CrearCustodio(r.Context(), r.Header.Get("Authorization"), catclient.CustodioEntrada{
		Nombre:    e.Nombre,
		Telefono:  e.Telefono,
		Estado:    catclient.CustodioEntradaEstado(e.Estado),
		UsuarioId: e.UsuarioId,
	})
	if err != nil {
		escribirError(w, err)
		return
	}
	escribirJSON(w, http.StatusCreated, c)
}

// ObtenerCustodio delega en catalogos.obtenerCustodio (versión vigente).
func (h *Handlers) ObtenerCustodio(w http.ResponseWriter, r *http.Request) {
	id, ok := h.recursoConRol(w, r, "obtenerCustodio", rolesLecturaRecursos)
	if !ok {
		return
	}
	c, err := h.deps.Catalogos.ObtenerCustodio(r.Context(), r.Header.Get("Authorization"), id)
	if err != nil {
		escribirError(w, err)
		return
	}
	escribirJSON(w, http.StatusOK, c)
}

// ActualizarCustodio delega en catalogos.actualizarCustodio (versión siguiente).
func (h *Handlers) ActualizarCustodio(w http.ResponseWriter, r *http.Request) {
	id, ok := h.recursoConRol(w, r, "actualizarCustodio", rolesEscrituraRecursos)
	if !ok {
		return
	}
	var cambios bffclient.CustodioCambios
	if err := decodificar(r, &cambios); err != nil {
		escribirError(w, err)
		return
	}
	c, err := h.deps.Catalogos.ActualizarCustodio(r.Context(), r.Header.Get("Authorization"), id, catclient.CustodioCambios{
		Nombre:    cambios.Nombre,
		Telefono:  cambios.Telefono,
		Estado:    (*catclient.CustodioCambiosEstado)(cambios.Estado),
		UsuarioId: cambios.UsuarioId,
	})
	if err != nil {
		escribirError(w, err)
		return
	}
	escribirJSON(w, http.StatusOK, c)
}
