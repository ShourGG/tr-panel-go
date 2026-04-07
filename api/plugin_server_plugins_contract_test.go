package api

import (
	"net/http"
	"testing"

	"github.com/gin-gonic/gin"
)

func TestRoomPluginHandlersRejectNonPositiveRoomID(t *testing.T) {
	gin.SetMode(gin.TestMode)

	testCases := []struct {
		name    string
		method  string
		route   string
		path    string
		body    any
		handler gin.HandlerFunc
	}{
		{name: "get room plugins zero id", method: http.MethodGet, route: "/rooms/:id/plugins", path: "/rooms/0/plugins", handler: GetRoomPlugins},
		{name: "get room plugins non numeric id", method: http.MethodGet, route: "/rooms/:id/plugins", path: "/rooms/abc/plugins", handler: GetRoomPlugins},
		{name: "add room plugin zero id", method: http.MethodPost, route: "/rooms/:id/plugins", path: "/rooms/0/plugins", handler: AddRoomPlugin},
		{name: "delete room plugin zero id", method: http.MethodDelete, route: "/rooms/:id/plugins/:plugin", path: "/rooms/0/plugins/example.dll", handler: DeleteRoomPlugin},
		{name: "copy shared plugin zero id", method: http.MethodPost, route: "/rooms/:id/plugins/copy", path: "/rooms/0/plugins/copy", body: map[string]any{"pluginName": "Example.dll"}, handler: CopyPluginFromShared},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			router := gin.New()
			router.Handle(tc.method, tc.route, tc.handler)

			response := performTaskRequest(t, router, tc.method, tc.path, tc.body)
			assertTaskErrorResponse(t, response, http.StatusBadRequest, "无效的房间ID")
		})
	}
}

func TestCopyPluginServerPluginToRoomRejectsNonPositiveRoomID(t *testing.T) {
	gin.SetMode(gin.TestMode)

	router := gin.New()
	router.POST("/plugin-server/plugins/:name/copy-to-room", CopyPluginServerPluginToRoom)

	response := performTaskRequest(t, router, http.MethodPost, "/plugin-server/plugins/Example.dll/copy-to-room", map[string]any{
		"roomId": 0,
	})

	assertTaskErrorResponse(t, response, http.StatusBadRequest, "无效的房间ID")
}
