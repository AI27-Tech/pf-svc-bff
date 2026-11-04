package http

import (
	"context"
	"fmt"
	"net/http"
	"time"

	opclient "github.com/AI27-Tech/pf-pkg-contracts/packages/api-client-go/operacion"
	bff "github.com/AI27-Tech/pf-svc-bff"
	"github.com/AI27-Tech/pf-svc-bff/internal/errores"
	"github.com/AI27-Tech/pf-svc-bff/internal/tokenctx"
)

// OperacionHTTP implementa bff.OperacionPort con el cliente generado de
// operacion. Respeta el presupuesto de operacion (bff.PresupuestoOperacion, el
// mismo que debe usar el adaptador en proceso de core, I3): presupuesto corto,
// sin reintento ciego, fallo visible y marcado (bff.md (i)). Traduce los errores
// a la taxonomia de la plataforma.
type OperacionHTTP struct {
	cli         *opclient.ClientWithResponses
	presupuesto time.Duration
}

var _ bff.OperacionPort = (*OperacionHTTP)(nil)

// NewOperacionHTTP crea el adaptador apuntando a la URL base de operacion.
func NewOperacionHTTP(baseURL string, doer opclient.HttpRequestDoer) (*OperacionHTTP, error) {
	opts := []opclient.ClientOption{}
	if doer != nil {
		opts = append(opts, opclient.WithHTTPClient(doer))
	}
	cli, err := opclient.NewClientWithResponses(baseURL, opts...)
	if err != nil {
		return nil, fmt.Errorf("bff: crear cliente de operacion: %w", err)
	}
	return &OperacionHTTP{cli: cli, presupuesto: bff.PresupuestoOperacion}, nil
}

// conBearerOperacion propaga el Bearer del usuario (tomado del ctx) hacia
// operacion. Operacion deriva rol/alcance/actor del token (DEC-053, E1-T7), así
// que toda operacion de usuario DEBE llevarlo. Bearer ausente no añade cabecera
// (operacion responderá 401). El adaptador en proceso de core no usa esto: ahí
// los claims ya viajan en el ctx.
func conBearerOperacion(ctx context.Context) opclient.RequestEditorFn {
	bearer := tokenctx.De(ctx)
	return func(_ context.Context, req *http.Request) error {
		if bearer != "" {
			req.Header.Set("Authorization", bearer)
		}
		return nil
	}
}

// CrearViaje reenvia la creacion a operacion.crearViaje. El estado inicial lo
// fija el rol del token en operacion (DEC-026); bff no decide.
func (a *OperacionHTTP) CrearViaje(ctx context.Context, body opclient.CrearViajeEntrada) (*opclient.Viaje, error) {
	ctx, cancel := context.WithTimeout(ctx, a.presupuesto)
	defer cancel()

	resp, err := a.cli.CrearViajeWithResponse(ctx, body, conBearerOperacion(ctx))
	if err != nil {
		return nil, errores.DeTransporte("CrearViaje", err)
	}
	if resp.JSON201 == nil {
		return nil, errores.DeStatus("CrearViaje", resp.StatusCode())
	}
	return resp.JSON201, nil
}

// ListarViajes lista por alcance. El alcance (unidad/cliente) lo resuelve
// operacion en servidor; bff solo reenvia el cursor (DEC-053).
func (a *OperacionHTTP) ListarViajes(ctx context.Context, params opclient.ListarViajesParams) (*opclient.PaginaViajes, error) {
	ctx, cancel := context.WithTimeout(ctx, a.presupuesto)
	defer cancel()

	resp, err := a.cli.ListarViajesWithResponse(ctx, &params, conBearerOperacion(ctx))
	if err != nil {
		return nil, errores.DeTransporte("ListarViajes", err)
	}
	if resp.JSON200 == nil {
		return nil, errores.DeStatus("ListarViajes", resp.StatusCode())
	}
	return resp.JSON200, nil
}

// ObtenerViaje lee el detalle para la composicion de hitos (ADR 27) y para la
// ficha tecnica del expediente de descarga (E6-T3).
func (a *OperacionHTTP) ObtenerViaje(ctx context.Context, id opclient.Id) (*opclient.ViajeDetalle, error) {
	ctx, cancel := context.WithTimeout(ctx, a.presupuesto)
	defer cancel()

	resp, err := a.cli.ObtenerViajeWithResponse(ctx, id, conBearerOperacion(ctx))
	if err != nil {
		return nil, errores.DeTransporte("ObtenerViaje", err)
	}
	if resp.JSON200 == nil {
		return nil, errores.DeStatus("ObtenerViaje", resp.StatusCode())
	}
	return resp.JSON200, nil
}

