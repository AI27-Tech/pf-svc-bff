package agregacion

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	bffclient "github.com/AI27-Tech/pf-pkg-contracts/packages/api-client-go/bff"
	opclient "github.com/AI27-Tech/pf-pkg-contracts/packages/api-client-go/operacion"
	repclient "github.com/AI27-Tech/pf-pkg-contracts/packages/api-client-go/reportes"
	"github.com/AI27-Tech/pf-pkg-contracts/packages/api-client-go/roles"
	"github.com/AI27-Tech/pf-pkg-platform-go/auth"
	platformerrors "github.com/AI27-Tech/pf-pkg-platform-go/errors"
	"github.com/AI27-Tech/pf-svc-bff/internal/tokenctx"
	"github.com/google/uuid"
)

// --- Dashboard y evidencia del monitoreo (E8-T17) ---------------------------

var monitoreoAdjunto = uuid.MustParse("33333333-3333-3333-3333-333333333333")

// ListarAdjuntos, CrearIntencionSubidaAdjunto y DescargarAdjunto completan
// OperacionPort en el doble. Registran la llamada con el mismo mecanismo del
// flujo interno y capturan el Bearer que el handler dejó en el ctx.
func (f *fakeOperacion) ListarAdjuntos(ctx context.Context, id opclient.Id) (*opclient.PaginaAdjuntos, error) {
	f.bearerRecibido = tokenctx.De(ctx)
	if err := f.flujo("ListarAdjuntos", id, opclient.Id{}, nil); err != nil {
		return nil, err
	}
	return f.adjuntos, nil
}

func (f *fakeOperacion) CrearIntencionSubidaAdjunto(ctx context.Context, id opclient.Id, body opclient.IntencionSubidaAdjuntoEntrada) (*opclient.IntencionSubidaAdjunto, error) {
	f.bearerRecibido = tokenctx.De(ctx)
	if err := f.flujo("CrearIntencionSubidaAdjunto", id, opclient.Id{}, body); err != nil {
		return nil, err
	}
	return f.intencion, nil
}

func (f *fakeOperacion) DescargarAdjunto(ctx context.Context, id, aid opclient.Id) (*opclient.DescargaAdjunto, error) {
	f.bearerRecibido = tokenctx.De(ctx)
	if err := f.flujo("DescargarAdjunto", id, aid, nil); err != nil {
		return nil, err
	}
	return f.descargaAdj, nil
}

// fakeReportes es el doble de ReportesPort: registra el periodo y el Bearer que
// recibe y devuelve respuestas o un error fijos.
type fakeReportes struct {
	viajes  *repclient.ReporteViajes
	estados *repclient.ReporteEstados
	err     error

	llamado string
	desde   time.Time
	hasta   time.Time
	bearer  string
}

func (f *fakeReportes) ReporteViajes(ctx context.Context, p repclient.ReporteViajesParams) (*repclient.ReporteViajes, error) {
	f.llamado, f.desde, f.hasta, f.bearer = "ReporteViajes", p.Desde, p.Hasta, tokenctx.De(ctx)
	return f.viajes, f.err
}

func (f *fakeReportes) ReporteEstados(ctx context.Context, p repclient.ReporteEstadosParams) (*repclient.ReporteEstados, error) {
	f.llamado, f.desde, f.hasta, f.bearer = "ReporteEstados", p.Desde, p.Hasta, tokenctx.De(ctx)
	return f.estados, f.err
}

// fakeMonitoreo devuelve los dos puertos con una respuesta para cada operación.
func fakeMonitoreo() (*fakeOperacion, *fakeReportes) {
	ahora := time.Date(2026, 11, 3, 12, 0, 0, 0, time.UTC)
	stale := false
	op := &fakeOperacion{
		adjuntos: &opclient.PaginaAdjuntos{Items: []opclient.Adjunto{{
			Id: monitoreoAdjunto, ViajeId: flujoViaje, Nombre: "acta.pdf", TipoMedio: "application/pdf",
			TamanoBytes: 2048, EstadoEscaneo: opclient.AdjuntoEstadoEscaneo("limpio"), CreadoEn: ahora,
		}}},
		intencion:   &opclient.IntencionSubidaAdjunto{AdjuntoId: monitoreoAdjunto, UrlSubida: "https://minio.local/subida?firma=x", ExpiraEn: ahora},
		descargaAdj: &opclient.DescargaAdjunto{UrlDescarga: "https://minio.local/descarga?firma=y", ExpiraEn: ahora},
	}
	rep := &fakeReportes{
		viajes: &repclient.ReporteViajes{
			Filas:        []repclient.FilaViaje{{ViajeId: flujoViaje, Estado: "EN_TRANSITO"}},
			Total:        1,
			ProyeccionAl: &ahora,
			Stale:        &stale,
		},
		estados: &repclient.ReporteEstados{
			Conteos:      []repclient.ConteoEstado{{Estado: "EN_TRANSITO", Conteo: 1}},
			ProyeccionAl: &ahora,
			Stale:        &stale,
		},
	}
	return op, rep
}

