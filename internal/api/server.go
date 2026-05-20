package api

import (
	"mediaproxy/internal/sigcache"

	"github.com/gin-gonic/gin"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/redis/go-redis/v9"
)

type Deps struct {
	PG        *pgxpool.Pool
	Redis     *redis.Client
	JWTSecret string
	SigCache  *sigcache.Writer
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

	// agent endpoints use agent_token, not JWT
	agent := r.Group("/api/v1/agent")
	agent.Use(requireAgentAuth(s.deps.PG))
	{
		agent.POST("/register", s.agentRegister)
		agent.POST("/heartbeat", s.agentHeartbeat)
		agent.POST("/command-result", s.agentCommandResult)
	}

	v1 := r.Group("/api/v1")
	{
		v1.POST("/auth/login", s.login)

		authed := v1.Group("")
		authed.Use(requireAuth(s.deps.JWTSecret))
		authed.GET("/auth/me", s.me)

		authed.GET("/resellers", s.listResellers)

		authed.GET("/clients", s.listClients)
		authed.GET("/clients/:id", s.getClientDetail)
		authed.GET("/clients/:id/dialer-ips", s.listDialerIPs)
		authed.POST("/clients/:id/dialer-ips", s.addDialerIP)
		authed.DELETE("/clients/:id/dialer-ips/:dialer_ip_id", s.removeDialerIP)
		authed.POST("/clients/:id/signaling-ip", s.assignSignalingIP)
		authed.DELETE("/clients/:id/signaling-ip", s.unassignSignalingIP)

		authed.GET("/nodes", s.listNodes)
		authed.POST("/nodes", s.createNode)

		authed.GET("/signaling-ips", s.listSignalingIPs)
		authed.POST("/signaling-ips", s.createSignalingIP)
		authed.DELETE("/signaling-ips/:id", s.deleteSignalingIP)
	}
	return r
}
