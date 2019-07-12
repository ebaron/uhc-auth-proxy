package server

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"

	"github.com/go-chi/chi"
	"github.com/go-chi/chi/middleware"
	"github.com/redhatinsights/platform-go-middlewares/request_id"
	"github.com/redhatinsights/uhc-auth-proxy/requests/client"
	"github.com/redhatinsights/uhc-auth-proxy/requests/cluster"
)

// returns the cluster id from the user agent string used by the support operator
// support-operator/commit cluster/cluster_id
func getClusterID(userAgent string) (string, error) {
	spl := strings.SplitN(userAgent, " ", 2)
	if !strings.HasPrefix(spl[0], `support-operator/`) {
		return "", errors.New("Invalid user-agent")
	}

	if !strings.HasPrefix(spl[1], `cluster/`) {
		return "", errors.New("Invalid user-agent")
	}

	return strings.TrimPrefix(spl[1], `cluster/`), nil
}

func getToken(authorizationHeader string) (string, error) {
	if !strings.HasPrefix(authorizationHeader, `Bearer `) {
		return "", fmt.Errorf("Not a bearer token: '%s'", authorizationHeader)
	}

	return strings.TrimPrefix(authorizationHeader, `Bearer `), nil
}

// RootHandler returns a handler that uses the given client and token
func RootHandler(wrapper client.Wrapper) func(w http.ResponseWriter, r *http.Request) {
	return func(w http.ResponseWriter, r *http.Request) {
		clusterID, err := getClusterID(r.Header.Get("user-agent"))
		if err != nil {
			w.WriteHeader(400)
			fmt.Fprintf(w, "Invalid user-agent: '%s'", err.Error())
			return
		}

		token, err := getToken(r.Header.Get("Authorization"))
		if err != nil {
			w.WriteHeader(400)
			fmt.Fprintf(w, "Invalid authorization header: '%s'", err.Error())
			return
		}

		reg := cluster.Registration{
			ClusterID:          clusterID,
			AuthorizationToken: token,
		}

		ident, err := cluster.GetIdentity(wrapper, reg)
		if err != nil {
			w.WriteHeader(401)
			fmt.Fprintf(w, "Unable to get identity: '%s'", err.Error())
			return
		}

		b, err := json.Marshal(ident)
		if err != nil {
			w.WriteHeader(500)
			fmt.Fprintf(w, "Unable to read identity returned by cluster manager: '%s' ", err.Error())
			return
		}

		w.Header().Add("Content-Type", "application/json")
		w.WriteHeader(200)
		w.Write(b)
	}
}

func Start(offlineAccessToken string) {
	r := chi.NewRouter()
	r.Use(
		request_id.ConfiguredRequestID("x-rh-insights-request-id"),
		middleware.RealIP,
		middleware.Logger,
		middleware.Recoverer,
	)

	r.Get("/", RootHandler(&client.HTTPWrapper{
		OfflineAccessToken: offlineAccessToken,
	}))

	srv := http.Server{
		Addr:    fmt.Sprintf(":3000"),
		Handler: r,
	}

	if err := srv.ListenAndServe(); err != http.ErrServerClosed {
		fmt.Printf("server closed with error: %v\n", err)
	}
}
