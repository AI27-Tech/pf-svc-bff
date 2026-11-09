package agregacion

import (
	"net/http"
	"strconv"
	"time"

	bffclient "github.com/AI27-Tech/pf-pkg-contracts/packages/api-client-go/bff"
	opclient "github.com/AI27-Tech/pf-pkg-contracts/packages/api-client-go/operacion"
	repclient "github.com/AI27-Tech/pf-pkg-contracts/packages/api-client-go/reportes"
	"github.com/AI27-Tech/pf-pkg-contracts/packages/api-client-go/roles"
	platformerrors "github.com/AI27-Tech/pf-pkg-platform-go/errors"
	"github.com/AI27-Tech/pf-svc-bff/internal/autorizacion"
	"github.com/AI27-Tech/pf-svc-bff/internal/tokenctx"
)

// --- Dashboard y evidencia del monitoreo (E8-T17) ---------------------------
//
// Reenvíos a reportes (dashboard) y a operacion (evidencia) con operationId
// homónimo. A diferencia del flujo interno, que solo exige audiencia Interna,
// estas cinco operaciones son del monitoreo: solo monitorista, supervisor,
// coordinador y administrador (DEC-022 #9, DEC-060). Cualquier otro rol,
// incluido cliente, recibe 403 antes de tocar el puerto. El alcance por unidad
// (DEC-045) aún no se aplica en ningún servicio (deuda DT-07): el límite
// efectivo hoy es el rol.

// rolesMonitoreo son los roles que pueden leer el dashboard y la evidencia.
var rolesMonitoreo = map[roles.Rol]bool{
	roles.RolMonitorista:   true,
	roles.RolSupervisor:    true,
	roles.RolCoordinador:   true,
	roles.RolAdministrador: true,
}

// soloMonitoreo exige uno de los roles de monitoreo. Devuelve false y ya escribió
// el error (401 sin claims, 403 con otro rol) cuando no procede.
func (h *Handlers) soloMonitoreo(w http.ResponseWriter, r *http.Request, op string) bool {
	return h.soloRoles(w, r, op, rolesMonitoreo)
}

// soloRoles exige que el rol del token esté en permitidos. Devuelve false y ya
// escribió el error (401 sin claims, 403 con otro rol, Cliente incluido) cuando
// no procede.
func (h *Handlers) soloRoles(w http.ResponseWriter, r *http.Request, op string, permitidos map[roles.Rol]bool) bool {
	claims := autorizacion.ClaimsDe(r.Context())
	if claims == nil {
		escribirError(w, platformerrors.NewNoAutorizado("bff: %s: sin claims", op))
		return false
	}
	if !permitidos[roles.Rol(claims.Rol)] {
		escribirError(w, platformerrors.NewProhibido("bff: %s: el rol %q no tiene esta facultad", op, claims.Rol))
		return false
	}
	return true
}

// viajeMonitoreo exige un rol de monitoreo y lee el {id} del viaje.
func (h *Handlers) viajeMonitoreo(w http.ResponseWriter, r *http.Request, op string) (opclient.Id, bool) {
	if !h.soloMonitoreo(w, r, op) {
		return opclient.Id{}, false
	}
	id, err := uuidDe(r.PathValue("id"))
	if err != nil {
		escribirError(w, platformerrors.NewValidacion("bff: %s: id invalido", op))
		return opclient.Id{}, false
	}
	return id, true
}

// periodoDe lee los query params obligatorios desde/hasta (RFC 3339). Un formato
// inválido o ausente es validación (400). El orden del rango lo valida reportes.
func periodoDe(r *http.Request) (desde, hasta time.Time, err error) {
	desde, err = time.Parse(time.RFC3339, r.URL.Query().Get("desde"))
	if err != nil {
		return desde, hasta, platformerrors.NewValidacion("bff: desde invalido (se espera RFC 3339)")
	}
	hasta, err = time.Parse(time.RFC3339, r.URL.Query().Get("hasta"))
	if err != nil {
		return desde, hasta, platformerrors.NewValidacion("bff: hasta invalido (se espera RFC 3339)")
	}
	return desde, hasta, nil
}

// ReporteViajes delega en reportes.reporteViajes y devuelve el reporte tal cual,
// con proyeccion_al y stale sin recortar (DEC-033). Acepta en_curso y hace
// desde/hasta opcionales; la regla (sin en_curso, el rango es obligatorio) la
// valida reportes, el bff solo reenvia. Ruta protegida, solo monitoreo.
func (h *Handlers) ReporteViajes(w http.ResponseWriter, r *http.Request) {
	if !h.soloMonitoreo(w, r, "reporteViajes") {
		return
	}
	params, err := reporteViajesParamsDe(r)
	if err != nil {
		escribirError(w, err)
		return
	}
	ctx := tokenctx.Con(r.Context(), r.Header.Get("Authorization"))
	rep, err := h.deps.Reportes.ReporteViajes(ctx, params)
	if err != nil {
		escribirError(w, err)
		return
	}
	escribirJSON(w, http.StatusOK, rep)
}

