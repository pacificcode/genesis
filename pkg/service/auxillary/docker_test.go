/*
	Copyright 2019 whiteblock Inc.
	This file is a part of the genesis.

	Genesis is free software: you can redistribute it and/or modify
	it under the terms of the GNU General Public License as published by
	the Free Software Foundation, either version 3 of the License, or
	(at your option) any later version.

	Genesis is distributed in the hope that it will be useful,
	but WITHOUT ANY WARRANTY; without even the implied warranty of
	MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE.  See the
	GNU General Public License for more details.

	You should have received a copy of the GNU General Public License
	along with this program.  If not, see <https://www.gnu.org/licenses/>.
*/

package auxillary

import (
	"testing"

	"github.com/docker/docker/api/types"
	"github.com/docker/docker/api/types/volume"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
	repository "github.com/whiteblock/genesis/mocks/pkg/repository"
	"io"
	"io/ioutil"
	"strings"
)

func TestDockerAuxillary_GetNetworkByName(t *testing.T) {
	results := []types.NetworkResource{
		types.NetworkResource{Name: "test1", ID: "id1"},
		types.NetworkResource{Name: "test2", ID: "id2"},
	}
	repo := new(repository.DockerRepository)
	repo.On("NetworkList", mock.Anything, mock.Anything, mock.Anything).Return(results, nil).Run(
		func(args mock.Arguments) {

			require.Len(t, args, 3)
			assert.Nil(t, args.Get(0))
			assert.Nil(t, args.Get(1))
		})
	ds := NewDockerAuxillary(repo)

	for _, result := range results {
		net, err := ds.GetNetworkByName(nil, nil, result.Name)
		assert.NoError(t, err)
		assert.Equal(t, result, net)
	}

	_, err := ds.GetNetworkByName(nil, nil, "DNE")
	assert.Error(t, err)

	repo.AssertNumberOfCalls(t, "NetworkList", len(results)+1)
}

func TestDockerAuxillary_HostHasImage(t *testing.T) {
	testImageList := []types.ImageSummary{
		types.ImageSummary{RepoDigests: []string{"test0"}, RepoTags: []string{"test2"}},
		types.ImageSummary{RepoDigests: []string{"test3"}, RepoTags: []string{"test4"}},
		types.ImageSummary{RepoDigests: []string{"test5"}, RepoTags: []string{"test6"}},
		types.ImageSummary{RepoDigests: []string{"test7"}, RepoTags: []string{"test8"}},
	}

	existingImageTags := []string{"test2", "test6"}
	existingImageDigests := []string{"test0", "test3"}
	noneExistingImageTags := []string{"A", "B"}
	noneExistingImageDigests := []string{"C", "D"}

	repo := new(repository.DockerRepository)
	repo.On("ImageList", mock.Anything, mock.Anything, mock.Anything).Return(testImageList, nil).Run(
		func(args mock.Arguments) {

			require.Len(t, args, 3)
			assert.Nil(t, args.Get(0))
			assert.Nil(t, args.Get(1))
		})

	ds := NewDockerAuxillary(repo)

	for _, term := range append(existingImageTags, existingImageDigests...) {
		exists, err := ds.HostHasImage(nil, nil, term)
		assert.NoError(t, err)
		assert.True(t, exists)
	}

	for _, term := range append(noneExistingImageTags, noneExistingImageDigests...) {
		exists, err := ds.HostHasImage(nil, nil, term)
		assert.NoError(t, err)
		assert.False(t, exists)
	}

}