// ListarBitacora lee UNA pagina del historico de bitacora del viaje (E6-T3). La
// composicion de la descarga la invoca en bucle hasta agotar el cursor; cada
// pagina respeta el presupuesto de operacion.
func (a *OperacionHTTP) ListarBitacora(ctx context.Context, id opclient.Id, params opclient.ListarBitacoraParams) (*opclient.PaginaBitacora, error) {
	ctx, cancel := context.WithTimeout(ctx, a.presupuesto)
	defer cancel()

	resp, err := a.cli.ListarBitacoraWithResponse(ctx, id, &params, conBearerOperacion(ctx))
	if err != nil {
		return nil, errores.DeTransporte("ListarBitacora", err)
	}
	if resp.JSON200 == nil {
		return nil, errores.DeStatus("ListarBitacora", resp.StatusCode())
	}
	return resp.JSON200, nil
}

// ConsultarAuditoria lee UNA pagina de la auditoria relevante del viaje (E6-T3).
// La composicion la invoca en bucle hasta agotar el cursor.
func (a *OperacionHTTP) ConsultarAuditoria(ctx context.Context, id opclient.Id, params opclient.ConsultarAuditoriaParams) (*opclient.PaginaAuditoria, error) {
	ctx, cancel := context.WithTimeout(ctx, a.presupuesto)
	defer cancel()

	resp, err := a.cli.ConsultarAuditoriaWithResponse(ctx, id, &params, conBearerOperacion(ctx))
	if err != nil {
		return nil, errores.DeTransporte("ConsultarAuditoria", err)
	}
	if resp.JSON200 == nil {
		return nil, errores.DeStatus("ConsultarAuditoria", resp.StatusCode())
	}
	return resp.JSON200, nil
}

// RegistrarDescargaViaje pide a operacion que deje el rastro de auditoria de la
// descarga (E1-T7). Sin cuerpo: el actor sale de los claims del token que el
// adaptador propaga aguas abajo. operacion escribe el registro en su propia
// transaccion (bff no tiene base de datos). Devuelve el Viaje descargado.
func (a *OperacionHTTP) RegistrarDescargaViaje(ctx context.Context, id opclient.Id) (*opclient.Viaje, error) {
	ctx, cancel := context.WithTimeout(ctx, a.presupuesto)
	defer cancel()

	resp, err := a.cli.RegistrarDescargaViajeWithResponse(ctx, id, conBearerOperacion(ctx))
	if err != nil {
		return nil, errores.DeTransporte("RegistrarDescargaViaje", err)
	}
	if resp.JSON200 == nil {
		return nil, errores.DeStatus("RegistrarDescargaViaje", resp.StatusCode())
	}
	return resp.JSON200, nil
}

// TransicionarViaje reenvia una transicion a operacion.transicionarViaje (E9-T7).
// operacion es la unica autoridad: valida la arista contra la maquina de estados
// y aplica el aislamiento por tenant y la facultad del cliente (DEC-111/112).
// Devuelve el Viaje plano; el handler recompone la vista de seguimiento.
func (a *OperacionHTTP) TransicionarViaje(ctx context.Context, id opclient.Id, body opclient.TransicionEntrada) (*opclient.Viaje, error) {
	ctx, cancel := context.WithTimeout(ctx, a.presupuesto)
	defer cancel()

	resp, err := a.cli.TransicionarViajeWithResponse(ctx, id, body, conBearerOperacion(ctx))
	if err != nil {
		return nil, errores.DeTransporte("TransicionarViaje", err)
	}
	if resp.JSON200 == nil {
		// Propaga el codigo de maquina del 409 (p. ej.
		// cierre_anticipado_requiere_incidencia) para que el cliente lo distinga
		// (Ola C). Los demas status no llevan codigo propio.
		var codigo string
		if resp.JSON409 != nil {
			codigo = resp.JSON409.Codigo
		}
		return nil, errores.DeRespuesta("TransicionarViaje", resp.StatusCode(), codigo)
	}
	return resp.JSON200, nil
}

// codigo409 extrae el codigo de maquina de un 409 de operacion (p. ej.
// recurso_no_disponible, condicion_ya_abierta) para que el panel lo distinga.
func codigo409(e *opclient.Error) string {
	if e == nil {
		return ""
	}
	return e.Codigo
}