// claimsDeRol arma los claims de un rol con su audiencia de dominio (cliente es
// Cliente; los otros siete roles son Interno).
func claimsDeRol(rol roles.Rol) *auth.Claims {
	aud := auth.AudienciaInterno
	if rol == roles.RolCliente {
		aud = auth.AudienciaCliente
	}
	return &auth.Claims{SesionID: "s", Subject: "u", Rol: string(rol), Audiencia: aud}
}

// peticionMonitoreo sirve la petición con {id} y {aid} fijos, el Bearer y los
// claims dados (nil = petición sin claims).
func peticionMonitoreo(t *testing.T, handler http.HandlerFunc, metodo, url string, cuerpo any, claims *auth.Claims) *httptest.ResponseRecorder {
	t.Helper()
	var raw []byte
	if cuerpo != nil {
		var err error
		if raw, err = json.Marshal(cuerpo); err != nil {
			t.Fatalf("marshal cuerpo: %v", err)
		}
	}
	req := httptest.NewRequest(metodo, url, bytes.NewReader(raw))
	req.Header.Set("Authorization", "Bearer tok-panel")
	req.SetPathValue("id", flujoViaje.String())
	req.SetPathValue("aid", monitoreoAdjunto.String())
	if claims != nil {
		req = req.WithContext(auth.ContextWithClaims(req.Context(), claims))
	}
	rec := httptest.NewRecorder()
	handler(rec, req)
	return rec
}

const periodoQuery = "?desde=2026-11-01T00:00:00Z&hasta=2026-11-08T00:00:00Z"

// casoMonitoreo es una de las cinco operaciones: su handler, su petición, el
// status de éxito y cómo saber si tocó su puerto.
type casoMonitoreo struct {
	op      string
	handler func(*Handlers) http.HandlerFunc
	metodo  string
	url     string
	cuerpo  any
	status  int
	tocado  func(*fakeOperacion, *fakeReportes) bool
}

func casosMonitoreo() []casoMonitoreo {
	base := "/api/v1/viajes/" + flujoViaje.String() + "/adjuntos"
	opTocada := func(f *fakeOperacion, _ *fakeReportes) bool { return f.flujoLlamado != "" }
	repTocada := func(_ *fakeOperacion, r *fakeReportes) bool { return r.llamado != "" }
	return []casoMonitoreo{
		{"reporteViajes", func(h *Handlers) http.HandlerFunc { return h.ReporteViajes }, http.MethodGet, "/api/v1/reportes/viajes" + periodoQuery, nil, http.StatusOK, repTocada},
		{"reporteEstados", func(h *Handlers) http.HandlerFunc { return h.ReporteEstados }, http.MethodGet, "/api/v1/reportes/estados" + periodoQuery, nil, http.StatusOK, repTocada},
		{"listarAdjuntos", func(h *Handlers) http.HandlerFunc { return h.ListarAdjuntos }, http.MethodGet, base, nil, http.StatusOK, opTocada},
		{
			"crearIntencionSubidaAdjunto", func(h *Handlers) http.HandlerFunc { return h.CrearIntencionSubidaAdjunto },
			http.MethodPost, base + "/intenciones-subida",
			bffclient.IntencionSubidaAdjuntoEntrada{Nombre: "acta.pdf", TipoMedio: "application/pdf", TamanoBytes: 2048, ChecksumSha256: "a1b2c3d4e5f60718293a4b5c6d7e8f90a1b2c3d4e5f60718293a4b5c6d7e8f90"},
			http.StatusCreated, opTocada,
		},
		{"descargarAdjunto", func(h *Handlers) http.HandlerFunc { return h.DescargarAdjunto }, http.MethodGet, base + "/" + monitoreoAdjunto.String() + "/descarga", nil, http.StatusOK, opTocada},
	}
}

