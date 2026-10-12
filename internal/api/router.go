package api

import (
	"fmt"
	"net/http"
	"os"
	"regexp"
	"strings"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"

	"github.com/spkd/medcredix/internal/audit"
	"github.com/spkd/medcredix/internal/auth"
	"github.com/spkd/medcredix/internal/model"
)

// ---------- helper respons ({ok, data} / {ok:false, error}) ----------

func ok(c *gin.Context, data interface{}) {
	c.JSON(http.StatusOK, gin.H{"ok": true, "data": data})
}

func fail(c *gin.Context, status int, msg string) {
	c.AbortWithStatusJSON(status, gin.H{"ok": false, "error": msg})
}

// ---------- middleware ----------

// Correlation ID dari klien: maksimal 64 karakter, tanpa spasi/baris baru.
var correlationIDPattern = regexp.MustCompile(`^[A-Za-z0-9._:-]{1,64}$`)

func correlationID() gin.HandlerFunc {
	return func(c *gin.Context) {
		id := c.GetHeader("X-Correlation-Id")
		if !correlationIDPattern.MatchString(id) {
			id = model.NewID("cor")
		}
		c.Set("correlation_id", id)
		c.Header("X-Correlation-Id", id)
		c.Next()
	}
}

// allowedOrigins dari CORS_ORIGIN (pisahkan dengan koma).
func allowedOrigins() map[string]bool {
	raw := os.Getenv("CORS_ORIGIN")
	if raw == "" {
		raw = "http://localhost:3000" // frontend Vite (dev)
	}
	out := map[string]bool{}
	for _, o := range strings.Split(raw, ",") {
		if o = strings.TrimRight(strings.TrimSpace(o), "/"); o != "" && o != "*" {
			out[o] = true
		}
	}
	return out
}

// cors hanya mengizinkan origin yang ada di daftar.
func cors() gin.HandlerFunc {
	allowed := allowedOrigins()
	return func(c *gin.Context) {
		origin := c.GetHeader("Origin")
		c.Header("Vary", "Origin")
		if origin != "" && allowed[origin] {
			c.Header("Access-Control-Allow-Origin", origin)
			c.Header("Access-Control-Allow-Headers", "Authorization, Content-Type, X-Correlation-Id")
			c.Header("Access-Control-Allow-Methods", "GET, POST, PATCH, DELETE, OPTIONS")
			c.Header("Access-Control-Expose-Headers", "X-Correlation-Id")
			c.Header("Access-Control-Max-Age", "600")
		}
		if c.Request.Method == http.MethodOptions {
			c.AbortWithStatus(http.StatusNoContent)
			return
		}
		c.Next()
	}
}

// securityHeaders: header keamanan dasar.
func securityHeaders() gin.HandlerFunc {
	return func(c *gin.Context) {
		h := c.Writer.Header()
		h.Set("X-Content-Type-Options", "nosniff")
		h.Set("X-Frame-Options", "DENY")
		h.Set("Referrer-Policy", "no-referrer")
		h.Set("Content-Security-Policy", "default-src 'none'; frame-ancestors 'none'")
		h.Set("Cache-Control", "no-store")
		if c.Request.TLS != nil || c.GetHeader("X-Forwarded-Proto") == "https" {
			h.Set("Strict-Transport-Security", "max-age=31536000; includeSubDomains")
		}
		c.Next()
	}
}

// maxBody membatasi ukuran body request.
func maxBody(n int64) gin.HandlerFunc {
	return func(c *gin.Context) {
		if c.Request.Body != nil {
			c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, n)
		}
		c.Next()
	}
}

// trustedProxies dari TRUSTED_PROXIES. Kosong = tidak percaya proxy mana pun.
func trustedProxies() []string {
	var out []string
	for _, p := range strings.Split(os.Getenv("TRUSTED_PROXIES"), ",") {
		if p = strings.TrimSpace(p); p != "" {
			out = append(out, p)
		}
	}
	return out
}

func metaOf(c *gin.Context) audit.Meta {
	cid, _ := c.Get("correlation_id")
	s, _ := cid.(string)
	return audit.Meta{IP: c.ClientIP(), CorrelationID: s}
}

