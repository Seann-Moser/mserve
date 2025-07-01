package mserve

import (
	"context"
	_ "embed"
	"fmt"
	"github.com/Seann-Moser/credentials/oauth/oclient"
	"github.com/Seann-Moser/credentials/oauth/oserver"
	"github.com/Seann-Moser/mserve/nuxt3FromOpenApi"
	"github.com/Seann-Moser/rbac/rbacServer"
	"gopkg.in/yaml.v3"
	"html/template"
	"log"
	"log/slog"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/Seann-Moser/credentials/session"

	"github.com/Seann-Moser/credentials/user"
	"github.com/Seann-Moser/rbac"
	"github.com/caddyserver/certmagic"
	"github.com/gorilla/mux"
	goCache "github.com/patrickmn/go-cache"
	"go.opentelemetry.io/contrib/instrumentation/net/http/otelhttp"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/exporters/prometheus"
	"go.opentelemetry.io/otel/sdk/metric"
)

// Server wraps HTTP server, router, RBAC manager, and CertMagic domains
// plus dynamic CORS origins and OpenTelemetry metrics
type Server struct {
	ServiceName    string
	Description    string
	Version        string
	rbac           *rbac.Manager
	router         *mux.Router
	domains        []string
	allowedOrigins []string
	muOrigins      sync.RWMutex
	sessionClient  *session.Client

	goCache         *goCache.Cache
	SSLConfig       SSLConfig
	healthCheckPath string
	endpoints       []Endpoint
}

// NewServer creates a new Server instance
func NewServer(serviceName string, r *rbac.Manager, domains []string, sessionClient *session.Client, ssl SSLConfig) *Server {
	router := mux.NewRouter()
	if serviceName == "" {
		serviceName = "default"
	}
	router.Use(corsMiddleware)
	return &Server{
		rbac:           r,
		ServiceName:    serviceName,
		router:         router,
		domains:        domains,
		allowedOrigins: []string{},
		muOrigins:      sync.RWMutex{},
		sessionClient:  sessionClient,
		goCache:        goCache.New(time.Minute*5, time.Minute),
		SSLConfig:      ssl,
	}
}

func corsMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Change "*" to a specific origin in prod!
		w.Header().Set("Access-Control-Allow-Origin", getOrigin(r))
		w.Header().Set("Access-Control-Allow-Methods", "GET, POST, PUT, DELETE, OPTIONS")
		w.Header().Set("Access-Control-Allow-Headers", "Content-Type, Authorization")
		w.Header().Set("Access-Control-Allow-Credentials", "true")
		// handle preflight
		if r.Method == http.MethodOptions {
			w.WriteHeader(http.StatusNoContent)
			return
		}
		next.ServeHTTP(w, r)
	})
}

func getOrigin(r *http.Request) string {
	if v := r.Header.Get("Origin"); v != "" {
		return v
	}
	if v := r.Header.Get("Referer"); v != "" {
		return v
	}
	return ""
}

func (s *Server) matchOrigin(r *http.Request) (string, error) {
	origin := getOrigin(r)
	for _, o := range s.allowedOrigins {
		if strings.EqualFold(o, origin) {
			return origin, nil
		}
	}
	if len(s.allowedOrigins) == 0 {
		return origin, nil
	}
	return "", fmt.Errorf("invalid origin %s", origin)
}

// AddMiddleware sets up dynamic CORS
func (s *Server) AddMiddleware(lists ...func(next http.Handler) http.Handler) {
	for _, list := range lists {
		s.router.Use(list)
	}
}

// AddOrigin dynamically adds a new CORS origin
func (s *Server) AddOrigin(origin string) {
	s.muOrigins.Lock()
	defer s.muOrigins.Unlock()
	for _, o := range s.allowedOrigins {
		if o == origin {
			return
		}
	}
	s.allowedOrigins = append(s.allowedOrigins, origin)
}

