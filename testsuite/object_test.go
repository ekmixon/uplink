// Copyright (C) 2020 Storj Labs, Inc.
// See LICENSE for copying information.

package testsuite_test

import (
	"bytes"
	"io"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"storj.io/common/memory"
	"storj.io/common/storj"
	"storj.io/common/testcontext"
	"storj.io/common/testrand"
	"storj.io/storj/private/testplanet"
	"storj.io/uplink"
)

func TestObject(t *testing.T) {
	testplanet.Run(t, testplanet.Config{
		SatelliteCount:   1,
		StorageNodeCount: 4,
		UplinkCount:      1,
	}, func(t *testing.T, ctx *testcontext.Context, planet *testplanet.Planet) {
		project := openProject(t, ctx, planet)
		defer ctx.Check(project.Close)

		createBucket(t, ctx, project, "testbucket")
		defer func() {
			err := project.DeleteBucket(ctx, "testbucket")
			require.NoError(t, err)
		}()

		upload, err := project.UploadObject(ctx, "testbucket", "test.dat")
		require.NoError(t, err)
		assertObjectEmptyCreated(t, upload.Info(), "test.dat")

		randData := testrand.Bytes(10 * memory.KiB)
		source := bytes.NewBuffer(randData)
		_, err = io.Copy(upload, source)
		require.NoError(t, err)
		assertObjectEmptyCreated(t, upload.Info(), "test.dat")

		err = upload.Commit()
		require.NoError(t, err)
		assertObject(t, upload.Info(), "test.dat")

		obj, err := project.StatObject(ctx, "testbucket", "test.dat")
		require.NoError(t, err)
		assertObject(t, obj, "test.dat")

		err = upload.Commit()
		require.True(t, uplink.ErrUploadDone.Has(err))

		download, err := project.DownloadObject(ctx, "testbucket", "test.dat")
		require.NoError(t, err)
		assertObject(t, download.Info(), "test.dat")

		var downloaded bytes.Buffer
		_, err = io.Copy(&downloaded, download)
		require.NoError(t, err)
		assert.Equal(t, randData, downloaded.Bytes())

		err = download.Close()
		require.NoError(t, err)

		downloadReq := uplink.DownloadRequest{
			Bucket: "testbucket",
			Key:    "test.dat",
			Offset: 100,
			Length: 500,
		}
		download, err = downloadReq.Do(ctx, project)
		require.NoError(t, err)
		assertObject(t, download.Info(), "test.dat")

		var downloadedRange bytes.Buffer
		_, err = io.Copy(&downloadedRange, download)
		require.NoError(t, err)
		assert.Equal(t, randData[100:600], downloadedRange.Bytes())

		err = download.Close()
		require.NoError(t, err)

		err = project.DeleteObject(ctx, "testbucket", "test.dat")
		require.NoError(t, err)

		obj, err = project.StatObject(ctx, "testbucket", "test.dat")
		require.True(t, storj.ErrObjectNotFound.Has(err))
	})
}

func TestAbortUpload(t *testing.T) {
	testplanet.Run(t, testplanet.Config{
		SatelliteCount:   1,
		StorageNodeCount: 4,
		UplinkCount:      1,
	}, func(t *testing.T, ctx *testcontext.Context, planet *testplanet.Planet) {
		project := openProject(t, ctx, planet)
		defer ctx.Check(project.Close)

		createBucket(t, ctx, project, "testbucket")
		defer func() {
			err := project.DeleteBucket(ctx, "testbucket")
			require.NoError(t, err)
		}()

		upload, err := project.UploadObject(ctx, "testbucket", "test.dat")
		require.NoError(t, err)
		assertObjectEmptyCreated(t, upload.Info(), "test.dat")

		randData := testrand.Bytes(10 * memory.KiB)
		source := bytes.NewBuffer(randData)
		_, err = io.Copy(upload, source)
		require.NoError(t, err)
		assertObjectEmptyCreated(t, upload.Info(), "test.dat")

		err = upload.Abort()
		require.NoError(t, err)

		err = upload.Commit()
		require.Error(t, err)

		_, err = project.StatObject(ctx, "testbucket", "test.dat")
		require.True(t, storj.ErrObjectNotFound.Has(err))

		err = upload.Abort()
		require.True(t, uplink.ErrUploadDone.Has(err))
	})
}

func assertObject(t *testing.T, obj *uplink.Object, expectedKey string) {
	assert.Equal(t, expectedKey, obj.Key)
	assert.Condition(t, func() bool {
		return time.Since(obj.Info.Created) < 10*time.Second
	})
}

func assertObjectEmptyCreated(t *testing.T, obj *uplink.Object, expectedKey string) {
	assert.Equal(t, expectedKey, obj.Key)
	assert.Empty(t, obj.Info.Created)
}

func uploadObject(t *testing.T, ctx *testcontext.Context, project *uplink.Project, bucket, key string) *uplink.Object {
	upload, err := project.UploadObject(ctx, bucket, key)
	require.NoError(t, err)
	assertObjectEmptyCreated(t, upload.Info(), key)

	randData := testrand.Bytes(10 * memory.KiB)
	source := bytes.NewBuffer(randData)
	_, err = io.Copy(upload, source)
	require.NoError(t, err)
	assertObjectEmptyCreated(t, upload.Info(), key)

	err = upload.Commit()
	require.NoError(t, err)
	assertObject(t, upload.Info(), key)

	return upload.Info()
}
