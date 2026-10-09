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
	"time"
)

// Frescura informa cuándo fue la última señal verificable del canal de
// revocación. La proyección la usa para decidir si el canal está obsoleto. Su
// implementación real (heartbeat del broadcast) es fase siguiente.
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
