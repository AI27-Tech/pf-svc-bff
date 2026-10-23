// Package errores traduce fallos aguas abajo a la taxonomia de
// pf-pkg-platform-go/errors y al Error de bff.yaml. Regla de bff (bff.md (d)):
// un aguas-abajo caido es DEGRADADO o ROTO, NUNCA NO_ENCONTRADO. Una pieza
// ausente por degradacion viaja marcada, no como vacio: "un dato ausente por
// degradacion no es un dato ausente".
package errores

import (
	"context"
	"errors"
	"fmt"
	"net/http"

	platformerrors "github.com/AI27-Tech/pf-pkg-platform-go/errors"
)

// Prohibido es el error de bff para el 403 del contrato (acceso prohibido por
// audiencia o alcance). La taxonomia de la plataforma NO distingue 403 de 401
// (CategoryNoAutorizado mapea a 401), pero el contrato de bff tiene una
// respuesta Prohibido (403) separada de NoAutorizado (401): p. ej. la descarga
// del expediente rechaza al cliente (audiencia) antes de componer nada (E6-T3),
// y propaga el 403 de alcance de operacion. Por eso bff lo modela localmente, en
// su capa de borde, sin tocar la taxonomia compartida. escribirError lo reconoce
// y escribe 403; EsProhibido permite detectarlo al propagar errores aguas abajo.
type Prohibido struct {
	Mensaje string
}

func (e *Prohibido) Error() string { return e.Mensaje }

// NewProhibido crea un error Prohibido (403) con el mensaje dado.
func NewProhibido(msg string, args ...any) *Prohibido {
	return &Prohibido{Mensaje: fmt.Sprintf(msg, args...)}
}

// EsProhibido indica si err (o algo que envuelve) es un Prohibido de bff.
func EsProhibido(err error) bool {
	var p *Prohibido
	return errors.As(err, &p)
}

// CodigoProhibido es el codigo de maquina del Error de bff.yaml para el 403.
// snake_case, estable; los consumidores ramifican sobre el, no sobre el mensaje.
const CodigoProhibido = "prohibido"

// DeStatus traduce un codigo HTTP de una respuesta aguas abajo a la taxonomia de
// la plataforma. op nombra la operacion para el contexto del error.
//
// Mapea tal cual los codigos de negocio (401/403/404/409/400), y trata 5xx y
// cualquier otro inesperado como ROTO: un fallo del propio aguas-abajo, no una
// ausencia de dato. El llamador decide si lo degrada (bff.md (d)).
func DeStatus(op string, status int) error {
	switch status {
	case http.StatusUnauthorized:
		return platformerrors.NewNoAutorizado("bff: %s: no autorizado aguas abajo", op)
	case http.StatusForbidden:
		// 403 de alcance aguas abajo (operacion). Se propaga como 403, no como
		// 401: el contrato de bff distingue Prohibido de NoAutorizado (E6-T3).
		return NewProhibido("bff: %s: prohibido aguas abajo", op)
	case http.StatusNotFound:
		return platformerrors.NewNoEncontrado("bff: %s: no encontrado aguas abajo", op)
	case http.StatusConflict:
		return platformerrors.NewConflicto("bff: %s: conflicto aguas abajo", op)
	case http.StatusBadRequest:
		return platformerrors.NewValidacion("bff: %s: solicitud invalida aguas abajo", op)
	default:
		return platformerrors.NewRoto("bff: %s: respuesta inesperada aguas abajo (status %d)", op, status)
	}
}

// DeTransporte traduce un error de transporte (la llamada ni siquiera obtuvo
// respuesta: timeout, cancelacion, conexion caida) a DEGRADADO. Nunca es
// NO_ENCONTRADO: el aguas-abajo no respondio, no afirmo ausencia (bff.md (d)).
func DeTransporte(op string, err error) error {
	if err == nil {
		return nil
	}
	if errors.Is(err, context.DeadlineExceeded) || errors.Is(err, context.Canceled) {
		return platformerrors.NewDegradadoErr(err, "bff: %s: presupuesto agotado aguas abajo", op)
	}
	return platformerrors.NewDegradadoErr(err, "bff: %s: fallo de transporte aguas abajo", op)
}
