package http

import (
	"context"
	"encoding/json"
	"net/http"

	msgclient "github.com/AI27-Tech/pf-pkg-contracts/packages/api-client-go/mensajeria"
	opclient "github.com/AI27-Tech/pf-pkg-contracts/packages/api-client-go/operacion"
	"github.com/AI27-Tech/pf-svc-bff/internal/errores"
)

// Solicitudes y su conversación (V2, ADR 38, DEC-148). Misma mecánica que el
// resto de los adaptadores: presupuesto por llamada, Bearer del usuario,
// transporte caído = degradado y status aguas abajo = taxonomía de la
// plataforma. El 409 conserva el `codigo` de máquina (transicion_no_permitida,
// fecha_pago_fuera_de_vigencia, fecha_hora_servicio_requerida,
// destino_no_es_cotizacion, conversacion_solo_lectura).

// deOperacion traduce un status de operacion; en un 409 lee el `codigo` del
// cuerpo, porque la respuesta generada trae el 409 con tipos distintos según la
// operación.
func deOperacion(op string, status int, cuerpo []byte) error {
	var e opclient.Error
	if status == http.StatusConflict {
		_ = json.Unmarshal(cuerpo, &e)
	}
	return errores.DeRespuesta(op, status, e.Codigo)
}

// solicitudDe devuelve la solicitud de una respuesta 2xx o el error traducido.
func solicitudDe(op string, sol *opclient.Solicitud, status int, cuerpo []byte) (*opclient.Solicitud, error) {
	if sol == nil {
		return nil, deOperacion(op, status, cuerpo)
	}
	return sol, nil
}

// CrearSolicitud reenvía a operacion.crearSolicitud (201).
func (a *OperacionHTTP) CrearSolicitud(ctx context.Context, body opclient.CrearSolicitudEntrada) (*opclient.Solicitud, error) {
	ctx, cancel := context.WithTimeout(ctx, a.presupuesto)
	defer cancel()
	r, err := a.cli.CrearSolicitudWithResponse(ctx, body, conBearerOperacion(ctx))
	if err != nil {
		return nil, errores.DeTransporte("CrearSolicitud", err)
	}
	return solicitudDe("CrearSolicitud", r.JSON201, r.StatusCode(), r.Body)
}

// ListarSolicitudes reenvía a operacion.listarSolicitudes; operacion filtra por
// rol y alcance.
func (a *OperacionHTTP) ListarSolicitudes(ctx context.Context, params opclient.ListarSolicitudesParams) (*opclient.PaginaSolicitudes, error) {
	ctx, cancel := context.WithTimeout(ctx, a.presupuesto)
	defer cancel()
	r, err := a.cli.ListarSolicitudesWithResponse(ctx, &params, conBearerOperacion(ctx))
	if err != nil {
		return nil, errores.DeTransporte("ListarSolicitudes", err)
	}
	if r.JSON200 == nil {
		return nil, deOperacion("ListarSolicitudes", r.StatusCode(), r.Body)
	}
	return r.JSON200, nil
}

// ObtenerSolicitud reenvía a operacion.obtenerSolicitud.
func (a *OperacionHTTP) ObtenerSolicitud(ctx context.Context, id opclient.Id) (*opclient.Solicitud, error) {
	ctx, cancel := context.WithTimeout(ctx, a.presupuesto)
	defer cancel()
	r, err := a.cli.ObtenerSolicitudWithResponse(ctx, id, conBearerOperacion(ctx))
	if err != nil {
		return nil, errores.DeTransporte("ObtenerSolicitud", err)
	}
	return solicitudDe("ObtenerSolicitud", r.JSON200, r.StatusCode(), r.Body)
}

// ListarHistoricoSolicitud reenvía a operacion.listarHistoricoSolicitud.
func (a *OperacionHTTP) ListarHistoricoSolicitud(ctx context.Context, id opclient.Id) (*opclient.HistoricoSolicitud, error) {
	ctx, cancel := context.WithTimeout(ctx, a.presupuesto)
	defer cancel()
	r, err := a.cli.ListarHistoricoSolicitudWithResponse(ctx, id, conBearerOperacion(ctx))
	if err != nil {
		return nil, errores.DeTransporte("ListarHistoricoSolicitud", err)
	}
	if r.JSON200 == nil {
		return nil, deOperacion("ListarHistoricoSolicitud", r.StatusCode(), r.Body)
	}
	return r.JSON200, nil
}

// CotizarSolicitud reenvía a operacion.cotizarSolicitud.
func (a *OperacionHTTP) CotizarSolicitud(ctx context.Context, id opclient.Id, body opclient.CotizacionEntrada) (*opclient.Solicitud, error) {
	ctx, cancel := context.WithTimeout(ctx, a.presupuesto)
	defer cancel()
	r, err := a.cli.CotizarSolicitudWithResponse(ctx, id, body, conBearerOperacion(ctx))
	if err != nil {
		return nil, errores.DeTransporte("CotizarSolicitud", err)
	}
	return solicitudDe("CotizarSolicitud", r.JSON200, r.StatusCode(), r.Body)
}

