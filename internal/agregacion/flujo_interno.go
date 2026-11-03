package agregacion

import (
	"net/http"

	bffclient "github.com/AI27-Tech/pf-pkg-contracts/packages/api-client-go/bff"
	opclient "github.com/AI27-Tech/pf-pkg-contracts/packages/api-client-go/operacion"
	platformerrors "github.com/AI27-Tech/pf-pkg-platform-go/errors"
	"github.com/AI27-Tech/pf-svc-bff/internal/tokenctx"
)

// --- Flujo interno del panel (bff como única puerta pública) ----------------
//
// Reenvíos a operacion con operationId homónimo (DEC-105): detalle interno, ETA,
// asignación, bitácora, incidencias y estadías. El bff solo exige audiencia
// Interna antes de reenviar (un Cliente recibe 403) y propaga el Bearer;
// operacion es la autoridad de rol, estado y conflictos, y su
// 400/401/403/404/409 (con codigo) se propaga tal cual. La vista NO se recorta:
// es la superficie interna. Las entradas se decodifican con el tipo de bff y se
// traducen al de operacion; las salidas de forma idéntica salen tal cual (como
// crearViaje/listarViajes).

// viajeInterno exige audiencia Interna y lee el {id} del viaje. Devuelve false y
// ya escribió el error cuando no procede.
func (h *Handlers) viajeInterno(w http.ResponseWriter, r *http.Request, op string) (opclient.Id, bool) {
	if !h.soloInterno(w, r, op) {
		return opclient.Id{}, false
	}
	id, err := uuidDe(r.PathValue("id"))
	if err != nil {
		escribirError(w, platformerrors.NewValidacion("bff: %s: id invalido", op))
		return opclient.Id{}, false
	}
	return id, true
}

// ObtenerViajeInterno delega en operacion.obtenerViaje y devuelve el detalle
// interno (estado canónico + condiciones), no la vista de seguimiento del
// cliente. Ruta protegida, solo Interna.
func (h *Handlers) ObtenerViajeInterno(w http.ResponseWriter, r *http.Request) {
	id, ok := h.viajeInterno(w, r, "obtenerViajeInterno")
	if !ok {
		return
	}
	ctx := tokenctx.Con(r.Context(), r.Header.Get("Authorization"))
	detalle, err := h.deps.Operacion.ObtenerViaje(ctx, id)
	if err != nil {
		escribirError(w, err)
		return
	}
	escribirJSON(w, http.StatusOK, aViajeDetalle(*detalle))
}

// FijarEta delega en operacion.fijarEta y, como transicionarViaje, recompone el
// detalle interno con ObtenerViaje (operacion responde el Viaje plano).
func (h *Handlers) FijarEta(w http.ResponseWriter, r *http.Request) {
	id, ok := h.viajeInterno(w, r, "fijarEta")
	if !ok {
		return
	}
	var cuerpo bffclient.EtaEntrada
	if err := decodificar(r, &cuerpo); err != nil {
		escribirError(w, err)
		return
	}
	ctx := tokenctx.Con(r.Context(), r.Header.Get("Authorization"))
	if _, err := h.deps.Operacion.FijarEta(ctx, id, opclient.EtaEntrada(cuerpo)); err != nil {
		escribirError(w, err)
		return
	}
	detalle, err := h.deps.Operacion.ObtenerViaje(ctx, id)
	if err != nil {
		escribirError(w, err)
		return
	}
	escribirJSON(w, http.StatusOK, aViajeDetalle(*detalle))
}

// AsignarRecursos delega en operacion.asignarRecursos (reserva atómica + APROBADO
// → ASIGNADO, ADR 31).
func (h *Handlers) AsignarRecursos(w http.ResponseWriter, r *http.Request) {
	id, ok := h.viajeInterno(w, r, "asignarRecursos")
	if !ok {
		return
	}
	var cuerpo bffclient.AsignacionEntrada
	if err := decodificar(r, &cuerpo); err != nil {
		escribirError(w, err)
		return
	}
	ctx := tokenctx.Con(r.Context(), r.Header.Get("Authorization"))
	asignacion, err := h.deps.Operacion.AsignarRecursos(ctx, id, opclient.AsignacionEntrada(cuerpo))
	if err != nil {
		escribirError(w, err)
		return
	}
	escribirJSON(w, http.StatusOK, asignacion)
}

// ListarBitacora delega en operacion.listarBitacora: UNA página, con el cursor
// opcional del query.
func (h *Handlers) ListarBitacora(w http.ResponseWriter, r *http.Request) {
	id, ok := h.viajeInterno(w, r, "listarBitacora")
	if !ok {
		return
	}
	var params opclient.ListarBitacoraParams
	if c := r.URL.Query().Get("cursor"); c != "" {
		params.Cursor = &c
	}
	ctx := tokenctx.Con(r.Context(), r.Header.Get("Authorization"))
	pagina, err := h.deps.Operacion.ListarBitacora(ctx, id, params)
	if err != nil {
		escribirError(w, err)
		return
	}
	escribirJSON(w, http.StatusOK, pagina)
}

