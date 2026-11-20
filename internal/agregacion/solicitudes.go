package agregacion

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"

	bffclient "github.com/AI27-Tech/pf-pkg-contracts/packages/api-client-go/bff"
	catclient "github.com/AI27-Tech/pf-pkg-contracts/packages/api-client-go/catalogos"
	msgclient "github.com/AI27-Tech/pf-pkg-contracts/packages/api-client-go/mensajeria"
	opclient "github.com/AI27-Tech/pf-pkg-contracts/packages/api-client-go/operacion"
	"github.com/AI27-Tech/pf-pkg-contracts/packages/api-client-go/roles"
	"github.com/AI27-Tech/pf-pkg-platform-go/auth"
	platformerrors "github.com/AI27-Tech/pf-pkg-platform-go/errors"
	"github.com/AI27-Tech/pf-svc-bff/internal/autorizacion"
	"github.com/AI27-Tech/pf-svc-bff/internal/tokenctx"
)

// --- Solicitudes (V2, ADR 38 y 39, DEC-148) --------------------------------
// bff.yaml las proyecta como espejo de operacion. El bff NO replica la máquina
// de estados ni sus reglas: reenvía y deja pasar el error de operacion con su
// `codigo`. Solo aplica las guardas de audiencia y rol del contrato (403 antes
// de tocar operacion) y compone `Solicitud.cliente` para la audiencia Interno.

// Rol × acción del contrato (DEC-148 #1, #12, #20, #22, #28). El administrador
// hereda las facultades de cotizacion y del Jefe Operativo (#20).
var (
	rolesLeenSolicitud    = map[roles.Rol]bool{roles.RolCotizacion: true, roles.RolAdministrador: true, roles.RolJefeOperativo: true}
	rolesCotizan          = map[roles.Rol]bool{roles.RolCotizacion: true, roles.RolAdministrador: true}
	rolesResuelvenPagadas = map[roles.Rol]bool{roles.RolJefeOperativo: true, roles.RolAdministrador: true}
	rolesRevierten        = map[roles.Rol]bool{roles.RolAdministrador: true}
	// rolesCreanViaje: solo Operación crea viajes; el cliente crea solicitudes
	// (bff.yaml crearViaje, DEC-026).
	rolesCreanViaje = map[roles.Rol]bool{roles.RolJefeOperativo: true, roles.RolAdministrador: true}
)

// autorizarSolicitud escribe 401 sin claims y 403 si el llamante no tiene la
// facultad: es Cliente (audiencia) y la acción es del cliente, o es interno con
// un rol de permitidos. Devuelve true si procede.
func autorizarSolicitud(w http.ResponseWriter, r *http.Request, op string, delCliente bool, permitidos map[roles.Rol]bool) bool {
	c := autorizacion.ClaimsDe(r.Context())
	if c == nil {
		escribirError(w, platformerrors.NewNoAutorizado("bff: %s: sin claims", op))
		return false
	}
	if c.Audiencia == auth.AudienciaCliente {
		if delCliente {
			return true
		}
	} else if permitidos[roles.Rol(c.Rol)] {
		return true
	}
	escribirError(w, platformerrors.NewProhibido("bff: %s: el rol %q no tiene esta facultad", op, c.Rol))
	return false
}

// decodificarOpcional lee un cuerpo que el contrato marca como no requerido:
// vacío es válido y deja el valor cero.
func decodificarOpcional(r *http.Request, v any) error {
	if err := json.NewDecoder(r.Body).Decode(v); err != nil && !errors.Is(err, io.EOF) {
		return platformerrors.NewValidacion("bff: cuerpo de la peticion invalido: %v", err)
	}
	return nil
}

