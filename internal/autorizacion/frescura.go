// frescura.go implementa la regla dura de ADR 26 §5 para la proyección de
// revocación: cuando el canal de difusión lleva más de un umbral sin señal
// verificable, cada petición protegida cae a un fallback síncrono contra
// identidad, y si identidad no responde se FALLA CERRADO (se rechaza).
//
// La frescura NO se infiere de la llegada de revocaciones (ADR 26 §5: puede no
// haber revocaciones durante un periodo sano). Se mide con una señal del canal
// (heartbeat/watermark), expuesta por el puerto Frescura. La implementación real
// de esa señal es fase siguiente; aquí va el puerto y la lógica que lo consume.
package autorizacion

import (
	"context"
	"sync"
	"time"
)

// SenalCanal implementa Frescura: registra cuándo llegó la última señal del
// canal de difusión. Cualquier sobre del broadcast (un heartbeat o un evento
// real) cuenta como señal de vida; lo alimenta el consumidor en cada entrega.
// Es seguro para uso concurrente: el consumidor marca y Revocada lee.
type SenalCanal struct {
	reloj  func() time.Time
	mu     sync.RWMutex
	ultima time.Time
}

// NuevaSenalCanal crea la señal con el reloj dado (time.Now en producción,
// inyectable en pruebas). Arranca con la última señal en el instante de
// creación: una réplica recién nacida se considera fresca hasta que pase el
// umbral sin recibir nada, no obsoleta de entrada.
func NuevaSenalCanal(reloj func() time.Time) *SenalCanal {
	if reloj == nil {
		reloj = time.Now
	}
	return &SenalCanal{reloj: reloj, ultima: reloj()}
}

// Marcar registra que acaba de llegar una señal del canal.
func (s *SenalCanal) Marcar() {
	s.mu.Lock()
	s.ultima = s.reloj()
	s.mu.Unlock()
}

// UltimaSenal implementa Frescura.
func (s *SenalCanal) UltimaSenal() time.Time {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.ultima
}

var _ Frescura = (*SenalCanal)(nil)

// Frescura informa cuándo fue la última señal verificable del canal de
// revocación. La proyección la usa para decidir si el canal está obsoleto.
type Frescura interface {
	// UltimaSenal es el instante de la última señal verificable del canal.
	UltimaSenal() time.Time
}

// Autoridad es el fallback durable a identidad (ADR 26 §5): consulta el estado
// de una sesión cuando el canal está obsoleto. Lo implementa un adaptador sobre
// IdentidadPort.ObtenerSesion. Un error (identidad no responde) es, para la
// regla de seguridad, indistinguible de "no puedo afirmar que esté viva".
type Autoridad interface {
	// SesionRevocada devuelve true si identidad confirma la revocación. El error
	// se propaga para que el llamador decida fail-closed.
	SesionRevocada(ctx context.Context, sesionID string) (bool, error)
}

// ConFrescura habilita la regla de obsolescencia/fallback de ADR 26 §5. Sin
// llamarla, Revocada se queda en la ruta local (solo el set de tombstones), que
// es el comportamiento de desarrollo sin bus. reloj permite inyectar el tiempo
// en pruebas; umbral es la ventana de frescura (15s en producción, ADR 26 §5).
func (p *Proyeccion) ConFrescura(reloj func() time.Time, fr Frescura, aut Autoridad, umbral time.Duration) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.reloj = reloj
	p.frescura = fr
	p.autoridad = aut
	p.umbral = umbral
}

// canalObsoleto indica si el canal lleva más del umbral sin señal verificable.
// Debe llamarse con el lock tomado (lo llama Revocada).
func (p *Proyeccion) canalObsoleto() bool {
	if p.frescura == nil || p.reloj == nil || p.umbral <= 0 {
		return false // sin frescura configurada, nunca se declara obsoleto
	}
	return p.reloj().Sub(p.frescura.UltimaSenal()) > p.umbral
}
