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
