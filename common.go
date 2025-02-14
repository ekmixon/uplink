// Copyright (C) 2020 Storj Labs, Inc.
// See LICENSE for copying information.

package uplink

import (
	"errors"
	"fmt"
	"io"
	"strings"

	"github.com/spacemonkeygo/monkit/v3"
	"github.com/zeebo/errs"

	"storj.io/common/encryption"
	"storj.io/common/errs2"
	"storj.io/common/rpc/rpcstatus"
	"storj.io/uplink/private/metaclient"
)

var mon = monkit.Package()

// Error is default error class for uplink.
var packageError = errs.Class("uplink")

// ErrTooManyRequests is returned when user has sent too many requests in a given amount of time.
var ErrTooManyRequests = errors.New("too many requests")

// ErrBandwidthLimitExceeded is returned when project will exceeded bandwidth limit.
var ErrBandwidthLimitExceeded = errors.New("bandwidth limit exceeded")

// ErrPermissionDenied is returned when the request is denied due to invalid permissions.
var ErrPermissionDenied = errors.New("permission denied")

func convertKnownErrors(err error, bucket, key string) error {
	switch {
	case errors.Is(err, io.EOF):
		return err
	case metaclient.ErrNoBucket.Has(err):
		return errwrapf("%w (%q)", ErrBucketNameInvalid, bucket)
	case metaclient.ErrNoPath.Has(err):
		return errwrapf("%w (%q)", ErrObjectKeyInvalid, key)
	case metaclient.ErrBucketNotFound.Has(err):
		return errwrapf("%w (%q)", ErrBucketNotFound, bucket)
	case metaclient.ErrObjectNotFound.Has(err):
		return errwrapf("%w (%q)", ErrObjectNotFound, key)
	case encryption.ErrMissingEncryptionBase.Has(err):
		return errwrapf("%w (%q)", ErrPermissionDenied, key)
	case encryption.ErrMissingDecryptionBase.Has(err):
		return errwrapf("%w (%q)", ErrPermissionDenied, key)
	case errs2.IsRPC(err, rpcstatus.ResourceExhausted):
		// TODO is a better way to do this?
		message := errs.Unwrap(err).Error()
		if strings.HasSuffix(message, "Exceeded Usage Limit") {
			return packageError.Wrap(rpcstatus.Wrap(rpcstatus.ResourceExhausted, ErrBandwidthLimitExceeded))
		} else if strings.HasSuffix(message, "Too Many Requests") {
			return packageError.Wrap(rpcstatus.Wrap(rpcstatus.ResourceExhausted, ErrTooManyRequests))
		}
	case errs2.IsRPC(err, rpcstatus.NotFound):
		message := errs.Unwrap(err).Error()
		if strings.HasPrefix(message, metaclient.ErrBucketNotFound.New("").Error()) {
			prefixLength := len(metaclient.ErrBucketNotFound.New("").Error())
			// remove error prefix + ": " from message
			bucket := message[prefixLength+2:]
			return errwrapf("%w (%q)", ErrBucketNotFound, bucket)
		} else if strings.HasPrefix(message, metaclient.ErrObjectNotFound.New("").Error()) {
			return errwrapf("%w (%q)", ErrObjectNotFound, key)
		}
	case errs2.IsRPC(err, rpcstatus.PermissionDenied):
		originalErr := err
		wrappedErr := errwrapf("%w (%v)", ErrPermissionDenied, originalErr)
		// TODO: once we have confirmed nothing downstream
		// is using errs2.IsRPC(err, rpcstatus.PermissionDenied), we should
		// just return wrappedErr instead of this.
		return &joinedErr{main: wrappedErr, alt: originalErr, code: rpcstatus.PermissionDenied}
	}

	return packageError.Wrap(err)
}

func errwrapf(format string, err error, args ...interface{}) error {
	var all []interface{}
	all = append(all, err)
	all = append(all, args...)
	return packageError.Wrap(fmt.Errorf(format, all...))
}

type joinedErr struct {
	main error
	alt  error
	code rpcstatus.StatusCode
}

func (err *joinedErr) Is(target error) bool {
	return errors.Is(err.main, target) || errors.Is(err.alt, target)
}

func (err *joinedErr) As(target interface{}) bool {
	if errors.As(err.main, target) {
		return true
	}
	if errors.As(err.alt, target) {
		return true
	}
	return false
}

func (err *joinedErr) Code() uint64 {
	return uint64(err.code)
}

func (err *joinedErr) Unwrap() error {
	return err.main
}

func (err *joinedErr) Error() string {
	return err.main.Error()
}

// Ungroup works with errs2.IsRPC and errs.IsFunc.
func (err *joinedErr) Ungroup() []error {
	return []error{err.main, err.alt}
}
