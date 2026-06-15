package web

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
)

func TestNormalizeRouteBasePath(t *testing.T) {
	cases := []struct {
		in   string
		want string
	}{
		{"", ""},
		{"/", ""},
		{"abc", "/abc"},
		{"/abc", "/abc"},
		{"/abc/", "/abc"},
		{"///abc///", "/abc"},
	}

	for _, tc := range cases {
		if got := normalizeRouteBasePath(tc.in); got != tc.want {
			t.Fatalf("normalizeRouteBasePath(%q) = %q, want %q", tc.in, got, tc.want)
		}
	}
}

func TestGroupedBasePathSupportsRootAndPanel(t *testing.T) {
	gin.SetMode(gin.TestMode)
	engine := gin.New()
	g := engine.Group(normalizeRouteBasePath("/abc/"))
	g.GET("/", func(c *gin.Context) {
		c.String(http.StatusOK, "root")
	})
	g.GET("/panel/", func(c *gin.Context) {
		c.String(http.StatusOK, "panel")
	})

	req := httptest.NewRequest(http.MethodGet, "/abc", nil)
	rec := httptest.NewRecorder()
	engine.ServeHTTP(rec, req)
	if rec.Code != http.StatusMovedPermanently {
		t.Fatalf("/abc status = %d, want %d", rec.Code, http.StatusMovedPermanently)
	}
	if loc := rec.Header().Get("Location"); loc != "/abc/" {
		t.Fatalf("/abc redirect = %q, want /abc/", loc)
	}

	for _, path := range []string{"/abc/", "/abc/panel/"} {
		req = httptest.NewRequest(http.MethodGet, path, nil)
		rec = httptest.NewRecorder()
		engine.ServeHTTP(rec, req)
		if rec.Code != http.StatusOK {
			t.Fatalf("%s status = %d, want %d", path, rec.Code, http.StatusOK)
		}
	}
}
