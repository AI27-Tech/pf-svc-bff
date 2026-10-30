package agregacion

import (
	"context"
	"encoding/json"
	"net/http"
	"strconv"
	"time"

	bffclient "github.com/AI27-Tech/pf-pkg-contracts/packages/api-client-go/bff"
	catclient "github.com/AI27-Tech/pf-pkg-contracts/packages/api-client-go/catalogos"
	idclient "github.com/AI27-Tech/pf-pkg-contracts/packages/api-client-go/identidad"
	msgclient "github.com/AI27-Tech/pf-pkg-contracts/packages/api-client-go/mensajeria"
	opclient "github.com/AI27-Tech/pf-pkg-contracts/packages/api-client-go/operacion"
	"github.com/AI27-Tech/pf-pkg-platform-go/auth"
	platformerrors "github.com/AI27-Tech/pf-pkg-platform-go/errors"
	"github.com/AI27-Tech/pf-svc-bff/internal/autorizacion"
	"github.com/AI27-Tech/pf-svc-bff/internal/descarga"
	"github.com/AI27-Tech/pf-svc-bff/internal/tokenctx"
	"github.com/AI27-Tech/pf-svc-bff/internal/tracking"
	"github.com/google/uuid"
	openapitypes "github.com/oapi-codegen/runtime/types"
)

// ahora es la fuente de tiempo para el `generado_en` del expediente de descarga.
// Variable de paquete para que la prueba pueda sustituirla por un reloj fijo; en
// produccion es la hora real en UTC.
var ahora = func() time.Time { return time.Now().UTC() }

// Handlers implementa las seis operaciones de la fase (a) de bff.yaml. Cada
// handler usa SOLO tipos generados de pf-pkg-contracts para su cuerpo de
// entrada/salida (regla 6 de 00): bff no define formas de wire a mano.
//
// bff agrega, autoriza y adapta; no guarda (bff.md (a)). Ningun handler orquesta
// una escritura multi-servicio (bff.md (h) R5).
type Handlers struct {
	deps Deps
}

// NewHandlers construye los handlers con los puertos ya resueltos.
func NewHandlers(d Deps) *Handlers {
	return &Handlers{deps: d}
}

// IniciarSesion delega en identidad.emitirSesion. Ruta publica: bff NO valida
// credenciales, solo reenvia (bff.yaml).
func (h *Handlers) IniciarSesion(w http.ResponseWriter, r *http.Request) {
	var cuerpo bffclient.CredencialesEntrada
	if err := decodificar(r, &cuerpo); err != nil {
		escribirError(w, err)
		return
	}
	// El contrato de bff y el de identidad comparten la forma de credenciales;
	// se traduce explicitamente para no acoplar los dos paquetes generados.
	sesion, err := h.deps.Identidad.EmitirSesion(r.Context(), idclient.CredencialesEntrada{
		Usuario:       cuerpo.Usuario,
		Contrasena:    cuerpo.Contrasena,
		SegundoFactor: cuerpo.SegundoFactor,
	})
	if err != nil {
		escribirError(w, err)
		return
	}
	escribirJSON(w, http.StatusCreated, aSesionBff(sesion))
}

// RefrescarSesion delega en identidad.refrescarSesion. Es el punto de
// reevaluacion de claims (ADR 26). Ruta publica (el refresh token es la prueba).
func (h *Handlers) RefrescarSesion(w http.ResponseWriter, r *http.Request) {
	var cuerpo bffclient.RefrescoEntrada
	if err := decodificar(r, &cuerpo); err != nil {
		escribirError(w, err)
		return
	}
	sesion, err := h.deps.Identidad.RefrescarSesion(r.Context(), idclient.RefrescoEntrada{
		RefreshToken: cuerpo.RefreshToken,
	})
	if err != nil {
		escribirError(w, err)
		return
	}
	escribirJSON(w, http.StatusOK, aSesionBff(sesion))
}

