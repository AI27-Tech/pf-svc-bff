// Package tracking es la composicion de hitos V1 del seguimiento del cliente
// (E2-T2 y E2-T3, ADR 27). Es funcion PURA, sin E/S: toma el detalle del viaje
// que devuelve operacion (opclient.ViajeDetalle) y produce la vista de
// seguimiento del contrato de bff (bffclient.ViajeSeguimiento). bff es el UNICO
// compositor de esta vista (ADR 27 §2).
//
// Reglas que fija E2-T2 (docs/historias-tecnicas/E2-seguimiento-del-cliente.md):
//   - El cliente NUNCA recibe el token tecnico del enum de estados; recibe el
//     hito traducido.
//   - El bucle multi-parada (EN_TRANSITO -> EN_DESTINO -> EN_DESCARGA de cada
//     parada intermedia) se colapsa en un unico hito "En camino".
//
// No hay marcador vivo, posicion, senal, geocerca ni ETA automatica (ADR 20,
// 27). La cadencia de polling es configuracion del cliente, no contrato.
package tracking

import (
	"time"

	bffclient "github.com/AI27-Tech/pf-pkg-contracts/packages/api-client-go/bff"
	estados "github.com/AI27-Tech/pf-pkg-contracts/packages/api-client-go/estados"
	opclient "github.com/AI27-Tech/pf-pkg-contracts/packages/api-client-go/operacion"
)

// ahora es la fuente de tiempo para el cálculo de stale. Variable de paquete
// para que la prueba pueda sustituirla por un reloj fijo; en producción es la
// hora real en UTC.
var ahora = func() time.Time { return time.Now().UTC() }

// hitosPorEstado mapea cada estado tecnico del enum canonico (ADR 19) a la
// etiqueta de hito que ve el cliente (DEC-001). Los tres estados del bucle
// multi-parada (EN_TRANSITO, EN_DESTINO, EN_DESCARGA) colapsan en "En camino":
// el cliente no ve el bucle por parada (E2-T2, criterio de aceptacion). Los
// estados de rechazo y cancelacion se presentan como cierre sin exponer el
// token tecnico.
//
// Es un dato (mapa), no una cadena de if/switch, para que la prueba por estado
// recorra el enum completo y falle si aparece un estado nuevo sin hito.
var hitosPorEstado = map[estados.EstadoViaje]string{
	estados.EstadoSolicitado:          "Solicitud recibida",
	estados.EstadoAprobado:            "Servicio confirmado",
	estados.EstadoAsignado:            "Servicio confirmado",
	estados.EstadoAptoParaSalida:      "En preparacion",
	estados.EstadoEnEmbarque:          "En preparacion",
	estados.EstadoPatrullaEnCamino:    "En preparacion",
	estados.EstadoEnTransito:          "En camino",
	estados.EstadoEnDestino:           "En camino",
	estados.EstadoEnDescarga:          "En camino",
	estados.EstadoCerrado:             "Servicio cerrado",
	estados.EstadoCancelado:           "Servicio cancelado",
	estados.EstadoRechazadoPorAi27:    "Servicio no procede",
	estados.EstadoRechazadoPorCliente: "Servicio no procede",
}

// Etiqueta traduce un estado tecnico al hito de negocio que ve el cliente.
// Devuelve la etiqueta y true si el estado esta mapeado; "" y false si no, para
// que el llamador decida la postura ante un estado desconocido (nunca filtrar el
// token tecnico).
func Etiqueta(e estados.EstadoViaje) (string, bool) {
	etiqueta, ok := hitosPorEstado[e]
	return etiqueta, ok
}

// Componer traduce el detalle tecnico del viaje a la vista de seguimiento del
// cliente. Es pura: no hace E/S y no muta su entrada.
//
// Compone:
//   - el hito (traduccion estado tecnico -> etiqueta, colapsando multi-parada);
//   - el ETA manual con su marca stale (ver nota de contradiccion abajo);
//   - las incidencias curadas (E2-T3): categoria del cliente + estado + marca de
//     tiempo, traducidas de las incidencias activas de operacion sin filtrar el
//     tipo interno (DEC-006). Ver incidenciasCuradas en incidencias.go.
func Componer(d opclient.ViajeDetalle) bffclient.ViajeSeguimiento {
	v := d.Viaje

	hito := bffclient.HitoCliente{
		// Estado es la referencia tecnica subyacente; el contrato la expone como
		// string. No es el hito que se presenta: la etiqueta legible va en Etiqueta.
		Estado: string(v.Estado),
	}
	if etiqueta, ok := Etiqueta(estados.EstadoViaje(v.Estado)); ok {
		hito.Etiqueta = &etiqueta
	}

	// Incidencias curadas (E2-T3): se traduce cada incidencia activa que operacion
	// expone (con su tipo_interno, consumo interno) a su categoria del cliente,
	// sin filtrar el termino tecnico (DEC-006). IncidenciasActivas es opcional en
	// el contrato (*[]IncidenciaActiva): si viene nil, no hay incidencias abiertas
	// y la lista va vacia (no nil), nunca se afirma ausencia como dato ausente.
	var activas []opclient.IncidenciaActiva
	if d.IncidenciasActivas != nil {
		activas = *d.IncidenciasActivas
	}

	seg := bffclient.ViajeSeguimiento{
		Id:          v.Id,
		Hito:        hito,
		Incidencias: incidenciasCuradas(activas),
	}

	if v.EtaEstimado != nil {
		// stale = el ETA guardado ya pasó (E2-T2, criterios: "stale es true si la
		// hora del ETA guardado ya pasó; false en otro caso"). Lo calcula bff;
		// operacion no lo expone (tracer.4, ADR 20). bff NUNCA recalcula el ETA:
		// solo compara la hora actual contra el valor fijo que capturó operacion.
		obsoleto := ahora().After(*v.EtaEstimado)
		seg.Eta = &struct {
			Obsoleto bool                  `json:"obsoleto"`
			Valor    bffclient.MarcaTiempo `json:"valor"`
		}{
			Obsoleto: obsoleto,
			Valor:    *v.EtaEstimado,
		}
	}

	return seg
}

// La vista curada de incidencias (E2-T3) la compone incidenciasCuradas en
// incidencias.go, a partir de ViajeDetalle.incidencias_activas (tracer.16). Se
// elimino el marcador incidenciasPendientes del scaffold: la brecha de contrato
// que documentaba (operacion no exponia el tipo_interno en la lectura del viaje)
// quedo cerrada por el paquete contractual de AI27-59.