// reporteViajesParamsDe lee desde, hasta (RFC 3339) y en_curso. Los tres son
// opcionales aqui: que falte el rango sin en_curso lo valida reportes. Un formato
// invalido es 400.
func reporteViajesParamsDe(r *http.Request) (repclient.ReporteViajesParams, error) {
	var p repclient.ReporteViajesParams
	q := r.URL.Query()
	var err error
	if p.Desde, err = fechaOpcionalRFC3339(q.Get("desde"), "desde"); err != nil {
		return p, err
	}
	if p.Hasta, err = fechaOpcionalRFC3339(q.Get("hasta"), "hasta"); err != nil {
		return p, err
	}
	if v := q.Get("en_curso"); v != "" {
		b, err := strconv.ParseBool(v)
		if err != nil {
			return p, platformerrors.NewValidacion("bff: en_curso invalido (se espera true o false)")
		}
		p.EnCurso = &b
	}
	return p, nil
}

// fechaOpcionalRFC3339 parsea una fecha RFC 3339; vacio es "no enviada" (nil).
func fechaOpcionalRFC3339(v, nombre string) (*time.Time, error) {
	if v == "" {
		return nil, nil
	}
	t, err := time.Parse(time.RFC3339, v)
	if err != nil {
		return nil, platformerrors.NewValidacion("bff: %s invalido (se espera RFC 3339)", nombre)
	}
	return &t, nil
}

// ReporteEstados delega en reportes.reporteEstados, igual que ReporteViajes.
func (h *Handlers) ReporteEstados(w http.ResponseWriter, r *http.Request) {
	if !h.soloMonitoreo(w, r, "reporteEstados") {
		return
	}
	desde, hasta, err := periodoDe(r)
	if err != nil {
		escribirError(w, err)
		return
	}
	ctx := tokenctx.Con(r.Context(), r.Header.Get("Authorization"))
	rep, err := h.deps.Reportes.ReporteEstados(ctx, repclient.ReporteEstadosParams{Desde: desde, Hasta: hasta})
	if err != nil {
		escribirError(w, err)
		return
	}
	escribirJSON(w, http.StatusOK, rep)
}

// ListarAdjuntos delega en operacion.listarAdjuntos: metadatos de la evidencia,
// sin bytes. Sin paginación en V1 (DT-08).
func (h *Handlers) ListarAdjuntos(w http.ResponseWriter, r *http.Request) {
	id, ok := h.viajeMonitoreo(w, r, "listarAdjuntos")
	if !ok {
		return
	}
	ctx := tokenctx.Con(r.Context(), r.Header.Get("Authorization"))
	pagina, err := h.deps.Operacion.ListarAdjuntos(ctx, id)
	if err != nil {
		escribirError(w, err)
		return
	}
	escribirJSON(w, http.StatusOK, pagina)
}

// CrearIntencionSubidaAdjunto delega en operacion.crearIntencionSubidaAdjunto.
// Los bytes no pasan por bff: devuelve la URL de vida corta del almacenamiento
// de objetos. 201.
func (h *Handlers) CrearIntencionSubidaAdjunto(w http.ResponseWriter, r *http.Request) {
	id, ok := h.viajeMonitoreo(w, r, "crearIntencionSubidaAdjunto")
	if !ok {
		return
	}
	var cuerpo bffclient.IntencionSubidaAdjuntoEntrada
	if err := decodificar(r, &cuerpo); err != nil {
		escribirError(w, err)
		return
	}
	ctx := tokenctx.Con(r.Context(), r.Header.Get("Authorization"))
	intencion, err := h.deps.Operacion.CrearIntencionSubidaAdjunto(ctx, id, opclient.IntencionSubidaAdjuntoEntrada(cuerpo))
	if err != nil {
		escribirError(w, err)
		return
	}
	escribirJSON(w, http.StatusCreated, intencion)
}

// DescargarAdjunto delega en operacion.descargarAdjunto: URL de descarga de vida
// corta. operacion audita cada acceso y rechaza un adjunto no `limpio`.
func (h *Handlers) DescargarAdjunto(w http.ResponseWriter, r *http.Request) {
	id, ok := h.viajeMonitoreo(w, r, "descargarAdjunto")
	if !ok {
		return
	}
	aid, err := uuidDe(r.PathValue("aid"))
	if err != nil {
		escribirError(w, platformerrors.NewValidacion("bff: descargarAdjunto: id de adjunto invalido"))
		return
	}
	ctx := tokenctx.Con(r.Context(), r.Header.Get("Authorization"))
	descarga, err := h.deps.Operacion.DescargarAdjunto(ctx, id, aid)
	if err != nil {
		escribirError(w, err)
		return
	}
	escribirJSON(w, http.StatusOK, descarga)
}
