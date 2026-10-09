package autorizacion

import (
	"context"
	"errors"
	"testing"

	"github.com/google/uuid"

	idclient "github.com/AI27-Tech/pf-pkg-contracts/packages/api-client-go/identidad"
)

// consultaFalsa implementa ConsultaSesion con una respuesta programada.
type consultaFalsa struct {
	estado  *idclient.EstadoSesion
	err     error
	llamada bool
}

func (c *consultaFalsa) ObtenerSesion(_ context.Context, _ idclient.Id) (*idclient.EstadoSesion, error) {
	c.llamada = true
	return c.estado, c.err
}

func sesionValida() string { return uuid.NewString() }

// TestAutoridadMapeaRevocada: identidad dice revocada → el adaptador devuelve true.
func TestAutoridadMapeaRevocada(t *testing.T) {
	c := &consultaFalsa{estado: &idclient.EstadoSesion{Revocada: true}}
	aut := NuevaAutoridad(c)
	rev, err := aut.SesionRevocada(context.Background(), sesionValida())
	if err != nil {
		t.Fatalf("no se esperaba error: %v", err)
	}
	if !rev {
		t.Error("identidad confirmó revocación; el adaptador debe devolver true")
	}
}

// TestAutoridadMapeaVigente: identidad dice no revocada → false.
func TestAutoridadMapeaVigente(t *testing.T) {
	c := &consultaFalsa{estado: &idclient.EstadoSesion{Revocada: false}}
	aut := NuevaAutoridad(c)
	rev, err := aut.SesionRevocada(context.Background(), sesionValida())
	if err != nil {
		t.Fatalf("no se esperaba error: %v", err)
	}
	if rev {
		t.Error("identidad confirmó vigencia; el adaptador debe devolver false")
	}
}

// TestAutoridadPropagaError: si identidad falla, el adaptador propaga el error
// para que la proyección decida fail-closed.
func TestAutoridadPropagaError(t *testing.T) {
	c := &consultaFalsa{err: errors.New("identidad caída")}
	aut := NuevaAutoridad(c)
	_, err := aut.SesionRevocada(context.Background(), sesionValida())
	if err == nil {
		t.Error("el error de identidad debe propagarse (la proyección hace fail-closed)")
	}
}

// TestAutoridadUUIDInvalido: un sesion_id que no es UUID no puede confirmarse
// vigente; se trata como revocado sin consultar a identidad (fail-closed).
func TestAutoridadUUIDInvalido(t *testing.T) {
	c := &consultaFalsa{estado: &idclient.EstadoSesion{Revocada: false}}
	aut := NuevaAutoridad(c)
	rev, err := aut.SesionRevocada(context.Background(), "no-es-uuid")
	if err != nil {
		t.Fatalf("no se esperaba error: %v", err)
	}
	if !rev {
		t.Error("un sesion_id inválido debe tratarse como revocado (fail-closed)")
	}
	if c.llamada {
		t.Error("no debió consultar identidad con un UUID inválido")
	}
}
