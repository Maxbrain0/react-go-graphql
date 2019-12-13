package middleware

import (
	"context"
	"net/http"

	"github.com/go-redis/redis"
	"github.com/graphql-go/handler"
	"github.com/maxbrain0/react-go-graphql/server/logger"
	"github.com/maxbrain0/react-go-graphql/server/models"
)

var ctxLogger = logger.CtxLogger

// Config holds references that will be accessed in middleware
type Config struct {
	GQLHandler *handler.Handler
	R          *redis.Client
}

// HTTPMiddleware adds the request header to a graphql handler function
func HTTPMiddleware(c *Config) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Get cookies and reconstruct token - verify token and append authorization roles to
		// the req context
		ctxUser := userFromCookies(&w, r, c.R)

		ctx := context.WithValue(r.Context(), models.ContextKeyUser, ctxUser)
		ctx = context.WithValue(ctx, models.ContextKeyWriter, &w)

		w.Header().Set("Access-Control-Allow-Origin", "*")
		w.Header().Set("Access-Control-Allow-Credentials", "true")
		c.GQLHandler.ContextHandler(ctx, w, r)
	})
}