package main

import (
	"context"
	"log/slog"
	"net/http"

	"github.com/nickbryan/httputil"
	"github.com/nickbryan/slogutil"
)

var names []string

func main() {
	logger := slogutil.NewJSONLogger()
	server := httputil.NewServer(logger)

	server.Register(
		httputil.EndpointsWithPrefix(
			"/api",
			newNameEndpoint(logger),
		)...,
	)

	server.Serve(context.Background())
}

func newNameEndpoint(logger *slog.Logger) httputil.Endpoint {
	return httputil.Endpoint{
		Method:  http.MethodPost,
		Path:    "/name",
		Handler: newNameHandler(logger),
	}
}

func newNameHandler(logger *slog.Logger) http.Handler {
	type (
		request struct {
			Name string `json:"name"`
		}

		response struct {
			Names []string `json:"names"`
		}
	)

	return httputil.NewJSONHandler(func(r httputil.RequestData[request]) (*httputil.Response, error) {
		logger.Info("POST request received")

		names = append(names, r.Data.Name)

		return httputil.NewResponse(http.StatusOK, response{Names: names}), nil
	})
}
