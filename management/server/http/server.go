package http

import (
	"context"
	"encoding/gob"
	log "github.com/sirupsen/logrus"
	s "github.com/wiretrustee/wiretrustee/management/server"
	handler2 "github.com/wiretrustee/wiretrustee/management/server/http/handler"
	middleware2 "github.com/wiretrustee/wiretrustee/management/server/http/middleware"
	"golang.org/x/crypto/acme/autocert"
	"net/http"
	"time"

	"github.com/codegangsta/negroni"
	"github.com/gorilla/sessions"
)

type Server struct {
	server      *http.Server
	config      *s.HttpServerConfig
	certManager *autocert.Manager
}

// NewHttpsServer creates a new HTTPs server (with HTTPS support)
// The listening address will be :443 no matter what was specified in s.HttpServerConfig.Address
func NewHttpsServer(config *s.HttpServerConfig, certManager *autocert.Manager) *Server {
	server := &http.Server{
		Addr:         config.Address,
		WriteTimeout: time.Second * 15,
		ReadTimeout:  time.Second * 15,
		IdleTimeout:  time.Second * 60,
	}
	return &Server{server: server, config: config, certManager: certManager}
}

// NewHttpServer creates a new HTTP server (without HTTPS)
func NewHttpServer(config *s.HttpServerConfig) *Server {
	return NewHttpsServer(config, nil)
}

// Stop stops the http server
func (s *Server) Stop(ctx context.Context) error {
	err := s.server.Shutdown(ctx)
	if err != nil {
		return err
	}
	return nil
}

// Start defines http handlers and starts the http server. Blocks until server is shutdown.
func (s *Server) Start() error {

	sessionStore := sessions.NewFilesystemStore("", []byte("something-very-secret"))
	authenticator, err := middleware2.NewAuthenticator(s.config.AuthDomain, s.config.AuthClientId, s.config.AuthClientSecret, s.config.AuthCallback)
	if err != nil {
		log.Errorf("failed cerating authentication middleware %v", err)
		return err
	}

	gob.Register(map[string]interface{}{})

	r := http.NewServeMux()
	s.server.Handler = r

	r.Handle("/login", handler2.NewLogin(authenticator, sessionStore))
	r.Handle("/logout", handler2.NewLogout(s.config.AuthDomain, s.config.AuthClientId))
	r.Handle("/callback", handler2.NewCallback(authenticator, sessionStore))
	r.Handle("/dashboard", negroni.New(
		negroni.HandlerFunc(middleware2.NewAuth(sessionStore).IsAuthenticated),
		negroni.Wrap(handler2.NewDashboard(sessionStore))),
	)
	http.Handle("/", r)

	if s.certManager != nil {
		// if HTTPS is enabled we reuse the listener from the cert manager
		listener := s.certManager.Listener()
		log.Infof("http server listening on %s", listener.Addr())
		if err = http.Serve(listener, s.certManager.HTTPHandler(r)); err != nil {
			log.Errorf("failed to serve https server: %v", err)
			return err
		}
	} else {
		log.Infof("http server listening on %s", s.server.Addr)
		if err = s.server.ListenAndServe(); err != nil {
			log.Errorf("failed to serve http server: %v", err)
			return err
		}
	}

	return nil
}
