package agregacion

import (
	"context"
	"encoding/json"
	"net/http"
	"testing"

	bffclient "github.com/AI27-Tech/pf-pkg-contracts/packages/api-client-go/bff"
	catclient "github.com/AI27-Tech/pf-pkg-contracts/packages/api-client-go/catalogos"
	opclient "github.com/AI27-Tech/pf-pkg-contracts/packages/api-client-go/operacion"
	"github.com/AI27-Tech/pf-pkg-platform-go/auth"
	platformerrors "github.com/AI27-Tech/pf-pkg-platform-go/errors"
	"github.com/AI27-Tech/pf-svc-bff/internal/tokenctx"
	"github.com/google/uuid"
)

// Métodos del grupo 2 del flujo interno en el doble de operacion. Registran la
// llamada con el mismo mecanismo (flujo) y capturan el Bearer del ctx.

func (f *fakeOperacion) ListarUnidades(ctx context.Context, params opclient.ListarUnidadesParams) (*opclient.PaginaUnidades, error) {
	f.bearerRecibido = tokenctx.De(ctx)
	// Sin {id} de viaje: registra flujoViaje para que pase la aserción común de id.
	if err := f.flujo("ListarUnidades", flujoViaje, opclient.Id{}, params); err != nil {
		return nil, err
	}
	return f.unidades, nil
}

func (f *fakeOperacion) CompletarFechaHora(_ context.Context, id opclient.Id, body opclient.FechaHoraEntrada) (*opclient.Viaje, error) {
	if err := f.flujo("CompletarFechaHora", id, opclient.Id{}, body); err != nil {
		return nil, err
	}
	return f.viaje, nil
}

func (f *fakeOperacion) ListarIncidencias(_ context.Context, id opclient.Id, params opclient.ListarIncidenciasParams) (*opclient.PaginaIncidencias, error) {
	if err := f.flujo("ListarIncidencias", id, opclient.Id{}, params); err != nil {
		return nil, err
	}
	return f.paginaIncidencias, nil
}

func (f *fakeOperacion) ObtenerFicha(_ context.Context, id opclient.Id) (*opclient.FichaTecnica, error) {
	if err := f.flujo("ObtenerFicha", id, opclient.Id{}, nil); err != nil {
		return nil, err
	}
	return f.ficha, nil
}

func (f *fakeOperacion) GuardarFicha(_ context.Context, id opclient.Id, body opclient.FichaTecnicaEntrada) (*opclient.FichaTecnica, error) {
	if err := f.flujo("GuardarFicha", id, opclient.Id{}, body); err != nil {
		return nil, err
	}
	return f.ficha, nil
}

func (f *fakeCatalogos) ListarGps(_ context.Context, bearer string, params catclient.ListarGpsParams) (*catclient.PaginaGps, error) {
	f.bearerRecibido, f.gpsRecibio = bearer, &params
	return f.gps, f.err
}

// TestGuardarFichaTractoNullBorraAusenteConserva: `"tracto": null` llega al
// puerto como &Tracto{} (borrar) y la propiedad ausente como nil (no tocar). Es
// la distinción que el tipo generado no hace solo.
func TestGuardarFichaTractoNullBorraAusenteConserva(t *testing.T) {
	url := "/api/v1/viajes/" + flujoViaje.String() + "/ficha"
	casos := []struct {
		nombre, cuerpo string
		borra          bool
	}{
		{"null borra", `{"tracto":null}`, true},
		{"ausente conserva", `{"contenedores":[{"economico":"C-1"}]}`, false},
	}
	for _, c := range casos {
		t.Run(c.nombre, func(t *testing.T) {
			f := fakeFlujo()
			rec := peticionFlujo(t, NewHandlers(Deps{Operacion: f}).GuardarFicha, http.MethodPatch, url, "", json.RawMessage(c.cuerpo), auth.AudienciaInterno)
			if rec.Code != http.StatusOK {
				t.Fatalf("status = %d; cuerpo: %s", rec.Code, rec.Body.String())
			}
			got := f.flujoRecibio.(opclient.FichaTecnicaEntrada)
			if c.borra && (got.Tracto == nil || *got.Tracto != (opclient.Tracto{})) {
				t.Errorf("tracto = %+v, se esperaba &Tracto{} (borrar)", got.Tracto)
			}
			if !c.borra && got.Tracto != nil {
				t.Errorf("tracto = %+v, se esperaba nil (no tocar)", got.Tracto)
			}
		})
	}
}

