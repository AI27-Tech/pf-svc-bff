package agregacion

import (
	"context"
	"encoding/json"
	"net/http"
	"time"

	bffclient "github.com/AI27-Tech/pf-pkg-contracts/packages/api-client-go/bff"
	idclient "github.com/AI27-Tech/pf-pkg-contracts/packages/api-client-go/identidad"
	msgclient "github.com/AI27-Tech/pf-pkg-contracts/packages/api-client-go/mensajeria"
	opclient "github.com/AI27-Tech/pf-pkg-contracts/packages/api-client-go/operacion"
	"github.com/AI27-Tech/pf-pkg-platform-go/auth"
	platformerrors "github.com/AI27-Tech/pf-pkg-platform-go/errors"
	"github.com/AI27-Tech/pf-svc-bff/internal/autorizacion"
	"github.com/AI27-Tech/pf-svc-bff/internal/descarga"
	"github.com/AI27-Tech/pf-svc-bff/internal/errores"
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
		Usuario:    cuerpo.Usuario,
		Contrasena: cuerpo.Contrasena,
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
	if err := h.deps.Identidad.RevocarSesion(r.Context(), id); err != nil {
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
	viaje, err := h.deps.Operacion.CrearViaje(r.Context(), aCrearViajeOperacion(cuerpo))
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
	pagina, err := h.deps.Operacion.ListarViajes(r.Context(), params)
	if err != nil {
		escribirError(w, err)
		return
	}
	escribirJSON(w, http.StatusOK, pagina)
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
	detalle, err := h.deps.Operacion.ObtenerViaje(r.Context(), id)
	if err != nil {
		escribirError(w, err)
		return
	}
	// Composicion pura de hitos (E2-T2/E2-T3). Traduce estado tecnico -> hito y
	// colapsa el bucle multi-parada; el cliente nunca ve el token tecnico.
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
		escribirError(w, errores.NewProhibido("bff: descargarViaje: la descarga del expediente es solo para usuarios internos"))
		return
	}

	id, err := uuidDe(r.PathValue("id"))
	if err != nil {
		escribirError(w, platformerrors.NewValidacion("bff: descargarViaje: id invalido"))
		return
	}

	// 2. Composicion: ficha + bitacora + auditoria, leyendo de operacion. El
	//    alcance fino lo resuelve operacion: cualquier 403 (fuera de alcance) o
	//    404 (viaje inexistente) se propaga tal cual, sin componer el resto.
	detalle, err := h.deps.Operacion.ObtenerViaje(r.Context(), id)
	if err != nil {
		escribirError(w, err)
		return
	}
	bitacora, err := h.bitacoraCompleta(r.Context(), id)
	if err != nil {
		escribirError(w, err)
		return
	}
	auditoria, err := h.auditoriaCompleta(r.Context(), id)
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
	if _, err := h.deps.Operacion.RegistrarDescargaViaje(r.Context(), id); err != nil {
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
// (Category.HTTPStatus y Category.String) en vez de duplicarlo. Un aguas-abajo
// caido es degradado o roto, nunca no encontrado (bff.md (d)).
//
// El 403 (Prohibido) es el unico caso que no vive en la taxonomia de la
// plataforma (que colapsa 403 en 401): el contrato de bff lo separa como
// respuesta propia (E6-T3). Por eso se reconoce aqui, antes del mapeo general.
func escribirError(w http.ResponseWriter, err error) {
	if errores.EsProhibido(err) {
		escribirJSON(w, http.StatusForbidden, bffclient.Error{
			Codigo:  errores.CodigoProhibido,
			Mensaje: err.Error(),
		})
		return
	}
	cat := platformerrors.AsCategory(err)
	escribirJSON(w, cat.HTTPStatus(), bffclient.Error{
		Codigo:  cat.String(),
		Mensaje: err.Error(),
	})
}
