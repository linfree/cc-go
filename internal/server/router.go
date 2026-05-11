package server

import (
	"fmt"
	"os"

	"github.com/gin-gonic/gin"
	"github.com/linfree/cc-go/internal/bridge"
	"github.com/linfree/cc-go/internal/config"
	"github.com/linfree/cc-go/internal/server/api"
	"github.com/linfree/cc-go/internal/server/ws"
	"github.com/linfree/cc-go/internal/store"
	"github.com/linfree/cc-go/internal/wechat"
)

type Server struct {
	router *gin.Engine
	cfg    *config.Config
	store  *store.Store
	bridge *bridge.Bridge
	wechat *wechat.Client
	wsHub  *ws.Hub
}

func New(cfg *config.Config, st *store.Store, br *bridge.Bridge, wc *wechat.Client) *Server {
	gin.SetMode(gin.ReleaseMode)
	r := gin.Default()

	hub := ws.NewHub()

	s := &Server{
		router: r,
		cfg:    cfg,
		store:  st,
		bridge: br,
		wechat: wc,
		wsHub:  hub,
	}

	go func() {
		defer func() {
			if r := recover(); r != nil {
				fmt.Fprintf(os.Stderr, "[ws] event bus goroutine panicked: %v\n", r)
			}
		}()
		for evt := range br.EventBus() {
			hub.Broadcast(evt)
		}
	}()

	api.RegisterRoutes(r, cfg, st, br, wc, hub)
	return s
}

func (s *Server) Router() *gin.Engine { return s.router }