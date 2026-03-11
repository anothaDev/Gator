package routes

import (
	"github.com/gin-gonic/gin"
	"github.com/anothaDev/gator/internal/handlers"
)

func registerOPNsenseRoutes(
	api *gin.RouterGroup,
	setup *handlers.SetupHandler,
	opnsenseHandler *handlers.OPNsenseHandler,
	vpn *handlers.VPNHandler,
	gateway *handlers.GatewayHandler,
	appRouting *handlers.AppRoutingHandler,
) {
	opnsense := api.Group("/opnsense")
	{
		opnsense.POST("/test-connection", setup.TestOPNsenseConnection)
		opnsense.GET("/overview", opnsenseHandler.Overview)
		opnsense.POST("/vpn/:id/apply", vpn.ApplyToOPNsense)
		opnsense.POST("/vpn/:id/apply-gateway", vpn.ApplyGatewayToOPNsense)
		opnsense.POST("/vpn/:id/apply-nat", vpn.ApplyNATToOPNsense)
		opnsense.POST("/vpn/:id/apply-policy-rule", vpn.ApplyPolicyRuleToOPNsense)
		opnsense.POST("/vpn/:id/source-interfaces", vpn.SetSourceInterfaces)
		opnsense.GET("/vpn/discover", gateway.DiscoverVPNs)
		opnsense.POST("/vpn/import", vpn.ImportFromOPNsense)
		opnsense.POST("/vpn/:id/activate", vpn.ActivateVPN)
		opnsense.POST("/vpn/:id/deactivate", vpn.DeactivateVPN)

		// Gateway management
		opnsense.GET("/gateways", gateway.ListGateways)
		opnsense.DELETE("/gateways/:uuid", gateway.DeleteGateway)
		opnsense.GET("/interfaces", gateway.ListInterfaces)
		opnsense.GET("/interfaces/selectable", gateway.ListSelectableInterfaces)

		// Firewall management
		opnsense.GET("/nat/mode", gateway.GetNATMode)
		opnsense.GET("/aliases", gateway.ListAliases)
		opnsense.GET("/nat-rules", gateway.ListNATRules)
		opnsense.GET("/rules", gateway.ListFilterRules)
		opnsense.DELETE("/rules/:uuid", gateway.DeleteFilterRule)
		opnsense.GET("/firewall/pending", gateway.PendingFirewall)
		opnsense.POST("/firewall/confirm", gateway.ConfirmFirewall)
		opnsense.POST("/firewall/revert", gateway.RevertFirewall)
		opnsense.POST("/firewall/detect-conflicts", gateway.DetectConflicts)
		opnsense.POST("/firewall/stale-rules", gateway.FindStaleRules)
		opnsense.POST("/firewall/adopt-rule", gateway.AdoptStaleRule)
		opnsense.DELETE("/firewall/cleanup/:uuid", gateway.DeleteNonGatorRule)

		// Per-app routing
		opnsense.GET("/vpn/:id/app-routes", appRouting.ListAppRoutes)
		opnsense.POST("/vpn/:id/app-routes/:appId/enable", appRouting.EnableAppRoute)
		opnsense.POST("/vpn/:id/app-routes/:appId/disable", appRouting.DisableAppRoute)
		opnsense.POST("/vpn/:id/routing-mode", appRouting.SetRoutingMode)

		// Migration assistant
		opnsense.GET("/migration/status", gateway.MigrationStatus)
		opnsense.GET("/migration/download", gateway.MigrationDownload)
		opnsense.POST("/migration/upload", gateway.MigrationUpload)
		opnsense.POST("/migration/apply", gateway.MigrationApply)
		opnsense.POST("/migration/confirm", gateway.MigrationConfirm)
		opnsense.POST("/migration/flush", gateway.MigrationFlush)

		// Config backups
		opnsense.GET("/backups", gateway.ListBackups)
		opnsense.POST("/backups", gateway.CreateBackup)
		opnsense.GET("/backups/:filename", gateway.DownloadBackup)
		opnsense.DELETE("/backups/:filename", gateway.DeleteBackup)
	}
}
