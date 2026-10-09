// reconstruccion.go implementa la reconstrucción por snapshot al arrancar
// (ADR 26 §4): una réplica nueva de bff carga las sesiones revocadas vigentes
// desde identidad ANTES de declararse lista para tráfico protegido, para no
// arrancar "ciega" sin conocer las revocaciones previas.
//
// Mientras la precarga no termina, el worker reporta not-ready: la raíz no debe
// enrutar tráfico protegido a una réplica que aún no convergió (ADR 26 §4).
//
// Alcance (sin cursor): esta pieza NO hace el catch-up por cursor del ADR §4,
// porque identidad todavía no pagina el snapshot (devuelve el conjunto completo,
// su propio TODO). La ventana entre el snapshot y el inicio del canal se mitiga
// —no se cierra— arrancando la suscripción ANTES de la precarga: una revocación
// que ocurra durante la carga ya la captura el consumidor. Cerrar la ventana del
// todo es el cursor, fase siguiente.
package autorizacion

import (
	"context"
	"fmt"
	"sync/atomic"

	idclient "github.com/AI27-Tech/pf-pkg-contracts/packages/api-client-go/identidad"
	"github.com/AI27-Tech/pf-pkg-platform-go/worker"
)

// Snapshot provee la lista de sesiones revocadas vigentes para la precarga. Lo
// implementa snapshotIdentidad (sobre IdentidadPort.ListarSesionesRevocadas).
// Es una interfaz mínima declarada aquí, donde se usa.
type Snapshot interface {
	// RevocadasActuales devuelve los sesion_id revocados aún vigentes.
	RevocadasActuales(ctx context.Context) ([]string, error)
}

// ConsultaRevocadas es la operación de identidad que la precarga necesita: el
// snapshot de sesiones revocadas vigentes (ADR 26 §4). Subconjunto de
// bff.IdentidadPort declarado aquí, donde se usa.
type ConsultaRevocadas interface {
	ListarSesionesRevocadas(ctx context.Context) (*idclient.PaginaRevocaciones, error)
}

// snapshotIdentidad implementa Snapshot sobre la consulta de revocadas de
// identidad, mapeando la página del contrato a la lista de sesion_id que la
// proyección marca.
type snapshotIdentidad struct {
	consulta ConsultaRevocadas
}

// NuevoSnapshot adapta la consulta de revocadas de identidad al puerto Snapshot.
// La raíz lo construye con el cliente de identidad y lo pasa al reconstructor.
func NuevoSnapshot(consulta ConsultaRevocadas) Snapshot {
	return &snapshotIdentidad{consulta: consulta}
}

// RevocadasActuales trae el snapshot y extrae los sesion_id. Sin cursor: hoy
// identidad devuelve el conjunto completo (su TODO de paginación); cuando
// pagine, aquí se recorrerá CursorSiguiente hasta agotarlo.
func (s *snapshotIdentidad) RevocadasActuales(ctx context.Context) ([]string, error) {
	pagina, err := s.consulta.ListarSesionesRevocadas(ctx)
	if err != nil {
		return nil, fmt.Errorf("listar sesiones revocadas: %w", err)
	}
	ids := make([]string, 0, len(pagina.Items))
	for _, item := range pagina.Items {
		ids = append(ids, item.SesionId.String())
	}
	return ids, nil
}

// reconstructor envuelve el worker de suscripción con la precarga por snapshot y
// la readiness de ADR 26 §4. Implementa worker.Worker.
type reconstructor struct {
	suscripcion worker.Worker
	snapshot    Snapshot
	proy        *Proyeccion
	convergido  atomic.Bool
	// errPrecarga guarda el último error de precarga para exponerlo en Ready. bff
	// no tiene logger (es sin estado); surfacear la causa en el mensaje de
	// readiness es más útil que un log perdido: el operador que ve la réplica
	// atascada en not-ready lee ahí por qué no convergió. atomic.Pointer para el
	// acceso concurrente entre Run (escribe) y Ready (lee).
	errPrecarga atomic.Pointer[error]
}

// NuevoReconstructor envuelve el worker de suscripción (el consumidor del
// broadcast) con la reconstrucción por snapshot. La raíz lo arranca en su
// RunGroup en lugar del consumidor desnudo.
func NuevoReconstructor(suscripcion worker.Worker, snapshot Snapshot, proy *Proyeccion) worker.Worker {
	return &reconstructor{suscripcion: suscripcion, snapshot: snapshot, proy: proy}
}

func (r *reconstructor) Name() string { return "bff-revocacion" }

// Ready reporta not-ready hasta que la precarga converge (ADR 26 §4). Si la
// precarga falló, incluye la causa en el mensaje (bff no loggea). Una vez
// convergido, delega en la readiness del worker de suscripción, de modo que una
// réplica que perdió el canal tras converger vuelva a not-ready.
func (r *reconstructor) Ready(ctx context.Context) error {
	if !r.convergido.Load() {
		if ep := r.errPrecarga.Load(); ep != nil {
			return fmt.Errorf("reconstrucción de revocación no convergió: %w", *ep)
		}
		return fmt.Errorf("reconstrucción de revocación en curso: not-ready")
	}
	return r.suscripcion.Ready(ctx)
}

// Run arranca la suscripción al canal, precarga el snapshot y, al converger,
// declara ready. El orden importa: la suscripción arranca ANTES de la precarga
// para MITIGAR la ventana sin cursor (una revocación durante la carga ya la
// captura el canal; la ventana de activación de la suscripción sigue abierta,
// ADR 26 §4 la cierra con el cursor, fase siguiente).
//
// Dos caminos de salida, ambos reciben de errSub (sin fuga de goroutine):
//   - precarga falla: queda not-ready con la causa en Ready; espera a que la
//     suscripción termine (al cancelar ctx) y devuelve su resultado.
//   - la suscripción falla temprano (p. ej. NATS caído al arrancar): su error se
//     propaga; RunGroup lo trata como fallo de arranque. Es fail-fast
//     deliberado: sin canal, la réplica no debe servir tráfico protegido.
func (r *reconstructor) Run(ctx context.Context) error {
	errSub := make(chan error, 1)
	go func() { errSub <- r.suscripcion.Run(ctx) }()

	if err := r.precargar(ctx); err != nil {
		// Guarda la causa para Ready; no se propaga directamente para no tumbar
		// a los demás workers de la raíz (RunGroup cancela ante error ≠ Canceled).
		r.errPrecarga.Store(&err)
		return <-errSub
	}
	r.convergido.Store(true)

	return <-errSub
}

// precargar carga las revocadas vigentes del snapshot en la proyección. Marcar
// es idempotente, así que si el consumidor ya marcó alguna por el canal, no hay
// doble efecto.
func (r *reconstructor) precargar(ctx context.Context) error {
	ids, err := r.snapshot.RevocadasActuales(ctx)
	if err != nil {
		return fmt.Errorf("precargar snapshot de revocadas: %w", err)
	}
	for _, id := range ids {
		r.proy.Marcar(id)
	}
	return nil
}
