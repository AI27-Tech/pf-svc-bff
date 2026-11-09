package agregacion

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"

	bffclient "github.com/AI27-Tech/pf-pkg-contracts/packages/api-client-go/bff"
	catclient "github.com/AI27-Tech/pf-pkg-contracts/packages/api-client-go/catalogos"
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

// --- Flujo interno, grupo 2 (E8-T9/T10/T12/T13) ------------------------------
//
// Roster, fecha/hora del servicio, histórico de incidencias, ficha técnica y
// catálogo de GPS. Misma mecánica: audiencia Interna antes de reenviar, vista sin
// recortar, errores por escribirError (409 con codigo incluido).

// ListarUnidades delega en operacion.listarUnidades: UNA página del roster, con
// el cursor opcional del query. Solo jefe_operativo y administrador; el resto
// recibe el 403 de operacion. Agrega el vehículo y los custodios legibles desde
// catalogos (tracer.38, DT-19) para que el panel no muestre UUID.
func (h *Handlers) ListarUnidades(w http.ResponseWriter, r *http.Request) {
	if !h.soloInterno(w, r, "listarUnidades") {
		return
	}
	var params opclient.ListarUnidadesParams
	if c := r.URL.Query().Get("cursor"); c != "" {
		params.Cursor = &c
	}
	bearer := r.Header.Get("Authorization")
	ctx := tokenctx.Con(r.Context(), bearer)
	pagina, err := h.deps.Operacion.ListarUnidades(ctx, params)
	if err != nil {
		escribirError(w, err)
		return
	}
	out := bffclient.PaginaUnidades{CursorSiguiente: pagina.CursorSiguiente}
	if out.Items, err = h.unidadesLegibles(ctx, bearer, pagina.Items); err != nil {
		escribirError(w, err)
		return
	}
	escribirJSON(w, http.StatusOK, out)
}

// unidadesLegibles aplica unidadLegible a cada unidad. Nunca devuelve nil: una
// lista vacía sale como [] en el JSON.
func (h *Handlers) unidadesLegibles(ctx context.Context, bearer string, us []opclient.Unidad) ([]bffclient.Unidad, error) {
	out := make([]bffclient.Unidad, 0, len(us))
	for _, u := range us {
		legible, err := h.unidadLegible(ctx, bearer, u)
		if err != nil {
			return nil, err
		}
		out = append(out, legible)
	}
	return out, nil
}

// unidadLegible copia la unidad de operacion y le agrega el económico, las
// placas y el nombre de los custodios. Un vehículo o custodio que ya no está en
// el catálogo (404) se omite y el panel muestra su id; cualquier otro error
// hace fallar la lista.
//
// shortcut: una lectura a catalogos por vehículo y por custodio de la página
// (N+1, en proceso dentro de core); pasar a una lectura por lote si el roster
// crece más allá de unas decenas de unidades.
func (h *Handlers) unidadLegible(ctx context.Context, bearer string, u opclient.Unidad) (bffclient.Unidad, error) {
	out := bffclient.Unidad{
		Id:           u.Id,
		VehiculoId:   u.VehiculoId,
		CustodiosIds: u.CustodiosIds,
		Activa:       u.Activa,
	}
	if u.Tipo != nil {
		t := bffclient.UnidadTipo(*u.Tipo)
		out.Tipo = &t
	}
	v, err := h.deps.Catalogos.ObtenerVehiculo(ctx, bearer, catclient.Id(u.VehiculoId))
	switch {
	case err == nil:
		out.Vehiculo = &bffclient.VehiculoUnidad{Economico: v.Economico, Placas: v.Placas}
	case platformerrors.AsCategory(err) != platformerrors.CategoryNoEncontrado:
		return out, err
	}
	custodios := make([]bffclient.CustodioUnidad, 0, len(u.CustodiosIds))
	for _, id := range u.CustodiosIds {
		c, err := h.deps.Catalogos.ObtenerCustodio(ctx, bearer, catclient.Id(id))
		switch {
		case err == nil:
			custodios = append(custodios, bffclient.CustodioUnidad{Id: id, Nombre: c.Nombre})
		case platformerrors.AsCategory(err) != platformerrors.CategoryNoEncontrado:
			return out, err
		}
	}
	out.Custodios = &custodios
	return out, nil
}

