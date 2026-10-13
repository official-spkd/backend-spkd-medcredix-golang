package router

import (
	"fmt"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"

	"github.com/spkd/medcredix/internal/auth"
	"github.com/spkd/medcredix/internal/config"
	"github.com/spkd/medcredix/internal/controller"
	"github.com/spkd/medcredix/internal/middleware"
)

// NewRouter merakit semua endpoint.
func NewRouter(db *gorm.DB) (*gin.Engine, error) {
	r := gin.New()
	r.Use(gin.Logger(), gin.Recovery())
	if err := r.SetTrustedProxies(config.TrustedProxies()); err != nil {
		return nil, fmt.Errorf("TRUSTED_PROXIES tidak valid: %w", err)
	}
	r.Use(middleware.SecurityHeaders(), middleware.CorrelationID(), middleware.CORS(), middleware.BodyLimit(1<<20))

	r.GET("/", controller.Home)

	api := r.Group("/api")
	api.GET("/health", controller.Health)
	api.POST("/auth", controller.Login(db)) // satu-satunya endpoint tanpa login

	s := api.Group("", middleware.Auth(db), middleware.RefreshScoreCache())
	view := middleware.RequirePermission(auth.PermView)

	s.POST("/auth/logout", controller.Logout(db))

	// Baca (izin VIEW, semua terisolasi per RS)
	s.GET("/bootstrap", view, controller.GetBootstrap(db))
	s.GET("/dashboard", view, controller.GetDashboard(db))
	s.GET("/scores", view, controller.GetScores(db))
	s.GET("/admin", view, controller.GetAdmin(db))
	s.GET("/risks", view, controller.GetRisks(db))
	s.GET("/warnings", view, controller.GetWarnings(db))
	s.GET("/actions", view, controller.GetActions(db))
	s.GET("/practitioners", view, controller.GetPractitioners(db))
	s.GET("/credentials", view, controller.GetCredentials(db))
	s.GET("/requirements", view, controller.GetRequirements(db))
	s.GET("/requirements/file", view, controller.GetRequirementFile(db))
	s.GET("/documents", view, controller.GetDocuments(db))
	s.GET("/documents/file", view, controller.GetDocumentFile(db))
	s.GET("/claims", view, controller.GetClaims(db))
	s.GET("/clinical", view, controller.GetClinical(db))
	s.GET("/compliance", view, controller.GetCompliance(db))
	s.GET("/evidence", view, controller.GetEvidence(db))
	s.GET("/audit", view, controller.GetAudit(db))
	s.GET("/search", view, controller.Search(db))
	s.GET("/reports", view, controller.GetReports(db))
	s.GET("/settings", view, controller.GetSettings(db))
	s.GET("/data", view, controller.GetData(db))

	// Ubah (izin per aksi dicek di dalam handler)
	s.PATCH("/risks", controller.PatchRisks(db))
	s.PATCH("/warnings", controller.PatchWarnings(db))
	s.POST("/actions", controller.PostActions(db))
	s.PATCH("/actions", controller.PatchActions(db))
	s.PATCH("/credentials", controller.PatchCredentials(db))
	s.PATCH("/requirements", controller.PatchRequirements(db))
	s.POST("/requirements/upload", controller.UploadRequirementFile(db))
	s.POST("/documents/upload", controller.UploadDocument(db))
	s.PATCH("/claims", controller.PatchClaims(db))
	s.POST("/reports", controller.PostReports(db))
	s.PATCH("/settings", controller.PatchSettings(db))
	s.POST("/data", middleware.RequirePermission(auth.PermConfigure), controller.PostData(db))
	s.POST("/scores/snapshot", middleware.RequirePermission(auth.PermConfigure), controller.PostScoreSnapshot(db))

	return r, nil
}
