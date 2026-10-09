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

// tipoSesionRevocada es el tipo de evento que marca un tombstone. bff reconoce
// su propio tipo de dominio (ADR 22: <agregado>.<participio>); cualquier otro
// sobre del broadcast (p. ej. el heartbeat del canal) solo cuenta como señal de
// frescura, no como revocación. Discriminar por INCLUSIÓN —no "todo lo que no
// es heartbeat"— evita acoplar bff a los tipos de otros emisores y lo mantiene
// robusto ante tipos futuros en el broadcast.
const tipoSesionRevocada = "sesion.revocada"

// NuevoConsumidorRevocacion crea el worker que suscribe la difusión del canal.
// Cada sobre recibido —sea un sesion.revocada o un heartbeat— refresca la señal
// del canal (senal); solo un sesion.revocada marca el tombstone en la proyección
// (proy). difusor es el puerto de broadcast del bus; todos los inyecta la raíz.
//
// Separar frescura de tombstone cumple ADR 26 §5: la frescura se mide por señal
// del canal, no por la llegada de revocaciones (que pueden faltar en un periodo
// sano). Marcar es idempotente, así que una difusión repetida no cambia nada.
func NuevoConsumidorRevocacion(difusor bus.Broadcaster, proy *Proyeccion, senal *SenalCanal) (worker.Worker, error) {
	manejar := func(ctx context.Context, s bus.Sobre) error {
		// Toda señal del canal cuenta como frescura, cualquiera que sea su tipo.
		if senal != nil {
			senal.Marcar()
		}
		// Solo una revocación real marca el tombstone.
		if s.Tipo == tipoSesionRevocada {
			proy.Marcar(s.AgregadoID)
		}
		return nil
	}
	return difusor.SubscribeBroadcast(manejar)
}