// CompletarFechaHora delega en operacion.completarFechaHora y, como fijarEta,
// recompone el detalle interno con ObtenerViaje (el contrato devuelve
// ViajeDetalle). Un viaje terminal es 409 viaje_en_estado_terminal.
func (h *Handlers) CompletarFechaHora(w http.ResponseWriter, r *http.Request) {
	id, ok := h.viajeInterno(w, r, "completarFechaHora")
	if !ok {
		return
	}
	var cuerpo bffclient.FechaHoraEntrada
	if err := decodificar(r, &cuerpo); err != nil {
		escribirError(w, err)
		return
	}
	ctx := tokenctx.Con(r.Context(), r.Header.Get("Authorization"))
	if _, err := h.deps.Operacion.CompletarFechaHora(ctx, id, opclient.FechaHoraEntrada(cuerpo)); err != nil {
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

// ListarIncidencias delega en operacion.listarIncidencias: UNA página del
// histórico (en_atencion y resuelta), con tipo_interno crudo.
func (h *Handlers) ListarIncidencias(w http.ResponseWriter, r *http.Request) {
	id, ok := h.viajeInterno(w, r, "listarIncidencias")
	if !ok {
		return
	}
	var params opclient.ListarIncidenciasParams
	if c := r.URL.Query().Get("cursor"); c != "" {
		params.Cursor = &c
	}
	ctx := tokenctx.Con(r.Context(), r.Header.Get("Authorization"))
	pagina, err := h.deps.Operacion.ListarIncidencias(ctx, id, params)
	if err != nil {
		escribirError(w, err)
		return
	}
	escribirJSON(w, http.StatusOK, pagina)
}

// ObtenerFicha delega en operacion.obtenerFicha.
func (h *Handlers) ObtenerFicha(w http.ResponseWriter, r *http.Request) {
	id, ok := h.viajeInterno(w, r, "obtenerFicha")
	if !ok {
		return
	}
	ctx := tokenctx.Con(r.Context(), r.Header.Get("Authorization"))
	ficha, err := h.deps.Operacion.ObtenerFicha(ctx, id)
	if err != nil {
		escribirError(w, err)
		return
	}
	escribirJSON(w, http.StatusOK, ficha)
}

// GuardarFicha delega en operacion.guardarFicha. Replica el handler de
// operacion: un campo desconocido es 400 (additionalProperties: false) y, como el
// tipo generado no distingue `"tracto": null` de la propiedad ausente, el null se
// detecta en el JSON crudo y viaja como &Tracto{} (borrar); ausente queda nil (no
// tocar). Se decodifica directo al tipo de operacion: mismo schema, y así no hay
// que traducir las colecciones anidadas.
func (h *Handlers) GuardarFicha(w http.ResponseWriter, r *http.Request) {
	id, ok := h.viajeInterno(w, r, "guardarFicha")
	if !ok {
		return
	}
	raw, err := io.ReadAll(r.Body)
	if err != nil {
		escribirError(w, platformerrors.NewValidacion("bff: guardarFicha: cuerpo de la peticion invalido: %v", err))
		return
	}
	var cuerpo opclient.FichaTecnicaEntrada
	dec := json.NewDecoder(bytes.NewReader(raw))
	dec.DisallowUnknownFields()
	if err := dec.Decode(&cuerpo); err != nil {
		escribirError(w, platformerrors.NewValidacion("bff: guardarFicha: cuerpo de la peticion invalido: %v", err))
		return
	}
	if cuerpo.Tracto == nil {
		var props map[string]json.RawMessage
		if json.Unmarshal(raw, &props) == nil && string(bytes.TrimSpace(props["tracto"])) == "null" {
			cuerpo.Tracto = &opclient.Tracto{}
		}
	}
	ctx := tokenctx.Con(r.Context(), r.Header.Get("Authorization"))
	ficha, err := h.deps.Operacion.GuardarFicha(ctx, id, cuerpo)
	if err != nil {
		escribirError(w, err)
		return
	}
	escribirJSON(w, http.StatusOK, ficha)
}

// ListarGps delega en catalogos.listarGps: UNA página de dispositivos activos
// para el selector de la ficha, con el cursor opcional del query.
func (h *Handlers) ListarGps(w http.ResponseWriter, r *http.Request) {
	if !h.soloInterno(w, r, "listarGps") {
		return
	}
	var params catclient.ListarGpsParams
	if c := r.URL.Query().Get("cursor"); c != "" {
		params.Cursor = &c
	}
	pagina, err := h.deps.Catalogos.ListarGps(r.Context(), r.Header.Get("Authorization"), params)
	if err != nil {
		escribirError(w, err)
		return
	}
	escribirJSON(w, http.StatusOK, pagina)
}
