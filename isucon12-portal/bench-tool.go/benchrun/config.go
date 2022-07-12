package benchrun

import (
	"os"
)

// GetTargetAddress returns a target address given by a supervisor ($ISUXBENCH_TARGET environment variable.)
func GetTargetAddress() string {
	return os.Getenv("ISUXBENCH_TARGET")
}

// GetAllAddresses returns all addresses given by a supervisor ($ISUXBENCH_ALL_ADDRESSES environment variable.)
func GetAllAddresses() string {
	return os.Getenv("ISUXBENCH_ALL_ADDRESSES")
}