// TestGuardarFichaCampoDesconocidoEs400: additionalProperties: false, igual que
// operacion; el puerto no se toca.
func TestGuardarFichaCampoDesconocidoEs400(t *testing.T) {
	f := fakeFlujo()
	rec := peticionFlujo(t, NewHandlers(Deps{Operacion: f}).GuardarFicha, http.MethodPatch,
		"/api/v1/viajes/"+flujoViaje.String()+"/ficha", "", json.RawMessage(`{"tracto":{"placas":"X"},"color":"rojo"}`), auth.AudienciaInterno)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, se esperaba 400; cuerpo: %s", rec.Code, rec.Body.String())
	}
	if f.flujoLlamado != "" {
		t.Errorf("no debió reenviar un cuerpo inválido (llamó %q)", f.flujoLlamado)
	}
}

// TestCompletarFechaHoraPropagaViajeTerminal: el 409 viaje_en_estado_terminal
// de operacion llega con su codigo y sin recomponer el detalle.
func TestCompletarFechaHoraPropagaViajeTerminal(t *testing.T) {
	f := fakeFlujo()
	f.flujoErr = platformerrors.NewConflictoCodigo("viaje_en_estado_terminal", "bff: CompletarFechaHora: conflicto aguas abajo")
	rec := peticionFlujo(t, NewHandlers(Deps{Operacion: f}).CompletarFechaHora, http.MethodPatch,
		"/api/v1/viajes/"+flujoViaje.String()+"/fecha-hora", "", json.RawMessage(`{"fecha_hora_servicio":"2026-11-05T10:00:00Z"}`), auth.AudienciaInterno)
	if rec.Code != http.StatusConflict {
		t.Fatalf("status = %d, se esperaba 409; cuerpo: %s", rec.Code, rec.Body.String())
	}
	var got bffclient.Error
	decodificarCuerpo(t, rec, &got)
	if got.Codigo != "viaje_en_estado_terminal" {
		t.Errorf("codigo = %q, se esperaba viaje_en_estado_terminal", got.Codigo)
	}
	if f.obtenerRecibio != uuid.Nil {
		t.Errorf("no debió recomponer el detalle tras el 409")
	}
}

// TestListarGps: audiencia Interna reenvía el cursor y el Bearer y devuelve la
// página tal cual; un Cliente recibe 403 sin tocar el puerto.
func TestListarGps(t *testing.T) {
	gps := uuid.New()
	pagina := &catclient.PaginaGps{Items: []catclient.DispositivoGps{{Id: gps, Identificador: "MOVIL ST7916", Activo: true}}}

	t.Run("interna reenvía", func(t *testing.T) {
		f := &fakeCatalogos{gps: pagina}
		rec := peticionFlujo(t, NewHandlers(Deps{Catalogos: f}).ListarGps, http.MethodGet, "/api/v1/gps?cursor=c-6", "", nil, auth.AudienciaInterno)
		if rec.Code != http.StatusOK {
			t.Fatalf("status = %d; cuerpo: %s", rec.Code, rec.Body.String())
		}
		if f.gpsRecibio == nil || f.gpsRecibio.Cursor == nil || *f.gpsRecibio.Cursor != "c-6" || f.bearerRecibido != "Bearer tok-panel" {
			t.Errorf("params %+v bearer %q", f.gpsRecibio, f.bearerRecibido)
		}
		var got bffclient.PaginaGps
		decodificarCuerpo(t, rec, &got)
		if len(got.Items) != 1 || got.Items[0].Id != gps || got.Items[0].Identificador != "MOVIL ST7916" {
			t.Errorf("página = %+v", got)
		}
	})
	t.Run("cliente 403", func(t *testing.T) {
		f := &fakeCatalogos{gps: pagina}
		rec := peticionFlujo(t, NewHandlers(Deps{Catalogos: f}).ListarGps, http.MethodGet, "/api/v1/gps", "", nil, auth.AudienciaCliente)
		if rec.Code != http.StatusForbidden {
			t.Fatalf("status = %d, se esperaba 403", rec.Code)
		}
		if f.gpsRecibio != nil {
			t.Errorf("no debió reenviar con audiencia cliente")
		}
	})
}
