package api

import (
	"github.com/gin-gonic/gin"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/redis/go-redis/v9"
)

type Deps struct {
	PG    *pgxpool.Pool
	Redis *redis.Client
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
		v1.GET("/resellers", s.listResellers)
		v1.GET("/clients", s.listClients)
	}
	return r
}
