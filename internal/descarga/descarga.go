// Package descarga es la composicion del expediente de descarga de UN viaje
// (E6-T3 / E6-HU-026, DEC-022 #7). Es funcion PURA, sin E/S: toma la ficha
// tecnica (ViajeDetalle), el historico de bitacora y la auditoria relevante que
// bff leyo de operacion, y produce el expediente del contrato de bff
// (bffclient.DescargaViaje).
//
// bff es el UNICO compositor de esta vista. Lee SIEMPRE de operacion (la fuente
// exacta y actual), NUNCA de reportes (analitico y eventualmente consistente,
// bff.yaml E6-T3). bff no guarda nada: el rastro de auditoria de la descarga lo
// escribe operacion en su transaccion vIa registrarDescargaViaje, no este
// paquete.
//
// Exclusiones duras (DEC-009/DEC-039): el expediente NO contiene cuenta espejo
// ni clave_amago. No existen en el modelo V1 de operacion ni en el contrato de
// la descarga, asi que no hay nada que filtrar aqui: solo se serializan los
// campos contratados. Es descarga de UN viaje; la exportacion de listados es V2
// (DEC-023), por eso no hay composicion de lote.
package descarga

import (
	"time"

	bffclient "github.com/AI27-Tech/pf-pkg-contracts/packages/api-client-go/bff"
	opclient "github.com/AI27-Tech/pf-pkg-contracts/packages/api-client-go/operacion"
)

// Componer arma el expediente de descarga a partir de las lecturas de operacion.
// Es pura: no hace E/S y no muta sus entradas.
//
//   - detalle: la ficha tecnica del viaje (operacion.obtenerViaje). Solo se
//     serializan los campos contratados de la ficha; las condiciones e
//     incidencias activas del detalle NO forman parte del expediente de descarga.
//   - bitacora: el historico completo de bitacora, ya agotado por paginas y en
//     el orden cronologico que devolvio operacion (append-only).
//   - auditoria: la auditoria relevante, ya agotada por paginas.
//   - generadoEn: el momento en que bff compuso el expediente (lo fija el
//     llamador con su reloj; se conserva como parametro para mantener la pureza).
//
// Las listas de salida nunca son nil: una bitacora o auditoria vacia es una
// lista vacia, no ausencia de dato.
func Componer(detalle opclient.ViajeDetalle, bitacora []opclient.EntradaBitacora, auditoria []opclient.RegistroAuditoria, generadoEn time.Time) bffclient.DescargaViaje {
	return bffclient.DescargaViaje{
		ViajeId:    detalle.Viaje.Id,
		Ficha:      fichaDe(detalle.Viaje),
		Bitacora:   bitacoraDe(bitacora),
		Auditoria:  auditoriaDe(auditoria),
		GeneradoEn: generadoEn.UTC(),
	}
}

// fichaDe traduce el Viaje de operacion a la ficha tecnica del expediente. Copia
// solo los campos contratados de FichaTecnicaDescarga; nada de cuenta espejo ni
// clave_amago (no existen en el origen, DEC-009/DEC-039).
func fichaDe(v opclient.Viaje) bffclient.FichaTecnicaDescarga {
	destinos := make([]struct {
		Nombre      string  `json:"nombre"`
		NumeroEnvio *string `json:"numero_envio,omitempty"`
	}, 0, len(v.Destinos))
	for _, d := range v.Destinos {
		destinos = append(destinos, struct {
			Nombre      string  `json:"nombre"`
			NumeroEnvio *string `json:"numero_envio,omitempty"`
		}{
			Nombre:      d.Nombre,
			NumeroEnvio: d.NumeroEnvio,
		})
	}
	return bffclient.FichaTecnicaDescarga{
		ClienteId:         v.ClienteId,
		Destinos:          destinos,
		Estado:            string(v.Estado),
		FechaHoraServicio: v.FechaHoraServicio,
		Origen:            v.Origen,
		TipoServicio:      nil,
	}
}

// bitacoraDe traduce las entradas de bitacora de operacion a la vista de
// descarga: autor, texto y marca de tiempo. Descarta id y viaje_id (ruido para
// el expediente; el viaje ya esta identificado por viaje_id a nivel raiz).
// Nunca devuelve nil.
func bitacoraDe(entradas []opclient.EntradaBitacora) []bffclient.EntradaBitacoraDescarga {
	out := make([]bffclient.EntradaBitacoraDescarga, 0, len(entradas))
	for _, e := range entradas {
		out = append(out, bffclient.EntradaBitacoraDescarga{
			Autor: e.Autor,
			Texto: e.Texto,
			Ts:    e.Ts.UTC(),
		})
	}
	return out
}

// auditoriaDe traduce los registros de auditoria de operacion a la vista de
// descarga: actor, entidad y marca de tiempo. Descarta entidad_id y los campos
// de/a de transicion (no son parte del expediente contratado). Nunca devuelve
// nil.
func auditoriaDe(registros []opclient.RegistroAuditoria) []bffclient.RegistroAuditoriaDescarga {
	out := make([]bffclient.RegistroAuditoriaDescarga, 0, len(registros))
	for _, r := range registros {
		out = append(out, bffclient.RegistroAuditoriaDescarga{
			Actor:   r.Actor,
			Entidad: r.Entidad,
			Ts:      r.Ts.UTC(),
		})
	}
	return out
}
