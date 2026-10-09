package autorizacion

import (
	"context"
	"errors"
	"testing"
	"time"
)

// relojFalso devuelve un instante fijo, para controlar la frescura en la prueba.
func relojFalso(t time.Time) func() time.Time {
	return func() time.Time { return t }
}

// frescuraFija implementa Frescura con un instante fijo de última señal.
type frescuraFija struct{ ultima time.Time }

func (f frescuraFija) UltimaSenal() time.Time { return f.ultima }

// autoridadFalsa implementa Autoridad con una respuesta programada.
type autoridadFalsa struct {
	revocada bool
	err      error
	llamada  bool
}

func (a *autoridadFalsa) SesionRevocada(_ context.Context, _ string) (bool, error) {
	a.llamada = true
	return a.revocada, a.err
}

const umbral = 15 * time.Second

// TestRevocadaLocalSiempreGana: una sesión en el set local se rechaza sin
// consultar la autoridad, aunque el canal tenga frescura.
func TestRevocadaLocalSiempreGana(t *testing.T) {
	ahora := time.Now()
	aut := &autoridadFalsa{}
	p := NuevaProyeccion()
	p.ConFrescura(relojFalso(ahora), frescuraFija{ahora}, aut, umbral)
	p.Marcar("s-local")

	if !p.Revocada(context.Background(), "s-local") {
		t.Error("una sesión en el set local debe constar revocada")
	}
	if aut.llamada {
		t.Error("no debió consultar la autoridad: la respuesta local basta")
	}
}

// TestCanalFrescoRutaLocal: con el canal fresco (señal reciente), una sesión que
// no está en el set local se considera viva SIN consultar la autoridad.
func TestCanalFrescoRutaLocal(t *testing.T) {
	ahora := time.Now()
	aut := &autoridadFalsa{revocada: true} // si la consultara, rechazaría
	p := NuevaProyeccion()
	// Última señal hace 5s: dentro del umbral de 15s.
	p.ConFrescura(relojFalso(ahora), frescuraFija{ahora.Add(-5 * time.Second)}, aut, umbral)

	if p.Revocada(context.Background(), "s-viva") {
		t.Error("con canal fresco y sin tombstone local, la sesión está viva")
	}
	if aut.llamada {
		t.Error("canal fresco no debe consultar la autoridad (ruta local)")
	}
}

// TestCanalObsoletoAutoridadConfirmaVigencia: canal obsoleto (>15s sin señal),
// la autoridad dice que la sesión está viva, se permite.
func TestCanalObsoletoAutoridadConfirmaVigencia(t *testing.T) {
	ahora := time.Now()
	aut := &autoridadFalsa{revocada: false, err: nil}
	p := NuevaProyeccion()
	p.ConFrescura(relojFalso(ahora), frescuraFija{ahora.Add(-20 * time.Second)}, aut, umbral)

	if p.Revocada(context.Background(), "s-x") {
		t.Error("si la autoridad confirma vigencia, se permite")
	}
	if !aut.llamada {
		t.Error("canal obsoleto debe consultar la autoridad")
	}
}

// TestCanalObsoletoAutoridadConfirmaRevocacion: canal obsoleto, la autoridad
// dice revocada, se rechaza.
func TestCanalObsoletoAutoridadConfirmaRevocacion(t *testing.T) {
	ahora := time.Now()
	aut := &autoridadFalsa{revocada: true}
	p := NuevaProyeccion()
	p.ConFrescura(relojFalso(ahora), frescuraFija{ahora.Add(-20 * time.Second)}, aut, umbral)

	if !p.Revocada(context.Background(), "s-x") {
		t.Error("si la autoridad confirma revocación, se rechaza")
	}
}

// TestCanalObsoletoAutoridadNoResponde: canal obsoleto y la autoridad falla.
// FAIL-CLOSED: se rechaza (ADR 26 §5). Es el invariante de seguridad central.
func TestCanalObsoletoAutoridadNoResponde(t *testing.T) {
	ahora := time.Now()
	aut := &autoridadFalsa{err: errors.New("identidad no responde")}
	p := NuevaProyeccion()
	p.ConFrescura(relojFalso(ahora), frescuraFija{ahora.Add(-20 * time.Second)}, aut, umbral)

	if !p.Revocada(context.Background(), "s-x") {
		t.Error("FAIL-CLOSED: si el canal está obsoleto y la autoridad no responde, se rechaza")
	}
}

// TestSinFrescuraConfiguradaRutaLocal: sin ConFrescura (desarrollo sin bus), se
// comporta como antes: solo el set local, sin fallback. Protege el zero value.
func TestSinFrescuraConfiguradaRutaLocal(t *testing.T) {
	p := NuevaProyeccion()
	if p.Revocada(context.Background(), "cualquiera") {
		t.Error("sin frescura configurada, una sesión no marcada no está revocada")
	}
	p.Marcar("s-1")
	if !p.Revocada(context.Background(), "s-1") {
		t.Error("sin frescura configurada, el set local sigue funcionando")
	}
}
