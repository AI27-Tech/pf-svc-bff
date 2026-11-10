package http

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	bffclient "github.com/AI27-Tech/pf-pkg-contracts/packages/api-client-go/bff"
	catclient "github.com/AI27-Tech/pf-pkg-contracts/packages/api-client-go/catalogos"
	idclient "github.com/AI27-Tech/pf-pkg-contracts/packages/api-client-go/identidad"
	"github.com/AI27-Tech/pf-svc-bff/internal/agregacion"
	"github.com/google/uuid"
)

// TestGobiernoConservaCodigoPorHTTP: con identidad real por HTTP (servidor de
// prueba), el 403 `autoescalamiento` y los 409 de gobierno llegan al panel con
// su codigo, y el Bearer del usuario viaja a identidad (resuelve el actor).
func TestGobiernoConservaCodigoPorHTTP(t *testing.T) {
	sujeto := uuid.NewString()
	casos := []struct {
		status                       int
		codigo, metodo, ruta, cuerpo string
		handler                      func(*agregacion.Handlers) http.HandlerFunc
	}{
		{http.StatusForbidden, "autoescalamiento", http.MethodPut, "/api/v1/sujetos/" + sujeto + "/rol", `{"rol":"supervisor"}`, func(h *agregacion.Handlers) http.HandlerFunc { return h.FijarRolSujeto }},
		{http.StatusConflict, "rol_cruza_audiencia", http.MethodPut, "/api/v1/sujetos/" + sujeto + "/rol", `{"rol":"cliente"}`, func(h *agregacion.Handlers) http.HandlerFunc { return h.FijarRolSujeto }},
		{http.StatusConflict, "ultimo_administrador", http.MethodDelete, "/api/v1/sujetos/" + sujeto, "", func(h *agregacion.Handlers) http.HandlerFunc { return h.DesactivarSujeto }},
		{http.StatusConflict, "rol_sin_alcance", http.MethodPut, "/api/v1/sujetos/" + sujeto + "/alcance/unidades", `{"unidades_ids":[]}`, func(h *agregacion.Handlers) http.HandlerFunc { return h.FijarUnidadesSujeto }},
		{http.StatusConflict, "subordinado_con_superior", http.MethodPut, "/api/v1/sujetos/" + sujeto + "/alcance/subordinados", `{"sujetos_ids":[]}`, func(h *agregacion.Handlers) http.HandlerFunc { return h.FijarSubordinadosSujeto }},
	}
	for _, c := range casos {
		t.Run(c.codigo, func(t *testing.T) {
			var ruta, authz string
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				ruta, authz = r.Method+" "+r.URL.Path, r.Header.Get("Authorization")
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(c.status)
				_, _ = io.WriteString(w, `{"codigo":"`+c.codigo+`","mensaje":"identidad: regla de gobierno"}`)
			}))
			defer srv.Close()
			id, err := NewIdentidadHTTP(srv.URL, srv.Client())
			if err != nil {
				t.Fatalf("NewIdentidadHTTP: %v", err)
			}
			h := agregacion.NewHandlers(agregacion.Deps{Identidad: id})
			rec := servirBff(c.handler(h), c.metodo, c.ruta, c.cuerpo, "administrador", sujeto)
			if ruta != c.metodo+" "+c.ruta || authz != "Bearer tok-roster" {
				t.Errorf("identidad recibió %q con Authorization %q", ruta, authz)
			}
			var e bffclient.Error
			if err := json.Unmarshal(rec.Body.Bytes(), &e); err != nil {
				t.Fatal(err)
			}
			if rec.Code != c.status || e.Codigo != c.codigo {
				t.Errorf("status %d codigo %q; se esperaba %d %q", rec.Code, e.Codigo, c.status, c.codigo)
			}
		})
	}
}

// TestAltasIdempotentesPorHTTP: crearCliente y crearTenant distinguen el 201
// (creado) del 200 (ya existía) por el status de la respuesta.
func TestAltasIdempotentesPorHTTP(t *testing.T) {
	for _, status := range []int{http.StatusCreated, http.StatusOK} {
		org := uuid.New()
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(status)
			if strings.Contains(r.URL.Path, "/tenants/") {
				_, _ = io.WriteString(w, `{"id":"`+org.String()+`","nombre":"ACME","tipo":"cliente","activa":true,"usuarios_activos":0}`)
				return
			}
			_, _ = io.WriteString(w, `{"id":"`+org.String()+`","nombre":"ACME","estado":"activo","contrato_vigente_id":null}`)
		}))
		cat, _ := NewCatalogosHTTP(srv.URL, srv.Client())
		id, _ := NewIdentidadHTTP(srv.URL, srv.Client())
		cl, creadoCl, err := cat.CrearCliente(context.Background(), "Bearer t", org, catclient.ClienteEntrada{Nombre: "ACME", Correo: "a@acme.mx", Telefono: "55"})
		if err != nil || cl.Id != org || creadoCl != (status == http.StatusCreated) {
			t.Errorf("CrearCliente %d: creado %v, err %v", status, creadoCl, err)
		}
		tn, creadoT, err := id.CrearTenant(context.Background(), org, idclient.TenantEntrada{Nombre: "ACME"})
		if err != nil || tn.Id != org || creadoT != (status == http.StatusCreated) {
			t.Errorf("CrearTenant %d: creado %v, err %v", status, creadoT, err)
		}
		srv.Close()
	}
}
