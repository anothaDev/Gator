package routes

import (
	"github.com/gin-gonic/gin"
	"github.com/raul/gator/internal/handlers"
)

func Register(
	r *gin.Engine,
	setup *handlers.SetupHandler,
	opnsense *handlers.OPNsenseHandler,
	vpn *handlers.VPNHandler,
	gateway *handlers.GatewayHandler,
	appRouting *handlers.AppRoutingHandler,
	ipRanges *handlers.IPRangesHandler,
	tunnels *handlers.TunnelHandler,
) {
	api := r.Group("/api")

	registerSetupRoutes(api, setup)
	registerOPNsenseRoutes(api, setup, opnsense, vpn, gateway, appRouting)
	registerPfSenseRoutes(api, setup)
	registerVPNRoutes(api, vpn)
	registerAppRoutingRoutes(api, appRouting)
	registerIPRangesRoutes(api, ipRanges)
	registerTunnelRoutes(api, tunnels)
}
