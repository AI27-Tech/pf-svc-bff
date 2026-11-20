package http

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	msgclient "github.com/AI27-Tech/pf-pkg-contracts/packages/api-client-go/mensajeria"
	opclient "github.com/AI27-Tech/pf-pkg-contracts/packages/api-client-go/operacion"
	platformerrors "github.com/AI27-Tech/pf-pkg-platform-go/errors"
	"github.com/AI27-Tech/pf-svc-bff/internal/tokenctx"
	"github.com/google/uuid"
)

const solJSON = `{"id":"aaaaaaaa-aaaa-aaaa-aaaa-aaaaaaaaaaaa","estado":"PAGADO","origen":"CDMX","destinos":[],"marca_penalizacion":false,"avisos":[],"creado_en":"2026-11-20T10:00:00Z"}`

// Cada operación de solicitudes llama a su ruta y verbo, propaga el Bearer y
// devuelve la solicitud; un 409 conserva su `codigo`.
func TestOperacionSolicitudesRutaBearerYCodigo409(t *testing.T) {
	id := uuid.MustParse("aaaaaaaa-aaaa-aaaa-aaaa-aaaaaaaaaaaa")
	p := "/api/v1/solicitudes/" + id.String()
	llamadas := []struct {
		verbo, ruta string
		llamar      func(a *OperacionHTTP, ctx context.Context) (any, error)
	}{
		{"POST", "/api/v1/solicitudes", func(a *OperacionHTTP, c context.Context) (any, error) {
			return a.CrearSolicitud(c, opclient.CrearSolicitudEntrada{Origen: "x"})
		}},
		{"GET", p, func(a *OperacionHTTP, c context.Context) (any, error) { return a.ObtenerSolicitud(c, id) }},
		{"PUT", p + "/cotizacion", func(a *OperacionHTTP, c context.Context) (any, error) {
			return a.CotizarSolicitud(c, id, opclient.CotizacionEntrada{})
		}},
		{"PUT", p + "/fecha-hora", func(a *OperacionHTTP, c context.Context) (any, error) {
			return a.ReprogramarSolicitud(c, id, opclient.ReprogramacionEntrada{})
		}},
		{"POST", p + "/rechazo-cotizacion", func(a *OperacionHTTP, c context.Context) (any, error) {
			return a.RechazarCotizacion(c, id, opclient.AccionClienteEntrada{})
		}},
		{"POST", p + "/cancelacion", func(a *OperacionHTTP, c context.Context) (any, error) {
			return a.CancelarSolicitud(c, id, opclient.AccionClienteEntrada{})
		}},
		{"PUT", p + "/pago", func(a *OperacionHTTP, c context.Context) (any, error) {
			return a.ConfirmarPago(c, id, opclient.PagoEntrada{})
		}},
		{"POST", p + "/reversa-pago", func(a *OperacionHTTP, c context.Context) (any, error) {
			return a.RevertirPago(c, id, opclient.MotivoEntrada{Motivo: "x"})
		}},
		{"POST", p + "/aprobacion", func(a *OperacionHTTP, c context.Context) (any, error) { return a.AprobarSolicitud(c, id) }},
		{"POST", p + "/rechazo", func(a *OperacionHTTP, c context.Context) (any, error) {
			return a.RechazarSolicitud(c, id, opclient.MotivoEntrada{Motivo: "x"})
		}},
		{"GET", "/api/v1/solicitudes", func(a *OperacionHTTP, c context.Context) (any, error) {
			return a.ListarSolicitudes(c, opclient.ListarSolicitudesParams{})
		}},
		{"GET", p + "/historico", func(a *OperacionHTTP, c context.Context) (any, error) {
			return a.ListarHistoricoSolicitud(c, id)
		}},
	}
	for _, l := range llamadas {
		t.Run(l.verbo+" "+l.ruta, func(t *testing.T) {
			var verbo, ruta, auth string
			status, cuerpo := http.StatusOK, solJSON
			switch {
			case l.ruta == "/api/v1/solicitudes" && l.verbo == "GET":
				cuerpo = `{"items":[],"cursor_siguiente":null}`
			case strings.HasSuffix(l.ruta, "/historico"):
				cuerpo = `{"items":[]}`
			case l.verbo == "POST" && l.ruta == "/api/v1/solicitudes":
				status = http.StatusCreated
			}
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				verbo, ruta, auth = r.Method, r.URL.Path, r.Header.Get("Authorization")
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(status)
				_, _ = w.Write([]byte(cuerpo))
			}))
			defer srv.Close()
			a, err := NewOperacionHTTP(srv.URL, nil)
			if err != nil {
				t.Fatal(err)
			}
			ctx := tokenctx.Con(context.Background(), "Bearer t")
			if _, err := l.llamar(a, ctx); err != nil {
				t.Fatalf("error: %v", err)
			}
			if verbo != l.verbo || ruta != l.ruta || auth != "Bearer t" {
				t.Errorf("llamó %s %s con %q", verbo, ruta, auth)
			}
			// 409 con código de máquina.
			srv409 := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(http.StatusConflict)
				_, _ = w.Write([]byte(`{"codigo":"fecha_pago_fuera_de_vigencia","mensaje":"x"}`))
			}))
			defer srv409.Close()
			a2, _ := NewOperacionHTTP(srv409.URL, nil)
			if _, err := l.llamar(a2, ctx); platformerrors.CodigoDe(err) != "fecha_pago_fuera_de_vigencia" {
				t.Errorf("codigo = %q (err %v)", platformerrors.CodigoDe(err), err)
			}
		})
	}
}

