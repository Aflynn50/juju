// Copyright 2019 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

//go:build !minimal || provider_lxd

package all

import (
	_ "github.com/juju/juju/internal/provider/lxd"
)
