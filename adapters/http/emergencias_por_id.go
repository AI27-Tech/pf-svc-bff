package http

import (
	"context"

	catclient "github.com/AI27-Tech/pf-pkg-contracts/packages/api-client-go/catalogos"
	msgclient "github.com/AI27-Tech/pf-pkg-contracts/packages/api-client-go/mensajeria"
	opclient "github.com/AI27-Tech/pf-pkg-contracts/packages/api-client-go/operacion"
	"github.com/AI27-Tech/pf-svc-bff/internal/errores"
)

// --- Bandeja de Reacción y caso por su id (tracer.34, DEC-121) -------------
// Misma mecánica que el resto de emergencias: presupuesto, propagación del
// Bearer y traducción de error. mensajeria autoriza por rol (DEC-130, DEC-131).

func (a *MensajeriaHTTP) ListarBandejaEmergencias(ctx context.Context, bearer string, params msgclient.ListarBandejaEmergenciasParams) (*msgclient.PaginaBandejaEmergencias, error) {
	ctx, cancel := context.WithTimeout(ctx, a.presupuesto)
	defer cancel()
	resp, err := a.cli.ListarBandejaEmergenciasWithResponse(ctx, &params, conBearer(bearer))
	if err != nil {
		return nil, errores.DeTransporte("ListarBandejaEmergencias", err)
	}
	if resp.JSON200 == nil {
		return nil, errores.DeStatus("ListarBandejaEmergencias", resp.StatusCode())
	}
	return resp.JSON200, nil
}

func (a *MensajeriaHTTP) ObtenerEmergencia(ctx context.Context, bearer string, emergenciaID msgclient.Id) (*msgclient.CasoEmergencia, error) {
	ctx, cancel := context.WithTimeout(ctx, a.presupuesto)
	defer cancel()
	resp, err := a.cli.ObtenerEmergenciaWithResponse(ctx, emergenciaID, conBearer(bearer))
	if err != nil {
		return nil, errores.DeTransporte("ObtenerEmergencia", err)
	}
	if resp.JSON200 == nil {
		return nil, errores.DeStatus("ObtenerEmergencia", resp.StatusCode())
	}
	return resp.JSON200, nil
}

func (a *MensajeriaHTTP) ListarNotasEmergenciaPorId(ctx context.Context, bearer string, emergenciaID msgclient.Id, params msgclient.ListarNotasEmergenciaPorIdParams) (*msgclient.PaginaNotasEmergencia, error) {
	ctx, cancel := context.WithTimeout(ctx, a.presupuesto)
	defer cancel()
	resp, err := a.cli.ListarNotasEmergenciaPorIdWithResponse(ctx, emergenciaID, &params, conBearer(bearer))
	if err != nil {
		return nil, errores.DeTransporte("ListarNotasEmergenciaPorId", err)
	}
	if resp.JSON200 == nil {
		return nil, errores.DeStatus("ListarNotasEmergenciaPorId", resp.StatusCode())
	}
	return resp.JSON200, nil
}

func (a *MensajeriaHTTP) ReclamarEmergenciaPorId(ctx context.Context, bearer string, emergenciaID msgclient.Id, body msgclient.VersionEsperadaEntrada) (*msgclient.Reclamo, error) {
	ctx, cancel := context.WithTimeout(ctx, a.presupuesto)
	defer cancel()
	resp, err := a.cli.ReclamarEmergenciaPorIdWithResponse(ctx, emergenciaID, body, conBearer(bearer))
	if err != nil {
		return nil, errores.DeTransporte("ReclamarEmergenciaPorId", err)
	}
	if resp.JSON201 == nil {
		return nil, errores.DeStatus("ReclamarEmergenciaPorId", resp.StatusCode())
	}
	return resp.JSON201, nil
}

func (a *MensajeriaHTTP) LiberarReclamoEmergenciaPorId(ctx context.Context, bearer string, emergenciaID, reclamoID msgclient.Id, params msgclient.LiberarReclamoEmergenciaPorIdParams) error {
	ctx, cancel := context.WithTimeout(ctx, a.presupuesto)
	defer cancel()
	resp, err := a.cli.LiberarReclamoEmergenciaPorIdWithResponse(ctx, emergenciaID, reclamoID, &params, conBearer(bearer))
	if err != nil {
		return errores.DeTransporte("LiberarReclamoEmergenciaPorId", err)
	}
	if resp.StatusCode() < 200 || resp.StatusCode() >= 300 {
		return errores.DeStatus("LiberarReclamoEmergenciaPorId", resp.StatusCode())
	}
	return nil
}

