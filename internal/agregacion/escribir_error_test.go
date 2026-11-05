package agregacion

import (
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	platformerrors "github.com/AI27-Tech/pf-pkg-platform-go/errors"
	"github.com/AI27-Tech/pf-svc-bff/internal/errores"
)

// causaInterna imita lo que NO debe llegar al cuerpo: texto de Postgres o el
// cuerpo de un aguas-abajo envuelto como causa del PlatformError.
var causaInterna = errors.New(`ERROR: relation "viajes.viaje" does not exist (SQLSTATE 42P01)`)

func TestEscribirErrorSoloMensajePublico(t *testing.T) {
	tests := []struct {
		name       string
		err        error
		wantStatus int
		wantBody   string
	}{
		{
			name:       "roto con causa envuelta: solo el mensaje público",
			err:        platformerrors.NewRotoErr(causaInterna, "bff: obtenerViaje: error interno"),
			wantStatus: http.StatusBadGateway,
			wantBody:   `{"codigo":"roto","mensaje":"bff: obtenerViaje: error interno"}` + "\n",
		},
		{
			name:       "degradado de transporte envuelto con %w: el PlatformError más externo",
			err:        fmt.Errorf("agregar: %w", errores.DeTransporte("ObtenerViaje", causaInterna)),
			wantStatus: platformerrors.CategoryDegradado.HTTPStatus(),
			wantBody:   `{"codigo":"degradado","mensaje":"bff: ObtenerViaje: fallo de transporte aguas abajo"}` + "\n",
		},
		{
			name:       "error fuera de la taxonomía: genérico, sin texto interno",
			err:        causaInterna,
			wantStatus: http.StatusInternalServerError,
			wantBody:   `{"codigo":"desconocido","mensaje":"error interno"}` + "\n",
		},
		{
			name:       "409 aguas abajo con código de máquina: conserva el código (PR #69)",
			err:        errores.DeRespuesta("TransicionarViaje", http.StatusConflict, "cierre_anticipado_requiere_incidencia"),
			wantStatus: http.StatusConflict,
			wantBody:   `{"codigo":"cierre_anticipado_requiere_incidencia","mensaje":"bff: TransicionarViaje: conflicto aguas abajo"}` + "\n",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			rec := httptest.NewRecorder()
			escribirError(rec, tt.err)
			if rec.Code != tt.wantStatus {
				t.Errorf("status: got %d, want %d", rec.Code, tt.wantStatus)
			}
			if got := rec.Body.String(); got != tt.wantBody {
				t.Errorf("cuerpo:\n got %q\nwant %q", got, tt.wantBody)
			}
		})
	}
}
