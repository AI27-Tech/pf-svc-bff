package http

import (
	"bytes"
	"context"
	"io"
	"net/http"
	"testing"

	"github.com/AI27-Tech/pf-svc-bff/internal/tokenctx"
	"github.com/google/uuid"
)

// doerCaptura implementa HttpRequestDoer capturando el header Authorization del
// request saliente y devolviendo una respuesta 200 fija con un cuerpo JSON.
type doerCaptura struct {
	authRecibido string
	cuerpo       string
}

func (d *doerCaptura) Do(req *http.Request) (*http.Response, error) {
	d.authRecibido = req.Header.Get("Authorization")
	return &http.Response{
		StatusCode: http.StatusOK,
		Header:     http.Header{"Content-Type": []string{"application/json"}},
		Body:       io.NopCloser(bytes.NewBufferString(d.cuerpo)),
	}, nil
}

// TestOperacionPropagaBearerDelContexto: con un Bearer en el ctx (como lo deja
// el handler), el adaptador HTTP de operacion lo reenvía en el header
// Authorization aguas abajo. Es el arreglo del bug: antes el adaptador prometía
// propagar el token (doc de registrarDescargaViaje) pero no lo hacía.
func TestOperacionPropagaBearerDelContexto(t *testing.T) {
	doer := &doerCaptura{cuerpo: `{"items":[],"cursor_siguiente":null}`}
	ad, err := NewOperacionHTTP("http://operacion.local", doer)
	if err != nil {
		t.Fatalf("NewOperacionHTTP: %v", err)
	}

	ctx := tokenctx.Con(context.Background(), "Bearer tok-abc")
	if _, err := ad.ObtenerViaje(ctx, uuid.New()); err != nil {
		// El cuerpo no decodifica a ViajeDetalle, pero el header ya se capturó
		// antes de leer la respuesta; basta con que la llamada haya salido.
		_ = err
	}
	if doer.authRecibido != "Bearer tok-abc" {
		t.Errorf("Authorization aguas abajo = %q, want %q", doer.authRecibido, "Bearer tok-abc")
	}
}

// TestOperacionSinBearerNoAgregaCabecera: sin Bearer en el ctx, el adaptador no
// inventa una cabecera Authorization (operacion responderá 401, el adaptador no
// lo enmascara).
func TestOperacionSinBearerNoAgregaCabecera(t *testing.T) {
	doer := &doerCaptura{cuerpo: `{}`}
	ad, err := NewOperacionHTTP("http://operacion.local", doer)
	if err != nil {
		t.Fatalf("NewOperacionHTTP: %v", err)
	}

	if _, err := ad.ObtenerViaje(context.Background(), uuid.New()); err != nil {
		_ = err
	}
	if doer.authRecibido != "" {
		t.Errorf("no debía ir Authorization sin bearer, fue %q", doer.authRecibido)
	}
}

// TestIdentidadRevocarPropagaBearer: revocarSesion (logout) propaga el Bearer
// del usuario desde el ctx.
func TestIdentidadRevocarPropagaBearer(t *testing.T) {
	doer := &doerCaptura{cuerpo: ``}
	ad, err := NewIdentidadHTTP("http://identidad.local", doer)
	if err != nil {
		t.Fatalf("NewIdentidadHTTP: %v", err)
	}

	ctx := tokenctx.Con(context.Background(), "Bearer tok-logout")
	if err := ad.RevocarSesion(ctx, uuid.New()); err != nil {
		_ = err
	}
	if doer.authRecibido != "Bearer tok-logout" {
		t.Errorf("Authorization aguas abajo = %q, want %q", doer.authRecibido, "Bearer tok-logout")
	}
}
