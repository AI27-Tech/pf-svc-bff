package agregacion

import (
	"context"
	"net/http"

	bffclient "github.com/AI27-Tech/pf-pkg-contracts/packages/api-client-go/bff"
	catclient "github.com/AI27-Tech/pf-pkg-contracts/packages/api-client-go/catalogos"
	"github.com/AI27-Tech/pf-pkg-contracts/packages/api-client-go/estados"
	msgclient "github.com/AI27-Tech/pf-pkg-contracts/packages/api-client-go/mensajeria"
	opclient "github.com/AI27-Tech/pf-pkg-contracts/packages/api-client-go/operacion"
	platformerrors "github.com/AI27-Tech/pf-pkg-platform-go/errors"
	"github.com/AI27-Tech/pf-svc-bff/internal/tokenctx"
)

// --- Emergencias del monitoreo por viaje (tracer.37, E8-T18, DEC-135) -------
// mensajeria revisa el rol (DEC-135 #4); el bff propaga el Bearer, recorta a la
// fila mínima y rechaza la activación sobre viaje terminal (DEC-135 #5).

// rechazarViajeTerminal lee el viaje y devuelve 409 viaje_en_estado_terminal si
// su estado es terminal (ADR 19). Deriva los terminales del paquete generado;
// no copia el enum.
//
// shortcut: hay una carrera entre esta lectura y la activación (DT-16); se
// retira cuando mensajeria vea el estado del viaje en su propia transacción.
func (h *Handlers) rechazarViajeTerminal(r *http.Request, id opclient.Id) error {
	ctx := tokenctx.Con(r.Context(), r.Header.Get("Authorization"))
	detalle, err := h.deps.Operacion.ObtenerViaje(ctx, id)
	if err != nil {
		return err
	}
	if estados.EstadoViaje(detalle.Viaje.Estado).EsTerminal() {
		return platformerrors.NewConflictoCodigo("viaje_en_estado_terminal", "bff: el viaje %s está en %s", id, detalle.Viaje.Estado)
	}
	return nil
}

// activar comparte la regla de las dos activaciones (cliente y monitoreo):
// viaje terminal → 409; si no, reenvía y devuelve 201 nuevo o 200 reentrega.
func (h *Handlers) activar(r *http.Request, id opclient.Id, activacion msgclient.Id) (*msgclient.CasoEmergencia, int, error) {
	if err := h.rechazarViajeTerminal(r, id); err != nil {
		return nil, 0, err
	}
	caso, idempotente, err := h.deps.Mensajeria.ActivarEmergencia(r.Context(), r.Header.Get("Authorization"), id,
		msgclient.ActivarEmergenciaEntrada{ActivacionId: activacion})
	if err != nil {
		return nil, 0, err
	}
	if idempotente {
		return caso, http.StatusOK, nil
	}
	return caso, http.StatusCreated, nil
}

func aEmergenciaViaje(c msgclient.CasoEmergencia) bffclient.EmergenciaViaje {
	return bffclient.EmergenciaViaje{Id: c.Id, Estado: bffclient.EstadoEmergencia(c.Estado), ActivadaEn: c.ActivadaEn}
}

// ListarEmergenciasViaje delega en mensajeria.listarEmergencias y recorta cada
// caso a la fila mínima (DEC-135 #1).
func (h *Handlers) ListarEmergenciasViaje(w http.ResponseWriter, r *http.Request) {
	id, err := uuidDe(r.PathValue("id"))
	if err != nil {
		escribirError(w, platformerrors.NewValidacion("bff: listarEmergenciasViaje: id invalido"))
		return
	}
	pagina, err := h.deps.Mensajeria.ListarEmergencias(r.Context(), r.Header.Get("Authorization"), id)
	if err != nil {
		escribirError(w, err)
		return
	}
	out := bffclient.PaginaEmergenciasViaje{Items: make([]bffclient.EmergenciaViaje, 0, len(pagina.Items))}
	for _, c := range pagina.Items {
		out.Items = append(out.Items, aEmergenciaViaje(c))
	}
	escribirJSON(w, http.StatusOK, out)
}