// getOrigins returns allowed origins safely
func (s *Server) getOrigins() []string {
	s.muOrigins.RLock()
	defer s.muOrigins.RUnlock()
	if len(s.allowedOrigins) == 0 {
		return []string{"*"}
	}
	return append([]string(nil), s.allowedOrigins...)
}

// AddEndpoints registers endpoints on the router
func (s *Server) AddEndpoints(ctx context.Context, endpoints ...*Endpoint) error {
	//s.sessionClient.Authenticate()
	//session.SetSessionCookie()
	for _, e := range endpoints {
		handler := e.Handler
		err := e.Init(ctx, s.ServiceName, s.rbac)
		if err != nil {
			return err
		}
		m := append(e.Methods, http.MethodOptions)
		s.router.HandleFunc(e.Path, handler).Methods(m...)
		s.endpoints = append(s.endpoints, *e)
	}
	return nil
}

// setupMetrics initializes OpenTelemetry metrics and mounts /metrics endpoint
func (s *Server) setupMetrics(ctx context.Context) error {
	exporter, err := prometheus.New(prometheus.WithNamespace(""))
	if err != nil {
		return err
	}
	provider := metric.NewMeterProvider(metric.WithReader(exporter))
	otel.SetMeterProvider(provider)
	//s.router.Handle("/metrics", exporter)
	return nil
}

type SSLConfig struct {
	Email   string
	Agreed  bool
	Enabled bool
	Port    int
}

// Run starts the server with CertMagic and OpenTelemetry
func (s *Server) Run(ctx context.Context) error {

	certmagic.DefaultACME.Agreed = s.SSLConfig.Agreed
	certmagic.DefaultACME.Email = s.SSLConfig.Email

	if err := s.setupMetrics(ctx); err != nil {
		log.Fatalf("failed to setup metrics: %v", err)
	}

	rootHandler := otelhttp.NewHandler(s.router, "http-server")

	go func() {
		slog.Info("starting https server")
		if !s.SSLConfig.Enabled {
			if s.SSLConfig.Port <= 0 {
				s.SSLConfig.Port = 8081
			}
			if err := http.ListenAndServe(":"+strconv.Itoa(s.SSLConfig.Port), rootHandler); err != nil {
				log.Fatalf("failed to start https server: %v", err)
			}
			return
		}
		if err := certmagic.HTTPS(s.domains, rootHandler); err != nil {
			log.Fatalf("CertMagic HTTPS failed: %v", err)
		}
	}()

	<-ctx.Done()
	log.Println("Shutting down server")
	return nil
}

// HealthCheck registers a health check handler
func (s *Server) HealthCheck(path string, f func(ctx context.Context) error) *Server {
	s.healthCheckPath = path
	s.router.HandleFunc(path, func(w http.ResponseWriter, r *http.Request) {
		if f == nil {
			w.WriteHeader(http.StatusOK)
			return
		}
		if err := f(r.Context()); err != nil {
			http.Error(w, err.Error(), http.StatusServiceUnavailable)
			return
		}
		w.WriteHeader(http.StatusOK)
	})
	return s
}

func (s *Server) SetupOServer(ctx context.Context, o oserver.OServer) *Server {
	handler := oserver.NewHandler(o, oserver.ContentTypeJSON)
	//fix this resouce needs to be calculated dynamicly
	s.AddMiddleware(func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			usersession, ctx, _ := s.sessionClient.Authenticate(w, r)
			if !s.hasAccess(ctx, endpointResourceName(s.ServiceName, r.URL.Path), usersession.UserID, usersession.AccountID, r.Method) {
				http.Error(w, "forbidden", http.StatusForbidden)
				return
			}
			next.ServeHTTP(w, r.WithContext(ctx))

		})
	})
	err := s.AddEndpoints(ctx, makeEndpoints(handler)...)

	if err != nil {
		slog.Error("failed adding o server endpoints", "err", err)
	}

	return s
}

