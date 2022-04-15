package server

import (
	"fmt"
	"html/template"
	"log"
	"net/http"

	"github.com/devict/job-board/pkg/config"
	"github.com/devict/job-board/pkg/data"
	"github.com/gin-contrib/multitemplate"
	"github.com/gin-contrib/sessions"
	"github.com/gin-contrib/sessions/cookie"
	"github.com/gin-gonic/gin"
	"github.com/jmoiron/sqlx"
)

func NewServer(c config.Config, db *sqlx.DB) (http.Server, error) {
	gin.SetMode(c.Env)
	gin.DefaultWriter = log.Writer()

	router := gin.Default()

	if err := router.SetTrustedProxies(nil); err != nil {
		return http.Server{}, fmt.Errorf("failed to SetTrustedProxies: %w", err)
	}

	sessionOpts := sessions.Options{
		Path:     "/",
		MaxAge:   24 * 60, // 1 day
		Secure:   c.Env != "debug",
		HttpOnly: true,
		SameSite: http.SameSiteStrictMode,
	}

	sessionStore := cookie.NewStore([]byte(c.AppSecret))
	sessionStore.Options(sessionOpts)
	router.Use(sessions.Sessions("mysession", sessionStore))

	router.Static("/assets", "assets")
	router.HTMLRender = renderer()

	ctrl := &Controller{DB: db, Config: c}
	router.GET("/", ctrl.Index)
	router.GET("/new", ctrl.NewJob)
	router.POST("/jobs", ctrl.CreateJob)
	router.GET("/jobs/:id", ctrl.ViewJob)

	authorized := router.Group("/")
	authorized.Use(requireAuth(db, c.AppSecret))
	{
		authorized.GET("/jobs/:id/edit", ctrl.EditJob)
		authorized.POST("/jobs/:id", ctrl.UpdateJob)
	}

	return http.Server{
		Addr:    c.Port,
		Handler: router,
	}, nil

}

func renderer() multitemplate.Renderer {
	funcMap := template.FuncMap{
		"formatAsDate":          formatAsDate,
		"formatAsRfc3339String": formatAsRfc3339String,
	}

	r := multitemplate.NewRenderer()
	r.AddFromFilesFuncs("index", funcMap, "./templates/base.html", "./templates/index.html")
	r.AddFromFilesFuncs("new", funcMap, "./templates/base.html", "./templates/new.html")
	r.AddFromFilesFuncs("edit", funcMap, "./templates/base.html", "./templates/edit.html")
	r.AddFromFilesFuncs("view", funcMap, "./templates/base.html", "./templates/view.html")

	return r
}

func requireAuth(db *sqlx.DB, secret string) func(*gin.Context) {
	return func(ctx *gin.Context) {
		jobID := ctx.Param("id")
		job, err := data.GetJob(jobID, db)
		if err != nil {
			log.Println(fmt.Errorf("requireAuth failed to getJob: %w", err))
			ctx.AbortWithStatus(http.StatusInternalServerError)
			return
		}

		token := ctx.Query("token")
		expected := signatureForJob(job, secret)

		if token != expected {
			ctx.AbortWithStatus(403)
			return
		}
	}
}