// ActivarEmergenciaInterno: el monitoreo activa desde el panel (DEC-045 #9).
func (h *Handlers) ActivarEmergenciaInterno(w http.ResponseWriter, r *http.Request) {
	id, err := uuidDe(r.PathValue("id"))
	if err != nil {
		escribirError(w, platformerrors.NewValidacion("bff: activarEmergenciaInterno: id invalido"))
		return
	}
	var cuerpo bffclient.ActivarEmergenciaInternoEntrada
	if err := decodificar(r, &cuerpo); err != nil {
		escribirError(w, err)
		return
	}
	caso, status, err := h.activar(r, id, cuerpo.ActivacionId)
	if err != nil {
		escribirError(w, err)
		return
	}
	escribirJSON(w, status, aEmergenciaViaje(*caso))
}

// AgregarNotaEmergenciaInterno: el monitoreo o el Administrador anotan (DEC-135
// #3); mensajeria rechaza a Reacción y Ejecutivo (403).
func (h *Handlers) AgregarNotaEmergenciaInterno(w http.ResponseWriter, r *http.Request) {
	id, eid, err := viajeYEmergenciaDe(r, "agregarNotaEmergenciaInterno")
	if err != nil {
		escribirError(w, err)
		return
	}
	var cuerpo bffclient.NotaEmergenciaInternoEntrada
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

// --- Bandeja de Reacción y caso por su id (tracer.34, DEC-121) -------------
// Reenvío con el Bearer; mensajeria autoriza por rol. La vista de mensajeria es
// copia exacta de la del bff, así que se responde tal cual, salvo
// ObtenerEmergencia, que compone el bloque de viaje.

func emergenciaDe(r *http.Request, op string) (msgclient.Id, error) {
	eid, err := uuidDe(r.PathValue("eid"))
	if err != nil {
		return eid, platformerrors.NewValidacion("bff: %s: id de emergencia invalido", op)
	}
	return eid, nil
}

func (h *Handlers) ListarBandejaEmergencias(w http.ResponseWriter, r *http.Request) {
	var params msgclient.ListarBandejaEmergenciasParams
	if c := r.URL.Query().Get("cursor"); c != "" {
		params.Cursor = &c
	}
	pagina, err := h.deps.Mensajeria.ListarBandejaEmergencias(r.Context(), r.Header.Get("Authorization"), params)
	if err != nil {
		escribirError(w, err)
		return
	}
	escribirJSON(w, http.StatusOK, pagina)
}

func (h *Handlers) ListarNotasEmergenciaPorId(w http.ResponseWriter, r *http.Request) {
	eid, err := emergenciaDe(r, "listarNotasEmergenciaPorId")
	if err != nil {
		escribirError(w, err)
		return
	}
	var params msgclient.ListarNotasEmergenciaPorIdParams
	if c := r.URL.Query().Get("cursor"); c != "" {
		params.Cursor = &c
	}
	pagina, err := h.deps.Mensajeria.ListarNotasEmergenciaPorId(r.Context(), r.Header.Get("Authorization"), eid, params)
	if err != nil {
		escribirError(w, err)
		return
	}
	escribirJSON(w, http.StatusOK, pagina)
}

func (h *Handlers) ReclamarEmergenciaPorId(w http.ResponseWriter, r *http.Request) {
	eid, err := emergenciaDe(r, "reclamarEmergenciaPorId")
	if err != nil {
		escribirError(w, err)
		return
	}
	var cuerpo msgclient.VersionEsperadaEntrada
	if err := decodificar(r, &cuerpo); err != nil {
		escribirError(w, err)
		return
	}
	reclamo, err := h.deps.Mensajeria.ReclamarEmergenciaPorId(r.Context(), r.Header.Get("Authorization"), eid, cuerpo)
	if err != nil {
		escribirError(w, err)
		return
	}
	escribirJSON(w, http.StatusCreated, reclamo)
}

func (h *Handlers) LiberarReclamoEmergenciaPorId(w http.ResponseWriter, r *http.Request) {
	eid, err := emergenciaDe(r, "liberarReclamoEmergenciaPorId")
	if err != nil {
		escribirError(w, err)
		return
	}
	rid, err := uuidDe(r.PathValue("rid"))
	if err != nil {
		escribirError(w, platformerrors.NewValidacion("bff: liberarReclamoEmergenciaPorId: id de reclamo invalido"))
		return
	}
	version, err := versionEsperadaDe(r)
	if err != nil {
		escribirError(w, err)
		return
	}
	if err := h.deps.Mensajeria.LiberarReclamoEmergenciaPorId(r.Context(), r.Header.Get("Authorization"), eid, rid,
		msgclient.LiberarReclamoEmergenciaPorIdParams{VersionEsperada: version}); err != nil {
		escribirError(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (h *Handlers) TransferirEmergenciaPorId(w http.ResponseWriter, r *http.Request) {
	eid, err := emergenciaDe(r, "transferirEmergenciaPorId")
	if err != nil {
		escribirError(w, err)
		return
	}
	var cuerpo msgclient.TransferenciaEntrada
	if err := decodificar(r, &cuerpo); err != nil {
		escribirError(w, err)
		return
	}
	reclamo, err := h.deps.Mensajeria.TransferirEmergenciaPorId(r.Context(), r.Header.Get("Authorization"), eid, cuerpo)
	if err != nil {
		escribirError(w, err)
		return
	}
	escribirJSON(w, http.StatusCreated, reclamo)
}

func (h *Handlers) ResolverEmergenciaPorId(w http.ResponseWriter, r *http.Request) {
	eid, err := emergenciaDe(r, "resolverEmergenciaPorId")
	if err != nil {
		escribirError(w, err)
		return
	}
	var cuerpo msgclient.ResolucionEmergenciaEntrada
	if err := decodificar(r, &cuerpo); err != nil {
		escribirError(w, err)
		return
	}
	caso, err := h.deps.Mensajeria.ResolverEmergenciaPorId(r.Context(), r.Header.Get("Authorization"), eid, cuerpo)
	if err != nil {
		escribirError(w, err)
		return
	}
	escribirJSON(w, http.StatusOK, caso)
}

func (h *Handlers) CerrarEmergenciaPorId(w http.ResponseWriter, r *http.Request) {
	eid, err := emergenciaDe(r, "cerrarEmergenciaPorId")
	if err != nil {
		escribirError(w, err)
		return
	}
	caso, err := h.deps.Mensajeria.CerrarEmergenciaPorId(r.Context(), r.Header.Get("Authorization"), eid)
	if err != nil {
		escribirError(w, err)
		return
	}
	escribirJSON(w, http.StatusOK, caso)
}

// ObtenerEmergencia compone DetalleEmergenciaReaccion (DEC-121 #2): el caso de
// mensajeria (que autoriza por rol), el viaje y su escolta de operacion, y los
// nombres, placas y teléfonos de catalogos. Sin viaje_id ni bitácora.
func (h *Handlers) ObtenerEmergencia(w http.ResponseWriter, r *http.Request) {
	eid, err := emergenciaDe(r, "obtenerEmergencia")
	if err != nil {
		escribirError(w, err)
		return
	}
	bearer := r.Header.Get("Authorization")
	caso, err := h.deps.Mensajeria.ObtenerEmergencia(r.Context(), bearer, eid)
	if err != nil {
		escribirError(w, err)
		return
	}
	viaje, err := h.viajeEmergencia(tokenctx.Con(r.Context(), bearer), bearer, caso.ViajeId)
	if err != nil {
		escribirError(w, err)
		return
	}
	escribirJSON(w, http.StatusOK, bffclient.DetalleEmergenciaReaccion{Caso: aEmergenciaReaccion(*caso), Viaje: viaje})
}

func aEmergenciaReaccion(c msgclient.CasoEmergencia) bffclient.EmergenciaReaccion {
	out := bffclient.EmergenciaReaccion{
		Id:             c.Id,
		Estado:         bffclient.EstadoEmergencia(c.Estado),
		Origen:         bffclient.OrigenActivacion(c.Origen),
		VersionReclamo: c.VersionReclamo,
		ActivadaEn:     c.ActivadaEn,
		ResueltaEn:     c.ResueltaEn,
		CerradaEn:      c.CerradaEn,
	}
	if c.ReclamoActivo != nil {
		r := bffclient.Reclamo(*c.ReclamoActivo)
		out.ReclamoActivo = &r
	}
	return out
}

// viajeEmergencia arma el bloque de viaje: estado, origen, destinos, fechas y
// ETA de operacion; nombre del cliente de catalogos; escolta de operacion con
// vehículo y custodios de catalogos. Cualquier lectura que falle hace fallar el
// detalle: Reacción no recibe una escolta incompleta como si fuera la real.
func (h *Handlers) viajeEmergencia(ctx context.Context, bearer string, viajeID opclient.Id) (bffclient.ViajeEmergencia, error) {
	detalle, err := h.deps.Operacion.ObtenerViaje(ctx, viajeID)
	if err != nil {
		return bffclient.ViajeEmergencia{}, err
	}
	v := detalle.Viaje
	out := bffclient.ViajeEmergencia{
		Estado:            string(v.Estado),
		Origen:            v.Origen,
		FechaHoraServicio: v.FechaHoraServicio,
		TipoServicio:      v.TipoServicio,
		EtaEstimado:       v.EtaEstimado,
		Destinos:          make([]bffclient.DestinoViajeEmergencia, 0, len(v.Destinos)),
		Escolta:           []bffclient.UnidadEscoltaEmergencia{},
	}
	for _, d := range v.Destinos {
		out.Destinos = append(out.Destinos, bffclient.DestinoViajeEmergencia{Nombre: d.Nombre, FechaCita: d.FechaCita, Arribo: d.Arribo})
	}
	if v.ClienteId != nil {
		cliente, err := h.deps.Catalogos.ObtenerCliente(ctx, bearer, *v.ClienteId)
		if err != nil {
			return bffclient.ViajeEmergencia{}, err
		}
		out.ClienteNombre = cliente.Nombre
	}

	escolta, err := h.deps.Operacion.ObtenerAsignacion(ctx, viajeID)
	if err != nil {
		return bffclient.ViajeEmergencia{}, err
	}
	for _, u := range escolta.Unidades {
		unidad, err := h.unidadEscolta(ctx, bearer, u)
		if err != nil {
			return bffclient.ViajeEmergencia{}, err
		}
		out.Escolta = append(out.Escolta, unidad)
	}
	return out, nil
}

func (h *Handlers) unidadEscolta(ctx context.Context, bearer string, u opclient.Unidad) (bffclient.UnidadEscoltaEmergencia, error) {
	out := bffclient.UnidadEscoltaEmergencia{Custodios: make([]bffclient.CustodioEscoltaEmergencia, 0, len(u.CustodiosIds))}
	if u.Tipo != nil {
		out.Tipo = string(*u.Tipo)
	}
	vehiculo, err := h.deps.Catalogos.ObtenerVehiculo(ctx, bearer, catclient.Id(u.VehiculoId))
	if err != nil {
		return out, err
	}
	out.Economico, out.Placas = vehiculo.Economico, vehiculo.Placas
	for _, id := range u.CustodiosIds {
		c, err := h.deps.Catalogos.ObtenerCustodio(ctx, bearer, catclient.Id(id))
		if err != nil {
			return out, err
		}
		out.Custodios = append(out.Custodios, bffclient.CustodioEscoltaEmergencia{Nombre: c.Nombre, Telefono: c.Telefono})
	}
	return out, nil
}