// TestMonitoreoRolesDeMonitoreoReenvian: monitorista, supervisor, coordinador y
// administrador pasan la guarda y la llamada llega a su puerto.
func TestMonitoreoRolesDeMonitoreoReenvian(t *testing.T) {
	permitidos := []roles.Rol{roles.RolMonitorista, roles.RolSupervisor, roles.RolCoordinador, roles.RolAdministrador}
	for _, rol := range permitidos {
		for _, c := range casosMonitoreo() {
			t.Run(string(rol)+"/"+c.op, func(t *testing.T) {
				op, rep := fakeMonitoreo()
				h := NewHandlers(Deps{Operacion: op, Reportes: rep})
				rec := peticionMonitoreo(t, c.handler(h), c.metodo, c.url, c.cuerpo, claimsDeRol(rol))
				if rec.Code != c.status {
					t.Fatalf("status = %d, se esperaba %d; cuerpo: %s", rec.Code, c.status, rec.Body.String())
				}
				if !c.tocado(op, rep) {
					t.Errorf("no llegó al puerto")
				}
			})
		}
	}
}

// TestMonitoreoOtrosRolesSon403: jefe_operativo, ejecutivo, reaccion y cliente
// reciben 403 antes de tocar el puerto (DEC-022 #9, DEC-060).
func TestMonitoreoOtrosRolesSon403(t *testing.T) {
	negados := []roles.Rol{roles.RolJefeOperativo, roles.RolEjecutivo, roles.RolReaccion, roles.RolCliente}
	for _, rol := range negados {
		for _, c := range casosMonitoreo() {
			t.Run(string(rol)+"/"+c.op, func(t *testing.T) {
				op, rep := fakeMonitoreo()
				h := NewHandlers(Deps{Operacion: op, Reportes: rep})
				rec := peticionMonitoreo(t, c.handler(h), c.metodo, c.url, c.cuerpo, claimsDeRol(rol))
				if rec.Code != http.StatusForbidden {
					t.Fatalf("status = %d, se esperaba 403; cuerpo: %s", rec.Code, rec.Body.String())
				}
				if c.tocado(op, rep) {
					t.Errorf("no debió tocar el puerto con el rol %s", rol)
				}
			})
		}
	}
}

// TestMonitoreoSinClaimsEs401: sin claims no se llega al puerto.
func TestMonitoreoSinClaimsEs401(t *testing.T) {
	for _, c := range casosMonitoreo() {
		t.Run(c.op, func(t *testing.T) {
			op, rep := fakeMonitoreo()
			h := NewHandlers(Deps{Operacion: op, Reportes: rep})
			rec := peticionMonitoreo(t, c.handler(h), c.metodo, c.url, c.cuerpo, nil)
			if rec.Code != http.StatusUnauthorized {
				t.Fatalf("status = %d, se esperaba 401; cuerpo: %s", rec.Code, rec.Body.String())
			}
			if c.tocado(op, rep) {
				t.Errorf("no debió tocar el puerto sin claims")
			}
		})
	}
}

// TestReportesConservanFrescura: proyeccion_al y stale llegan al panel sin
// recortar (DEC-033). Con ambos presentes salen tal cual; con ambos ausentes,
// proyeccion_al sale null y stale no sale (frescura no informada, P4).
func TestReportesConservanFrescura(t *testing.T) {
	monitorista := claimsDeRol(roles.RolMonitorista)
	url := "/api/v1/reportes/viajes" + periodoQuery

	t.Run("presentes", func(t *testing.T) {
		op, rep := fakeMonitoreo()
		stale := true
		rep.viajes.Stale = &stale
		rec := peticionMonitoreo(t, NewHandlers(Deps{Operacion: op, Reportes: rep}).ReporteViajes, http.MethodGet, url, nil, monitorista)
		var got map[string]any
		decodificarCuerpo(t, rec, &got)
		if got["proyeccion_al"] != "2026-11-03T12:00:00Z" {
			t.Errorf("proyeccion_al = %v, se esperaba 2026-11-03T12:00:00Z", got["proyeccion_al"])
		}
		if got["stale"] != true {
			t.Errorf("stale = %v, se esperaba true", got["stale"])
		}
	})

	t.Run("ausentes", func(t *testing.T) {
		op, rep := fakeMonitoreo()
		rep.viajes.ProyeccionAl, rep.viajes.Stale = nil, nil
		rec := peticionMonitoreo(t, NewHandlers(Deps{Operacion: op, Reportes: rep}).ReporteViajes, http.MethodGet, url, nil, monitorista)
		var got map[string]any
		decodificarCuerpo(t, rec, &got)
		if v, ok := got["proyeccion_al"]; !ok || v != nil {
			t.Errorf("proyeccion_al = %v (presente=%v), se esperaba null", v, ok)
		}
		if _, ok := got["stale"]; ok {
			t.Errorf("stale presente = %v, se esperaba ausente", got["stale"])
		}
	})

	t.Run("estados", func(t *testing.T) {
		op, rep := fakeMonitoreo()
		rec := peticionMonitoreo(t, NewHandlers(Deps{Operacion: op, Reportes: rep}).ReporteEstados, http.MethodGet, "/api/v1/reportes/estados"+periodoQuery, nil, monitorista)
		var got map[string]any
		decodificarCuerpo(t, rec, &got)
		if got["proyeccion_al"] == nil || got["stale"] != false {
			t.Errorf("estados = %v, se esperaban proyeccion_al y stale=false", got)
		}
	})
}