func (a *MensajeriaHTTP) TransferirEmergenciaPorId(ctx context.Context, bearer string, emergenciaID msgclient.Id, body msgclient.TransferenciaEntrada) (*msgclient.Reclamo, error) {
	ctx, cancel := context.WithTimeout(ctx, a.presupuesto)
	defer cancel()
	resp, err := a.cli.TransferirEmergenciaPorIdWithResponse(ctx, emergenciaID, body, conBearer(bearer))
	if err != nil {
		return nil, errores.DeTransporte("TransferirEmergenciaPorId", err)
	}
	if resp.JSON201 == nil {
		return nil, errores.DeStatus("TransferirEmergenciaPorId", resp.StatusCode())
	}
	return resp.JSON201, nil
}

func (a *MensajeriaHTTP) ResolverEmergenciaPorId(ctx context.Context, bearer string, emergenciaID msgclient.Id, body msgclient.ResolucionEmergenciaEntrada) (*msgclient.EmergenciaReaccion, error) {
	ctx, cancel := context.WithTimeout(ctx, a.presupuesto)
	defer cancel()
	resp, err := a.cli.ResolverEmergenciaPorIdWithResponse(ctx, emergenciaID, body, conBearer(bearer))
	if err != nil {
		return nil, errores.DeTransporte("ResolverEmergenciaPorId", err)
	}
	if resp.JSON200 == nil {
		return nil, errores.DeStatus("ResolverEmergenciaPorId", resp.StatusCode())
	}
	return resp.JSON200, nil
}

func (a *MensajeriaHTTP) CerrarEmergenciaPorId(ctx context.Context, bearer string, emergenciaID msgclient.Id) (*msgclient.CasoEmergencia, error) {
	ctx, cancel := context.WithTimeout(ctx, a.presupuesto)
	defer cancel()
	resp, err := a.cli.CerrarEmergenciaPorIdWithResponse(ctx, emergenciaID, conBearer(bearer))
	if err != nil {
		return nil, errores.DeTransporte("CerrarEmergenciaPorId", err)
	}
	if resp.JSON200 == nil {
		return nil, errores.DeStatus("CerrarEmergenciaPorId", resp.StatusCode())
	}
	return resp.JSON200, nil
}

// --- Lecturas para el detalle de Reacción (E8-T18) --------------------------

// ObtenerAsignacion reenvía a operacion.obtenerAsignacion: la escolta vigente
// del viaje. operacion rechaza al Cliente (404).
func (a *OperacionHTTP) ObtenerAsignacion(ctx context.Context, id opclient.Id) (*opclient.EscoltaVigente, error) {
	ctx, cancel := context.WithTimeout(ctx, a.presupuesto)
	defer cancel()
	resp, err := a.cli.ObtenerAsignacionWithResponse(ctx, id, conBearerOperacion(ctx))
	if err != nil {
		return nil, errores.DeTransporte("ObtenerAsignacion", err)
	}
	if resp.JSON200 == nil {
		return nil, errores.DeStatus("ObtenerAsignacion", resp.StatusCode())
	}
	return resp.JSON200, nil
}

func (a *CatalogosHTTP) ObtenerCliente(ctx context.Context, bearer string, id catclient.Id) (*catclient.Cliente, error) {
	ctx, cancel := context.WithTimeout(ctx, a.presupuesto)
	defer cancel()
	resp, err := a.cli.ObtenerClienteWithResponse(ctx, id, conBearerCatalogos(bearer))
	if err != nil {
		return nil, errores.DeTransporte("ObtenerCliente", err)
	}
	if resp.JSON200 == nil {
		return nil, errores.DeStatus("ObtenerCliente", resp.StatusCode())
	}
	return resp.JSON200, nil
}

func (a *CatalogosHTTP) ObtenerVehiculo(ctx context.Context, bearer string, id catclient.Id) (*catclient.Vehiculo, error) {
	ctx, cancel := context.WithTimeout(ctx, a.presupuesto)
	defer cancel()
	resp, err := a.cli.ObtenerVehiculoWithResponse(ctx, id, &catclient.ObtenerVehiculoParams{}, conBearerCatalogos(bearer))
	if err != nil {
		return nil, errores.DeTransporte("ObtenerVehiculo", err)
	}
	if resp.JSON200 == nil {
		return nil, errores.DeStatus("ObtenerVehiculo", resp.StatusCode())
	}
	return resp.JSON200, nil
}

func (a *CatalogosHTTP) ObtenerCustodio(ctx context.Context, bearer string, id catclient.Id) (*catclient.Custodio, error) {
	ctx, cancel := context.WithTimeout(ctx, a.presupuesto)
	defer cancel()
	resp, err := a.cli.ObtenerCustodioWithResponse(ctx, id, &catclient.ObtenerCustodioParams{}, conBearerCatalogos(bearer))
	if err != nil {
		return nil, errores.DeTransporte("ObtenerCustodio", err)
	}
	if resp.JSON200 == nil {
		return nil, errores.DeStatus("ObtenerCustodio", resp.StatusCode())
	}
	return resp.JSON200, nil
}
