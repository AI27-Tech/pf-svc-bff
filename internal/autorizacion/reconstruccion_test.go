package autorizacion

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"
)

// snapshotFalso implementa Snapshot con una lista fija y un gate para controlar
// cuándo responde (y así observar el estado not-ready mientras tanto).
type snapshotFalso struct {
	ids      []string
	err      error
	liberar  chan struct{} // si no es nil, ListarRevocadas bloquea hasta cerrarlo
	llamadas int
	mu       sync.Mutex
}

func (s *snapshotFalso) RevocadasActuales(ctx context.Context) ([]string, error) {
	s.mu.Lock()
	s.llamadas++
	s.mu.Unlock()
	if s.liberar != nil {
		select {
		case <-s.liberar:
		case <-ctx.Done():
			return nil, ctx.Err()
		}
	}
	return s.ids, s.err
}

// suscripcionFalsa es un worker.Worker trivial que bloquea hasta cancelar.
type suscripcionFalsa struct{}

func (suscripcionFalsa) Name() string                  { return "suscripcion-falsa" }
func (suscripcionFalsa) Ready(context.Context) error   { return nil }
func (suscripcionFalsa) Run(ctx context.Context) error { <-ctx.Done(); return nil }

// suscripcionFalla devuelve un error de inmediato (p. ej. NATS caído al
// arrancar), sin bloquear.
type suscripcionFalla struct{ err error }

func (suscripcionFalla) Name() string                { return "suscripcion-falla" }
func (suscripcionFalla) Ready(context.Context) error { return nil }
func (s suscripcionFalla) Run(context.Context) error { return s.err }

// TestReconstruccionPrecargaYMarcaReady: el worker precarga el snapshot en la
// proyección y pasa de not-ready a ready cuando converge.
func TestReconstruccionPrecargaYMarcaReady(t *testing.T) {
	proy := NuevaProyeccion()
	snap := &snapshotFalso{ids: []string{"s-1", "s-2"}}
	w := NuevoReconstructor(suscripcionFalsa{}, snap, proy)

	// Antes de arrancar: not-ready.
	if err := w.Ready(context.Background()); err == nil {
		t.Error("antes de converger, Ready debe reportar not-ready")
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go func() { _ = w.Run(ctx) }()

	// Tras converger: ready, y la proyección conoce las sesiones del snapshot.
	esperarReady(t, w)
	if !proy.Revocada(context.Background(), "s-1") || !proy.Revocada(context.Background(), "s-2") {
		t.Error("la precarga debió marcar las sesiones del snapshot en la proyección")
	}
}

// TestReconstruccionNotReadyDuranteCarga: mientras el snapshot no responde, el
// worker permanece not-ready (ADR 26 §4: not-ready hasta converger).
func TestReconstruccionNotReadyDuranteCarga(t *testing.T) {
	proy := NuevaProyeccion()
	gate := make(chan struct{})
	snap := &snapshotFalso{ids: []string{"s-1"}, liberar: gate}
	w := NuevoReconstructor(suscripcionFalsa{}, snap, proy)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go func() { _ = w.Run(ctx) }()

	time.Sleep(100 * time.Millisecond)
	if err := w.Ready(context.Background()); err == nil {
		t.Error("con el snapshot aún cargando, Ready debe seguir not-ready")
	}
	close(gate) // deja terminar la precarga
	esperarReady(t, w)
}

// TestReconstruccionFallaSnapshotNoConverge: si el snapshot falla, el worker NO
// declara ready (no sirve tráfico protegido con una proyección incompleta).
func TestReconstruccionFallaSnapshotNoConverge(t *testing.T) {
	proy := NuevaProyeccion()
	snap := &snapshotFalso{err: errors.New("identidad caída")}
	w := NuevoReconstructor(suscripcionFalsa{}, snap, proy)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go func() { _ = w.Run(ctx) }()

	time.Sleep(200 * time.Millisecond)
	if err := w.Ready(context.Background()); err == nil {
		t.Error("si el snapshot falla, el worker no debe declararse ready")
	}
}

// TestReconstruccionSuscripcionFallaTemprano: si el worker de suscripción
// devuelve error de inmediato (NATS caído al arrancar), Run propaga ese error
// (fail-fast de arranque), no se cuelga esperando una cancelación que no llega.
func TestReconstruccionSuscripcionFallaTemprano(t *testing.T) {
	proy := NuevaProyeccion()
	snap := &snapshotFalso{ids: []string{"s-1"}}
	errNats := errors.New("NATS no disponible")
	w := NuevoReconstructor(suscripcionFalla{err: errNats}, snap, proy)

	hecho := make(chan error, 1)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go func() { hecho <- w.Run(ctx) }()

	select {
	case err := <-hecho:
		if !errors.Is(err, errNats) {
			t.Errorf("Run debió propagar el error de la suscripción, devolvió %v", err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("Run se colgó: no manejó la suscripción que falla temprano")
	}
}

func esperarReady(t *testing.T, w interface {
	Ready(context.Context) error
}) {
	t.Helper()
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		if w.Ready(context.Background()) == nil {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatal("el worker nunca convergió a ready")
}
