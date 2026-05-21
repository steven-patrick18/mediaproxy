package api

import (
	"errors"
	"net/http"

	"mediaproxy/internal/router"

	"github.com/gin-gonic/gin"
)

// GET /api/v1/route?src_ip=...&dnis=...
//
// Called by Kamailio (via http_async_client) on every INVITE. Returns the
// full routing decision so Kamailio can:
//   - set $fs (force-send-socket) to the client's signaling IP
//   - rewrite SDP media to the chosen media IP via rtpengine
//   - relay the INVITE to the carrier
//
// Auth: any authenticated request works — admin JWT OR agent token. Agents
// are the only legitimate callers in practice, but admins call it from
// the panel for the "test-route" diagnostic.
func (s *Server) routeResolve(c *gin.Context) {
	src := c.Query("src_ip")
	dnis := c.Query("dnis")
	if src == "" || dnis == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "src_ip and dnis are required"})
		return
	}
	dec, err := router.Resolve(c.Request.Context(), s.deps.PG, s.deps.Redis, src, dnis)
	if err != nil {
		var re *router.Error
		if errors.As(err, &re) {
			c.JSON(http.StatusOK, gin.H{"error": re.Message, "code": re.Code})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, dec)
}
