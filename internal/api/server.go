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
	r.GET("/agent-binary", s.serveAgentBinary)

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

		a := v1.Group("")
		a.Use(requireAuth(s.deps.JWTSecret))
		a.Use(auditMiddleware(s.deps.PG))

		a.GET("/auth/me", s.me)

		// Resellers
		a.GET("/resellers", s.listResellers)
		a.POST("/resellers", s.createReseller)
		a.PATCH("/resellers/:id", s.patchReseller)
		a.DELETE("/resellers/:id", s.deleteReseller)

		// Clients
		a.GET("/clients", s.listClients)
		a.POST("/clients", s.createClient)
		a.GET("/clients/:id", s.getClientDetail)
		a.PATCH("/clients/:id", s.patchClient)
		a.DELETE("/clients/:id", s.deleteClient)
		a.GET("/clients/:id/dialer-ips", s.listDialerIPs)
		a.POST("/clients/:id/dialer-ips", s.addDialerIP)
		a.DELETE("/clients/:id/dialer-ips/:dialer_ip_id", s.removeDialerIP)
		a.POST("/clients/:id/signaling-ip", s.assignSignalingIP)
		a.DELETE("/clients/:id/signaling-ip", s.unassignSignalingIP)

		// Nodes
		a.GET("/nodes", s.listNodes)
		a.POST("/nodes", s.createNode)
		a.PATCH("/nodes/:id", s.patchNode)
		a.DELETE("/nodes/:id", s.deleteNode)
		a.POST("/nodes/:id/drain", s.drainNode)
		a.POST("/nodes/:id/undrain", s.undrainNode)
		a.GET("/nodes/:id/metrics", s.nodeMetrics)
		a.POST("/nodes/:id/provision", s.provisionNode)
		a.GET("/nodes/:id/commands", s.listNodeCommands)
		a.POST("/nodes/:id/commands", s.createNodeCommand)

		// IP pool
		a.GET("/node-ips", s.listNodeIPs)
		a.POST("/node-ips", s.createNodeIP)
		a.POST("/node-ips/bulk", s.bulkCreateNodeIPs)
		a.PATCH("/node-ips/:id", s.patchNodeIP)
		a.DELETE("/node-ips/:id", s.deleteNodeIP)

		// Signaling IPs
		a.GET("/signaling-ips", s.listSignalingIPs)
		a.POST("/signaling-ips", s.createSignalingIP)
		a.PATCH("/signaling-ips/:id", s.patchSignalingIP)
		a.DELETE("/signaling-ips/:id", s.deleteSignalingIP)

		// Carriers
		a.GET("/carriers", s.listCarriers)
		a.POST("/carriers", s.createCarrier)
		a.PATCH("/carriers/:id", s.patchCarrier)
		a.DELETE("/carriers/:id", s.deleteCarrier)
		a.GET("/carriers/:id/node-history", s.carrierHistory)

		// IP groups
		a.GET("/ip-groups", s.listIPGroups)
		a.POST("/ip-groups", s.createIPGroup)
		a.PATCH("/ip-groups/:id", s.patchIPGroup)
		a.DELETE("/ip-groups/:id", s.deleteIPGroup)
		a.GET("/ip-groups/:id/members", s.listIPGroupMembers)
		a.POST("/ip-groups/:id/members", s.addIPGroupMember)
		a.DELETE("/ip-groups/:id/members/:ip_id", s.removeIPGroupMember)

		// Routes
		a.GET("/routes", s.listRoutes)
		a.POST("/routes", s.createRoute)
		a.PATCH("/routes/:id", s.patchRoute)
		a.DELETE("/routes/:id", s.deleteRoute)

		// Assignments
		a.GET("/assignments", s.listAssignments)
		a.POST("/assignments", s.createAssignment)
		a.DELETE("/assignments/:id", s.endAssignment)

		// CDRs + active calls
		a.GET("/cdrs", s.listCDRs)
		a.GET("/cdrs/stats", s.cdrStats)
		a.GET("/calls/active", s.listActiveCalls)

		// Admin users
		a.GET("/admin-users", s.listAdminUsers)
		a.POST("/admin-users", s.createAdminUser)
		a.PATCH("/admin-users/:id", s.patchAdminUser)
		a.DELETE("/admin-users/:id", s.deleteAdminUser)

		// External API integrations (SignalWire, FreeSWITCH, ...)
		a.GET("/integrations", s.listIntegrations)
		a.POST("/integrations", s.createIntegration)
		a.PATCH("/integrations/:id", s.patchIntegration)
		a.DELETE("/integrations/:id", s.deleteIntegration)
		a.POST("/integrations/:id/verify", s.verifyIntegration)

		// Audit log
		a.GET("/audit", s.listAudit)
	}
	return r
}
