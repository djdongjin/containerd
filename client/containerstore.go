/*
   Copyright The containerd Authors.

   Licensed under the Apache License, Version 2.0 (the "License");
   you may not use this file except in compliance with the License.
   You may obtain a copy of the License at

       http://www.apache.org/licenses/LICENSE-2.0

   Unless required by applicable law or agreed to in writing, software
   distributed under the License is distributed on an "AS IS" BASIS,
   WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
   See the License for the specific language governing permissions and
   limitations under the License.
*/

package client

import (
	"context"
	"errors"
	"io"

	containersapi "github.com/containerd/containerd/api/services/containers/v1"
	"github.com/containerd/containerd/v2/core/containers"
	ptypes "github.com/containerd/containerd/v2/pkg/protobuf/types"
	"github.com/containerd/errdefs"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

type remoteContainers struct {
	client containersapi.ContainersClient
}

var _ containers.Store = &remoteContainers{}

// NewRemoteContainerStore returns the container Store connected with the provided client
func NewRemoteContainerStore(client containersapi.ContainersClient) containers.Store {
	return &remoteContainers{
		client: client,
	}
}

func (r *remoteContainers) Get(ctx context.Context, id string) (containers.Container, error) {
	resp, err := r.client.Get(ctx, &containersapi.GetContainerRequest{
		ID: id,
	})
	if err != nil {
		return containers.Container{}, errdefs.FromGRPC(err)
	}

	return containers.ContainerFromProto(resp.Container), nil
}

func (r *remoteContainers) List(ctx context.Context, filters ...string) ([]containers.Container, error) {
	containers, err := r.stream(ctx, filters...)
	if err != nil {
		if err == errStreamNotAvailable {
			return r.list(ctx, filters...)
		}
		return nil, err
	}
	return containers, nil
}

func (r *remoteContainers) list(ctx context.Context, filters ...string) ([]containers.Container, error) {
	resp, err := r.client.List(ctx, &containersapi.ListContainersRequest{
		Filters: filters,
	})
	if err != nil {
		return nil, errdefs.FromGRPC(err)
	}
	return containers.ContainersFromProto(resp.Containers), nil
}

var errStreamNotAvailable = errors.New("streaming api not available")

func (r *remoteContainers) stream(ctx context.Context, filters ...string) ([]containers.Container, error) {
	session, err := r.client.ListStream(ctx, &containersapi.ListContainersRequest{
		Filters: filters,
	})
	if err != nil {
		return nil, errdefs.FromGRPC(err)
	}
	var cs []containers.Container
	for {
		c, err := session.Recv()
		if err != nil {
			if err == io.EOF {
				return cs, nil
			}
			if s, ok := status.FromError(err); ok {
				if s.Code() == codes.Unimplemented {
					return nil, errStreamNotAvailable
				}
			}
			return nil, errdefs.FromGRPC(err)
		}
		select {
		case <-ctx.Done():
			return cs, ctx.Err()
		default:
			cs = append(cs, containers.ContainerFromProto(c.Container))
		}
	}
}

func (r *remoteContainers) Create(ctx context.Context, container containers.Container) (containers.Container, error) {
	created, err := r.client.Create(ctx, &containersapi.CreateContainerRequest{
		Container: containers.ContainerToProto(&container),
	})
	if err != nil {
		return containers.Container{}, errdefs.FromGRPC(err)
	}

	return containers.ContainerFromProto(created.Container), nil

}

func (r *remoteContainers) Update(ctx context.Context, container containers.Container, fieldpaths ...string) (containers.Container, error) {
	var updateMask *ptypes.FieldMask
	if len(fieldpaths) > 0 {
		updateMask = &ptypes.FieldMask{
			Paths: fieldpaths,
		}
	}

	updated, err := r.client.Update(ctx, &containersapi.UpdateContainerRequest{
		Container:  containers.ContainerToProto(&container),
		UpdateMask: updateMask,
	})
	if err != nil {
		return containers.Container{}, errdefs.FromGRPC(err)
	}

	return containers.ContainerFromProto(updated.Container), nil

}

func (r *remoteContainers) Delete(ctx context.Context, id string) error {
	_, err := r.client.Delete(ctx, &containersapi.DeleteContainerRequest{
		ID: id,
	})

	return errdefs.FromGRPC(err)

}