func aSolicitud(s opclient.Solicitud) bffclient.Solicitud {
	out := bffclient.Solicitud{
		Avisos:            s.Avisos,
		ClienteId:         s.ClienteId,
		CreadoEn:          s.CreadoEn,
		Destinos:          make([]bffclient.Destino, len(s.Destinos)),
		Estado:            bffclient.EstadoSolicitud(s.Estado),
		FechaHoraServicio: s.FechaHoraServicio,
		Id:                s.Id,
		MarcaPenalizacion: s.MarcaPenalizacion,
		MotivoRechazo:     s.MotivoRechazo,
		Origen:            s.Origen,
		TipoServicio:      s.TipoServicio,
	}
	for i, d := range s.Destinos {
		out.Destinos[i] = bffclient.Destino(d)
	}
	if s.Cotizacion != nil {
		c := bffclient.Cotizacion(*s.Cotizacion)
		out.Cotizacion = &c
	}
	if s.Pago != nil {
		p := bffclient.Pago(*s.Pago)
		out.Pago = &p
	}
	if s.Tracto != nil {
		t := bffclient.Tracto(*s.Tracto)
		out.Tracto = &t
	}
	return out
}

func aHistoricoSolicitud(h *opclient.HistoricoSolicitud) bffclient.HistoricoSolicitud {
	out := bffclient.HistoricoSolicitud{Items: make([]bffclient.HistoricoSolicitudItem, len(h.Items))}
	for i, it := range h.Items {
		out.Items[i] = bffclient.HistoricoSolicitudItem{
			Accion:            it.Accion,
			ActorId:           it.ActorId,
			En:                it.En,
			MarcaPenalizacion: it.MarcaPenalizacion,
			Motivo:            it.Motivo,
		}
		if it.AEstado != nil {
			e := bffclient.EstadoSolicitud(*it.AEstado)
			out.Items[i].AEstado = &e
		}
		if it.DeEstado != nil {
			e := bffclient.EstadoSolicitud(*it.DeEstado)
			out.Items[i].DeEstado = &e
		}
		if it.Cotizacion != nil {
			c := bffclient.Cotizacion(*it.Cotizacion)
			out.Items[i].Cotizacion = &c
		}
	}
	return out
}

// componerClienteSolicitudes llena `cliente` (id y nombre de catalogos, o solo el
// id si ya no está) en las solicitudes, en una lectura por lote. Solo se llama
// para la audiencia Interno (DEC-149 #14); el Cliente no recibe el campo.
func (h *Handlers) componerClienteSolicitudes(ctx context.Context, bearer string, items []bffclient.Solicitud) error {
	var ids []catclient.Id
	vistos := map[catclient.Id]bool{}
	for _, s := range items {
		if s.ClienteId != nil && !vistos[*s.ClienteId] {
			vistos[*s.ClienteId] = true
			ids = append(ids, *s.ClienteId)
		}
	}
	clientes, err := h.clientesPorLote(ctx, bearer, ids)
	if err != nil {
		return err
	}
	for i, s := range items {
		if s.ClienteId == nil {
			continue
		}
		c := bffclient.ClienteViajeResumen{Id: *s.ClienteId}
		if cl, ok := clientes[*s.ClienteId]; ok {
			c.Nombre = &cl.Nombre
		}
		items[i].Cliente = &c
	}
	return nil
}

// responderSolicitudes escribe las solicitudes ya espejadas y, para la audiencia
// Interno, con el cliente compuesto.
func (h *Handlers) responderSolicitudes(w http.ResponseWriter, r *http.Request, status int, items []bffclient.Solicitud, respuesta func() any) {
	if c := autorizacion.ClaimsDe(r.Context()); c != nil && c.Audiencia == auth.AudienciaInterno {
		if err := h.componerClienteSolicitudes(tokenctx.Con(r.Context(), r.Header.Get("Authorization")), r.Header.Get("Authorization"), items); err != nil {
			escribirError(w, err)
			return
		}
	}
	escribirJSON(w, status, respuesta())
}

// sobreSolicitud es el esqueleto de las acciones sobre {id}: autoriza, lee el id
// y el cuerpo (si lo hay), reenvía a operacion y responde la solicitud.
func (h *Handlers) sobreSolicitud(w http.ResponseWriter, r *http.Request, op string, delCliente bool, permitidos map[roles.Rol]bool,
	cuerpo any, opcional bool, llamar func(ctx context.Context, id opclient.Id) (*opclient.Solicitud, error)) {
	if !autorizarSolicitud(w, r, op, delCliente, permitidos) {
		return
	}
	id, err := uuidDe(r.PathValue("id"))
	if err != nil {
		escribirError(w, platformerrors.NewValidacion("bff: %s: id invalido", op))
		return
	}
	if cuerpo != nil {
		dec := decodificar
		if opcional {
			dec = decodificarOpcional
		}
		if err := dec(r, cuerpo); err != nil {
			escribirError(w, err)
			return
		}
	}
	sol, err := llamar(tokenctx.Con(r.Context(), r.Header.Get("Authorization")), id)
	if err != nil {
		escribirError(w, err)
		return
	}
	items := []bffclient.Solicitud{aSolicitud(*sol)}
	h.responderSolicitudes(w, r, http.StatusOK, items, func() any { return items[0] })
}