// tenantID = ResolveHospitalID + pastikan RS yang diminta benar-benar ada.
func tenantID(c *gin.Context, db *gorm.DB) (model.User, string, bool) {
	user, _ := auth.CurrentUser(c)
	hid, err := auth.ResolveHospitalID(user, c.Query("hospitalId"))
	if err != nil {
		code := http.StatusForbidden
		if err == auth.ErrNoHospital {
			code = http.StatusBadRequest
		}
		fail(c, code, err.Error())
		return user, "", false
	}
	var n int64
	if err := db.Model(&model.Hospital{}).Where("id = ?", hid).Count(&n).Error; err != nil || n == 0 {
		fail(c, http.StatusNotFound, "Rumah sakit tidak ditemukan.")
		return user, "", false
	}
	return user, hid, true
}

// refreshIntelOnWrite membuang cache skor setelah request yang berhasil mengubah data,
// supaya dashboard langsung menampilkan angka terbaru.
func refreshIntelOnWrite() gin.HandlerFunc {
	return func(c *gin.Context) {
		c.Next()
		if c.Request.Method != http.MethodGet && c.Writer.Status() < 400 {
			invalidateIntel()
		}
	}
}

// NewRouter merakit semua endpoint.
func NewRouter(db *gorm.DB) (*gin.Engine, error) {
	r := gin.New()
	r.Use(gin.Logger(), gin.Recovery())
	if err := r.SetTrustedProxies(trustedProxies()); err != nil {
		return nil, fmt.Errorf("TRUSTED_PROXIES tidak valid: %w", err)
	}
	r.Use(securityHeaders(), correlationID(), cors(), maxBody(1<<20))

	r.GET("/", func(c *gin.Context) {
		ok(c, gin.H{"app": "MedCredix API", "health": "/api/health"})
	})

	api := r.Group("/api")
	api.GET("/health", func(c *gin.Context) { ok(c, gin.H{"status": "up"}) })
	api.POST("/auth", loginHandler(db)) // satu-satunya endpoint tanpa Wrap

	s := api.Group("", auth.Wrap(db), refreshIntelOnWrite())
	view := auth.RequirePermission(auth.PermView)

	s.POST("/auth/logout", logoutHandler(db))

	// Baca (izin VIEW, semua terisolasi per RS)
	s.GET("/bootstrap", view, bootstrapHandler(db))
	s.GET("/dashboard", view, dashboardHandler(db))
	s.GET("/scores", view, scoresHandler(db))
	s.GET("/admin", view, adminHandler(db))
	s.GET("/risks", view, risksGetHandler(db))
	s.GET("/warnings", view, warningsGetHandler(db))
	s.GET("/actions", view, actionsGetHandler(db))
	s.GET("/practitioners", view, practitionersGetHandler(db))
	s.GET("/credentials", view, credentialsGetHandler(db))
	s.GET("/requirements", view, requirementsGetHandler(db))
	s.GET("/claims", view, claimsGetHandler(db))
	s.GET("/clinical", view, clinicalGetHandler(db))
	s.GET("/compliance", view, complianceGetHandler(db))
	s.GET("/evidence", view, evidenceGetHandler(db))
	s.GET("/audit", view, auditGetHandler(db))
	s.GET("/search", view, searchGetHandler(db))
	s.GET("/reports", view, reportsGetHandler(db))
	s.GET("/settings", view, settingsGetHandler(db))
	s.GET("/data", view, dataGetHandler(db))

	// Ubah (izin per aksi dicek di dalam handler)
	s.PATCH("/risks", risksPatchHandler(db))
	s.PATCH("/warnings", warningsPatchHandler(db))
	s.POST("/actions", actionsPostHandler(db))
	s.PATCH("/actions", actionsPatchHandler(db))
	s.PATCH("/credentials", credentialsPatchHandler(db))
	s.PATCH("/requirements", requirementsPatchHandler(db))
	s.PATCH("/claims", claimsPatchHandler(db))
	s.POST("/reports", reportsPostHandler(db))
	s.PATCH("/settings", settingsPatchHandler(db))
	s.POST("/data", auth.RequirePermission(auth.PermConfigure), dataPostHandler(db))
	s.POST("/scores/snapshot", auth.RequirePermission(auth.PermConfigure), snapshotHandler(db))

	return r, nil
}