// CerrarSesion delega en identidad.revocarSesion. Ruta protegida: el sesion_id a
// revocar sale de las claims verificadas, nunca de un parametro (P10). identidad
// publica sesion.revocada.
func (h *Handlers) CerrarSesion(w http.ResponseWriter, r *http.Request) {
	claims := autorizacion.ClaimsDe(r.Context())
	if claims == nil {
		escribirError(w, platformerrors.NewNoAutorizado("bff: cerrarSesion: sin claims"))
		return
	}
	id, err := uuidDe(claims.SesionID)
	if err != nil {
		escribirError(w, platformerrors.NewNoAutorizado("bff: cerrarSesion: sesion_id invalido en el token"))
		return
	}
	if err := h.deps.Identidad.RevocarSesion(tokenctx.Con(r.Context(), r.Header.Get("Authorization")), id); err != nil {
		escribirError(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// CrearViaje delega en operacion.crearViaje. El estado inicial lo fija el rol del
// token en operacion (DEC-026); bff reenvia, no decide. Ruta protegida.
func (h *Handlers) CrearViaje(w http.ResponseWriter, r *http.Request) {
	var cuerpo bffclient.CrearViajeEntrada
	if err := decodificar(r, &cuerpo); err != nil {
		escribirError(w, err)
		return
	}
	ctx := tokenctx.Con(r.Context(), r.Header.Get("Authorization"))
	viaje, err := h.deps.Operacion.CrearViaje(ctx, aCrearViajeOperacion(cuerpo))
	if err != nil {
		escribirError(w, err)
		return
	}
	escribirJSON(w, http.StatusCreated, viaje)
}

// ListarViajes delega en operacion.listarViajes. El tenant y el rol salen del
// token (P9, P10); el alcance por unidad/cliente se resuelve en servidor, no
// viaja en el token (DEC-053). Ruta protegida.
func (h *Handlers) ListarViajes(w http.ResponseWriter, r *http.Request) {
	// En fase (a) solo se reenvia el cursor; los filtros de alcance los aplica
	// operacion a partir del contexto autenticado que propaga el adaptador.
	var params opclient.ListarViajesParams
	if cursor := r.URL.Query().Get("cursor"); cursor != "" {
		c := opclient.Cursor(cursor)
		params.Cursor = &c
	}
	ctx := tokenctx.Con(r.Context(), r.Header.Get("Authorization"))
	pagina, err := h.deps.Operacion.ListarViajes(ctx, params)
	if err != nil {
		escribirError(w, err)
		return
	}
	escribirJSON(w, http.StatusOK, pagina)
}

// ListarClientes delega en catalogos.listarClientes para poblar el selector de
// cliente al crear un viaje por Operación (E8-T11). Reenvía el cursor y propaga
// el Bearer; catalogos devuelve SOLO clientes activos (DEC-101 #1). El filtro de
// alcance del jefe_operativo (DEC-026 #3) todavía NO se aplica aquí (pendiente,
// DEC-101 #2). La respuesta se traduce al tipo de bff (espejo de catalogos,
// paquetes generados distintos). Ruta protegida.
func (h *Handlers) ListarClientes(w http.ResponseWriter, r *http.Request) {
	var params catclient.ListarClientesParams
	if cursor := r.URL.Query().Get("cursor"); cursor != "" {
		c := catclient.Cursor(cursor)
		params.Cursor = &c
	}
	pagina, err := h.deps.Catalogos.ListarClientes(r.Context(), r.Header.Get("Authorization"), params)
	if err != nil {
		escribirError(w, err)
		return
	}
	escribirJSON(w, http.StatusOK, aClientesBff(pagina))
}

// SeguirViaje delega en operacion.obtenerViaje y compone los hitos V1 por
// polling (ADR 27). El cliente recibe el hito traducido, nunca el token tecnico
// (E2-T2). Ruta protegida.
func (h *Handlers) SeguirViaje(w http.ResponseWriter, r *http.Request) {
	id, err := uuidDe(r.PathValue("id"))
	if err != nil {
		escribirError(w, platformerrors.NewValidacion("bff: seguirViaje: id invalido"))
		return
	}
	ctx := tokenctx.Con(r.Context(), r.Header.Get("Authorization"))
	detalle, err := h.deps.Operacion.ObtenerViaje(ctx, id)
	if err != nil {
		escribirError(w, err)
		return
	}
	// Composicion pura de hitos (E2-T2/E2-T3). Traduce estado tecnico -> hito y
	// colapsa el bucle multi-parada; el cliente nunca ve el token tecnico.
	escribirJSON(w, http.StatusOK, tracking.Componer(*detalle))
}

// TransicionarViajeCliente delega en operacion.transicionarViaje la única acción
// de escritura del cliente sobre el ciclo de vida del viaje: cancelar o rechazar
// (E9-T7 / DEC-111). Solo audiencia Cliente: un actor interno recibe 403 antes de
// reenviar. operacion es la única autoridad de transición y aplica el aislamiento
// por tenant (el cliente solo su viaje, DEC-112; viaje ajeno -> 404) y la facultad
// (solo CANCELADO/RECHAZADO_POR_CLIENTE) más el motivo obligatorio (DEC-111). El
// bff solo filtra audiencia y reenvía; no replica el grafo. Devuelve la vista de
// seguimiento del cliente (como seguirViaje), nunca el detalle interno (DEC-006):
// operacion.transicionarViaje responde el Viaje plano, así que se recompone la
// vista leyendo el detalle con ObtenerViaje. Ruta protegida.
func (h *Handlers) TransicionarViajeCliente(w http.ResponseWriter, r *http.Request) {
	// Audiencia Cliente ANTES de reenviar: esta superficie es del portal cliente.
	// Un actor interno transiciona por la superficie interna, no por aquí.
	claims := autorizacion.ClaimsDe(r.Context())
	if claims == nil {
		escribirError(w, platformerrors.NewNoAutorizado("bff: transicionarViajeCliente: sin claims"))
		return
	}
	if claims.Audiencia != auth.AudienciaCliente {
		escribirError(w, platformerrors.NewProhibido("bff: transicionarViajeCliente: solo para el portal del cliente"))
		return
	}

	id, err := uuidDe(r.PathValue("id"))
	if err != nil {
		escribirError(w, platformerrors.NewValidacion("bff: transicionarViajeCliente: id invalido"))
		return
	}

	var cuerpo bffclient.TransicionEntrada
	if err := decodificar(r, &cuerpo); err != nil {
		escribirError(w, err)
		return
	}

	ctx := tokenctx.Con(r.Context(), r.Header.Get("Authorization"))
	// operacion valida arista, aislamiento, facultad y motivo. Su 400/401/403/404/
	// 409 se propaga tal cual. El Viaje devuelto se descarta: la respuesta al
	// cliente es la vista de seguimiento recompuesta.
	if _, err := h.deps.Operacion.TransicionarViaje(ctx, id, aTransicionOperacion(cuerpo)); err != nil {
		escribirError(w, err)
		return
	}
	// Recompone la vista de cliente tras aplicar la transición, sin exponer el
	// estado canónico ni las condiciones (DEC-006), igual que seguirViaje.
	detalle, err := h.deps.Operacion.ObtenerViaje(ctx, id)
	if err != nil {
		escribirError(w, err)
		return
	}
	escribirJSON(w, http.StatusOK, tracking.Componer(*detalle))
}

// --- Descarga del expediente de UN viaje (E6-T3 / E6-HU-026, DEC-022 #7) ----

// DescargarViaje compone el expediente de descarga de UN viaje leyendo de
// operacion (la fuente exacta y actual), NUNCA de reportes. Solo usuarios
// internos (DEC-022 #7): un cliente recibe 403 ANTES de componer dato alguno. El
// alcance fino lo resuelve operacion; su 403/404 se propaga tal cual. Tras
// componer con exito, pide a operacion que DEJE el rastro de auditoria de la
// descarga (exactamente una vez por descarga exitosa); bff no escribe auditoria
// (no tiene base de datos). Ruta protegida.
//
// Exclusiones duras (DEC-009/DEC-039): el expediente no contiene cuenta espejo
// ni clave_amago (no existen en el origen ni en el contrato). Es descarga de UN
// viaje; no hay ruta de lote (V2, DEC-023).
func (h *Handlers) DescargarViaje(w http.ResponseWriter, r *http.Request) {
	// 1. Autorizacion por audiencia ANTES de componer nada: la descarga es solo
	//    para usuarios internos (DEC-022 #7). Un cliente -> 403 (Prohibido).
	claims := autorizacion.ClaimsDe(r.Context())
	if claims == nil {
		escribirError(w, platformerrors.NewNoAutorizado("bff: descargarViaje: sin claims"))
		return
	}
	if claims.Audiencia != auth.AudienciaInterno {
		escribirError(w, platformerrors.NewProhibido("bff: descargarViaje: la descarga del expediente es solo para usuarios internos"))
		return
	}

	id, err := uuidDe(r.PathValue("id"))
	if err != nil {
		escribirError(w, platformerrors.NewValidacion("bff: descargarViaje: id invalido"))
		return
	}

	// El Bearer del usuario viaja en el ctx para que el adaptador HTTP lo
	// propague a operacion: registrarDescargaViaje deja el actor del rastro de
	// auditoria a partir de claims.Subject (E1-T7), que sin el token quedaria
	// vacio. El adaptador en proceso de core ignora el ctx-bearer (ya tiene los
	// claims en el ctx).
	ctx := tokenctx.Con(r.Context(), r.Header.Get("Authorization"))

	// 2. Composicion: ficha + bitacora + auditoria, leyendo de operacion. El
	//    alcance fino lo resuelve operacion: cualquier 403 (fuera de alcance) o
	//    404 (viaje inexistente) se propaga tal cual, sin componer el resto.
	detalle, err := h.deps.Operacion.ObtenerViaje(ctx, id)
	if err != nil {
		escribirError(w, err)
		return
	}
	bitacora, err := h.bitacoraCompleta(ctx, id)
	if err != nil {
		escribirError(w, err)
		return
	}
	auditoria, err := h.auditoriaCompleta(ctx, id)
	if err != nil {
		escribirError(w, err)
		return
	}

	expediente := descarga.Componer(*detalle, bitacora, auditoria, ahora())

	// 3. Rastro de auditoria: tras componer con exito, operacion deja el registro
	//    de la descarga (actor, viaje, momento; E1-T7) en SU transaccion.
	//    Exactamente una vez por descarga exitosa. Si operacion falla al
	//    registrar, la descarga no se entrega: no se sirve un expediente cuya
	//    descarga no quedo auditada.
	if _, err := h.deps.Operacion.RegistrarDescargaViaje(ctx, id); err != nil {
		escribirError(w, err)
		return
	}

	escribirJSON(w, http.StatusOK, expediente)
}

// bitacoraCompleta agota la paginacion de la bitacora del viaje: pide pagina a
// pagina hasta que operacion no devuelve cursor siguiente. El expediente lleva
// el historico completo, no una pagina (E6-T3). Nunca devuelve nil.
func (h *Handlers) bitacoraCompleta(ctx context.Context, id opclient.Id) ([]opclient.EntradaBitacora, error) {
	todas := []opclient.EntradaBitacora{}
	var cursor *opclient.Cursor
	for {
		pagina, err := h.deps.Operacion.ListarBitacora(ctx, id, opclient.ListarBitacoraParams{Cursor: cursor})
		if err != nil {
			return nil, err
		}
		if pagina == nil {
			break
		}
		todas = append(todas, pagina.Items...)
		if pagina.CursorSiguiente == nil || *pagina.CursorSiguiente == "" {
			break
		}
		sig := opclient.Cursor(*pagina.CursorSiguiente)
		cursor = &sig
	}
	return todas, nil
}

// auditoriaCompleta agota la paginacion de la auditoria del viaje, igual que
// bitacoraCompleta. Nunca devuelve nil.
func (h *Handlers) auditoriaCompleta(ctx context.Context, id opclient.Id) ([]opclient.RegistroAuditoria, error) {
	todos := []opclient.RegistroAuditoria{}
	var cursor *opclient.Cursor
	for {
		pagina, err := h.deps.Operacion.ConsultarAuditoria(ctx, id, opclient.ConsultarAuditoriaParams{Cursor: cursor})
		if err != nil {
			return nil, err
		}
		if pagina == nil {
			break
		}
		todos = append(todos, pagina.Items...)
		if pagina.CursorSiguiente == nil || *pagina.CursorSiguiente == "" {
			break
		}
		sig := opclient.Cursor(*pagina.CursorSiguiente)
		cursor = &sig
	}
	return todos, nil
}

// --- Emergencia del portal cliente (E7/tracer.17, ADR 21 §6) --------------
// bff reenvía a mensajeria propagando el Bearer del usuario; mensajeria autoriza
// por los claims y proyecta la vista mínima del cliente. bff traduce la vista
// interna del caso a la vista cliente (sin origen, actor ni reclamo).

// ListarEmergenciasCliente delega en mensajeria.listarEmergencias y devuelve la
// vista mínima: el indicador agregado "emergencia activa" y los casos por
// fecha/hora. Ruta protegida.
func (h *Handlers) ListarEmergenciasCliente(w http.ResponseWriter, r *http.Request) {
	id, err := uuidDe(r.PathValue("id"))
	if err != nil {
		escribirError(w, platformerrors.NewValidacion("bff: listarEmergencias: id invalido"))
		return
	}
	pagina, err := h.deps.Mensajeria.ListarEmergencias(r.Context(), r.Header.Get("Authorization"), id)
	if err != nil {
		escribirError(w, err)
		return
	}
	escribirJSON(w, http.StatusOK, aEmergenciasCliente(pagina))
}

// ActivarEmergenciaCliente delega en mensajeria.activarEmergencia (idempotente
// por activacion_id). Devuelve 201 (nuevo) o 200 (reentrega) con la vista
// cliente del caso. Ruta protegida.
func (h *Handlers) ActivarEmergenciaCliente(w http.ResponseWriter, r *http.Request) {
	id, err := uuidDe(r.PathValue("id"))
	if err != nil {
		escribirError(w, platformerrors.NewValidacion("bff: activarEmergencia: id invalido"))
		return
	}
	var cuerpo bffclient.ActivarEmergenciaClienteEntrada
	if err := decodificar(r, &cuerpo); err != nil {
		escribirError(w, err)
		return
	}
	caso, err := h.deps.Mensajeria.ActivarEmergencia(r.Context(), r.Header.Get("Authorization"), id,
		msgclient.ActivarEmergenciaEntrada{ActivacionId: cuerpo.ActivacionId})
	if err != nil {
		escribirError(w, err)
		return
	}
	escribirJSON(w, http.StatusCreated, aEmergenciaCliente(caso))
}

// AgregarNotaEmergenciaCliente delega en mensajeria.agregarNotaEmergencia. La
// respuesta del portal cliente no lleva cuerpo: 201 al agregar. Ruta protegida.
func (h *Handlers) AgregarNotaEmergenciaCliente(w http.ResponseWriter, r *http.Request) {
	id, err := uuidDe(r.PathValue("id"))
	if err != nil {
		escribirError(w, platformerrors.NewValidacion("bff: agregarNota: id de viaje invalido"))
		return
	}
	eid, err := uuidDe(r.PathValue("eid"))
	if err != nil {
		escribirError(w, platformerrors.NewValidacion("bff: agregarNota: id de emergencia invalido"))
		return
	}
	var cuerpo bffclient.NotaEmergenciaClienteEntrada
	if err := decodificar(r, &cuerpo); err != nil {
		escribirError(w, err)
		return
	}
	if err := h.deps.Mensajeria.AgregarNotaEmergencia(r.Context(), r.Header.Get("Authorization"), id, eid,
		msgclient.NotaEmergenciaEntrada{Cuerpo: cuerpo.Cuerpo}); err != nil {
		escribirError(w, err)
		return
	}
	w.WriteHeader(http.StatusCreated)
}

// --- Conversación del viaje para el portal cliente (E9-T8, DEC-029) --------
// La conversación se identifica por el viaje (DEC-029): el {id} de la ruta es el
// viaje. Solo audiencia Cliente (un actor interno usa la superficie interna de
// mensajería, no esta). bff propaga el Bearer; mensajeria autoriza por los claims
// y devuelve el Conversacion/Mensaje completos; el bff recorta a la vista de
// cliente (sin actor_real, rol, bandeja, reclamo), igual que emergencias.

// soloCliente exige audiencia Cliente antes de reenviar. Devuelve false y ya
// escribió el error (401 sin claims, 403 si es interno) cuando no procede.
func (h *Handlers) soloCliente(w http.ResponseWriter, r *http.Request, op string) bool {
	claims := autorizacion.ClaimsDe(r.Context())
	if claims == nil {
		escribirError(w, platformerrors.NewNoAutorizado("bff: %s: sin claims", op))
		return false
	}
	if claims.Audiencia != auth.AudienciaCliente {
		escribirError(w, platformerrors.NewProhibido("bff: %s: solo para el portal del cliente", op))
		return false
	}
	return true
}

// ObtenerConversacionViaje delega en mensajeria.obtenerConversacion (por viaje) y
// recorta a la vista de cliente. Ruta protegida, solo audiencia Cliente.
func (h *Handlers) ObtenerConversacionViaje(w http.ResponseWriter, r *http.Request) {
	if !h.soloCliente(w, r, "obtenerConversacionViaje") {
		return
	}
	id, err := uuidDe(r.PathValue("id"))
	if err != nil {
		escribirError(w, platformerrors.NewValidacion("bff: obtenerConversacionViaje: id invalido"))
		return
	}
	conv, err := h.deps.Mensajeria.ObtenerConversacion(r.Context(), r.Header.Get("Authorization"), id)
	if err != nil {
		escribirError(w, err)
		return
	}
	escribirJSON(w, http.StatusOK, aConversacionCliente(conv))
}

// ListarMensajesViaje delega en mensajeria.listarMensajes. Pagina por secuencia
// (antes_de/despues_de/limite); reenvía los params tal cual y recorta cada
// mensaje a la vista de cliente. Ruta protegida, solo audiencia Cliente.
func (h *Handlers) ListarMensajesViaje(w http.ResponseWriter, r *http.Request) {
	if !h.soloCliente(w, r, "listarMensajesViaje") {
		return
	}
	id, err := uuidDe(r.PathValue("id"))
	if err != nil {
		escribirError(w, platformerrors.NewValidacion("bff: listarMensajesViaje: id invalido"))
		return
	}
	params, err := mensajesParamsDe(r)
	if err != nil {
		escribirError(w, err)
		return
	}
	pagina, err := h.deps.Mensajeria.ListarMensajes(r.Context(), r.Header.Get("Authorization"), id, params)
	if err != nil {
		escribirError(w, err)
		return
	}
	escribirJSON(w, http.StatusOK, aPaginaMensajesCliente(pagina))
}

// EnviarMensajeViaje delega en mensajeria.enviarMensaje (idempotente por
// client_message_id). Devuelve 201 (nuevo) o 200 (reintento) con la vista de
// cliente del mensaje; el 409 solo_lectura de mensajeria se propaga. Ruta
// protegida, solo audiencia Cliente.
func (h *Handlers) EnviarMensajeViaje(w http.ResponseWriter, r *http.Request) {
	if !h.soloCliente(w, r, "enviarMensajeViaje") {
		return
	}
	id, err := uuidDe(r.PathValue("id"))
	if err != nil {
		escribirError(w, platformerrors.NewValidacion("bff: enviarMensajeViaje: id invalido"))
		return
	}
	var cuerpo bffclient.EnviarMensajeClienteEntrada
	if err := decodificar(r, &cuerpo); err != nil {
		escribirError(w, err)
		return
	}
	msg, err := h.deps.Mensajeria.EnviarMensaje(r.Context(), r.Header.Get("Authorization"), id,
		msgclient.EnviarMensajeEntrada{
			ClientMessageId: cuerpo.ClientMessageId,
			Cuerpo:          cuerpo.Cuerpo,
		})
	if err != nil {
		escribirError(w, err)
		return
	}
	// shortcut: 201 también en el reintento idempotente (mensajeria distingue 200);
	// actualizar si el frontend llega a ramificar sobre 200 vs 201. El cuerpo es
	// el mismo Mensaje en ambos casos, así que el cliente lo trata igual.
	escribirJSON(w, http.StatusCreated, aMensajeCliente(msg))
}

// mensajesParamsDe lee la paginación por secuencia (antes_de/despues_de/limite).
// Ausentes = sin filtro (mensajeria devuelve los más recientes). Mal formados =
// error de validación. No valida la exclusión mutua antes_de/despues_de: eso lo
// hace mensajeria, única autoridad de la historia.
func mensajesParamsDe(r *http.Request) (msgclient.ListarMensajesParams, error) {
	var p msgclient.ListarMensajesParams
	q := r.URL.Query()
	if s := q.Get("antes_de"); s != "" {
		v, err := strconv.ParseInt(s, 10, 64)
		if err != nil {
			return p, platformerrors.NewValidacion("bff: listarMensajesViaje: antes_de invalido: %v", err)
		}
		p.AntesDe = &v
	}
	if s := q.Get("despues_de"); s != "" {
		v, err := strconv.ParseInt(s, 10, 64)
		if err != nil {
			return p, platformerrors.NewValidacion("bff: listarMensajesViaje: despues_de invalido: %v", err)
		}
		p.DespuesDe = &v
	}
	if s := q.Get("limite"); s != "" {
		v, err := strconv.Atoi(s)
		if err != nil {
			return p, platformerrors.NewValidacion("bff: listarMensajesViaje: limite invalido: %v", err)
		}
		p.Limite = &v
	}
	return p, nil
}

// aConversacionCliente recorta el Conversacion de mensajeria a la vista de
// cliente: viaje_id, estado y atendida (derivada de atendida_en != null,
// DEC-014). Descarta bandeja, reclamo, pool, secuencias y cliente_id.
func aConversacionCliente(c *msgclient.Conversacion) bffclient.ConversacionCliente {
	if c == nil {
		return bffclient.ConversacionCliente{}
	}
	atendida := c.AtendidaEn != nil
	return bffclient.ConversacionCliente{
		ViajeId:  c.ViajeId,
		Estado:   bffclient.ConversacionClienteEstado(c.Estado),
		Atendida: &atendida,
	}
}

// aMensajeCliente recorta el Mensaje de mensajeria a la vista de cliente. NUNCA
// expone actor_real ni rol interno: de_cliente deriva de la audiencia del autor
// (Cliente = lo escribió el cliente; Interno = AI27, "AI27 Operations", DEC-021).
func aMensajeCliente(m *msgclient.Mensaje) bffclient.MensajeCliente {
	if m == nil {
		return bffclient.MensajeCliente{}
	}
	return bffclient.MensajeCliente{
		Id:        m.Id,
		Secuencia: m.Secuencia,
		Cuerpo:    m.Cuerpo,
		EnviadoEn: m.CreadoEn,
		DeCliente: m.AudienciaAutor == msgclient.AudienciaCliente,
	}
}

// aPaginaMensajesCliente recorta la PaginaMensajes a la vista de cliente,
// conservando hay_mas (paginación por secuencia). Items nunca nil.
func aPaginaMensajesCliente(p *msgclient.PaginaMensajes) bffclient.PaginaMensajesCliente {
	out := bffclient.PaginaMensajesCliente{Items: []bffclient.MensajeCliente{}}
	if p == nil {
		return out
	}
	out.HayMas = p.HayMas
	for _, m := range p.Items {
		mm := m
		out.Items = append(out.Items, aMensajeCliente(&mm))
	}
	return out
}

// --- Atención interna del caso (panel de Reacción, E7/tracer.17) -----------
// El panel interno (Reacción, supervisor, coordinador) lista y atiende el caso.
// bff propaga el Bearer y reenvía a mensajeria, que autoriza por los claims
// (PuedeAtenderEmergencia, ADR 21 §6) y proyecta la vista interna completa. A
// diferencia del portal cliente, estos handlers NO traducen la respuesta: la
// vista interna es la del contrato de mensajeria sin recortar. El contrato de
// bff solo define la vista cliente; estas rutas reenvían tipos de mensajeria.

// versionEsperadaDe lee el query param version_esperada (concurrencia optimista
// del reclamo). Ausente o vacío = 0 ("nunca reclamado"); mal formado = error.
func versionEsperadaDe(r *http.Request) (msgclient.VersionReclamo, error) {
	s := r.URL.Query().Get("version_esperada")
	if s == "" {
		return 0, nil
	}
	v, err := strconv.ParseInt(s, 10, 64)
	if err != nil {
		return 0, platformerrors.NewValidacion("bff: version_esperada invalida: %v", err)
	}
	return v, nil
}

// ListarEmergenciasPanel delega en mensajeria.listarEmergencias y reenvía la
// vista interna completa (origen, actor, reclamo). mensajeria proyecta según la
// audiencia del token. Ruta protegida del panel interno.
func (h *Handlers) ListarEmergenciasPanel(w http.ResponseWriter, r *http.Request) {
	id, err := uuidDe(r.PathValue("id"))
	if err != nil {
		escribirError(w, platformerrors.NewValidacion("bff: listarEmergenciasPanel: id invalido"))
		return
	}
	pagina, err := h.deps.Mensajeria.ListarEmergencias(r.Context(), r.Header.Get("Authorization"), id)
	if err != nil {
		escribirError(w, err)
		return
	}
	escribirJSON(w, http.StatusOK, pagina)
}

// ReclamarEmergenciaPanel delega en mensajeria.reclamarEmergencia. El operador
// del token toma el caso; devuelve el reclamo (201). Ruta protegida.
func (h *Handlers) ReclamarEmergenciaPanel(w http.ResponseWriter, r *http.Request) {
	id, eid, err := viajeYEmergenciaDe(r, "reclamar")
	if err != nil {
		escribirError(w, err)
		return
	}
	var cuerpo msgclient.ReclamarEmergenciaJSONRequestBody
	if err := decodificar(r, &cuerpo); err != nil {
		escribirError(w, err)
		return
	}
	reclamo, err := h.deps.Mensajeria.ReclamarEmergencia(r.Context(), r.Header.Get("Authorization"), id, eid, cuerpo)
	if err != nil {
		escribirError(w, err)
		return
	}
	escribirJSON(w, http.StatusCreated, reclamo)
}

// LiberarReclamoEmergenciaPanel delega en mensajeria.liberarReclamoEmergencia.
// La version_esperada va como query. Sin cuerpo de respuesta: 204. Ruta protegida.
func (h *Handlers) LiberarReclamoEmergenciaPanel(w http.ResponseWriter, r *http.Request) {
	id, eid, err := viajeYEmergenciaDe(r, "liberar")
	if err != nil {
		escribirError(w, err)
		return
	}
	rid, err := uuidDe(r.PathValue("rid"))
	if err != nil {
		escribirError(w, platformerrors.NewValidacion("bff: liberar: id de reclamo invalido"))
		return
	}
	version, err := versionEsperadaDe(r)
	if err != nil {
		escribirError(w, err)
		return
	}
	if err := h.deps.Mensajeria.LiberarReclamoEmergencia(r.Context(), r.Header.Get("Authorization"), id, eid, rid,
		msgclient.LiberarReclamoEmergenciaParams{VersionEsperada: version}); err != nil {
		escribirError(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// TransferirEmergenciaPanel delega en mensajeria.transferirEmergencia. Devuelve
// el reclamo resultante (201). Ruta protegida.
func (h *Handlers) TransferirEmergenciaPanel(w http.ResponseWriter, r *http.Request) {
	id, eid, err := viajeYEmergenciaDe(r, "transferir")
	if err != nil {
		escribirError(w, err)
		return
	}
	var cuerpo msgclient.TransferirEmergenciaJSONRequestBody
	if err := decodificar(r, &cuerpo); err != nil {
		escribirError(w, err)
		return
	}
	reclamo, err := h.deps.Mensajeria.TransferirEmergencia(r.Context(), r.Header.Get("Authorization"), id, eid, cuerpo)
	if err != nil {
		escribirError(w, err)
		return
	}
	escribirJSON(w, http.StatusCreated, reclamo)
}

// ResolverEmergenciaPanel delega en mensajeria.resolverEmergencia (nota
// obligatoria). Devuelve el caso actualizado (200). Ruta protegida.
func (h *Handlers) ResolverEmergenciaPanel(w http.ResponseWriter, r *http.Request) {
	id, eid, err := viajeYEmergenciaDe(r, "resolver")
	if err != nil {
		escribirError(w, err)
		return
	}
	var cuerpo msgclient.ResolverEmergenciaJSONRequestBody
	if err := decodificar(r, &cuerpo); err != nil {
		escribirError(w, err)
		return
	}
	caso, err := h.deps.Mensajeria.ResolverEmergencia(r.Context(), r.Header.Get("Authorization"), id, eid, cuerpo)
	if err != nil {
		escribirError(w, err)
		return
	}
	escribirJSON(w, http.StatusOK, caso)
}

// CerrarEmergenciaPanel delega en mensajeria.cerrarEmergencia (sin cuerpo).
// Devuelve el caso cerrado (200). Ruta protegida.
func (h *Handlers) CerrarEmergenciaPanel(w http.ResponseWriter, r *http.Request) {
	id, eid, err := viajeYEmergenciaDe(r, "cerrar")
	if err != nil {
		escribirError(w, err)
		return
	}
	caso, err := h.deps.Mensajeria.CerrarEmergencia(r.Context(), r.Header.Get("Authorization"), id, eid)
	if err != nil {
		escribirError(w, err)
		return
	}
	escribirJSON(w, http.StatusOK, caso)
}

// viajeYEmergenciaDe parsea los path values {id} (viaje) y {eid} (emergencia)
// comunes a las acciones del panel. op nombra la acción para el mensaje de error.
func viajeYEmergenciaDe(r *http.Request, op string) (msgclient.Id, msgclient.Id, error) {
	id, err := uuidDe(r.PathValue("id"))
	if err != nil {
		return msgclient.Id{}, msgclient.Id{}, platformerrors.NewValidacion("bff: %s: id de viaje invalido", op)
	}
	eid, err := uuidDe(r.PathValue("eid"))
	if err != nil {
		return msgclient.Id{}, msgclient.Id{}, platformerrors.NewValidacion("bff: %s: id de emergencia invalido", op)
	}
	return id, eid, nil
}

// aEmergenciaCliente traduce la vista interna del caso (CasoEmergencia) a la
// vista cliente (EmergenciaCliente): solo id, estado y fecha. Descarta origen,
// actor y reclamo (solo-Interno, ADR 21 §6).
func aEmergenciaCliente(c *msgclient.CasoEmergencia) bffclient.EmergenciaCliente {
	if c == nil {
		return bffclient.EmergenciaCliente{}
	}
	return bffclient.EmergenciaCliente{
		Id:         c.Id,
		Estado:     bffclient.EmergenciaClienteEstado(c.Estado),
		ActivadaEn: c.ActivadaEn,
	}
}

// aEmergenciasCliente arma la vista mínima de la lista: el indicador agregado
// (true si hay al menos un caso en abierta o en_atencion, ADR 21 §6) y los casos
// traducidos. mensajeria ya excluye las cerradas para el cliente.
func aEmergenciasCliente(p *msgclient.PaginaEmergencias) bffclient.EmergenciasCliente {
	out := bffclient.EmergenciasCliente{Casos: []bffclient.EmergenciaCliente{}}
	if p == nil {
		return out
	}
	for _, c := range p.Items {
		cc := c
		out.Casos = append(out.Casos, aEmergenciaCliente(&cc))
		if c.Estado == msgclient.EstadoEmergenciaAbierta || c.Estado == msgclient.EstadoEmergenciaEnAtencion {
			out.EmergenciaActiva = true
		}
	}
	return out
}

// aClientesBff traduce la PaginaClientes de catalogos a la de bff. Ambas
// comparten forma (espejo), pero son tipos de paquetes generados distintos; la
// traduccion es explicita para no acoplarlos. Items nunca nil (lista vacía como
// forma estable).
func aClientesBff(p *catclient.PaginaClientes) bffclient.PaginaClientes {
	out := bffclient.PaginaClientes{Items: []bffclient.Cliente{}}
	if p == nil {
		return out
	}
	out.CursorSiguiente = p.CursorSiguiente
	for _, c := range p.Items {
		out.Items = append(out.Items, bffclient.Cliente{
			Id:                c.Id,
			Nombre:            c.Nombre,
			Estado:            bffclient.ClienteEstado(c.Estado),
			ContratoVigenteId: c.ContratoVigenteId,
		})
	}
	return out
}

// aSesionBff traduce la SesionEmitida de identidad a la Sesion de bff. Ambas
// comparten forma, pero son tipos de paquetes generados distintos; la traduccion
// es explicita para no acoplarlos.
func aSesionBff(s *idclient.SesionEmitida) bffclient.Sesion {
	if s == nil {
		return bffclient.Sesion{}
	}
	return bffclient.Sesion{
		SesionId:     s.SesionId,
		AccessToken:  s.AccessToken,
		ExpiraEn:     s.ExpiraEn,
		RefreshToken: s.RefreshToken,
	}
}

// aCrearViajeOperacion traduce la entrada de bff a la de operacion. bff reenvia
// origen, destinos y datos opcionales; NO fija asignacion (bff.md (f)).
func aCrearViajeOperacion(e bffclient.CrearViajeEntrada) opclient.CrearViajeEntrada {
	destinos := make([]opclient.Destino, 0, len(e.Destinos))
	for _, d := range e.Destinos {
		destinos = append(destinos, opclient.Destino{
			Nombre:      d.Nombre,
			NumeroEnvio: d.NumeroEnvio,
		})
	}
	return opclient.CrearViajeEntrada{
		ClienteId:         e.ClienteId,
		Origen:            e.Origen,
		Destinos:          destinos,
		FechaHoraServicio: e.FechaHoraServicio,
		TipoServicio:      e.TipoServicio,
	}
}

// aTransicionOperacion traduce la TransicionEntrada de bff a la de operacion.
// Difieren solo en el tipo de `a_estado`: string libre en bff (el bff no replica
// el enum, DEC-104), EstadoViaje tipado en operacion. La validación de la arista
// y del motivo vive en operacion, no aquí.
func aTransicionOperacion(e bffclient.TransicionEntrada) opclient.TransicionEntrada {
	return opclient.TransicionEntrada{
		AEstado: opclient.EstadoViaje(e.AEstado),
		Causa:   e.Causa,
	}
}

// uuidDe parsea un UUID de dominio (Id del contrato = openapi_types.UUID, que a
// su vez es un alias de uuid.UUID de google/uuid).
func uuidDe(s string) (openapitypes.UUID, error) {
	return uuid.Parse(s)
}

// decodificar lee el cuerpo JSON de la peticion en v, traduciendo un cuerpo
// invalido a un error de validacion de la plataforma.
func decodificar(r *http.Request, v any) error {
	if err := json.NewDecoder(r.Body).Decode(v); err != nil {
		return platformerrors.NewValidacion("bff: cuerpo de la peticion invalido: %v", err)
	}
	return nil
}

// escribirJSON serializa v como respuesta JSON con el status dado.
func escribirJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

// escribirError traduce un error de la taxonomia de la plataforma al Error de
// bff.yaml y al status HTTP correspondiente. Reusa el mapeo de la plataforma
// (Category.HTTPStatus y CodigoDe) en vez de duplicarlo. Un aguas-abajo caido es
// degradado o roto, nunca no encontrado (bff.md (d)).
//
// El 403 (Prohibido) y el codigo especifico de un conflicto ya viven en la
// taxonomia de la plataforma (Ola C): CodigoDe emite el codigo del error si lo
// trae, si no el generico de la categoria. No hay caso especial que reconocer
// antes del mapeo general.
func escribirError(w http.ResponseWriter, err error) {
	cat := platformerrors.AsCategory(err)
	escribirJSON(w, cat.HTTPStatus(), bffclient.Error{
		Codigo:  platformerrors.CodigoDe(err),
		Mensaje: err.Error(),
	})
}