// CrearSolicitud: solo el Cliente. La organización sale del token en operacion.
func (h *Handlers) CrearSolicitud(w http.ResponseWriter, r *http.Request) {
	if !h.soloCliente(w, r, "crearSolicitud") {
		return
	}
	var cuerpo bffclient.CrearSolicitudEntrada
	if err := decodificar(r, &cuerpo); err != nil {
		escribirError(w, err)
		return
	}
	entrada := opclient.CrearSolicitudEntrada{
		Destinos:          make([]opclient.Destino, len(cuerpo.Destinos)),
		FechaHoraServicio: cuerpo.FechaHoraServicio,
		Origen:            cuerpo.Origen,
		TipoServicio:      cuerpo.TipoServicio,
	}
	for i, d := range cuerpo.Destinos {
		entrada.Destinos[i] = opclient.Destino(d)
	}
	if cuerpo.Tracto != nil {
		t := opclient.Tracto(*cuerpo.Tracto)
		entrada.Tracto = &t
	}
	sol, err := h.deps.Operacion.CrearSolicitud(tokenctx.Con(r.Context(), r.Header.Get("Authorization")), entrada)
	if err != nil {
		escribirError(w, err)
		return
	}
	escribirJSON(w, http.StatusCreated, aSolicitud(*sol))
}

// ListarSolicitudes reenvía el filtro y el cursor tal cual: operacion filtra por
// rol y alcance (jefe_operativo solo PAGADO). El bff no filtra por estado.
func (h *Handlers) ListarSolicitudes(w http.ResponseWriter, r *http.Request) {
	if !autorizarSolicitud(w, r, "listarSolicitudes", true, rolesLeenSolicitud) {
		return
	}
	var params opclient.ListarSolicitudesParams
	if e := r.URL.Query().Get("estado"); e != "" {
		v := opclient.EstadoSolicitud(e)
		params.Estado = &v
	}
	if c := r.URL.Query().Get("cursor"); c != "" {
		v := opclient.Cursor(c)
		params.Cursor = &v
	}
	pagina, err := h.deps.Operacion.ListarSolicitudes(tokenctx.Con(r.Context(), r.Header.Get("Authorization")), params)
	if err != nil {
		escribirError(w, err)
		return
	}
	out := bffclient.PaginaSolicitudes{Items: make([]bffclient.Solicitud, len(pagina.Items)), CursorSiguiente: pagina.CursorSiguiente}
	for i, s := range pagina.Items {
		out.Items[i] = aSolicitud(s)
	}
	h.responderSolicitudes(w, r, http.StatusOK, out.Items, func() any { return out })
}

func (h *Handlers) ObtenerSolicitud(w http.ResponseWriter, r *http.Request) {
	h.sobreSolicitud(w, r, "obtenerSolicitud", true, rolesLeenSolicitud, nil, false,
		func(ctx context.Context, id opclient.Id) (*opclient.Solicitud, error) {
			return h.deps.Operacion.ObtenerSolicitud(ctx, id)
		})
}

// ListarHistoricoSolicitud: personal interno (cotizacion, jefe_operativo,
// administrador); el Cliente recibe 403.
func (h *Handlers) ListarHistoricoSolicitud(w http.ResponseWriter, r *http.Request) {
	if !autorizarSolicitud(w, r, "listarHistoricoSolicitud", false, rolesLeenSolicitud) {
		return
	}
	id, err := uuidDe(r.PathValue("id"))
	if err != nil {
		escribirError(w, platformerrors.NewValidacion("bff: listarHistoricoSolicitud: id invalido"))
		return
	}
	hist, err := h.deps.Operacion.ListarHistoricoSolicitud(tokenctx.Con(r.Context(), r.Header.Get("Authorization")), id)
	if err != nil {
		escribirError(w, err)
		return
	}
	escribirJSON(w, http.StatusOK, aHistoricoSolicitud(hist))
}

