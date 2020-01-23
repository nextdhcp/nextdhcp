package drivers

import (
	// import all supported drivers
	_ "github.com/nextdhcp/nextdhcp/core/lease/storage/drivers/bolt"
	_ "github.com/nextdhcp/nextdhcp/core/lease/storage/drivers/memory"
)
