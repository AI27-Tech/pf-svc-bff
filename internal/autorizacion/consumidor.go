// El consumidor de sesion.revocada es el worker de bff que escucha la difusión
// del broker (ADR 26 §3, ADR 29 §3) y alimenta la proyección local de
// tombstones. Cada réplica de bff corre el suyo: la difusión llega a TODAS las
// réplicas (no es un consumer group), de modo que todas invaliden la sesión.
//
// Alcance de esta pieza (tramo de transporte): recibir el broadcast y marcar la
// proyección. La reconstrucción por snapshot al arrancar y la regla de frescura
// de 15 s con fallback a identidad (ADR 26) son fases siguientes; aquí quedan
// como TODO explícito, no se fingen.
package autorizacion

import (
	"context"

	"github.com/AI27-Tech/pf-pkg-platform-go/bus"
	"github.com/AI27-Tech/pf-pkg-platform-go/worker"
)

// NuevoConsumidorRevocacion crea el worker que suscribe la difusión de
// sesion.revocada y marca cada sesión en la proyección. difusor es el puerto de
// broadcast del bus (lo inyecta la raíz); proy es la proyección local que el
// middleware consulta. El worker lo arranca la raíz en su RunGroup.
//
// El handler marca por el AgregadoID del sobre, que para sesion.revocada es el
// sesion_id (la clave de orden que fija el productor, identidad). Marcar es
// idempotente, así que una difusión repetida (at-least-once del transporte) no
// cambia el resultado.
func NuevoConsumidorRevocacion(difusor bus.Broadcaster, proy *Proyeccion) (worker.Worker, error) {
	marcar := func(ctx context.Context, s bus.Sobre) error {
		// La frescura del canal NO se registra aquí: ADR 26 §5 es explícito en que
		// la frescura no se infiere de la llegada de revocaciones (puede no haber
		// ninguna durante un periodo sano). La señal de frescura vendrá de un
		// heartbeat/watermark del canal (puerto autorizacion.Frescura), que es
		// fase siguiente. Este handler solo marca el tombstone recibido.
		proy.Marcar(s.AgregadoID)
		return nil
	}
	return difusor.SubscribeBroadcast(marcar)
}
