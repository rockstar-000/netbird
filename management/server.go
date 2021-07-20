package management

import (
	"context"
	"github.com/wiretrustee/wiretrustee/management/proto"
	"google.golang.org/grpc/status"
)

// Server an instance of a Management server
type Server struct {
	Store *FileStore
	proto.UnimplementedManagementServiceServer
}

// NewServer creates a new Management server
func NewServer(dataDir string) (*Server, error) {
	store, err := NewStore(dataDir)
	if err != nil {
		return nil, err
	}
	return &Server{
		Store: store,
	}, nil
}

// RegisterPeer adds a peer to the Store. Returns 404 in case the provided setup key doesn't exist
func (s *Server) RegisterPeer(ctx context.Context, req *proto.RegisterPeerRequest) (*proto.RegisterPeerResponse, error) {

	err := s.Store.AddPeer(req.SetupKey, req.Key)
	if err != nil {
		return &proto.RegisterPeerResponse{}, status.Errorf(404, "provided setup key doesn't exists")
	}

	return &proto.RegisterPeerResponse{}, nil
}

// IsHealthy indicates whether the service is healthy
func (s *Server) IsHealthy(ctx context.Context, req *proto.Empty) (*proto.Empty, error) {
	return &proto.Empty{}, nil
}