func (h *Handlers) CotizarSolicitud(w http.ResponseWriter, r *http.Request) {
	var c bffclient.CotizacionEntrada
	h.sobreSolicitud(w, r, "cotizarSolicitud", false, rolesCotizan, &c, false,
		func(ctx context.Context, id opclient.Id) (*opclient.Solicitud, error) {
			return h.deps.Operacion.CotizarSolicitud(ctx, id, opclient.CotizacionEntrada(c))
		})
}

func (h *Handlers) ReprogramarSolicitud(w http.ResponseWriter, r *http.Request) {
	var c bffclient.ReprogramacionEntrada
	h.sobreSolicitud(w, r, "reprogramarSolicitud", false, rolesCotizan, &c, false,
		func(ctx context.Context, id opclient.Id) (*opclient.Solicitud, error) {
			return h.deps.Operacion.ReprogramarSolicitud(ctx, id, opclient.ReprogramacionEntrada(c))
		})
}

func (h *Handlers) RechazarCotizacion(w http.ResponseWriter, r *http.Request) {
	var c bffclient.AccionClienteEntrada
	h.sobreSolicitud(w, r, "rechazarCotizacion", true, nil, &c, true,
		func(ctx context.Context, id opclient.Id) (*opclient.Solicitud, error) {
			return h.deps.Operacion.RechazarCotizacion(ctx, id, opclient.AccionClienteEntrada(c))
		})
}

func (h *Handlers) CancelarSolicitud(w http.ResponseWriter, r *http.Request) {
	var c bffclient.AccionClienteEntrada
	h.sobreSolicitud(w, r, "cancelarSolicitud", true, nil, &c, true,
		func(ctx context.Context, id opclient.Id) (*opclient.Solicitud, error) {
			return h.deps.Operacion.CancelarSolicitud(ctx, id, opclient.AccionClienteEntrada(c))
		})
}

func (h *Handlers) ConfirmarPago(w http.ResponseWriter, r *http.Request) {
	var c bffclient.PagoEntrada
	h.sobreSolicitud(w, r, "confirmarPago", false, rolesCotizan, &c, false,
		func(ctx context.Context, id opclient.Id) (*opclient.Solicitud, error) {
			return h.deps.Operacion.ConfirmarPago(ctx, id, opclient.PagoEntrada(c))
		})
}

func (h *Handlers) RevertirPago(w http.ResponseWriter, r *http.Request) {
	var c bffclient.MotivoEntrada
	h.sobreSolicitud(w, r, "revertirPago", false, rolesRevierten, &c, false,
		func(ctx context.Context, id opclient.Id) (*opclient.Solicitud, error) {
			return h.deps.Operacion.RevertirPago(ctx, id, opclient.MotivoEntrada(c))
		})
}

func (h *Handlers) AprobarSolicitud(w http.ResponseWriter, r *http.Request) {
	h.sobreSolicitud(w, r, "aprobarSolicitud", false, rolesResuelvenPagadas, nil, false,
		func(ctx context.Context, id opclient.Id) (*opclient.Solicitud, error) {
			return h.deps.Operacion.AprobarSolicitud(ctx, id)
		})
}

func (h *Handlers) RechazarSolicitud(w http.ResponseWriter, r *http.Request) {
	var c bffclient.MotivoEntrada
	h.sobreSolicitud(w, r, "rechazarSolicitud", false, rolesResuelvenPagadas, &c, false,
		func(ctx context.Context, id opclient.Id) (*opclient.Solicitud, error) {
			return h.deps.Operacion.RechazarSolicitud(ctx, id, opclient.MotivoEntrada(c))
		})
}

// --- Conversación de la solicitud ------------------------------------------
// Audiencia Cliente (vista mínima, igual que la del viaje). El personal de
// cotizacion la atiende por la bandeja y las operaciones por id de conversación;
// el resto de los roles internos recibe 403 por soloCliente.