// ListarMensajesSolicitud y EnviarMensajeSolicitud resuelven la conversación de
// la solicitud y usan su id; el 409 conserva su código.
func TestMensajeriaSolicitudResuelveConversacionYUsaSuId(t *testing.T) {
	sol := uuid.New()
	conv := uuid.New()
	var rutas []string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		rutas = append(rutas, r.Method+" "+r.URL.Path)
		w.Header().Set("Content-Type", "application/json")
		switch {
		case strings.HasSuffix(r.URL.Path, "/conversacion"):
			_, _ = w.Write([]byte(`{"id":"` + conv.String() + `","viaje_id":"` + sol.String() + `","cliente_id":"` + uuid.NewString() + `","estado":"abierta","abierta_en":"2026-11-20T10:00:00Z","atendida_en":null,"escritura_hasta":null,"ultima_secuencia":0,"version_reclamo":0}`))
		case r.Method == "POST":
			w.WriteHeader(http.StatusConflict)
			_, _ = w.Write([]byte(`{"codigo":"conversacion_solo_lectura","mensaje":"x"}`))
		default:
			_, _ = w.Write([]byte(`{"items":[],"hay_mas":false}`))
		}
	}))
	defer srv.Close()
	a, _ := NewMensajeriaHTTP(srv.URL, nil)
	ctx := context.Background()

	if _, err := a.ListarMensajesSolicitud(ctx, "Bearer t", sol, msgclient.ListarMensajesParams{}); err != nil {
		t.Fatal(err)
	}
	_, err := a.EnviarMensajeSolicitud(ctx, "Bearer t", sol, msgclient.EnviarMensajeEntrada{ClientMessageId: uuid.New(), Cuerpo: "x"})
	if platformerrors.CodigoDe(err) != "conversacion_solo_lectura" {
		t.Errorf("codigo = %q (err %v)", platformerrors.CodigoDe(err), err)
	}
	quieren := []string{
		"GET /api/v1/solicitudes/" + sol.String() + "/conversacion", "GET /api/v1/conversaciones/" + conv.String() + "/mensajes",
		"GET /api/v1/solicitudes/" + sol.String() + "/conversacion", "POST /api/v1/conversaciones/" + conv.String() + "/mensajes",
	}
	if strings.Join(rutas, "|") != strings.Join(quieren, "|") {
		t.Errorf("rutas = %v", rutas)
	}
}

func TestMensajeriaSolicitudObtenerYBandeja(t *testing.T) {
	var ruta string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ruta = r.URL.Path
		w.Header().Set("Content-Type", "application/json")
		if strings.HasSuffix(ruta, "/bandeja/solicitudes") {
			_, _ = w.Write([]byte(`{"items":[],"revision":3,"cursor_siguiente":null}`))
			return
		}
		w.WriteHeader(http.StatusNotFound)
		_, _ = w.Write([]byte(`{"codigo":"no_encontrado","mensaje":"x"}`))
	}))
	defer srv.Close()
	a, _ := NewMensajeriaHTTP(srv.URL, nil)
	if _, err := a.ListarTrabajoBandejaSolicitudes(context.Background(), "Bearer t", msgclient.ListarTrabajoMiBandejaSolicitudesParams{}); err != nil || ruta != "/api/v1/bandeja/solicitudes" {
		t.Errorf("bandeja: ruta %q err %v", ruta, err)
	}
	if _, err := a.ObtenerConversacionSolicitud(context.Background(), "Bearer t", uuid.New()); platformerrors.AsCategory(err) != platformerrors.CategoryNoEncontrado {
		t.Errorf("404 debe ser no encontrado: %v", err)
	}
}
