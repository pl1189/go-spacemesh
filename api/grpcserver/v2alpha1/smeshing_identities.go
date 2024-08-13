package v2alpha1

import (
	"context"
	"errors"
	"github.com/grpc-ecosystem/grpc-gateway/v2/runtime"
	"golang.org/x/exp/maps"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/types/known/timestamppb"

	pb "github.com/spacemeshos/api/release/go/spacemesh/v2alpha1"
	"github.com/spacemeshos/go-spacemesh/common/types"
	"github.com/spacemeshos/go-spacemesh/sql"
	"github.com/spacemeshos/go-spacemesh/sql/localsql/nipost"
)

const SmeshingIdentities = "smeshing_identities_v2alpha1"

const poetsMismatchWarning = "poet is not configured, reconfiguration required"

var ErrInvalidSmesherId = errors.New("smesher id is invalid")

type SmeshingIdentitiesService struct {
	db                     sql.Executor
	states                 identityState
	signers                map[types.NodeID]struct{}
	configuredPoetServices map[string]struct{}
}

func NewSmeshingIdentitiesService(
	db sql.Executor,
	configuredPoetServices map[string]struct{},
	states identityState,
	signers map[types.NodeID]struct{}) *SmeshingIdentitiesService {
	return &SmeshingIdentitiesService{
		db:                     db,
		configuredPoetServices: configuredPoetServices,
		states:                 states,
		signers:                signers,
	}
}

func (s *SmeshingIdentitiesService) RegisterService(server *grpc.Server) {
	pb.RegisterSmeshingIdentitiesServiceServer(server, s)
}

func (s *SmeshingIdentitiesService) RegisterHandlerService(mux *runtime.ServeMux) error {
	return pb.RegisterSmeshingIdentitiesServiceHandlerServer(context.Background(), mux, s)
}

// String returns the name of this service.
func (s *SmeshingIdentitiesService) String() string {
	return "SmeshingIdentitiesService"
}

func (s *SmeshingIdentitiesService) PoetServices(_ context.Context, req *pb.PoetServicesRequest) (*pb.PoetServicesResponse, error) {
	states := s.states.IdentityStates()

	nodeIdsToRequest := make([]types.NodeID, 0)
	for desc, state := range states {
		if state != types.WaitForPoetRoundEnd {
			continue
		}

		_, ok := s.signers[desc.NodeID()]
		if !ok {
			// TODO log
			continue
		}
		nodeIdsToRequest = append(nodeIdsToRequest, desc.NodeID())
	}

	regs, err := nipost.PoetRegistrations(s.db, nodeIdsToRequest...)
	if err != nil {
		return nil, status.Error(codes.Internal, err.Error())
	}

	identities := make(map[types.NodeID]map[string]*pb.PoetServicesResponse_Identity_PoetInfo)
	for _, reg := range regs {
		poetInfos, ok := identities[reg.NodeId]
		if !ok {
			poetInfos = make(map[string]*pb.PoetServicesResponse_Identity_PoetInfo)
		}

		var (
			regStatus pb.PoetServicesResponse_Identity_PoetInfo_RegistrationStatus
			warning   string
		)

		if reg.RoundID == "" {
			regStatus = pb.PoetServicesResponse_Identity_PoetInfo_STATUS_FAILED_REG
		} else if _, ok := s.configuredPoetServices[reg.Address]; !ok {
			regStatus = pb.PoetServicesResponse_Identity_PoetInfo_STATUS_RESIDUAL_REG
			warning = poetsMismatchWarning
		} else {
			regStatus = pb.PoetServicesResponse_Identity_PoetInfo_STATUS_SUCCESS_REG
		}

		poetInfos[reg.Address] = &pb.PoetServicesResponse_Identity_PoetInfo{
			Url:                reg.Address,
			PoetRoundEnd:       timestamppb.New(reg.RoundEnd),
			RegistrationStatus: regStatus,
			Warning:            warning,
		}

		identities[reg.NodeId] = poetInfos
	}

	pbIdentities := make([]*pb.PoetServicesResponse_Identity, 0)

	for id, poets := range identities {
		for poetAddr := range s.configuredPoetServices {
			if _, ok := poets[poetAddr]; !ok {
				// registration is missed
				identities[id][poetAddr] = &pb.PoetServicesResponse_Identity_PoetInfo{
					Url:                poetAddr,
					RegistrationStatus: pb.PoetServicesResponse_Identity_PoetInfo_STATUS_NO_REG,
				}
			}
		}

		pbIdentities = append(pbIdentities, &pb.PoetServicesResponse_Identity{
			SmesherIdHex: id.String(),
			PoetInfos:    maps.Values(poets),
		})
	}

	return &pb.PoetServicesResponse{Identities: pbIdentities}, nil
}
