// Copyright (c) 2026 Benjamin Borbe All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package migrate

import "errors"

// errUnexpectedStatus is a sentinel wrapped with request context whenever a
// TeamVault or Lockbox HTTP call returns a non-2xx status code.
var errUnexpectedStatus = errors.New("unexpected http status")
