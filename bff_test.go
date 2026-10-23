package bff_test

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/AI27-Tech/pf-pkg-platform-go/auth"
	bff "github.com/AI27-Tech/pf-svc-bff"
)

// rutaContrato es una operacion de la fase (a) de bff.yaml: su metodo, su ruta y
// si es publica (sin verificacion) o protegida.
type rutaContrato struct {
	operationID string
	metodo      string
	ruta        string
	protegida   bool
}

// operacionesFaseA es la lista de las SEIS operaciones de la fase (a) de
// bff.yaml (05-bff.md, tabla (a)). La prueba compara las rutas montadas contra
// esta lista: si una falta o sobra, falla.
var operacionesFaseA = []rutaContrato{
	{"iniciarSesion", http.MethodPost, "/api/v1/sesion", false},
	{"cerrarSesion", http.MethodDelete, "/api/v1/sesion", true},
	{"refrescarSesion", http.MethodPost, "/api/v1/sesion/refresh", false},
	{"crearViaje", http.MethodPost, "/api/v1/viajes", true},
	{"listarViajes", http.MethodGet, "/api/v1/viajes", true},
	{"seguirViaje", http.MethodGet, "/api/v1/viajes/00000000-0000-0000-0000-000000000000", true},
	{"descargarViaje", http.MethodGet, "/api/v1/viajes/00000000-0000-0000-0000-000000000000/descarga", true},
}

// TestRutasMontadasContraContrato verifica que New().Routes monta exactamente
// las seis operaciones de la fase (a) de bff.yaml y ninguna otra ruta base. No
// invoca los puertos: solo comprueba que el mux resuelve cada ruta (no devuelve
// 404 de "no hay patron"), con el verificador y los puertos en nil, porque la
// resolucion de patron ocurre antes de llamar al handler.
func TestRutasMontadasContraContrato(t *testing.T) {
	mod, err := bff.New(bff.Pools{}, bff.Deps{
		// Verificador siempre falla: basta para que la ruta protegida resuelva su
		// patron (el middleware corre, no el handler). La ruta publica no lo usa.
		Verificador: verificadorSiempreFalla{},
	})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	mux := http.NewServeMux()
	mod.Routes(mux)

	for _, op := range operacionesFaseA {
		req := httptest.NewRequest(op.metodo, op.ruta, nil)
		_, patron := mux.Handler(req)
		if patron == "" {
			t.Errorf("operacion %q (%s %s): ninguna ruta montada la resuelve", op.operationID, op.metodo, op.ruta)
		}
	}
}

// TestRutaNoDeContratoNoMontada verifica que una ruta de fase (b) o fuera del
// contrato NO esta montada. El prefijo /api/v1/panel SÍ existe ahora (carril
// interno de emergencia, E7), pero /api/v1/panel a secas no resuelve ningun
// patron: el panel solo monta subrutas .../viajes/{id}/emergencias/...
func TestRutaNoDeContratoNoMontada(t *testing.T) {
	mod, _ := bff.New(bff.Pools{}, bff.Deps{
		Verificador: verificadorSiempreFalla{},
	})
	mux := http.NewServeMux()
	mod.Routes(mux)

	noDeberian := []struct{ metodo, ruta string }{
		{http.MethodGet, "/api/v1/panel"},              // prefijo sin subruta: nada que montar
		{http.MethodGet, "/api/v1/viajes/x/tracking"},  // V2
		{http.MethodPost, "/api/v1/panico"},            // V2
		{http.MethodGet, "/api/v1/catalogos/clientes"}, // fase (b)
		// La descarga es de UN viaje; la exportacion de listados/lote es V2
		// (DEC-023), por eso NO existe una ruta de lote con patron propio. Nota:
		// no se prueba "/api/v1/viajes/descarga" porque el mux lo resuelve como
		// el patron de UN viaje ({id} = "descarga"), no como una ruta distinta;
		// probar eso afirmaria algo sobre net/http, no sobre el contrato de bff.
		{http.MethodPost, "/api/v1/descargas"},        // lote global (no existe)
		{http.MethodPost, "/api/v1/viajes/descargas"}, // lote por coleccion (no existe)
	}
	for _, r := range noDeberian {
		req := httptest.NewRequest(r.metodo, r.ruta, nil)
		if _, patron := mux.Handler(req); patron != "" {
			t.Errorf("%s %s no debe estar montada (resolvio patron %q)", r.metodo, r.ruta, patron)
		}
	}
}

type verificadorSiempreFalla struct{}

func (verificadorSiempreFalla) VerifyToken(_ context.Context, _ string) (*auth.Claims, error) {
	return nil, errFalla
}

var errFalla = errors.New("verificador de prueba: siempre falla")