func (s *Server) SetupRbac(ctx context.Context) *Server {
	err := s.AddEndpoints(ctx, makeRBACEndpoints(rbacServer.NewServer(s.rbac))...)
	if err != nil {
		slog.Error("failed adding o server endpoints", "err", err)
	}
	return s
}

func (s *Server) SetupUserLogin(ctx context.Context, userServer *user.Server) *Server {
	err := s.AddEndpoints(ctx, makeUserEndpoints(userServer)...)
	if err != nil {
		slog.Error("failed adding o server endpoints", "err", err)
	}
	s.AddMiddleware(userServer.AuthMiddleware)
	return s
}

func (s *Server) SetupOClient(o oclient.OAuthService) *Server {
	return s
}

func (s *Server) hasAccess(ctx context.Context, resource string, userId, accountId string, method string, scopes ...string) bool {
	if endpointResourceName(s.ServiceName, s.healthCheckPath) == resource {
		return true
	}
	k := resource + userId + accountId + method
	hasAccess, ok := s.goCache.Get(k)
	if ok {
		if h, ok := hasAccess.(bool); ok && h {
			slog.Info("has access from cache")
			return true
		}
	}
	slog.Info("access from cache", "user_id", userId, "account_id", accountId, "method", method, "resource", resource)
	can, err := s.rbac.Can(ctx, userId, resource, rbac.HTTPMethodToAction(method))
	if err != nil {
		slog.Error("Error checking permissions", "err", err)
		return false
	}
	slog.Info("setting access from cache", "can", can)

	//todo support scopes
	s.goCache.Set(k, can, goCache.DefaultExpiration)
	return can
}

func (s *Server) GenerateOpenAPIDocs() *Server {
	api, err := GenerateOpenAPI(s, s.endpoints)
	if err != nil {
		return s
	}

	swaggerUITmpl := template.Must(template.New("swaggerUI").Parse(swaggerUIMasterTemplate))

	_ = s.AddEndpoints(context.Background(), &Endpoint{
		Name:        "OpenAPI",
		Description: "",
		Methods:     []string{"GET"},
		Path:        "/openapi/v2.yaml",
		Handler: func(w http.ResponseWriter, r *http.Request) {
			renderUI := r.URL.Query().Get("render")
			switch renderUI {
			case "json":
				d, err := api.MarshalJSON()
				if err != nil {
					http.Error(w, err.Error(), http.StatusInternalServerError)
					return
				}
				w.Header().Set("Content-Type", "application/json")

				_, writeErr := w.Write(d)
				if writeErr != nil {
					http.Error(w, "Failed to write JSON: "+writeErr.Error(), http.StatusInternalServerError)
					log.Printf("Error writing JSON: %v", writeErr)
				}
			case "yaml":
				d, err := api.MarshalYAML()
				if err != nil {
					http.Error(w, err.Error(), http.StatusInternalServerError)
					return
				}
				raw, err := yaml.Marshal(d)
				if err != nil {
					http.Error(w, err.Error(), http.StatusInternalServerError)
					return
				}
				w.Header().Set("Content-Type", "application/x-yaml")
				_, writeErr := w.Write(raw)
				if writeErr != nil {
					http.Error(w, "Failed to write YAML: "+writeErr.Error(), http.StatusInternalServerError)
					log.Printf("Error writing YAML: %v", writeErr)
				}
			default:
				// Serve Swagger UI HTML
				w.Header().Set("Content-Type", "text/html; charset=utf-8")
				data := struct {
					OpenAPIYAMLURL string
				}{
					OpenAPIYAMLURL: "/openapi/v2.yaml?render=yaml", // Swagger UI will fetch YAML from this path
				}
				if err := swaggerUITmpl.Execute(w, data); err != nil {
					http.Error(w, "Failed to render Swagger UI: "+err.Error(), http.StatusInternalServerError)
					log.Printf("Error rendering Swagger UI: %v", err)
				}
			}
		},
		Internal: false,
		Request: Request{
			Params: map[string]ROption{
				"render": {
					Default:  "",
					Required: false,
					Enum:     []string{"json", "yaml"},
				},
			},
			Headers: nil,
			Body:    nil,
		},
		Scope:     "",
		Responses: nil,
		Roles:     nil,
	})

	_ = s.AddEndpoints(context.Background(), &Endpoint{
		Name:        "Nuxt Plugin",
		Description: "",
		Methods:     []string{"GET"},
		Path:        "/openapi/nuxt/plugins",
		Handler: func(w http.ResponseWriter, r *http.Request) {
			d, err := api.MarshalYAML()
			if err != nil {
				http.Error(w, err.Error(), http.StatusInternalServerError)
				return
			}
			raw, err := yaml.Marshal(d)
			if err != nil {
				http.Error(w, err.Error(), http.StatusInternalServerError)
				return
			}
			resp, err := nuxt3FromOpenApi.GenerateNuxt3Plugin(raw)
			if err != nil {
				http.Error(w, err.Error(), http.StatusInternalServerError)
				return
			}
			w.Header().Set("Content-Type", "text/javascript")
			w.Header().Set("Name", nuxt3FromOpenApi.ToPascalCase(api.Info.Title)+".js")
			_, _ = w.Write([]byte(resp))

		},
	})

	//_ = s.AddEndpoints(context.Background(), &Endpoint{
	//	Name:        "Nuxt Plugin Setup",
	//	Description: "",
	//	Methods:     []string{"GET"},
	//	Path:        "/openapi/nuxt/plugins/setup",
	//	Handler: func(w http.ResponseWriter, r *http.Request) {
	//		resp := strings.ReplaceAll(nuxtPluginTemplate, "remotePluginUrl", nuxt3FromOpenApi.ToPascalCase(api.Info.Title)+"PluginUrl")
	//		w.Header().Set("Content-Type", "text/typescript")
	//		w.Header().Set("Name", nuxt3FromOpenApi.ToPascalCase(api.Info.Title)+".ts")
	//		_, _ = w.Write([]byte(resp))
	//	},
	//})

	return s
}

