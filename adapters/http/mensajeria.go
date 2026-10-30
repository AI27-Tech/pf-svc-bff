package http

import (
	"context"
	"fmt"
	"net/http"
	"time"

	msgclient "github.com/AI27-Tech/pf-pkg-contracts/packages/api-client-go/mensajeria"
	bff "github.com/AI27-Tech/pf-svc-bff"
	"github.com/AI27-Tech/pf-svc-bff/internal/errores"
)

// MensajeriaHTTP implementa bff.MensajeriaPort con el cliente generado de
// mensajeria. Primera arista bff→mensajeria (emergencia del portal cliente,
// E7/tracer.17). A diferencia de operacion/identidad, estos métodos reciben el
// Bearer del usuario y lo propagan aguas abajo: mensajeria autoriza por los
// claims del token (cliente_id == Subject, audiencia; ADR 21 §6), así que el
// token DEBE llegar. Se inyecta con un RequestEditorFn por llamada; no toca el
// middleware ni los adaptadores existentes.
type MensajeriaHTTP struct {
	cli         *msgclient.ClientWithResponses
	presupuesto time.Duration
}

var _ bff.MensajeriaPort = (*MensajeriaHTTP)(nil)

// NewMensajeriaHTTP crea el adaptador apuntando a la URL base de mensajeria.
func NewMensajeriaHTTP(baseURL string, doer msgclient.HttpRequestDoer) (*MensajeriaHTTP, error) {
	opts := []msgclient.ClientOption{}
	if doer != nil {
		opts = append(opts, msgclient.WithHTTPClient(doer))
	}
	cli, err := msgclient.NewClientWithResponses(baseURL, opts...)
	if err != nil {
		return nil, fmt.Errorf("bff: crear cliente de mensajeria: %w", err)
	}
	return &MensajeriaHTTP{cli: cli, presupuesto: bff.PresupuestoMensajeria}, nil
}

// conBearer devuelve un RequestEditorFn que propaga el Authorization del usuario
// aguas abajo. Bearer vacío no añade cabecera (mensajeria responderá 401).
func conBearer(bearer string) msgclient.RequestEditorFn {
	return func(ctx context.Context, req *http.Request) error {
		if bearer != "" {
			req.Header.Set("Authorization", bearer)
		}
		return nil
	}
}

// ListarEmergencias reenvía a mensajeria.listarEmergencias. mensajeria proyecta
// por audiencia según el token (vista mínima para el cliente, ADR 21 §6).
func (a *MensajeriaHTTP) ListarEmergencias(ctx context.Context, bearer string, viajeID msgclient.Id) (*msgclient.PaginaEmergencias, error) {
	ctx, cancel := context.WithTimeout(ctx, a.presupuesto)
	defer cancel()

	resp, err := a.cli.ListarEmergenciasWithResponse(ctx, viajeID, conBearer(bearer))
	if err != nil {
		return nil, errores.DeTransporte("ListarEmergencias", err)
	}
	if resp.JSON200 == nil {
		return nil, errores.DeStatus("ListarEmergencias", resp.StatusCode())
	}
	return resp.JSON200, nil
}

// ActivarEmergencia reenvía a mensajeria.activarEmergencia. Idempotente por
// activacion_id; 200 (reentrega) o 201 (nuevo) traen el caso.
func (a *MensajeriaHTTP) ActivarEmergencia(ctx context.Context, bearer string, viajeID msgclient.Id, body msgclient.ActivarEmergenciaEntrada) (*msgclient.CasoEmergencia, error) {
	ctx, cancel := context.WithTimeout(ctx, a.presupuesto)
	defer cancel()

	resp, err := a.cli.ActivarEmergenciaWithResponse(ctx, viajeID, body, conBearer(bearer))
	if err != nil {
		return nil, errores.DeTransporte("ActivarEmergencia", err)
	}
	if resp.JSON201 != nil {
		return resp.JSON201, nil
	}
	if resp.JSON200 != nil {
		return resp.JSON200, nil
	}
	return nil, errores.DeStatus("ActivarEmergencia", resp.StatusCode())
}

