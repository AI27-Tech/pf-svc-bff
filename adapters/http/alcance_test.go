package http

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	bffclient "github.com/AI27-Tech/pf-pkg-contracts/packages/api-client-go/bff"
	"github.com/AI27-Tech/pf-svc-bff/internal/agregacion"
	"github.com/google/uuid"
)

// TestListarViajesInternoPorHTTP (DEC-149 #11): con operacion y catalogos por
// HTTP, la fila interna lleva el cliente con su nombre y la escolta con su
// económico. La escolta se lee por viaje con el Bearer del usuario y los
// nombres y económicos en un lote cada uno (`?ids=`).
func TestListarViajesInternoPorHTTP(t *testing.T) {
	viaje, cliente, unidad, vehiculo := uuid.NewString(), uuid.NewString(), uuid.NewString(), uuid.NewString()
	rutas := map[string]string{}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		rutas[r.URL.Path] = r.URL.Query().Get("ids") + " " + r.Header.Get("Authorization")
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/api/v1/viajes":
			_, _ = io.WriteString(w, `{"items":[{"id":"`+viaje+`","estado":"EN_TRANSITO","cliente_id":"`+cliente+`","origen":"Manzanillo","destinos":[],"creado_en":"2026-11-17T10:00:00Z"}],"cursor_siguiente":null}`)
		case "/api/v1/viajes/" + viaje + "/asignacion":
			_, _ = io.WriteString(w, `{"viaje_id":"`+viaje+`","unidades":[{"id":"`+unidad+`","vehiculo_id":"`+vehiculo+`","custodios_ids":[],"tipo":"patrulla","activa":true,"cliente_id":null}]}`)
		case "/api/v1/clientes":
			_, _ = io.WriteString(w, `{"items":[{"id":"`+cliente+`","nombre":"Acme Logística","estado":"activo"}],"cursor_siguiente":null}`)
		case "/api/v1/vehiculos":
			_, _ = io.WriteString(w, `{"items":[{"id":"`+vehiculo+`","version":1,"tipo":"coche","economico":"P-17","estado":"activa","vigencia_hasta":null}],"cursor_siguiente":null}`)
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer srv.Close()
	op, _ := NewOperacionHTTP(srv.URL, srv.Client())
	cat, _ := NewCatalogosHTTP(srv.URL, srv.Client())
	h := agregacion.NewHandlers(agregacion.Deps{Operacion: op, Catalogos: cat})

	rec := servirBff(h.ListarViajes, http.MethodGet, "/api/v1/viajes", "", "monitorista", "")
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d: %s", rec.Code, rec.Body)
	}
	var got bffclient.PaginaViajesResumen
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil || len(got.Items) != 1 {
		t.Fatalf("cuerpo %s", rec.Body)
	}
	v := got.Items[0]
	if v.Cliente == nil || v.Cliente.Id.String() != cliente || v.Cliente.Nombre == nil || *v.Cliente.Nombre != "Acme Logística" {
		t.Errorf("cliente = %+v", v.Cliente)
	}
	if v.Unidades == nil || len(*v.Unidades) != 1 || (*v.Unidades)[0].Id.String() != unidad || *(*v.Unidades)[0].Economico != "P-17" {
		t.Errorf("unidades = %+v", v.Unidades)
	}
	want := map[string]string{
		"/api/v1/viajes": " Bearer tok-roster",
		"/api/v1/viajes/" + viaje + "/asignacion": " Bearer tok-roster",
		"/api/v1/clientes":                        cliente + " Bearer tok-roster",
		"/api/v1/vehiculos":                       vehiculo + " Bearer tok-roster",
	}
	for ruta, w := range want {
		if rutas[ruta] != w {
			t.Errorf("%s recibió %q, se esperaba %q", ruta, rutas[ruta], w)
		}
	}
}

// TestObtenerViajeInternoFueraDeAlcancePorHTTP (DEC-149): el 404 de operacion
// para un viaje fuera de alcance llega al panel como 404.
func TestObtenerViajeInternoFueraDeAlcancePorHTTP(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusNotFound)
		_, _ = io.WriteString(w, `{"codigo":"no_encontrado","mensaje":"operacion: viaje no encontrado"}`)
	}))
	defer srv.Close()
	op, _ := NewOperacionHTTP(srv.URL, srv.Client())
	id := uuid.NewString()
	rec := servirBff(agregacion.NewHandlers(agregacion.Deps{Operacion: op}).ObtenerViajeInterno, http.MethodGet, "/api/v1/viajes/"+id+"/detalle", "", "monitorista", id)
	if rec.Code != http.StatusNotFound {
		t.Errorf("status = %d, se esperaba 404", rec.Code)
	}
}