// TestReportesReenvianPeriodoYBearer: el puerto recibe el periodo parseado y el
// Bearer del usuario.
func TestReportesReenvianPeriodoYBearer(t *testing.T) {
	op, rep := fakeMonitoreo()
	rec := peticionMonitoreo(t, NewHandlers(Deps{Operacion: op, Reportes: rep}).ReporteViajes, http.MethodGet,
		"/api/v1/reportes/viajes"+periodoQuery, nil, claimsDeRol(roles.RolSupervisor))
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d; cuerpo: %s", rec.Code, rec.Body.String())
	}
	if want := time.Date(2026, 11, 1, 0, 0, 0, 0, time.UTC); !rep.desde.Equal(want) {
		t.Errorf("desde = %v, se esperaba %v", rep.desde, want)
	}
	if want := time.Date(2026, 11, 8, 0, 0, 0, 0, time.UTC); !rep.hasta.Equal(want) {
		t.Errorf("hasta = %v, se esperaba %v", rep.hasta, want)
	}
	if rep.bearer != "Bearer tok-panel" {
		t.Errorf("bearer = %q, se esperaba Bearer tok-panel", rep.bearer)
	}
}

// TestReportesPeriodoInvalidoEs400: desde/hasta ausentes o mal formados son 400
// y no llegan al puerto.
func TestReportesPeriodoInvalidoEs400(t *testing.T) {
	consultas := map[string]string{
		"sin periodo": "",
		"sin hasta":   "?desde=2026-11-01T00:00:00Z",
		"desde malo":  "?desde=ayer&hasta=2026-11-08T00:00:00Z",
		"hasta malo":  "?desde=2026-11-01T00:00:00Z&hasta=2026-11-08",
	}
	for nombre, q := range consultas {
		t.Run(nombre, func(t *testing.T) {
			op, rep := fakeMonitoreo()
			rec := peticionMonitoreo(t, NewHandlers(Deps{Operacion: op, Reportes: rep}).ReporteViajes, http.MethodGet,
				"/api/v1/reportes/viajes"+q, nil, claimsDeRol(roles.RolMonitorista))
			if rec.Code != http.StatusBadRequest {
				t.Fatalf("status = %d, se esperaba 400; cuerpo: %s", rec.Code, rec.Body.String())
			}
			if rep.llamado != "" {
				t.Errorf("no debió llamar a reportes con un periodo inválido")
			}
		})
	}
}

// TestIntencionSubidaReenviaCuerpoYResponde201: el cuerpo (con nombre) llega a
// operacion y la respuesta 201 sale intacta, con la URL de vida corta.
func TestIntencionSubidaReenviaCuerpoYResponde201(t *testing.T) {
	op, rep := fakeMonitoreo()
	entrada := bffclient.IntencionSubidaAdjuntoEntrada{
		Nombre: "acta.pdf", TipoMedio: "application/pdf", TamanoBytes: 2048,
		ChecksumSha256: "a1b2c3d4e5f60718293a4b5c6d7e8f90a1b2c3d4e5f60718293a4b5c6d7e8f90",
	}
	rec := peticionMonitoreo(t, NewHandlers(Deps{Operacion: op, Reportes: rep}).CrearIntencionSubidaAdjunto, http.MethodPost,
		"/api/v1/viajes/"+flujoViaje.String()+"/adjuntos/intenciones-subida", entrada, claimsDeRol(roles.RolMonitorista))
	if rec.Code != http.StatusCreated {
		t.Fatalf("status = %d, se esperaba 201; cuerpo: %s", rec.Code, rec.Body.String())
	}
	if got := op.flujoRecibio.(opclient.IntencionSubidaAdjuntoEntrada); got.Nombre != "acta.pdf" || got.TamanoBytes != 2048 || got.ChecksumSha256 != entrada.ChecksumSha256 {
		t.Errorf("cuerpo reenviado = %+v, se esperaba %+v", got, entrada)
	}
	var got bffclient.IntencionSubidaAdjunto
	decodificarCuerpo(t, rec, &got)
	if got.AdjuntoId != monitoreoAdjunto || got.UrlSubida != "https://minio.local/subida?firma=x" {
		t.Errorf("respuesta = %+v, se esperaba la intención de operacion sin cambios", got)
	}
	if op.bearerRecibido != "Bearer tok-panel" {
		t.Errorf("bearer = %q, se esperaba Bearer tok-panel", op.bearerRecibido)
	}
}

