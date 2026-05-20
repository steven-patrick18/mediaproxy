package api

import (
	"github.com/gin-gonic/gin"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/redis/go-redis/v9"
)

type Deps struct {
	PG        *pgxpool.Pool
	Redis     *redis.Client
	JWTSecret string
}

type Server struct {
	deps Deps
}

func New(d Deps) *Server {
	return &Server{deps: d}
}

func (s *Server) Router() *gin.Engine {
	gin.SetMode(gin.ReleaseMode)
	r := gin.New()
	r.Use(gin.Recovery())
	r.Use(requestLogger())

	r.GET("/healthz", s.healthz)
	r.GET("/readyz", s.readyz)

	v1 := r.Group("/api/v1")
	{
		v1.POST("/auth/login", s.login)

		authed := v1.Group("")
		authed.Use(requireAuth(s.deps.JWTSecret))
		authed.GET("/auth/me", s.me)
		authed.GET("/resellers", s.listResellers)
		authed.GET("/clients", s.listClients)
		authed.GET("/nodes", s.listNodes)
		authed.POST("/nodes", s.createNode)
	}
	return r
}
