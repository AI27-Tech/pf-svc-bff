package errores

import (
	"net/http"
	"testing"

	platformerrors "github.com/AI27-Tech/pf-pkg-platform-go/errors"
)

// TestDeRespuestaPropagaCodigoDeConflicto: un 409 aguas abajo con código de
// máquina (p. ej. el cierre de operacion) conserva ese código, que es lo que el
// cliente observa en el cuerpo. Antes de la Ola C se aplastaba a "conflicto".
func TestDeRespuestaPropagaCodigoDeConflicto(t *testing.T) {
	err := DeRespuesta("TransicionarViaje", http.StatusConflict, "cierre_anticipado_requiere_incidencia")
	if got := platformerrors.AsCategory(err); got != platformerrors.CategoryConflicto {
		t.Errorf("categoria = %d, se esperaba Conflicto", got)
	}
	if got := platformerrors.CodigoDe(err); got != "cierre_anticipado_requiere_incidencia" {
		t.Errorf("codigo = %q, se esperaba cierre_anticipado_requiere_incidencia", got)
	}
}

// TestDeRespuestaConflictoSinCodigo: un 409 sin código cae en el genérico
// "conflicto" (comportamiento previo intacto para quien no propaga código).
func TestDeRespuestaConflictoSinCodigo(t *testing.T) {
	err := DeRespuesta("X", http.StatusConflict, "")
	if got := platformerrors.CodigoDe(err); got != "conflicto" {
		t.Errorf("codigo = %q, se esperaba conflicto", got)
	}
}

// TestDeRespuesta403EsProhibido: un 403 aguas abajo se traduce a la categoría
// Prohibido de plataforma (403), no a NoAutorizado (401).
func TestDeRespuesta403EsProhibido(t *testing.T) {
	err := DeRespuesta("X", http.StatusForbidden, "")
	if got := platformerrors.AsCategory(err); got != platformerrors.CategoryProhibido {
		t.Errorf("categoria = %d, se esperaba Prohibido", got)
	}
	if got := platformerrors.CodigoDe(err); got != "prohibido" {
		t.Errorf("codigo = %q, se esperaba prohibido", got)
	}
}

// TestDeStatusEsDeRespuestaSinCodigo: DeStatus sigue siendo DeRespuesta con
// código vacío (no se tocó a los call sites sin código).
func TestDeStatusEsDeRespuestaSinCodigo(t *testing.T) {
	if platformerrors.CodigoDe(DeStatus("X", http.StatusNotFound)) != "no_encontrado" {
		t.Error("DeStatus 404 debe dar no_encontrado")
	}
}
