package routes

import (
	"github.com/anothaDev/gator/internal/handlers"
	"github.com/gin-gonic/gin"
)

func Register(
	r *gin.Engine,
	auth *handlers.AuthHandler,
	setup *handlers.SetupHandler,
	opnsense *handlers.OPNsenseHandler,
	tailscale *handlers.TailscaleHandler,
	vpn *handlers.VPNHandler,
	gateway *handlers.GatewayHandler,
	appRouting *handlers.AppRoutingHandler,
	ipRanges *handlers.IPRangesHandler,
	tunnels *handlers.TunnelHandler,
) {
	api := r.Group("/api")

	registerAuthRoutes(api, auth)
	registerSetupRoutes(api, setup)
	registerOPNsenseRoutes(api, setup, opnsense, tailscale, vpn, gateway, appRouting)
	registerPfSenseRoutes(api, setup)
	registerVPNRoutes(api, vpn)
	registerAppRoutingRoutes(api, appRouting)
	registerIPRangesRoutes(api, ipRanges)
	registerTunnelRoutes(api, tunnels)
}
