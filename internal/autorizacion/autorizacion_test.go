package autorizacion

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/AI27-Tech/pf-pkg-platform-go/auth"
)

// TestProyeccionMarcarEsIdempotente verifica P13: marcar el mismo sesion_id dos
// veces no cambia el resultado.
func TestProyeccionMarcarEsIdempotente(t *testing.T) {
	p := NuevaProyeccion()
	const sesion = "s-1"
	if p.Revocada(context.Background(), sesion) {
		t.Fatal("una sesion desconocida no debe estar revocada")
	}
	p.Marcar(sesion)
	p.Marcar(sesion) // idempotente
	if !p.Revocada(context.Background(), sesion) {
		t.Error("tras Marcar, la sesion debe constar como revocada")
	}
}

// TestProyeccionZeroValueNoRompe verifica el zero value util: una Proyeccion sin
// construir no entra en panico y responde que nada esta revocado todavia.
func TestProyeccionZeroValueNoRompe(t *testing.T) {
	var p Proyeccion
	if p.Revocada(context.Background(), "cualquiera") {
		t.Error("el zero value no conoce revocaciones")
	}
}

// TestMiddlewareRechazaSesionRevocada verifica el camino de revocacion de ADR
// 26: una ruta protegida consulta los tombstones y rechaza el token cuya sesion
// fue revocada, aunque el token verifique su firma. El handler protegido no debe
// ejecutarse.
func TestMiddlewareRechazaSesionRevocada(t *testing.T) {
	const sesionRevocada = "s-revocada"
	proy := NuevaProyeccion()
	proy.Marcar(sesionRevocada)

	verif := verificadorFijo{claims: &auth.Claims{SesionID: sesionRevocada, Subject: "u-1"}}
	mw := Middleware(verif, proy)

	alcanzado := false
	protegido := mw(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		alcanzado = true
		w.WriteHeader(http.StatusOK)
	}))

	req := httptest.NewRequest(http.MethodGet, "/api/v1/viajes", nil)
	req.Header.Set("Authorization", "Bearer token-valido")
	rec := httptest.NewRecorder()
	protegido.ServeHTTP(rec, req)

	if alcanzado {
		t.Error("el handler protegido NO debe ejecutarse con sesion revocada (ADR 26)")
	}
	if rec.Code == http.StatusOK {
		t.Errorf("se esperaba rechazo, no %d", rec.Code)
	}
}

// TestMiddlewareAceptaSesionViva verifica el camino feliz: con firma valida y
// sesion no revocada, el handler protegido se ejecuta y recibe las claims.
func TestMiddlewareAceptaSesionViva(t *testing.T) {
	verif := verificadorFijo{claims: &auth.Claims{SesionID: "s-viva", Subject: "u-2"}}
	mw := Middleware(verif, NuevaProyeccion())

	var claimsVistas *auth.Claims
	protegido := mw(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		claimsVistas = ClaimsDe(r.Context())
		w.WriteHeader(http.StatusOK)
	}))

	req := httptest.NewRequest(http.MethodGet, "/api/v1/viajes", nil)
	req.Header.Set("Authorization", "Bearer token-valido")
	rec := httptest.NewRecorder()
	protegido.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("sesion viva debe aceptarse, dio %d", rec.Code)
	}
	if claimsVistas == nil || claimsVistas.Subject != "u-2" {
		t.Error("el handler debe recibir las claims verificadas en el contexto")
	}
}

// verificadorFijo es un auth.Authenticator de prueba: acepta cualquier token y
// devuelve claims fijas, o un error si claims es nil.
type verificadorFijo struct {
	claims *auth.Claims
}

func (v verificadorFijo) VerifyToken(_ context.Context, _ string) (*auth.Claims, error) {
	if v.claims == nil {
		return nil, errors.New("token invalido")
	}
	return v.claims, nil
}