// ReprogramarSolicitud reenvía a operacion.reprogramarSolicitud.
func (a *OperacionHTTP) ReprogramarSolicitud(ctx context.Context, id opclient.Id, body opclient.ReprogramacionEntrada) (*opclient.Solicitud, error) {
	ctx, cancel := context.WithTimeout(ctx, a.presupuesto)
	defer cancel()
	r, err := a.cli.ReprogramarSolicitudWithResponse(ctx, id, body, conBearerOperacion(ctx))
	if err != nil {
		return nil, errores.DeTransporte("ReprogramarSolicitud", err)
	}
	return solicitudDe("ReprogramarSolicitud", r.JSON200, r.StatusCode(), r.Body)
}

// RechazarCotizacion reenvía a operacion.rechazarCotizacion.
func (a *OperacionHTTP) RechazarCotizacion(ctx context.Context, id opclient.Id, body opclient.AccionClienteEntrada) (*opclient.Solicitud, error) {
	ctx, cancel := context.WithTimeout(ctx, a.presupuesto)
	defer cancel()
	r, err := a.cli.RechazarCotizacionWithResponse(ctx, id, body, conBearerOperacion(ctx))
	if err != nil {
		return nil, errores.DeTransporte("RechazarCotizacion", err)
	}
	return solicitudDe("RechazarCotizacion", r.JSON200, r.StatusCode(), r.Body)
}

// CancelarSolicitud reenvía a operacion.cancelarSolicitud.
func (a *OperacionHTTP) CancelarSolicitud(ctx context.Context, id opclient.Id, body opclient.AccionClienteEntrada) (*opclient.Solicitud, error) {
	ctx, cancel := context.WithTimeout(ctx, a.presupuesto)
	defer cancel()
	r, err := a.cli.CancelarSolicitudWithResponse(ctx, id, body, conBearerOperacion(ctx))
	if err != nil {
		return nil, errores.DeTransporte("CancelarSolicitud", err)
	}
	return solicitudDe("CancelarSolicitud", r.JSON200, r.StatusCode(), r.Body)
}

// ConfirmarPago reenvía a operacion.confirmarPago.
func (a *OperacionHTTP) ConfirmarPago(ctx context.Context, id opclient.Id, body opclient.PagoEntrada) (*opclient.Solicitud, error) {
	ctx, cancel := context.WithTimeout(ctx, a.presupuesto)
	defer cancel()
	r, err := a.cli.ConfirmarPagoWithResponse(ctx, id, body, conBearerOperacion(ctx))
	if err != nil {
		return nil, errores.DeTransporte("ConfirmarPago", err)
	}
	return solicitudDe("ConfirmarPago", r.JSON200, r.StatusCode(), r.Body)
}

// RevertirPago reenvía a operacion.revertirPago.
func (a *OperacionHTTP) RevertirPago(ctx context.Context, id opclient.Id, body opclient.MotivoEntrada) (*opclient.Solicitud, error) {
	ctx, cancel := context.WithTimeout(ctx, a.presupuesto)
	defer cancel()
	r, err := a.cli.RevertirPagoWithResponse(ctx, id, body, conBearerOperacion(ctx))
	if err != nil {
		return nil, errores.DeTransporte("RevertirPago", err)
	}
	return solicitudDe("RevertirPago", r.JSON200, r.StatusCode(), r.Body)
}

// AprobarSolicitud reenvía a operacion.aprobarSolicitud: la aprobación y el
// nacimiento del viaje son una sola transacción en operacion (ADR 39).
func (a *OperacionHTTP) AprobarSolicitud(ctx context.Context, id opclient.Id) (*opclient.Solicitud, error) {
	ctx, cancel := context.WithTimeout(ctx, a.presupuesto)
	defer cancel()
	r, err := a.cli.AprobarSolicitudWithResponse(ctx, id, conBearerOperacion(ctx))
	if err != nil {
		return nil, errores.DeTransporte("AprobarSolicitud", err)
	}
	return solicitudDe("AprobarSolicitud", r.JSON200, r.StatusCode(), r.Body)
}

// RechazarSolicitud reenvía a operacion.rechazarSolicitud.
func (a *OperacionHTTP) RechazarSolicitud(ctx context.Context, id opclient.Id, body opclient.MotivoEntrada) (*opclient.Solicitud, error) {
	ctx, cancel := context.WithTimeout(ctx, a.presupuesto)
	defer cancel()
	r, err := a.cli.RechazarSolicitudWithResponse(ctx, id, body, conBearerOperacion(ctx))
	if err != nil {
		return nil, errores.DeTransporte("RechazarSolicitud", err)
	}
	return solicitudDe("RechazarSolicitud", r.JSON200, r.StatusCode(), r.Body)
}

