// autoridad.go adapta la consulta durable de sesión de identidad al puerto
// Autoridad que la proyección usa en el fallback de ADR 26 §5. La proyección no
// conoce el cliente de identidad: solo pregunta "¿esta sesión está revocada?".
package autorizacion

import (
	"context"

	"github.com/google/uuid"

	idclient "github.com/AI27-Tech/pf-pkg-contracts/packages/api-client-go/identidad"
)

// ConsultaSesion es la operación de identidad que el fallback necesita: el
// estado durable de una sesión (ADR 26 §5). Es un subconjunto de bff.IdentidadPort
// declarado aquí, donde se usa, para no acoplar este paquete a toda la arista
// ni crear un ciclo con internal/agregacion.
type ConsultaSesion interface {
	ObtenerSesion(ctx context.Context, id idclient.Id) (*idclient.EstadoSesion, error)
}

// autoridadIdentidad implementa Autoridad sobre la consulta durable de identidad.
type autoridadIdentidad struct {
	consulta ConsultaSesion
}

// NuevaAutoridad adapta la consulta de sesión de identidad al puerto Autoridad.
// La raíz la construye con el cliente de identidad y la inyecta en la proyección
// vía ConFrescura.
func NuevaAutoridad(consulta ConsultaSesion) Autoridad {
	return &autoridadIdentidad{consulta: consulta}
}

// SesionRevocada consulta el estado durable de la sesión en identidad. Propaga
// el error tal cual: la proyección lo trata como fail-closed (ADR 26 §5), no es
// esta capa la que decide la postura de seguridad.
func (a *autoridadIdentidad) SesionRevocada(ctx context.Context, sesionID string) (bool, error) {
	id, err := uuid.Parse(sesionID)
	if err != nil {
		// Un sesion_id del token que no es un UUID válido no puede confirmarse
		// vigente: se trata como revocado (fail-closed), no se consulta.
		return true, nil
	}
	estado, err := a.consulta.ObtenerSesion(ctx, id)
	if err != nil {
		return false, err // la proyección decide fail-closed ante error.
	}
	return estado.Revocada, nil
}