const swaggerUIMasterTemplate = `
<!DOCTYPE html>
<html lang="en">
<head>
    <meta charset="UTF-8">
    <title>Swagger UI</title>
    <link rel="stylesheet" type="text/css" href="https://cdnjs.cloudflare.com/ajax/libs/swagger-ui/5.17.4/swagger-ui.min.css" >
    <link rel="icon" type="image/png" href="https://petstore.swagger.io/favicon-32x32.png" sizes="32x32" />
    <link rel="icon" type="image/png" href="https://petstore.swagger.io/favicon-16x16.png" sizes="16x16" />
    <style>
        html { box-sizing: border-box; overflow: -moz-scrollbars-vertical; overflow-y: scroll; }
        *, *:before, *:after { box-sizing: inherit; }
        body { margin:0; background: #fafafa; }
    </style>
</head>
<body>
    <div id="swagger-ui"></div>
    <script src="https://cdnjs.cloudflare.com/ajax/libs/swagger-ui/5.17.4/swagger-ui-bundle.min.js"></script>
    <script src="https://cdnjs.cloudflare.com/ajax/libs/swagger-ui/5.17.4/swagger-ui-standalone-preset.min.js"></script>
    <script>
        window.onload = function() {
            // Begin Swagger UI call
            const ui = SwaggerUIBundle({
                url: "{{.OpenAPIYAMLURL}}", // Placeholder for your OpenAPI YAML endpoint
                dom_id: '#swagger-ui',
                deepLinking: true,
                presets: [
                    SwaggerUIBundle.presets.apis,
                    SwaggerUIStandalonePreset
                ],
                plugins: [
                    SwaggerUIBundle.plugins.DownloadUrl
                ],
                layout: "StandaloneLayout"
            });
            window.ui = ui;
        };
    </script>
</body>
</html>
`