func (h *Handlers) ObtenerConversacionSolicitud(w http.ResponseWriter, r *http.Request) {
	if !h.soloCliente(w, r, "obtenerConversacionSolicitud") {
		return
	}
	id, err := uuidDe(r.PathValue("id"))
	if err != nil {
		escribirError(w, platformerrors.NewValidacion("bff: obtenerConversacionSolicitud: id invalido"))
		return
	}
	conv, err := h.deps.Mensajeria.ObtenerConversacionSolicitud(r.Context(), r.Header.Get("Authorization"), id)
	if err != nil {
		escribirError(w, err)
		return
	}
	escribirJSON(w, http.StatusOK, aConversacionCliente(conv))
}

func (h *Handlers) ListarMensajesSolicitud(w http.ResponseWriter, r *http.Request) {
	if !h.soloCliente(w, r, "listarMensajesSolicitud") {
		return
	}
	id, err := uuidDe(r.PathValue("id"))
	if err != nil {
		escribirError(w, platformerrors.NewValidacion("bff: listarMensajesSolicitud: id invalido"))
		return
	}
	params, err := mensajesParamsDe(r)
	if err != nil {
		escribirError(w, err)
		return
	}
	pagina, err := h.deps.Mensajeria.ListarMensajesSolicitud(r.Context(), r.Header.Get("Authorization"), id, params)
	if err != nil {
		escribirError(w, err)
		return
	}
	escribirJSON(w, http.StatusOK, aPaginaMensajesCliente(pagina))
}

func (h *Handlers) EnviarMensajeSolicitud(w http.ResponseWriter, r *http.Request) {
	if !h.soloCliente(w, r, "enviarMensajeSolicitud") {
		return
	}
	id, err := uuidDe(r.PathValue("id"))
	if err != nil {
		escribirError(w, platformerrors.NewValidacion("bff: enviarMensajeSolicitud: id invalido"))
		return
	}
	var cuerpo bffclient.EnviarMensajeClienteEntrada
	if err := decodificar(r, &cuerpo); err != nil {
		escribirError(w, err)
		return
	}
	msg, err := h.deps.Mensajeria.EnviarMensajeSolicitud(r.Context(), r.Header.Get("Authorization"), id,
		msgclient.EnviarMensajeEntrada{ClientMessageId: cuerpo.ClientMessageId, Cuerpo: cuerpo.Cuerpo})
	if err != nil {
		escribirError(w, err)
		return
	}
	// shortcut: 201 también en el reintento idempotente, igual que enviarMensajeViaje.
	escribirJSON(w, http.StatusCreated, aMensajeCliente(msg))
}

// rolesBandejaSolicitudes ven la bandeja de solicitudes: el pool de cotizacion y
// el administrador (DEC-148 #20, #31).
var rolesBandejaSolicitudes = map[roles.Rol]bool{roles.RolCotizacion: true, roles.RolAdministrador: true}

// ListarTrabajoBandejaSolicitudes: cola única de cotizacion. Según bff.yaml, el
// Cliente recibe 403 y los demás roles internos no tienen esta bandeja: 404.
func (h *Handlers) ListarTrabajoBandejaSolicitudes(w http.ResponseWriter, r *http.Request) {
	c := autorizacion.ClaimsDe(r.Context())
	if c == nil {
		escribirError(w, platformerrors.NewNoAutorizado("bff: listarTrabajoBandejaSolicitudes: sin claims"))
		return
	}
	if c.Audiencia != auth.AudienciaInterno {
		escribirError(w, platformerrors.NewProhibido("bff: listarTrabajoBandejaSolicitudes: solo para la superficie interna"))
		return
	}
	if !rolesBandejaSolicitudes[roles.Rol(c.Rol)] {
		escribirError(w, platformerrors.NewNoEncontrado("bff: listarTrabajoBandejaSolicitudes: el rol %q no tiene esta bandeja", c.Rol))
		return
	}
	p, err := trabajoBandejaParamsDe(r)
	if err != nil {
		escribirError(w, err)
		return
	}
	trabajo, err := h.deps.Mensajeria.ListarTrabajoBandejaSolicitudes(r.Context(), r.Header.Get("Authorization"),
		msgclient.ListarTrabajoMiBandejaSolicitudesParams(p))
	if err != nil {
		escribirError(w, err)
		return
	}
	escribirJSON(w, http.StatusOK, aTrabajoBandeja(trabajo))
}