// AgregarNotaEmergencia reenvía a mensajeria.agregarNotaEmergencia. La respuesta
// del portal cliente no lleva cuerpo; basta con el éxito (201).
func (a *MensajeriaHTTP) AgregarNotaEmergencia(ctx context.Context, bearer string, viajeID, emergenciaID msgclient.Id, body msgclient.NotaEmergenciaEntrada) error {
	ctx, cancel := context.WithTimeout(ctx, a.presupuesto)
	defer cancel()

	resp, err := a.cli.AgregarNotaEmergenciaWithResponse(ctx, viajeID, emergenciaID, body, conBearer(bearer))
	if err != nil {
		return errores.DeTransporte("AgregarNotaEmergencia", err)
	}
	if resp.JSON201 == nil {
		return errores.DeStatus("AgregarNotaEmergencia", resp.StatusCode())
	}
	return nil
}

// --- Atención interna del caso (panel de Reacción, E7/tracer.17) -----------
// Cinco acciones de escritura del operador interno. Misma mecánica que arriba:
// presupuesto, propagación del Bearer y traducción de error de transporte/status.
// mensajeria autoriza por los claims (ADR 21 §6).

// ReclamarEmergencia reenvía a mensajeria.reclamarEmergencia. Devuelve el reclamo
// abierto (201); un 409 lo traduce DeStatus a conflicto de versión.
func (a *MensajeriaHTTP) ReclamarEmergencia(ctx context.Context, bearer string, viajeID, emergenciaID msgclient.Id, body msgclient.ReclamarEmergenciaJSONRequestBody) (*msgclient.Reclamo, error) {
	ctx, cancel := context.WithTimeout(ctx, a.presupuesto)
	defer cancel()

	resp, err := a.cli.ReclamarEmergenciaWithResponse(ctx, viajeID, emergenciaID, body, conBearer(bearer))
	if err != nil {
		return nil, errores.DeTransporte("ReclamarEmergencia", err)
	}
	if resp.JSON201 == nil {
		return nil, errores.DeStatus("ReclamarEmergencia", resp.StatusCode())
	}
	return resp.JSON201, nil
}

// LiberarReclamoEmergencia reenvía a mensajeria.liberarReclamoEmergencia. La
// version_esperada viaja como query (params); la respuesta no lleva cuerpo, así
// que el éxito es un 2xx.
func (a *MensajeriaHTTP) LiberarReclamoEmergencia(ctx context.Context, bearer string, viajeID, emergenciaID, reclamoID msgclient.Id, params msgclient.LiberarReclamoEmergenciaParams) error {
	ctx, cancel := context.WithTimeout(ctx, a.presupuesto)
	defer cancel()

	resp, err := a.cli.LiberarReclamoEmergenciaWithResponse(ctx, viajeID, emergenciaID, reclamoID, &params, conBearer(bearer))
	if err != nil {
		return errores.DeTransporte("LiberarReclamoEmergencia", err)
	}
	if resp.StatusCode() < 200 || resp.StatusCode() >= 300 {
		return errores.DeStatus("LiberarReclamoEmergencia", resp.StatusCode())
	}
	return nil
}

// TransferirEmergencia reenvía a mensajeria.transferirEmergencia. Devuelve el
// reclamo resultante (201).
func (a *MensajeriaHTTP) TransferirEmergencia(ctx context.Context, bearer string, viajeID, emergenciaID msgclient.Id, body msgclient.TransferirEmergenciaJSONRequestBody) (*msgclient.Reclamo, error) {
	ctx, cancel := context.WithTimeout(ctx, a.presupuesto)
	defer cancel()

	resp, err := a.cli.TransferirEmergenciaWithResponse(ctx, viajeID, emergenciaID, body, conBearer(bearer))
	if err != nil {
		return nil, errores.DeTransporte("TransferirEmergencia", err)
	}
	if resp.JSON201 == nil {
		return nil, errores.DeStatus("TransferirEmergencia", resp.StatusCode())
	}
	return resp.JSON201, nil
}

// ResolverEmergencia reenvía a mensajeria.resolverEmergencia. Devuelve el caso
// actualizado (200).
func (a *MensajeriaHTTP) ResolverEmergencia(ctx context.Context, bearer string, viajeID, emergenciaID msgclient.Id, body msgclient.ResolverEmergenciaJSONRequestBody) (*msgclient.CasoEmergencia, error) {
	ctx, cancel := context.WithTimeout(ctx, a.presupuesto)
	defer cancel()

	resp, err := a.cli.ResolverEmergenciaWithResponse(ctx, viajeID, emergenciaID, body, conBearer(bearer))
	if err != nil {
		return nil, errores.DeTransporte("ResolverEmergencia", err)
	}
	if resp.JSON200 == nil {
		return nil, errores.DeStatus("ResolverEmergencia", resp.StatusCode())
	}
	return resp.JSON200, nil
}

