package grpcapi

import (
	"net"
	"net/http"
	"unsafe"

	"golang.org/x/net/context"

	"github.com/grpc-ecosystem/grpc-gateway/runtime"
	"github.com/prometheus/alertmanager/api/grpcapi/apipb"
	"github.com/prometheus/alertmanager/provider"
	"github.com/weaveworks/mesh"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
)

func Handle(grpcl net.Listener, alerts provider.Alerts, mrouter *mesh.Router) (*grpc.Server, http.Handler) {
	grpcServer := grpc.NewServer()

	asvc := &alertsServer{
		alerts:  alerts,
		mrouter: mrouter,
	}
	apipb.RegisterAlertsServer(grpcServer, asvc)

	ctx := context.Background()
	gwmux := runtime.NewServeMux()

	opts := []grpc.DialOption{grpc.WithInsecure()}
	err := apipb.RegisterAlertsHandlerFromEndpoint(ctx, gwmux, grpcl.Addr().String(), opts)
	if err != nil {
		panic(err)
	}

	return grpcServer, gwmux
}

type alertsServer struct {
	alerts  provider.Alerts
	mrouter *mesh.Router
}

func (s *alertsServer) Get(ctx context.Context, req *apipb.AlertsGetRequest) (*apipb.AlertsGetResponse, error) {
	alerts := s.alerts.GetPending()
	defer alerts.Close()

	var (
		err error
		res []*apipb.Alert
	)
	for a := range alerts.Next() {
		if err = alerts.Err(); err != nil {
			return nil, err
		}

		res = append(res, &apipb.Alert{
			Labels:      *(*map[string]string)(unsafe.Pointer(&a.Labels)),
			Annotations: *(*map[string]string)(unsafe.Pointer(&a.Annotations)),
			StartsAt:    a.StartsAt,
			EndsAt:      a.EndsAt,
		})
	}

	resp := &apipb.AlertsGetResponse{
		Header: s.header(),
		Alerts: res,
	}
	return resp, nil
}

func (s *alertsServer) Add(ctx context.Context, req *apipb.AlertsAddRequest) (*apipb.AlertsAddResponse, error) {
	resp := &apipb.AlertsAddResponse{
		Header: s.header(),
	}
	return resp, grpc.Errorf(codes.Unimplemented, "not implemented")
}

func (s *alertsServer) header() *apipb.ResponseHeader {
	status := mesh.NewStatus(s.mrouter)
	return &apipb.ResponseHeader{PeerName: status.Name}
}
