package http

import (
	"context"
	"encoding/json"
	"net/http"

	idclient "github.com/AI27-Tech/pf-pkg-contracts/packages/api-client-go/identidad"
	"github.com/AI27-Tech/pf-svc-bff/internal/errores"
)

// Gobierno de usuarios, alcance y tenants (tracer.40, E8-T19). Todas propagan
// el Bearer del ctx: identidad resuelve el actor desde el token. Mismo
// presupuesto que el resto de la arista (I3).

// deIdentidad traduce el status de identidad y, en un 403 o un 409, conserva
// el código de máquina del cuerpo Error (autoescalamiento, rol_igual,
// ultimo_administrador, rol_sin_alcance, …). Se lee del cuerpo porque varias
// respuestas generadas tipan el 403/409 como la respuesta común, no como Error.
func deIdentidad(op string, status int, cuerpo []byte) error {
	var e idclient.Error
	if status == http.StatusConflict || status == http.StatusForbidden {
		_ = json.Unmarshal(cuerpo, &e)
	}
	return errores.DeRespuesta(op, status, e.Codigo)
}

// sinCuerpo traduce una respuesta que en éxito es 204.
func sinCuerpo(op string, status int, cuerpo []byte) error {
	if status != http.StatusNoContent {
		return deIdentidad(op, status, cuerpo)
	}
	return nil
}

// ListarSujetos reenvía a identidad.listarSujetos.
func (a *IdentidadHTTP) ListarSujetos(ctx context.Context, params idclient.ListarSujetosParams) (*idclient.PaginaSujetos, error) {
	ctx, cancel := context.WithTimeout(ctx, a.presupuesto)
	defer cancel()

	resp, err := a.cli.ListarSujetosWithResponse(ctx, &params, conBearerIdentidad(ctx))
	if err != nil {
		return nil, errores.DeTransporte("ListarSujetos", err)
	}
	if resp.JSON200 == nil {
		return nil, deIdentidad("ListarSujetos", resp.StatusCode(), resp.Body)
	}
	return resp.JSON200, nil
}

// AltaSujeto reenvía a identidad.altaSujeto.
func (a *IdentidadHTTP) AltaSujeto(ctx context.Context, body idclient.AltaSujetoEntrada) (*idclient.Sujeto, error) {
	ctx, cancel := context.WithTimeout(ctx, a.presupuesto)
	defer cancel()

	resp, err := a.cli.AltaSujetoWithResponse(ctx, body, conBearerIdentidad(ctx))
	if err != nil {
		return nil, errores.DeTransporte("AltaSujeto", err)
	}
	if resp.JSON201 == nil {
		return nil, deIdentidad("AltaSujeto", resp.StatusCode(), resp.Body)
	}
	return resp.JSON201, nil
}

// ObtenerSujeto reenvía a identidad.obtenerSujeto.
func (a *IdentidadHTTP) ObtenerSujeto(ctx context.Context, id idclient.Id) (*idclient.SujetoDetalle, error) {
	ctx, cancel := context.WithTimeout(ctx, a.presupuesto)
	defer cancel()

	resp, err := a.cli.ObtenerSujetoWithResponse(ctx, id, conBearerIdentidad(ctx))
	if err != nil {
		return nil, errores.DeTransporte("ObtenerSujeto", err)
	}
	if resp.JSON200 == nil {
		return nil, deIdentidad("ObtenerSujeto", resp.StatusCode(), resp.Body)
	}
	return resp.JSON200, nil
}

// EditarSujeto reenvía a identidad.editarSujeto.
func (a *IdentidadHTTP) EditarSujeto(ctx context.Context, id idclient.Id, body idclient.EdicionSujetoEntrada) (*idclient.SujetoDetalle, error) {
	ctx, cancel := context.WithTimeout(ctx, a.presupuesto)
	defer cancel()

	resp, err := a.cli.EditarSujetoWithResponse(ctx, id, body, conBearerIdentidad(ctx))
	if err != nil {
		return nil, errores.DeTransporte("EditarSujeto", err)
	}
	if resp.JSON200 == nil {
		return nil, deIdentidad("EditarSujeto", resp.StatusCode(), resp.Body)
	}
	return resp.JSON200, nil
}

// DesactivarSujeto reenvía a identidad.desactivarSujeto.
func (a *IdentidadHTTP) DesactivarSujeto(ctx context.Context, id idclient.Id) error {
	ctx, cancel := context.WithTimeout(ctx, a.presupuesto)
	defer cancel()

	resp, err := a.cli.DesactivarSujetoWithResponse(ctx, id, conBearerIdentidad(ctx))
	if err != nil {
		return errores.DeTransporte("DesactivarSujeto", err)
	}
	return sinCuerpo("DesactivarSujeto", resp.StatusCode(), resp.Body)
}

// ReactivarSujeto reenvía a identidad.reactivarSujeto.
func (a *IdentidadHTTP) ReactivarSujeto(ctx context.Context, id idclient.Id) error {
	ctx, cancel := context.WithTimeout(ctx, a.presupuesto)
	defer cancel()

	resp, err := a.cli.ReactivarSujetoWithResponse(ctx, id, conBearerIdentidad(ctx))
	if err != nil {
		return errores.DeTransporte("ReactivarSujeto", err)
	}
	return sinCuerpo("ReactivarSujeto", resp.StatusCode(), resp.Body)
}

