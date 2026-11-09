package http

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	bffclient "github.com/AI27-Tech/pf-pkg-contracts/packages/api-client-go/bff"
	"github.com/AI27-Tech/pf-pkg-platform-go/auth"
	"github.com/AI27-Tech/pf-svc-bff/internal/agregacion"
	"github.com/google/uuid"
)

// servirBff sirve una petición con el handler dado y los claims del rol, como
// lo haría el mux tras el middleware.
func servirBff(handler http.HandlerFunc, metodo, url, cuerpo, rol string, id string) *httptest.ResponseRecorder {
	req := httptest.NewRequest(metodo, url, strings.NewReader(cuerpo))
	req.Header.Set("Authorization", "Bearer tok-roster")
	if id != "" {
		req.SetPathValue("id", id)
	}
	req = req.WithContext(auth.ContextWithClaims(req.Context(), &auth.Claims{SesionID: "s", Subject: "u", Rol: rol, Audiencia: auth.AudienciaInterno}))
	rec := httptest.NewRecorder()
	handler(rec, req)
	return rec
}

// TestUnidades409ConservanCodigoPorHTTP: con operacion real por HTTP (servidor
// de prueba), cada 409 del roster llega al llamante del bff con su codigo de
// máquina; el adaptador no lo aplana al genérico `conflicto`.
func TestUnidades409ConservanCodigoPorHTTP(t *testing.T) {
	unidad := uuid.New().String()
	casos := []struct {
		codigo, metodo, ruta, cuerpo string
		handler                      func(*agregacion.Handlers) http.HandlerFunc
		id                           string
	}{
		{"vehiculo_en_otra_unidad", http.MethodPost, "/api/v1/unidades", `{"vehiculo_id":"` + uuid.NewString() + `","custodios_ids":["` + uuid.NewString() + `"]}`, func(h *agregacion.Handlers) http.HandlerFunc { return h.CrearUnidad }, ""},
		{"custodio_en_otra_unidad", http.MethodPost, "/api/v1/unidades", `{"vehiculo_id":"` + uuid.NewString() + `","custodios_ids":["` + uuid.NewString() + `"]}`, func(h *agregacion.Handlers) http.HandlerFunc { return h.CrearUnidad }, ""},
		{"custodio_en_otra_unidad", http.MethodPatch, "/api/v1/unidades/" + unidad, `{"activa":true}`, func(h *agregacion.Handlers) http.HandlerFunc { return h.ModificarUnidad }, unidad},
		{"unidad_con_reserva_vigente", http.MethodPatch, "/api/v1/unidades/" + unidad, `{"activa":false}`, func(h *agregacion.Handlers) http.HandlerFunc { return h.ModificarUnidad }, unidad},
	}
	for _, c := range casos {
		t.Run(c.metodo+" "+c.codigo, func(t *testing.T) {
			var ruta, auth string
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				ruta, auth = r.Method+" "+r.URL.Path, r.Header.Get("Authorization")
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(http.StatusConflict)
				_, _ = io.WriteString(w, `{"codigo":"`+c.codigo+`","mensaje":"operacion: choque del roster"}`)
			}))
			defer srv.Close()
			op, err := NewOperacionHTTP(srv.URL, srv.Client())
			if err != nil {
				t.Fatalf("NewOperacionHTTP: %v", err)
			}
			h := agregacion.NewHandlers(agregacion.Deps{Operacion: op})

			rec := servirBff(c.handler(h), c.metodo, c.ruta, c.cuerpo, "jefe_operativo", c.id)
			if rec.Code != http.StatusConflict {
				t.Fatalf("status = %d, se esperaba 409; cuerpo: %s", rec.Code, rec.Body)
			}
			var got bffclient.Error
			if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
				t.Fatalf("decodificar: %v", err)
			}
			if got.Codigo != c.codigo {
				t.Errorf("codigo = %q, se esperaba %q", got.Codigo, c.codigo)
			}
			if ruta != c.metodo+" "+c.ruta || auth != "Bearer tok-roster" {
				t.Errorf("aguas abajo: %q con Authorization %q", ruta, auth)
			}
		})
	}
}

// TestCrearVehiculoNoReenviaClienteId: la flota es de AI27 (DEC-143). Aunque el
// llamante mande cliente_id, el cuerpo que llega a catalogos por HTTP no lo
// lleva; el resto del alta viaja completo.
func TestCrearVehiculoNoReenviaClienteId(t *testing.T) {
	var recibido map[string]any
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewDecoder(r.Body).Decode(&recibido) // doble de prueba: un cuerpo ilegible deja el mapa vacío y la prueba falla
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusCreated)
		_, _ = io.WriteString(w, `{"id":"`+uuid.NewString()+`","version":1,"tipo":"coche","economico":"P-102","estado":"activa","vigencia_hasta":null}`)
	}))
	defer srv.Close()
	cat, err := NewCatalogosHTTP(srv.URL, srv.Client())
	if err != nil {
		t.Fatalf("NewCatalogosHTTP: %v", err)
	}
	h := agregacion.NewHandlers(agregacion.Deps{Catalogos: cat})

	rec := servirBff(h.CrearVehiculo, http.MethodPost, "/api/v1/vehiculos",
		`{"tipo":"coche","economico":"P-102","placas":"XYZ-9","estado":"activa","cliente_id":"`+uuid.NewString()+`"}`, "administrador", "")
	if rec.Code != http.StatusCreated {
		t.Fatalf("status = %d, se esperaba 201; cuerpo: %s", rec.Code, rec.Body)
	}
	if _, ok := recibido["cliente_id"]; ok {
		t.Errorf("cliente_id llegó a catalogos: %v", recibido)
	}
	if recibido["tipo"] != "coche" || recibido["economico"] != "P-102" || recibido["placas"] != "XYZ-9" || recibido["estado"] != "activa" {
		t.Errorf("cuerpo aguas abajo = %v", recibido)
	}
}