// FijarEta reenvia la captura manual del ETA a operacion.fijarEta (ADR 20).
func (a *OperacionHTTP) FijarEta(ctx context.Context, id opclient.Id, body opclient.EtaEntrada) (*opclient.Viaje, error) {
	ctx, cancel := context.WithTimeout(ctx, a.presupuesto)
	defer cancel()

	resp, err := a.cli.FijarEtaWithResponse(ctx, id, body, conBearerOperacion(ctx))
	if err != nil {
		return nil, errores.DeTransporte("FijarEta", err)
	}
	if resp.JSON200 == nil {
		return nil, errores.DeRespuesta("FijarEta", resp.StatusCode(), codigo409(resp.JSON409))
	}
	return resp.JSON200, nil
}

// AsignarRecursos reenvia la reserva atomica de la escolta a
// operacion.asignarRecursos (ADR 31).
func (a *OperacionHTTP) AsignarRecursos(ctx context.Context, id opclient.Id, body opclient.AsignacionEntrada) (*opclient.Asignacion, error) {
	ctx, cancel := context.WithTimeout(ctx, a.presupuesto)
	defer cancel()

	resp, err := a.cli.AsignarRecursosWithResponse(ctx, id, body, conBearerOperacion(ctx))
	if err != nil {
		return nil, errores.DeTransporte("AsignarRecursos", err)
	}
	if resp.JSON200 == nil {
		return nil, errores.DeRespuesta("AsignarRecursos", resp.StatusCode(), codigo409(resp.JSON409))
	}
	return resp.JSON200, nil
}

// RegistrarEntradaBitacora reenvia una entrada append-only a
// operacion.registrarEntradaBitacora; el autor sale del Bearer propagado.
func (a *OperacionHTTP) RegistrarEntradaBitacora(ctx context.Context, id opclient.Id, body opclient.EntradaBitacoraEntrada) (*opclient.EntradaBitacora, error) {
	ctx, cancel := context.WithTimeout(ctx, a.presupuesto)
	defer cancel()

	resp, err := a.cli.RegistrarEntradaBitacoraWithResponse(ctx, id, body, conBearerOperacion(ctx))
	if err != nil {
		return nil, errores.DeTransporte("RegistrarEntradaBitacora", err)
	}
	if resp.JSON201 == nil {
		return nil, errores.DeStatus("RegistrarEntradaBitacora", resp.StatusCode())
	}
	return resp.JSON201, nil
}

// AbrirIncidencia reenvia a operacion.abrirIncidencia; el rol del Bearer decide
// la facultad.
func (a *OperacionHTTP) AbrirIncidencia(ctx context.Context, id opclient.Id, body opclient.IncidenciaEntrada) (*opclient.Incidencia, error) {
	ctx, cancel := context.WithTimeout(ctx, a.presupuesto)
	defer cancel()

	resp, err := a.cli.AbrirIncidenciaWithResponse(ctx, id, body, conBearerOperacion(ctx))
	if err != nil {
		return nil, errores.DeTransporte("AbrirIncidencia", err)
	}
	if resp.JSON201 == nil {
		return nil, errores.DeRespuesta("AbrirIncidencia", resp.StatusCode(), codigo409(resp.JSON409))
	}
	return resp.JSON201, nil
}

// ResolverIncidencia reenvia a operacion.resolverIncidencia (solo supervisor o
// administrador, lo impone operacion).
func (a *OperacionHTTP) ResolverIncidencia(ctx context.Context, id, iid opclient.Id, body opclient.ResolucionIncidenciaEntrada) (*opclient.Incidencia, error) {
	ctx, cancel := context.WithTimeout(ctx, a.presupuesto)
	defer cancel()

	resp, err := a.cli.ResolverIncidenciaWithResponse(ctx, id, iid, body, conBearerOperacion(ctx))
	if err != nil {
		return nil, errores.DeTransporte("ResolverIncidencia", err)
	}
	if resp.JSON200 == nil {
		return nil, errores.DeStatus("ResolverIncidencia", resp.StatusCode())
	}
	return resp.JSON200, nil
}

// ListarEstadias lee UNA pagina de estadias del viaje (operacion.listarEstadias).
func (a *OperacionHTTP) ListarEstadias(ctx context.Context, id opclient.Id, params opclient.ListarEstadiasParams) (*opclient.PaginaEstadias, error) {
	ctx, cancel := context.WithTimeout(ctx, a.presupuesto)
	defer cancel()

	resp, err := a.cli.ListarEstadiasWithResponse(ctx, id, &params, conBearerOperacion(ctx))
	if err != nil {
		return nil, errores.DeTransporte("ListarEstadias", err)
	}
	if resp.JSON200 == nil {
		return nil, errores.DeStatus("ListarEstadias", resp.StatusCode())
	}
	return resp.JSON200, nil
}