// --- Conversación de la solicitud ------------------------------------------

// conversacionDeSolicitud resuelve la conversación (tipo=solicitud) por el id de
// la solicitud. La usan las tres operaciones por solicitud: las operaciones por
// id de conversación sirven a los dos tipos, pero la solicitud y el viaje
// comparten id, así que el id de la solicitud solo no identifica la conversación.
// El ctx ya trae el presupuesto de la operación que llama.
func (a *MensajeriaHTTP) conversacionDeSolicitud(ctx context.Context, op, bearer string, solicitudID msgclient.Id) (*msgclient.Conversacion, error) {
	r, err := a.cli.ObtenerConversacionDeSolicitudWithResponse(ctx, solicitudID, conBearer(bearer))
	if err != nil {
		return nil, errores.DeTransporte(op, err)
	}
	if r.JSON200 == nil {
		return nil, deMensajeria(op, r.StatusCode(), r.Body)
	}
	return r.JSON200, nil
}

// ObtenerConversacionSolicitud reenvía a mensajeria.obtenerConversacionDeSolicitud.
func (a *MensajeriaHTTP) ObtenerConversacionSolicitud(ctx context.Context, bearer string, solicitudID msgclient.Id) (*msgclient.Conversacion, error) {
	ctx, cancel := context.WithTimeout(ctx, a.presupuesto)
	defer cancel()
	return a.conversacionDeSolicitud(ctx, "ObtenerConversacionSolicitud", bearer, solicitudID)
}

// ListarMensajesSolicitud resuelve la conversación de la solicitud y lee su
// historia por id de conversación. Las dos llamadas comparten UN presupuesto.
func (a *MensajeriaHTTP) ListarMensajesSolicitud(ctx context.Context, bearer string, solicitudID msgclient.Id, params msgclient.ListarMensajesParams) (*msgclient.PaginaMensajes, error) {
	ctx, cancel := context.WithTimeout(ctx, a.presupuesto)
	defer cancel()
	conv, err := a.conversacionDeSolicitud(ctx, "ListarMensajesSolicitud", bearer, solicitudID)
	if err != nil {
		return nil, err
	}
	r, err := a.cli.ListarMensajesWithResponse(ctx, conv.Id, &params, conBearer(bearer))
	if err != nil {
		return nil, errores.DeTransporte("ListarMensajesSolicitud", err)
	}
	if r.JSON200 == nil {
		return nil, deMensajeria("ListarMensajesSolicitud", r.StatusCode(), r.Body)
	}
	return r.JSON200, nil
}

// EnviarMensajeSolicitud resuelve la conversación de la solicitud y envía por id
// de conversación (200 reintento, 201 nuevo). El 409 conserva su `codigo`
// (conversacion_solo_lectura tras la ventana de 3 días).
func (a *MensajeriaHTTP) EnviarMensajeSolicitud(ctx context.Context, bearer string, solicitudID msgclient.Id, body msgclient.EnviarMensajeEntrada) (*msgclient.Mensaje, error) {
	ctx, cancel := context.WithTimeout(ctx, a.presupuesto)
	defer cancel()
	conv, err := a.conversacionDeSolicitud(ctx, "EnviarMensajeSolicitud", bearer, solicitudID)
	if err != nil {
		return nil, err
	}
	r, err := a.cli.EnviarMensajeWithResponse(ctx, conv.Id, body, conBearer(bearer))
	if err != nil {
		return nil, errores.DeTransporte("EnviarMensajeSolicitud", err)
	}
	if r.JSON201 != nil {
		return r.JSON201, nil
	}
	if r.JSON200 != nil {
		return r.JSON200, nil
	}
	return nil, deMensajeria("EnviarMensajeSolicitud", r.StatusCode(), r.Body)
}

// ListarTrabajoBandejaSolicitudes reenvía a mensajeria.listarTrabajoMiBandejaSolicitudes:
// cola única del pool de cotizacion, sin {id}. El 409 stream_resync_required
// conserva su código.
func (a *MensajeriaHTTP) ListarTrabajoBandejaSolicitudes(ctx context.Context, bearer string, params msgclient.ListarTrabajoMiBandejaSolicitudesParams) (*msgclient.TrabajoBandeja, error) {
	ctx, cancel := context.WithTimeout(ctx, a.presupuesto)
	defer cancel()
	r, err := a.cli.ListarTrabajoMiBandejaSolicitudesWithResponse(ctx, &params, conBearer(bearer))
	if err != nil {
		return nil, errores.DeTransporte("ListarTrabajoBandejaSolicitudes", err)
	}
	if r.JSON200 == nil {
		return nil, deMensajeria("ListarTrabajoBandejaSolicitudes", r.StatusCode(), r.Body)
	}
	return r.JSON200, nil
}
