package http

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	msgclient "github.com/AI27-Tech/pf-pkg-contracts/packages/api-client-go/mensajeria"
	platformerrors "github.com/AI27-Tech/pf-pkg-platform-go/errors"
	"github.com/google/uuid"
)

// TestMensajeriaConservaCodigoDel409: un 409 de mensajeria llega al bff con el
// código de máquina de su cuerpo (emergencia_cerrada), no como el conflicto
// genérico. Un cuerpo sin código deja "conflicto"; otro status
// conserva su categoría.
func TestMensajeriaConservaCodigoDel409(t *testing.T) {
	casos := []struct {
		nombre, cuerpo string
		status         int
		quiere         string
	}{
		{"409 con código", `{"codigo":"emergencia_cerrada","mensaje":"mensajeria: emergencia_cerrada"}`, http.StatusConflict, "emergencia_cerrada"},
		{"409 sin código", `{"mensaje":"x"}`, http.StatusConflict, "conflicto"},
		{"403 ignora el cuerpo", `{"codigo":"emergencia_cerrada"}`, http.StatusForbidden, "prohibido"},
	}
	for _, c := range casos {
		t.Run(c.nombre, func(t *testing.T) {
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(c.status)
				_, _ = w.Write([]byte(c.cuerpo))
			}))
			defer srv.Close()
			a, err := NewMensajeriaHTTP(srv.URL, nil)
			if err != nil {
				t.Fatalf("NewMensajeriaHTTP: %v", err)
			}
			err = a.AgregarNotaEmergencia(context.Background(), "Bearer t", uuid.New(), uuid.New(), msgclient.NotaEmergenciaEntrada{Cuerpo: "x"})
			if got := platformerrors.CodigoDe(err); got != c.quiere {
				t.Fatalf("codigo = %q (err %v), quiere %q", got, err, c.quiere)
			}
		})
	}
}