// TestAdjuntosEntradasInvalidasSon400: cuerpo ilegible o ids que no son UUID son
// 400 y no llegan al puerto.
func TestAdjuntosEntradasInvalidasSon400(t *testing.T) {
	monitorista := claimsDeRol(roles.RolMonitorista)

	t.Run("cuerpo invalido", func(t *testing.T) {
		op, rep := fakeMonitoreo()
		req := httptest.NewRequest(http.MethodPost, "/", bytes.NewReader([]byte("{no es json")))
		req.SetPathValue("id", flujoViaje.String())
		req = req.WithContext(auth.ContextWithClaims(req.Context(), monitorista))
		rec := httptest.NewRecorder()
		NewHandlers(Deps{Operacion: op, Reportes: rep}).CrearIntencionSubidaAdjunto(rec, req)
		if rec.Code != http.StatusBadRequest || op.flujoLlamado != "" {
			t.Fatalf("status = %d (llamó %q), se esperaba 400 sin llamar", rec.Code, op.flujoLlamado)
		}
	})

	for _, pv := range []struct{ nombre, id, aid string }{
		{"id de viaje", "no-uuid", monitoreoAdjunto.String()},
		{"id de adjunto", flujoViaje.String(), "no-uuid"},
	} {
		t.Run(pv.nombre, func(t *testing.T) {
			op, rep := fakeMonitoreo()
			req := httptest.NewRequest(http.MethodGet, "/", nil)
			req.SetPathValue("id", pv.id)
			req.SetPathValue("aid", pv.aid)
			req = req.WithContext(auth.ContextWithClaims(req.Context(), monitorista))
			rec := httptest.NewRecorder()
			NewHandlers(Deps{Operacion: op, Reportes: rep}).DescargarAdjunto(rec, req)
			if rec.Code != http.StatusBadRequest || op.flujoLlamado != "" {
				t.Fatalf("status = %d (llamó %q), se esperaba 400 sin llamar", rec.Code, op.flujoLlamado)
			}
		})
	}
}

// TestMonitoreoPropagaErroresAguasAbajo: el 403 (fuera de alcance) y el 404 de
// operacion o reportes llegan tal cual al panel.
func TestMonitoreoPropagaErroresAguasAbajo(t *testing.T) {
	errores := map[string]struct {
		err    error
		status int
	}{
		"fuera de alcance": {platformerrors.NewProhibido("fuera de alcance"), http.StatusForbidden},
		"no encontrado":    {platformerrors.NewNoEncontrado("viaje inexistente"), http.StatusNotFound},
	}
	monitorista := claimsDeRol(roles.RolMonitorista)
	for nombre, e := range errores {
		t.Run("adjuntos/"+nombre, func(t *testing.T) {
			op, rep := fakeMonitoreo()
			op.flujoErr = e.err
			rec := peticionMonitoreo(t, NewHandlers(Deps{Operacion: op, Reportes: rep}).ListarAdjuntos, http.MethodGet,
				"/api/v1/viajes/"+flujoViaje.String()+"/adjuntos", nil, monitorista)
			if rec.Code != e.status {
				t.Fatalf("status = %d, se esperaba %d; cuerpo: %s", rec.Code, e.status, rec.Body.String())
			}
		})
		t.Run("reportes/"+nombre, func(t *testing.T) {
			op, rep := fakeMonitoreo()
			rep.viajes, rep.err = nil, e.err
			rec := peticionMonitoreo(t, NewHandlers(Deps{Operacion: op, Reportes: rep}).ReporteViajes, http.MethodGet,
				"/api/v1/reportes/viajes"+periodoQuery, nil, monitorista)
			if rec.Code != e.status {
				t.Fatalf("status = %d, se esperaba %d; cuerpo: %s", rec.Code, e.status, rec.Body.String())
			}
		})
	}
}