// RegistrarEntradaBitacora delega en operacion.registrarEntradaBitacora
// (append-only; el autor sale del token aguas abajo). 201.
func (h *Handlers) RegistrarEntradaBitacora(w http.ResponseWriter, r *http.Request) {
	id, ok := h.viajeInterno(w, r, "registrarEntradaBitacora")
	if !ok {
		return
	}
	var cuerpo bffclient.EntradaBitacoraEntrada
	if err := decodificar(r, &cuerpo); err != nil {
		escribirError(w, err)
		return
	}
	ctx := tokenctx.Con(r.Context(), r.Header.Get("Authorization"))
	entrada, err := h.deps.Operacion.RegistrarEntradaBitacora(ctx, id, opclient.EntradaBitacoraEntrada(cuerpo))
	if err != nil {
		escribirError(w, err)
		return
	}
	escribirJSON(w, http.StatusCreated, entrada)
}

// AbrirIncidencia delega en operacion.abrirIncidencia (el rol del token decide
// la facultad). 201.
func (h *Handlers) AbrirIncidencia(w http.ResponseWriter, r *http.Request) {
	id, ok := h.viajeInterno(w, r, "abrirIncidencia")
	if !ok {
		return
	}
	var cuerpo bffclient.IncidenciaEntrada
	if err := decodificar(r, &cuerpo); err != nil {
		escribirError(w, err)
		return
	}
	ctx := tokenctx.Con(r.Context(), r.Header.Get("Authorization"))
	inc, err := h.deps.Operacion.AbrirIncidencia(ctx, id, opclient.IncidenciaEntrada(cuerpo))
	if err != nil {
		escribirError(w, err)
		return
	}
	escribirJSON(w, http.StatusCreated, inc)
}

// ResolverIncidencia delega en operacion.resolverIncidencia (solo supervisor o
// administrador, lo impone operacion).
func (h *Handlers) ResolverIncidencia(w http.ResponseWriter, r *http.Request) {
	id, ok := h.viajeInterno(w, r, "resolverIncidencia")
	if !ok {
		return
	}
	iid, err := uuidDe(r.PathValue("iid"))
	if err != nil {
		escribirError(w, platformerrors.NewValidacion("bff: resolverIncidencia: id de incidencia invalido"))
		return
	}
	var cuerpo bffclient.ResolucionIncidenciaEntrada
	if err := decodificar(r, &cuerpo); err != nil {
		escribirError(w, err)
		return
	}
	ctx := tokenctx.Con(r.Context(), r.Header.Get("Authorization"))
	inc, err := h.deps.Operacion.ResolverIncidencia(ctx, id, iid, opclient.ResolucionIncidenciaEntrada(cuerpo))
	if err != nil {
		escribirError(w, err)
		return
	}
	escribirJSON(w, http.StatusOK, inc)
}

// ListarEstadias delega en operacion.listarEstadias: UNA página, con el cursor
// opcional del query.
func (h *Handlers) ListarEstadias(w http.ResponseWriter, r *http.Request) {
	id, ok := h.viajeInterno(w, r, "listarEstadias")
	if !ok {
		return
	}
	var params opclient.ListarEstadiasParams
	if c := r.URL.Query().Get("cursor"); c != "" {
		params.Cursor = &c
	}
	ctx := tokenctx.Con(r.Context(), r.Header.Get("Authorization"))
	pagina, err := h.deps.Operacion.ListarEstadias(ctx, id, params)
	if err != nil {
		escribirError(w, err)
		return
	}
	escribirJSON(w, http.StatusOK, pagina)
}

// AbrirEstadia delega en operacion.abrirEstadia. Los enums de motivo y eje son
// tipos distintos en cada paquete generado; operacion valida sus valores. 201.
func (h *Handlers) AbrirEstadia(w http.ResponseWriter, r *http.Request) {
	id, ok := h.viajeInterno(w, r, "abrirEstadia")
	if !ok {
		return
	}
	var cuerpo bffclient.EstadiaEntrada
	if err := decodificar(r, &cuerpo); err != nil {
		escribirError(w, err)
		return
	}
	ctx := tokenctx.Con(r.Context(), r.Header.Get("Authorization"))
	est, err := h.deps.Operacion.AbrirEstadia(ctx, id, opclient.EstadiaEntrada{
		Motivo:          opclient.MotivoEstadia(cuerpo.Motivo),
		EjeAutorizacion: opclient.EjeAutorizacionEstadia(cuerpo.EjeAutorizacion),
	})
	if err != nil {
		escribirError(w, err)
		return
	}
	escribirJSON(w, http.StatusCreated, est)
}

// CerrarEstadia delega en operacion.cerrarEstadia. El cuerpo es vacío por
// contrato (el servidor fija el cierre), así que no se lee.
func (h *Handlers) CerrarEstadia(w http.ResponseWriter, r *http.Request) {
	id, ok := h.viajeInterno(w, r, "cerrarEstadia")
	if !ok {
		return
	}
	eid, err := uuidDe(r.PathValue("eid"))
	if err != nil {
		escribirError(w, platformerrors.NewValidacion("bff: cerrarEstadia: id de estadia invalido"))
		return
	}
	ctx := tokenctx.Con(r.Context(), r.Header.Get("Authorization"))
	est, err := h.deps.Operacion.CerrarEstadia(ctx, id, eid)
	if err != nil {
		escribirError(w, err)
		return
	}
	escribirJSON(w, http.StatusOK, est)
}
