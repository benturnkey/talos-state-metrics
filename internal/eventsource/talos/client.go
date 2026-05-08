package talos

import (
	"context"
	"fmt"

	cosistate "github.com/cosi-project/runtime/pkg/state"
	talosclient "github.com/siderolabs/talos/pkg/machinery/client"
)

func (s *Source) openState(ctx context.Context) (cosistate.CoreState, *talosclient.Client, error) {
	if s.state != nil {
		return s.state, nil, nil
	}

	newClient := s.newClient
	if newClient == nil {
		newClient = talosclient.New
	}

	client, err := newClient(ctx,
		talosclient.WithConfigFromFile(s.ConfigPath),
		talosclient.WithEndpoints(s.Endpoint),
		talosclient.WithDefaultGRPCDialOptions(),
	)
	if err != nil {
		return nil, nil, fmt.Errorf("create talos client: %w", err)
	}

	s.client = client

	return client.COSI, client, nil
}
