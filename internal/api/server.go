package api

import (
	"context"
	"fmt"
	"time"

	"github.com/dgraph-io/badger/v4"
	"github.com/go-playground/validator/v10"
	"github.com/labstack/echo/v4"
	"github.com/labstack/echo/v4/middleware"
	log "github.com/sirupsen/logrus"

	"github.com/swissborg/galactica-kyc-guardian/config"
	"github.com/swissborg/galactica-kyc-guardian/internal/zkcert"
)

type Server struct {
	echo      *echo.Echo
	mem       *badger.DB
	generator *zkcert.Service
}

func NewServer(generator *zkcert.Service, mem *badger.DB) *Server {
	return &Server{mem: mem, generator: generator}
}

func (s *Server) Start(cfg config.APIConf) error {
	log.Infof("API server starting...")

	s.echo = s.makeEcho()

	err := s.echo.Start(fmt.Sprintf("%s:%s", cfg.Host, cfg.Port))
	if err != nil {
		return err
	}
	return nil
}

func (s *Server) Stop() error {
	const shutdownTimeout = time.Second * 10

	ctx, cancelTimeout := context.WithTimeout(context.Background(), shutdownTimeout)
	defer cancelTimeout()

	if err := s.echo.Shutdown(ctx); err != nil {
		return err
	}

	return nil
}

func (s *Server) makeEcho() *echo.Echo {
	e := echo.New()
	e.Use(middleware.Recover())

	e.Validator = &CustomValidator{validator: validator.New()}

	handlers := NewHandlers(s.generator, s.mem)

	certGroup := e.Group("/cert")
	certGroup.POST("/generate", handlers.GenerateCert)
	certGroup.POST("/get", handlers.GetCert)

	return e
}

type CustomValidator struct {
	validator *validator.Validate
}

func (cv *CustomValidator) Validate(i interface{}) error {
	return cv.validator.Struct(i)
}