// FijarRolSujeto reenvía a identidad.fijarRolSujeto.
func (a *IdentidadHTTP) FijarRolSujeto(ctx context.Context, id idclient.Id, body idclient.AsignacionRolEntrada) error {
	ctx, cancel := context.WithTimeout(ctx, a.presupuesto)
	defer cancel()

	resp, err := a.cli.FijarRolSujetoWithResponse(ctx, id, body, conBearerIdentidad(ctx))
	if err != nil {
		return errores.DeTransporte("FijarRolSujeto", err)
	}
	return sinCuerpo("FijarRolSujeto", resp.StatusCode(), resp.Body)
}

// RevocarSesionesSujeto reenvía a identidad.revocarSesionesSujeto.
func (a *IdentidadHTTP) RevocarSesionesSujeto(ctx context.Context, id idclient.Id, body idclient.RevocacionSesionesEntrada) (*idclient.RevocacionSesionesResultado, error) {
	ctx, cancel := context.WithTimeout(ctx, a.presupuesto)
	defer cancel()

	resp, err := a.cli.RevocarSesionesSujetoWithResponse(ctx, id, body, conBearerIdentidad(ctx))
	if err != nil {
		return nil, errores.DeTransporte("RevocarSesionesSujeto", err)
	}
	if resp.JSON200 == nil {
		return nil, deIdentidad("RevocarSesionesSujeto", resp.StatusCode(), resp.Body)
	}
	return resp.JSON200, nil
}

// ObtenerAlcanceSujeto reenvía a identidad.obtenerAlcanceSujeto.
func (a *IdentidadHTTP) ObtenerAlcanceSujeto(ctx context.Context, id idclient.Id) (*idclient.AlcanceSujeto, error) {
	ctx, cancel := context.WithTimeout(ctx, a.presupuesto)
	defer cancel()

	resp, err := a.cli.ObtenerAlcanceSujetoWithResponse(ctx, id, conBearerIdentidad(ctx))
	if err != nil {
		return nil, errores.DeTransporte("ObtenerAlcanceSujeto", err)
	}
	if resp.JSON200 == nil {
		return nil, deIdentidad("ObtenerAlcanceSujeto", resp.StatusCode(), resp.Body)
	}
	return resp.JSON200, nil
}

// FijarUnidadesSujeto reenvía a identidad.fijarUnidadesSujeto.
func (a *IdentidadHTTP) FijarUnidadesSujeto(ctx context.Context, id idclient.Id, body idclient.UnidadesSujetoEntrada) error {
	ctx, cancel := context.WithTimeout(ctx, a.presupuesto)
	defer cancel()

	resp, err := a.cli.FijarUnidadesSujetoWithResponse(ctx, id, body, conBearerIdentidad(ctx))
	if err != nil {
		return errores.DeTransporte("FijarUnidadesSujeto", err)
	}
	return sinCuerpo("FijarUnidadesSujeto", resp.StatusCode(), resp.Body)
}

// FijarSubordinadosSujeto reenvía a identidad.fijarSubordinadosSujeto.
func (a *IdentidadHTTP) FijarSubordinadosSujeto(ctx context.Context, id idclient.Id, body idclient.SubordinadosSujetoEntrada) error {
	ctx, cancel := context.WithTimeout(ctx, a.presupuesto)
	defer cancel()

	resp, err := a.cli.FijarSubordinadosSujetoWithResponse(ctx, id, body, conBearerIdentidad(ctx))
	if err != nil {
		return errores.DeTransporte("FijarSubordinadosSujeto", err)
	}
	return sinCuerpo("FijarSubordinadosSujeto", resp.StatusCode(), resp.Body)
}

// ListarTenants reenvía a identidad.listarTenants.
func (a *IdentidadHTTP) ListarTenants(ctx context.Context, params idclient.ListarTenantsParams) (*idclient.PaginaTenants, error) {
	ctx, cancel := context.WithTimeout(ctx, a.presupuesto)
	defer cancel()

	resp, err := a.cli.ListarTenantsWithResponse(ctx, &params, conBearerIdentidad(ctx))
	if err != nil {
		return nil, errores.DeTransporte("ListarTenants", err)
	}
	if resp.JSON200 == nil {
		return nil, deIdentidad("ListarTenants", resp.StatusCode(), resp.Body)
	}
	return resp.JSON200, nil
}

// CrearTenant reenvía a identidad.crearTenant: 201 si lo creó, 200 si ya
// existía.
func (a *IdentidadHTTP) CrearTenant(ctx context.Context, id idclient.Id, body idclient.TenantEntrada) (*idclient.Tenant, bool, error) {
	ctx, cancel := context.WithTimeout(ctx, a.presupuesto)
	defer cancel()

	resp, err := a.cli.CrearTenantWithResponse(ctx, id, body, conBearerIdentidad(ctx))
	if err != nil {
		return nil, false, errores.DeTransporte("CrearTenant", err)
	}
	if resp.JSON201 != nil {
		return resp.JSON201, true, nil
	}
	if resp.JSON200 != nil {
		return resp.JSON200, false, nil
	}
	return nil, false, deIdentidad("CrearTenant", resp.StatusCode(), resp.Body)
}

// DesactivarTenant reenvía a identidad.desactivarTenant.
func (a *IdentidadHTTP) DesactivarTenant(ctx context.Context, id idclient.Id) error {
	ctx, cancel := context.WithTimeout(ctx, a.presupuesto)
	defer cancel()

	resp, err := a.cli.DesactivarTenantWithResponse(ctx, id, conBearerIdentidad(ctx))
	if err != nil {
		return errores.DeTransporte("DesactivarTenant", err)
	}
	return sinCuerpo("DesactivarTenant", resp.StatusCode(), resp.Body)
}