func TestDockerAuxillary_EnsureImagePulled(t *testing.T) {
	testImageList := []types.ImageSummary{
		types.ImageSummary{RepoDigests: []string{"test0"}, RepoTags: []string{"test2"}},
		types.ImageSummary{RepoDigests: []string{"test3"}, RepoTags: []string{"test4"}},
		types.ImageSummary{RepoDigests: []string{"test5"}, RepoTags: []string{"test6"}},
		types.ImageSummary{RepoDigests: []string{"test7"}, RepoTags: []string{"test8"}},
	}

	existingImages := []string{"test7", "test6"}
	nonExistingImages := []string{"A", "B"}
	testReader := strings.NewReader("TTTTTTTTTTTTTTTTTTTTTTTTTTTTTTT")

	repo := new(repository.DockerRepository)
	repo.On("ImageList", mock.Anything, mock.Anything, mock.Anything).Return(testImageList, nil).Run(
		func(args mock.Arguments) {

			require.Len(t, args, 3)
			assert.Nil(t, args.Get(0))
			assert.Nil(t, args.Get(1))
		}).Times(len(nonExistingImages) + len(existingImages))

	repo.On("ImagePull", mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(
		ioutil.NopCloser(testReader), nil).Run(func(args mock.Arguments) {
		testReader.Reset("TTTTTTTTTTTTTTTTTTTTTTTTTTTTTTT")
		require.Len(t, args, 4)
		assert.Nil(t, args.Get(0))
		assert.Nil(t, args.Get(1))
		ipo, ok := args.Get(3).(types.ImagePullOptions)
		require.True(t, ok)
		assert.Equal(t, "Linux", ipo.Platform)
	}).Times(len(nonExistingImages))

	repo.On("ImageLoad", mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(
		types.ImageLoadResponse{
			Body: ioutil.NopCloser(testReader),
		}, nil).Run(
		func(args mock.Arguments) {

			require.Len(t, args, 4)
			assert.Nil(t, args.Get(0))
			assert.Nil(t, args.Get(1))
			rdr, ok := args.Get(2).(io.Reader)
			require.True(t, ok)
			require.NotNil(t, rdr)
		}).Times(len(nonExistingImages))

	ds := NewDockerAuxillary(repo)

	for _, img := range existingImages {
		err := ds.EnsureImagePulled(nil, nil, img)
		assert.NoError(t, err)
	}

	for _, img := range nonExistingImages {
		err := ds.EnsureImagePulled(nil, nil, img)
		assert.NoError(t, err)
	}
	repo.AssertExpectations(t)
}

func TestDockerAuxillary_GetContainerByName(t *testing.T) {
	results := []types.Container{
		types.Container{Names: []string{"test1", "test3"}, ID: "id1"},
		types.Container{Names: []string{"test2", "test4"}, ID: "id2"},
	}
	repo := new(repository.DockerRepository)
	repo.On("ContainerList", mock.Anything, mock.Anything, mock.Anything).Return(results, nil).Run(
		func(args mock.Arguments) {

			require.Len(t, args, 3)
			assert.Nil(t, args.Get(0))
			assert.Nil(t, args.Get(1))
		})
	ds := NewDockerAuxillary(repo)

	for _, result := range results {
		for _, name := range result.Names {
			cntr, err := ds.GetContainerByName(nil, nil, name)
			assert.NoError(t, err)
			assert.Equal(t, result, cntr)
		}

	}

	_, err := ds.GetContainerByName(nil, nil, "DNE")
	assert.Error(t, err)

	repo.AssertNumberOfCalls(t, "ContainerList", (2*len(results))+1)
}

func TestDockerAuxillary_GetVolumeByName(t *testing.T) {
	results := volume.VolumeListOKBody{
		Volumes: []*types.Volume{
			&types.Volume{Name: "test1"},
			&types.Volume{Name: "test2"},
		},
	}
	repo := new(repository.DockerRepository)
	repo.On("VolumeList", mock.Anything, mock.Anything, mock.Anything).Return(results, nil).Run(
		func(args mock.Arguments) {

			require.Len(t, args, 3)
			assert.Nil(t, args.Get(0))
			assert.Nil(t, args.Get(1))
		}).Times(len(results.Volumes) + 1)
	ds := NewDockerAuxillary(repo)

	for _, vol := range results.Volumes {
		result, err := ds.GetVolumeByName(nil, nil, vol.Name)
		assert.NoError(t, err)
		assert.Equal(t, result, vol)

	}

	res, err := ds.GetVolumeByName(nil, nil, "DNE")
	assert.Error(t, err)
	assert.Nil(t, res)

	repo.AssertExpectations(t)
}