// AbrirEstadia reenvia a operacion.abrirEstadia.
func (a *OperacionHTTP) AbrirEstadia(ctx context.Context, id opclient.Id, body opclient.EstadiaEntrada) (*opclient.Estadia, error) {
	ctx, cancel := context.WithTimeout(ctx, a.presupuesto)
	defer cancel()

	resp, err := a.cli.AbrirEstadiaWithResponse(ctx, id, body, conBearerOperacion(ctx))
	if err != nil {
		return nil, errores.DeTransporte("AbrirEstadia", err)
	}
	if resp.JSON201 == nil {
		return nil, errores.DeStatus("AbrirEstadia", resp.StatusCode())
	}
	return resp.JSON201, nil
}

// CerrarEstadia reenvia a operacion.cerrarEstadia. El cuerpo es vacio por
// contrato: el servidor fija el cierre.
func (a *OperacionHTTP) CerrarEstadia(ctx context.Context, id, eid opclient.Id) (*opclient.Estadia, error) {
	ctx, cancel := context.WithTimeout(ctx, a.presupuesto)
	defer cancel()

	resp, err := a.cli.CerrarEstadiaWithResponse(ctx, id, eid, opclient.CierreEstadiaEntrada{}, conBearerOperacion(ctx))
	if err != nil {
		return nil, errores.DeTransporte("CerrarEstadia", err)
	}
	if resp.JSON200 == nil {
		return nil, errores.DeRespuesta("CerrarEstadia", resp.StatusCode(), codigo409(resp.JSON409))
	}
	return resp.JSON200, nil
}

// ListarAdjuntos reenvia a operacion.listarAdjuntos (E6-HU-027): metadatos de la
// evidencia del viaje, sin bytes. operacion responde 403 fuera de alcance antes
// de listar nada.
func (a *OperacionHTTP) ListarAdjuntos(ctx context.Context, id opclient.Id) (*opclient.PaginaAdjuntos, error) {
	ctx, cancel := context.WithTimeout(ctx, a.presupuesto)
	defer cancel()

	resp, err := a.cli.ListarAdjuntosWithResponse(ctx, id, conBearerOperacion(ctx))
	if err != nil {
		return nil, errores.DeTransporte("ListarAdjuntos", err)
	}
	if resp.JSON200 == nil {
		return nil, errores.DeStatus("ListarAdjuntos", resp.StatusCode())
	}
	return resp.JSON200, nil
}

// CrearIntencionSubidaAdjunto reenvia a operacion.crearIntencionSubidaAdjunto. Los
// bytes no pasan por bff: el cliente sube a la URL de vida corta devuelta.
func (a *OperacionHTTP) CrearIntencionSubidaAdjunto(ctx context.Context, id opclient.Id, body opclient.IntencionSubidaAdjuntoEntrada) (*opclient.IntencionSubidaAdjunto, error) {
	ctx, cancel := context.WithTimeout(ctx, a.presupuesto)
	defer cancel()

	resp, err := a.cli.CrearIntencionSubidaAdjuntoWithResponse(ctx, id, body, conBearerOperacion(ctx))
	if err != nil {
		return nil, errores.DeTransporte("CrearIntencionSubidaAdjunto", err)
	}
	if resp.JSON201 == nil {
		return nil, errores.DeStatus("CrearIntencionSubidaAdjunto", resp.StatusCode())
	}
	return resp.JSON201, nil
}

// DescargarAdjunto reenvia a operacion.descargarAdjunto: URL de descarga de vida
// corta. operacion audita cada acceso (E1-T7) y rechaza un adjunto no `limpio`.
func (a *OperacionHTTP) DescargarAdjunto(ctx context.Context, id, aid opclient.Id) (*opclient.DescargaAdjunto, error) {
	ctx, cancel := context.WithTimeout(ctx, a.presupuesto)
	defer cancel()

	resp, err := a.cli.DescargarAdjuntoWithResponse(ctx, id, aid, conBearerOperacion(ctx))
	if err != nil {
		return nil, errores.DeTransporte("DescargarAdjunto", err)
	}
	if resp.JSON200 == nil {
		return nil, errores.DeStatus("DescargarAdjunto", resp.StatusCode())
	}
	return resp.JSON200, nil
}