// CerrarEmergencia reenvía a mensajeria.cerrarEmergencia (sin cuerpo). Devuelve
// el caso cerrado (200).
func (a *MensajeriaHTTP) CerrarEmergencia(ctx context.Context, bearer string, viajeID, emergenciaID msgclient.Id) (*msgclient.CasoEmergencia, error) {
	ctx, cancel := context.WithTimeout(ctx, a.presupuesto)
	defer cancel()

	resp, err := a.cli.CerrarEmergenciaWithResponse(ctx, viajeID, emergenciaID, conBearer(bearer))
	if err != nil {
		return nil, errores.DeTransporte("CerrarEmergencia", err)
	}
	if resp.JSON200 == nil {
		return nil, errores.DeStatus("CerrarEmergencia", resp.StatusCode())
	}
	return resp.JSON200, nil
}

// --- Conversación del viaje para el portal cliente (E9-T8) -----------------
// La conversación se identifica por el viaje (DEC-029): el id de ruta es el
// viaje_id. Misma mecánica que emergencias: presupuesto, propagación del Bearer
// y traducción de error. mensajeria autoriza por los claims (ADR 21 §6).

// ObtenerConversacion reenvía a mensajeria.obtenerConversacion. Devuelve el
// Conversacion completo; el handler lo recorta a la vista de cliente.
func (a *MensajeriaHTTP) ObtenerConversacion(ctx context.Context, bearer string, viajeID msgclient.Id) (*msgclient.Conversacion, error) {
	ctx, cancel := context.WithTimeout(ctx, a.presupuesto)
	defer cancel()

	resp, err := a.cli.ObtenerConversacionWithResponse(ctx, viajeID, conBearer(bearer))
	if err != nil {
		return nil, errores.DeTransporte("ObtenerConversacion", err)
	}
	if resp.JSON200 == nil {
		return nil, errores.DeStatus("ObtenerConversacion", resp.StatusCode())
	}
	return resp.JSON200, nil
}

// ListarMensajes reenvía a mensajeria.listarMensajes. Pagina por secuencia
// (antes_de/despues_de/limite); la respuesta trae hay_mas, no cursor.
func (a *MensajeriaHTTP) ListarMensajes(ctx context.Context, bearer string, viajeID msgclient.Id, params msgclient.ListarMensajesParams) (*msgclient.PaginaMensajes, error) {
	ctx, cancel := context.WithTimeout(ctx, a.presupuesto)
	defer cancel()

	resp, err := a.cli.ListarMensajesWithResponse(ctx, viajeID, &params, conBearer(bearer))
	if err != nil {
		return nil, errores.DeTransporte("ListarMensajes", err)
	}
	if resp.JSON200 == nil {
		return nil, errores.DeStatus("ListarMensajes", resp.StatusCode())
	}
	return resp.JSON200, nil
}

// EnviarMensaje reenvía a mensajeria.enviarMensaje (idempotente por
// client_message_id). 200 (reintento) o 201 (nuevo) traen el mensaje; el 409
// (solo lectura tras el cierre) lo traduce DeStatus a conflicto.
func (a *MensajeriaHTTP) EnviarMensaje(ctx context.Context, bearer string, viajeID msgclient.Id, body msgclient.EnviarMensajeEntrada) (*msgclient.Mensaje, error) {
	ctx, cancel := context.WithTimeout(ctx, a.presupuesto)
	defer cancel()

	resp, err := a.cli.EnviarMensajeWithResponse(ctx, viajeID, body, conBearer(bearer))
	if err != nil {
		return nil, errores.DeTransporte("EnviarMensaje", err)
	}
	if resp.JSON201 != nil {
		return resp.JSON201, nil
	}
	if resp.JSON200 != nil {
		return resp.JSON200, nil
	}
	return nil, errores.DeStatus("EnviarMensaje", resp.StatusCode())
}
